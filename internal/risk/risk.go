package risk

import (
	"fmt"
	"math"

	"github.com/atps/atps/internal/config"
)

// MarketState snapshot at sizing time.
type MarketState struct {
	Equity              float64 // current equity (realized + unrealized marked)
	Price               float64 // entry reference price
	ATR                 float64
	StopPrice           float64 // proposed stop (absolute price)
	Side                int     // 1 long, -1 short
	VolRegime           float64 // ATR percentile 0-100 (NaN unknown)
	ADX                 float64 // NaN unknown
	FundingZ            float64 // NaN unknown
	VolAnnualizedPct    float64 // realized vol annualized % (0 unknown)
	PortfolioHeatPct    float64 // sum of open positions risk % (total)
	PortfolioCorrelatedPct float64 // sum of same-side correlated risk %
	EquityDDPct         float64 // current drawdown from peak, positive number
}

// RiskLimits — base/min/max adaptive, dynamic leverage, heat, satellite
type RiskLimits struct {
	RiskPerTradePct    float64 // legacy MAX (alias for MaxRiskPct)
	BaseRiskPct        float64 // new spec: base 1.0% (0.01)
	MinRiskPct         float64 // new spec: min 0.25% (0.0025)
	MaxRiskPct         float64 // new spec: max 2.0% (0.02)
	MaxHeatPct         float64 // max_open_risk: 3% total
	MaxCorrelatedPct   float64 // max_correlated_risk: 2% same side
	MaxLeverage        float64 // HARD cap, 5
	MinLeverageCap     float64
	MaxNotional        float64
	VolTargetPct       float64
	KellyCapPct        float64
	DDDeleverageStart  float64
	DDFlatPct          float64
	ADXSoftThreshold   float64
	AdaptiveVol        bool // volatility adaptive_risk
	AdaptiveDD         bool // drawdown adaptive_risk
	PyramidingRiskNeutral bool
	SatelliteEnabled   bool
	SatelliteAlloc     float64 // 0.30
}

// SizingDecision full audit trail.
type SizingDecision struct {
	Accept       bool
	Qty          float64
	Notional     float64
	RiskAmount   float64
	RiskPct      float64 // EFFECTIVE risk after all caps
	StopDist     float64
	Leverage     float64 // notional / equity — DERIVED, never fixed
	LeverageCap  float64 // dynamic cap applied
	Factors      []string
}

func DefaultLimits() RiskLimits {
	return RiskLimits{
		RiskPerTradePct:   2.0,
		BaseRiskPct:       1.0,  // 0.01 user spec
		MinRiskPct:        0.25, // 0.0025
		MaxRiskPct:        2.0,  // 0.02
		MaxHeatPct:        3.0,  // user spec 0.03
		MaxCorrelatedPct:  2.0,  // user spec 0.02
		MaxLeverage:       5.0,  // user spec 5
		MinLeverageCap:    0.7,
		VolTargetPct:      50.0,
		DDDeleverageStart: 10.0,
		DDFlatPct:         25.0,
		ADXSoftThreshold:  20.0, // user spec adx_min 20
		AdaptiveVol:       true,
		AdaptiveDD:        true,
		PyramidingRiskNeutral: true,
		SatelliteEnabled:  false, // default false for tests; enabled via config profit.satellite.enabled
		SatelliteAlloc:    0.30,
	}
}

// Size computes position honoring max risk per trade with dynamic leverage.
// Leverage is an OUTPUT (notional/equity), capped by market riskiness.
func Size(ms MarketState, lim RiskLimits) SizingDecision {
	dec := SizingDecision{LeverageCap: lim.MaxLeverage}

	if ms.Equity <= 0 || ms.Price <= 0 {
		dec.Factors = append(dec.Factors, "reject: equity/price invalid")
		return dec
	}

	// ── 0. stop distance ────────────────────────────────────────────────
	stopDist := math.Abs(ms.Price - ms.StopPrice)
	if math.IsNaN(stopDist) || stopDist <= 0 {
		if ms.ATR > 0 {
			stopDist = 2 * ms.ATR
			dec.Factors = append(dec.Factors, fmt.Sprintf("stop fallback 2×ATR=%.4f", stopDist))
		} else {
			dec.Factors = append(dec.Factors, "reject: no stop distance nor ATR")
			return dec
		}
	}
	dec.StopDist = stopDist

	// ── 1. base risk budget — NEW SPEC: base/min/max adaptive ───────
	// risk% = base (1%) → scaled by vol & DD between min 0.25% and max 2%
	base := lim.BaseRiskPct
	if base <= 0 {
		base = lim.RiskPerTradePct
		if base <= 0 {
			base = 1.0
		}
	}
	minRisk := lim.MinRiskPct
	if minRisk <= 0 {
		minRisk = 0.25
	}
	maxRisk := lim.MaxRiskPct
	if maxRisk <= 0 {
		maxRisk = lim.RiskPerTradePct
		if maxRisk <= 0 {
			maxRisk = 2.0
		}
	}
	riskPct := base

	// ── 1a. volatility adaptive (user spec: volatility.adaptive_risk true) ─
	if lim.AdaptiveVol {
		// Vol regime percentile 0-100: low vol (<20) → max risk, high vol (>80) → min
		if !math.IsNaN(ms.VolRegime) {
			if ms.VolRegime > 80 {
				riskPct = minRisk + (base-minRisk)*0.3
				dec.Factors = append(dec.Factors, fmt.Sprintf("vol adaptive: high vol regime %.0f → risk %.2f%%→%.2f%% (min)", ms.VolRegime, base, riskPct))
			} else if ms.VolRegime > 60 {
				riskPct = minRisk + (base-minRisk)*0.6
				dec.Factors = append(dec.Factors, fmt.Sprintf("vol adaptive: mid-high vol %.0f → %.2f%%", ms.VolRegime, riskPct))
			} else if ms.VolRegime < 20 {
				riskPct = base + (maxRisk-base)*0.7
				dec.Factors = append(dec.Factors, fmt.Sprintf("vol adaptive: low vol %.0f → risk %.2f%%→%.2f%% (max)", ms.VolRegime, base, riskPct))
			}
		}
	}
	// clamp to min/max after regime
	if riskPct < minRisk {
		riskPct = minRisk
	}
	if riskPct > maxRisk {
		riskPct = maxRisk
	}
	// Vol target scaling (always if vol > target — independent of adaptive flag)
	if lim.VolTargetPct > 0 && ms.VolAnnualizedPct > lim.VolTargetPct {
		scale := lim.VolTargetPct / ms.VolAnnualizedPct
		before := riskPct
		riskPct *= scale
		if riskPct < minRisk {
			riskPct = minRisk
		}
		dec.Factors = append(dec.Factors, fmt.Sprintf("vol target ×%.2f (%.2f%%→%.2f%%, realVol %.0f%%)", scale, before, riskPct, ms.VolAnnualizedPct))
	}

	// ── 2. portfolio heat budget deferred — check after adaptive (see step 3-4) ──

	// ── 3. drawdown de-leverage — NEW SPEC: drawdown.adaptive_risk true ─
	if lim.AdaptiveDD && ms.EquityDDPct > lim.DDDeleverageStart && lim.DDFlatPct > lim.DDDeleverageStart {
		span := lim.DDFlatPct - lim.DDDeleverageStart
		scale := 1 - (ms.EquityDDPct-lim.DDDeleverageStart)/span
		if scale < 0 {
			scale = 0
		}
		before := riskPct
		// scale towards min, not to zero, to keep min risk alive
		riskPct = minRisk + (riskPct-minRisk)*scale
		dec.Factors = append(dec.Factors, fmt.Sprintf("dd adaptive ×%.2f (%.2f%%→%.2f%%, dd %.1f%%)", scale, before, riskPct, ms.EquityDDPct))
	} else if !lim.AdaptiveDD && ms.EquityDDPct > lim.DDDeleverageStart {
		// legacy non-adaptive: scale to zero
		span := lim.DDFlatPct - lim.DDDeleverageStart
		scale := 1 - (ms.EquityDDPct-lim.DDDeleverageStart)/span
		if scale < 0 {
			scale = 0
		}
		before := riskPct
		riskPct *= scale
		dec.Factors = append(dec.Factors, fmt.Sprintf("dd de-leverage ×%.2f (%.2f%%→%.2f%%, dd %.1f%%)", scale, before, riskPct, ms.EquityDDPct))
	}
	if riskPct < minRisk {
		riskPct = minRisk
	}
	if riskPct <= 0 {
		dec.Factors = append(dec.Factors, "reject: drawdown flat zone")
		return dec
	}

	// ── 4. portfolio heat (total + correlated) — after adaptive, before Kelly ──
	if lim.MaxHeatPct > 0 {
		avail := lim.MaxHeatPct - ms.PortfolioHeatPct
		if avail < riskPct {
			before := riskPct
			riskPct = avail
			dec.Factors = append(dec.Factors, fmt.Sprintf("heat cap total: %.2f%%→%.2f%% (open %.2f%%/%.0f%%)", before, riskPct, ms.PortfolioHeatPct, lim.MaxHeatPct))
		}
	}
	if lim.MaxCorrelatedPct > 0 && ms.Side != 0 {
		availCorr := lim.MaxCorrelatedPct - ms.PortfolioCorrelatedPct
		if availCorr < riskPct {
			before := riskPct
			riskPct = availCorr
			dec.Factors = append(dec.Factors, fmt.Sprintf("heat cap correlated: %.2f%%→%.2f%% (corr %.2f%%/%.0f%%)", before, riskPct, ms.PortfolioCorrelatedPct, lim.MaxCorrelatedPct))
		}
	}
	if riskPct <= 0 {
		dec.Factors = append(dec.Factors, "reject: portfolio heat exhausted")
		return dec
	}
	// heat may push below min — that's ok, heat is hard limit (allow 0.1% < min 0.25%)

	// ── 5. Kelly hard cap ───────────────────────────────────────────────
	if lim.KellyCapPct > 0 && riskPct > lim.KellyCapPct {
		dec.Factors = append(dec.Factors, fmt.Sprintf("kelly cap %.2f%%→%.2f%%", riskPct, lim.KellyCapPct))
		riskPct = lim.KellyCapPct
	}

	riskAmount := ms.Equity * riskPct / 100.0
	qty := riskAmount / stopDist
	notional := qty * ms.Price

	// ── 6. DYNAMIC LEVERAGE CAP — BIG IMPROVE: meno punitivo ─────────
	// Hard cap 10×, scaling più soft per non castrate redditività
	levCap := lim.MaxLeverage
	if !math.IsNaN(ms.VolRegime) {
		switch {
		case ms.VolRegime > 80:
			levCap *= 0.70 // era 0.50 → meno punitivo
			dec.Factors = append(dec.Factors, fmt.Sprintf("lev ×0.70 (vol regime %.0f>80)", ms.VolRegime))
		case ms.VolRegime > 60:
			levCap *= 0.85 // era 0.75
			dec.Factors = append(dec.Factors, fmt.Sprintf("lev ×0.85 (vol regime %.0f>60)", ms.VolRegime))
		case ms.VolRegime < 20:
			levCap *= 1.30 // era 1.20 → premia low vol di più
			dec.Factors = append(dec.Factors, fmt.Sprintf("lev ×1.30 (vol regime %.0f<20)", ms.VolRegime))
		}
	}
	if !math.IsNaN(ms.ADX) && ms.ADX < lim.ADXSoftThreshold {
		levCap *= 0.80 // era 0.60 → meno punitivo
		dec.Factors = append(dec.Factors, fmt.Sprintf("lev ×0.80 (ADX %.0f weak trend)", ms.ADX))
	}
	if !math.IsNaN(ms.FundingZ) && math.Abs(ms.FundingZ) > 2.5 { // threshold ↑ 2→2.5
		levCap *= 0.85 // era 0.70
		dec.Factors = append(dec.Factors, fmt.Sprintf("lev ×0.85 (funding z %.1f extreme)", ms.FundingZ))
	}
	if levCap > lim.MaxLeverage {
		levCap = lim.MaxLeverage
	}
	if levCap < lim.MinLeverageCap {
		levCap = lim.MinLeverageCap
	}
	dec.LeverageCap = levCap

	capNotional := ms.Equity * levCap
	if lim.MaxNotional > 0 && lim.MaxNotional < capNotional {
		capNotional = lim.MaxNotional
		dec.Factors = append(dec.Factors, "notional cap (absolute)")
	}
	if notional > capNotional {
		notional = capNotional
		qty = notional / ms.Price
		dec.Factors = append(dec.Factors, fmt.Sprintf("notional capped by dyn lev %.2fx → $%.0f", levCap, notional))
	}

	// ── 7. recompute EFFECTIVE risk after caps ─────────────────────────
	riskAmount = qty * stopDist
	if ms.Equity > 0 {
		riskPct = riskAmount / ms.Equity * 100.0
	}

	dec.Qty = qty
	dec.Notional = notional
	dec.RiskAmount = riskAmount
	dec.RiskPct = riskPct
	dec.Leverage = notional / ms.Equity
	dec.Accept = qty > 0 && riskPct > 0

	if dec.Accept {
		dec.Factors = append(dec.Factors, fmt.Sprintf("final: qty %.5f, $%.0f notional, %.2fx lev, risk %.2f%%", qty, notional, dec.Leverage, riskPct))
	}
	return dec
}

// LimitsFromConfig builds limits merging global risk cfg + variant risk pct.
// Supports new spec: risk.base/min/max, portfolio.max_open/correlated, leverage.max, volatility/drawdown adaptive
func LimitsFromConfig(cfg *config.Config, variant string) RiskLimits {
	lim := DefaultLimits()
	if cfg == nil {
		return lim
	}
	// ── new spec risk base/min/max (decimal 0.01) ──
	if cfg.Risk.Base != 0 {
		lim.BaseRiskPct = cfg.Risk.Base * 100
		lim.RiskPerTradePct = cfg.Risk.Base * 100 // legacy alias
	}
	if cfg.Risk.Min != 0 {
		lim.MinRiskPct = cfg.Risk.Min * 100
	}
	if cfg.Risk.Max != 0 {
		lim.MaxRiskPct = cfg.Risk.Max * 100
		lim.RiskPerTradePct = cfg.Risk.Max * 100
	}
	// legacy overrides (still honored)
	if cfg.Risk.MaxRiskPerTradePct > 0 {
		lim.RiskPerTradePct = cfg.Risk.MaxRiskPerTradePct
		lim.MaxRiskPct = cfg.Risk.MaxRiskPerTradePct
	}
	if cfg.Risk.MaxHeatPct > 0 {
		lim.MaxHeatPct = cfg.Risk.MaxHeatPct
	}
	// new portfolio heat (0.03 → 3%)
	if cfg.Portfolio.MaxOpenRisk != 0 {
		lim.MaxHeatPct = cfg.Portfolio.MaxOpenRisk * 100
	}
	if cfg.Portfolio.MaxCorrelatedRisk != 0 {
		lim.MaxCorrelatedPct = cfg.Portfolio.MaxCorrelatedRisk * 100
	}
	// leverage max (new spec leverage.max)
	if cfg.LeverageCfg.Max != 0 {
		lim.MaxLeverage = cfg.LeverageCfg.Max
	} else if cfg.Risk.MaxLeverage > 0 {
		lim.MaxLeverage = cfg.Risk.MaxLeverage
	}
	if cfg.Risk.MinLeverageCap > 0 {
		lim.MinLeverageCap = cfg.Risk.MinLeverageCap
	}
	if cfg.Risk.MaxNotional > 0 {
		lim.MaxNotional = cfg.Risk.MaxNotional
	}
	if cfg.Costs.MaxNotionalPerTrade > 0 && lim.MaxNotional == 0 {
		lim.MaxNotional = cfg.Costs.MaxNotionalPerTrade
	}
	if cfg.Risk.VolTargetPct > 0 {
		lim.VolTargetPct = cfg.Risk.VolTargetPct
	}
	if cfg.Risk.KellyCapPct > 0 {
		lim.KellyCapPct = cfg.Risk.KellyCapPct
	}
	if cfg.Risk.DDDeleverageStart > 0 {
		lim.DDDeleverageStart = cfg.Risk.DDDeleverageStart
	}
	if cfg.Risk.DDFlatPct > 0 {
		lim.DDFlatPct = cfg.Risk.DDFlatPct
	}
	if cfg.Risk.ADXSoftThreshold > 0 {
		lim.ADXSoftThreshold = cfg.Risk.ADXSoftThreshold
	}
	// new spec adaptive flags
	if cfg.Volatility.AdaptiveRisk {
		lim.AdaptiveVol = true
	}
	if cfg.Drawdown.AdaptiveRisk {
		lim.AdaptiveDD = true
	}
	// pyramiding risk_neutral
	if cfg.Pyramiding.RiskNeutral {
		lim.PyramidingRiskNeutral = true
	}
	// satellite
	if cfg.Profit.Satellite.Enabled {
		lim.SatelliteEnabled = true
		lim.SatelliteAlloc = cfg.Profit.Satellite.Allocation
		if lim.SatelliteAlloc == 0 {
			lim.SatelliteAlloc = 0.30
		}
	}
	// variant risk pct is the per-variant MAX (e.g. A 2%, D 1.2%)
	switch variant {
	case "A":
		if cfg.VariantA.RiskPct > 0 {
			lim.RiskPerTradePct = cfg.VariantA.RiskPct
		}
	case "B":
		if cfg.VariantB.RiskPct > 0 {
			lim.RiskPerTradePct = cfg.VariantB.RiskPct
		}
		// B: no vol targeting, regime filter handles risk itself
		lim.VolTargetPct = 0
	case "C":
		if cfg.VariantC.RiskPct > 0 {
			lim.RiskPerTradePct = cfg.VariantC.RiskPct
		}
	case "D":
		if cfg.VariantD.RiskPct > 0 {
			lim.RiskPerTradePct = cfg.VariantD.RiskPct
		}
		if cfg.VariantD.VolTargetPct > 0 {
			lim.VolTargetPct = cfg.VariantD.VolTargetPct
		}
		if cfg.VariantD.KellyCapPct > 0 {
			lim.KellyCapPct = cfg.VariantD.KellyCapPct
		}
	}
	// global risk_per_trade overrides variant if smaller (defense in depth)
	if cfg.Risk.MaxRiskPerTradePct > 0 && lim.RiskPerTradePct > cfg.Risk.MaxRiskPerTradePct {
		lim.RiskPerTradePct = cfg.Risk.MaxRiskPerTradePct
	}
	return lim
}

// ── kept helpers ──────────────────────────────────────────────────────

func CanPyramid(entryPrice, currentPrice float64, atr float64, side int, units int, maxUnits int, stepATR float64) bool {
	if units >= maxUnits {
		return false
	}
	if atr <= 0 {
		return false
	}
	if side == 1 {
		return currentPrice >= entryPrice+stepATR*atr*float64(units+1)
	}
	if side == -1 {
		return currentPrice <= entryPrice-stepATR*atr*float64(units+1)
	}
	return false
}

func TrailStopPosition(currentStop, newStop float64, side int) float64 {
	if math.IsNaN(newStop) {
		return currentStop
	}
	if math.IsNaN(currentStop) {
		return newStop
	}
	if side == 1 {
		if newStop > currentStop {
			return newStop
		}
		return currentStop
	}
	if side == -1 {
		if newStop < currentStop {
			return newStop
		}
		return currentStop
	}
	return currentStop
}

// IsCrashBar returns true if the bar return triggers the crash brake.
func IsCrashBar(retPct float64, threshold float64) bool {
	return math.Abs(retPct) >= threshold
}

// AnnualizedVolPct from ATR: (atr/price) * sqrt(intervals per year) * 100.
func AnnualizedVolPct(atr, price float64, intervalHours float64) float64 {
	if atr <= 0 || price <= 0 || intervalHours <= 0 {
		return 0
	}
	perYear := 8760.0 / intervalHours
	return (atr / price) * math.Sqrt(perYear) * 100.0
}

// ValidateLimitInvariants panics-proof check used in tests.
func ValidateLimitInvariants(lim RiskLimits) error {
	if lim.RiskPerTradePct <= 0 || lim.RiskPerTradePct > 10 {
		return fmt.Errorf("risk per trade %.2f%% out of sane range (0,10]", lim.RiskPerTradePct)
	}
	if lim.MaxHeatPct < lim.RiskPerTradePct {
		return fmt.Errorf("max heat %.2f%% < risk per trade %.2f%%", lim.MaxHeatPct, lim.RiskPerTradePct)
	}
	if lim.MaxLeverage < 1 || lim.MaxLeverage > 20 {
		return fmt.Errorf("max leverage %.1f out of sane range [1,20]", lim.MaxLeverage)
	}
	return nil
}
