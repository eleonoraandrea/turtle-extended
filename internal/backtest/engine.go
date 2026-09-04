package backtest

import (
	"fmt"
	"math"
	"time"

	"github.com/atps/atps/internal/config"
	"github.com/atps/atps/internal/data"
	"github.com/atps/atps/internal/indicators"
	"github.com/atps/atps/internal/risk"
	"github.com/atps/atps/internal/strategy"
)

// Trade represents one round-trip (entry to exit)
type Trade struct {
	Symbol      string    `json:"symbol"`
	Side        int       `json:"side"` // 1 long, -1 short
	EntryTime   time.Time `json:"entry_time"`
	ExitTime    time.Time `json:"exit_time"`
	EntryPrice  float64   `json:"entry_price"`
	ExitPrice   float64   `json:"exit_price"`
	Qty         float64   `json:"qty"`
	EntryATR    float64   `json:"entry_atr"`
	StopPrice   float64   `json:"stop_price"`
	ExitReason  string    `json:"exit_reason"`
	EntryReason string    `json:"entry_reason"`
	PnL         float64   `json:"pnl"`     // gross PnL in quote (USDC)
	PnLNet      float64   `json:"pnl_net"` // net after fees+funding
	Fee         float64   `json:"fee"`
	FundingCost float64   `json:"funding_cost"`
	Slippage    float64   `json:"slippage"`
	BarsHeld    int       `json:"bars_held"`
	MAE         float64   `json:"mae"`
	MFE         float64   `json:"mfe"`
	ReturnPct   float64   `json:"return_pct"`
	// ── risk engine audit ──
	RiskPct     float64 `json:"risk_pct"`   // effective risk % of equity at entry
	Leverage    float64 `json:"leverage"`   // notional/equity at entry (DYNAMIC)
	Notional    float64 `json:"notional"`   // entry notional
	StopDist    float64 `json:"stop_dist"`  // |entry-stop|
	RMultiple   float64 `json:"r_multiple"` // PnLNet / riskAmount
	SizingLog   string  `json:"sizing_log,omitempty"`
	IsSatellite bool    `json:"is_satellite,omitempty"`
}

// Position open — supports satellite 30% for large winners (positive skew)
type Position struct {
	Symbol       string
	Side         int
	Qty          float64
	EntryPrice   float64
	EntryTime    time.Time
	EntryATR     float64
	StopPrice    float64
	EntryReason  string
	Units        int
	EntryBarIdx  int
	MAE          float64
	MFE          float64
	FundingAccum float64
	RiskPct      float64
	Leverage     float64
	Notional     float64
	RiskAmount   float64
	SizingLog    string
	EntryFee     float64 // entry-side fee already charged to equity
	IsSatellite  bool    // true = satellite 30% (wide trailing, captures +5R/+10R)
	DonExitLen   int     // per-position Donchian exit length (core 20, satellite 55)
}

type EquityPoint struct {
	Time     time.Time `json:"time"`
	Equity   float64   `json:"equity"`
	Gross    float64   `json:"gross,omitempty"`
	Drawdown float64   `json:"drawdown,omitempty"`
	Price    float64   `json:"price"`
	Heat     float64   `json:"heat,omitempty"`     // open risk % at this bar
	Leverage float64   `json:"leverage,omitempty"` // open notional/equity
}

type Result struct {
	Symbol         string        `json:"symbol"`
	Variant        string        `json:"variant"`
	Bars           data.Bars     `json:"-"`
	Trades         []Trade       `json:"trades"`
	Equity         []EquityPoint `json:"equity"`
	InitialCapital float64       `json:"initial_capital"`
	FinalEquity    float64       `json:"final_equity"`
	GrossPnL       float64       `json:"gross_pnl"`
	NetPnL         float64       `json:"net_pnl"`
	TotalFee       float64       `json:"total_fee"`
	TotalFunding   float64       `json:"total_funding"`
	TotalSlippage  float64       `json:"total_slippage"`
	MaxUnits       int           `json:"max_units"`
	// risk audit aggregates
	AvgLeverage       float64         `json:"avg_leverage"`
	MaxLeverageUsed   float64         `json:"max_leverage_used"`
	AvgRiskPct        float64         `json:"avg_risk_pct"`
	MaxRiskPctUsed    float64         `json:"max_risk_pct_used"`
	MaxHeatSeen       float64         `json:"max_heat_seen"`
	RiskLimitsUsed    risk.RiskLimits `json:"risk_limits_used"`
	Warnings          []string        `json:"warnings,omitempty"`
	ScalingCeilingPct float64         `json:"scaling_ceiling_pct"`
	ScalingBinding    string          `json:"scaling_binding"`
	NotionalCapHits   int             `json:"notional_cap_hits"`
}

type EngineConfig struct {
	Variant        string
	Symbol         string
	InitialCapital float64
	FeeBps         float64
	SlippageBps    float64
	Leverage       float64 // legacy fallback hard cap if risk.max_leverage==0
	UseNextOpen    bool
	PyramidingMax  int
	PyramidingMode string // merged|separate (default merged)
	PyramidStepATR float64
	TrailATRMult   float64
	TrailMode      string
	DonExit        int
	EntryMode      string // close|intrabar (default close; intrabar = fill a livello canale)
}

func Run(bars data.Bars, strat strategy.Strategy, cfg *config.Config, eng EngineConfig) *Result {
	res := &Result{Symbol: eng.Symbol, Variant: eng.Variant, InitialCapital: eng.InitialCapital, FinalEquity: eng.InitialCapital}
	if len(bars) == 0 {
		return res
	}

	// ── risk limits: dynamic leverage, max risk per trade ──
	lim := risk.LimitsFromConfig(cfg, eng.Variant)
	if lim.MaxLeverage == 0 {
		lim.MaxLeverage = eng.Leverage // legacy fallback
		if lim.MaxLeverage == 0 {
			lim.MaxLeverage = 3
		}
	}
	if lim.MaxNotional == 0 && cfg != nil && cfg.Costs.MaxNotionalPerTrade > 0 {
		lim.MaxNotional = cfg.Costs.MaxNotionalPerTrade
	}
	res.RiskLimitsUsed = lim
	// ── scaling guardrails: tetto effettivo + warning (nessun cambio sizing) ──
	res.ScalingCeilingPct, res.ScalingBinding = risk.ScalingCeiling(lim)
	if res.ScalingCeilingPct < lim.MaxRiskPct {
		res.Warnings = append(res.Warnings, fmt.Sprintf("scaling: risk richiesto %.2f%% → tetto effettivo %.2f%% (%s lega)",
			lim.MaxRiskPct, res.ScalingCeilingPct, res.ScalingBinding))
	}
	if eng.PyramidingMode == "separate" && lim.PyramidingRiskNeutral {
		res.Warnings = append(res.Warnings, "pyramiding.mode=separate ignora risk_neutral (vale solo per merged)")
	}

	ctx := strat.Prepare(bars)
	n := len(bars)
	equity := eng.InitialCapital
	peak := equity
	var positions []*Position
	var trades []Trade
	var equityCurve []EquityPoint
	var totalFee, totalFundingNet, totalSlippage float64

	var donExitH, donExitL []float64
	exitLen := eng.DonExit
	if exitLen == 0 {
		exitLen = 20
	}
	high := make([]float64, n)
	low := make([]float64, n)
	for i, b := range bars {
		high[i] = b.High
		low[i] = b.Low
	}
	donExitH = indicators.DonchianHigh(high, exitLen)
	donExitL = indicators.DonchianLow(low, exitLen)
	// satellite exit (wider, for 30% satellite to capture large winners)
	donExitH55 := indicators.DonchianHigh(high, 55)
	donExitL55 := indicators.DonchianLow(low, 55)

	intervalH := intervalHours(cfg.General.Interval)
	if intervalH == 0 {
		intervalH = 4
	}

	brakeUntil := -1

	// ultimo stop-out per logica re-entry (interfaccia ReEntryChecker, Task 4)
	type stopOutState struct {
		valid      bool
		side       int
		exitBarIdx int
	}
	var lastStop stopOutState

	// helpers -----------------------------------------------------------------
	// recordExit — registra chiusura posizione (usato dal path intrabar same-bar stop)
	recordExit := func(pos *Position, exitPrice float64, reason string, barIdx int) {
		var pnl float64
		if pos.Side == 1 {
			pnl = (exitPrice - pos.EntryPrice) * pos.Qty
		} else {
			pnl = (pos.EntryPrice - exitPrice) * pos.Qty
		}
		exitFee := exitPrice * pos.Qty * eng.FeeBps / 10000.0
		fee := pos.EntryFee + exitFee
		pnlNet := pnl - fee - pos.FundingAccum
		equity += pnl - exitFee
		totalFee += exitFee
		rMult := 0.0
		if pos.RiskAmount > 0 {
			rMult = pnlNet / pos.RiskAmount
		}
		trades = append(trades, Trade{
			Symbol: eng.Symbol, Side: pos.Side,
			EntryTime: pos.EntryTime, ExitTime: bars[barIdx].Time,
			EntryPrice: pos.EntryPrice, ExitPrice: exitPrice,
			Qty: pos.Qty, EntryATR: pos.EntryATR, StopPrice: pos.StopPrice,
			EntryReason: pos.EntryReason, ExitReason: reason,
			PnL: pnl, PnLNet: pnlNet, Fee: fee, FundingCost: pos.FundingAccum,
			BarsHeld: barIdx - pos.EntryBarIdx, MAE: pos.MAE, MFE: pos.MFE,
			ReturnPct: pnlNet / (pos.EntryPrice * pos.Qty) * 100,
			RiskPct:   pos.RiskPct, Leverage: pos.Leverage, Notional: pos.Notional,
			StopDist: math.Abs(pos.EntryPrice - pos.StopPrice), RMultiple: rMult,
			SizingLog: pos.SizingLog, IsSatellite: pos.IsSatellite,
		})
	}

	openHeat := func() float64 {
		sum := 0.0
		for _, p := range positions {
			sum += p.RiskPct
		}
		return sum
	}
	openNotional := func(px float64) float64 {
		sum := 0.0
		for _, p := range positions {
			sum += p.Qty * px
		}
		return sum
	}

	for i := 0; i < n; i++ {
		bar := bars[i]

		// ── funding accrual (8h funding scaled to bar interval) ──
		// nota: una posizione fillata intrabar nella barra i inizia a pagare funding
		// dalla barra i+1 (loop funding corre prima del signal block) — bias minore,
		// una barra di granularità
		for _, pos := range positions {
			if bar.FundingRate != 0 {
				scale := intervalH / 8.0
				notional := pos.Qty * bar.Close
				pay := notional * bar.FundingRate * scale
				if pos.Side == 1 {
					// long pays positive funding
					equity -= pay
					pos.FundingAccum += pay
					totalFundingNet += pay
				} else {
					// short receives positive funding
					equity += pay
					pos.FundingAccum -= pay
					totalFundingNet -= pay
				}
			}
		}

		// ── exits (stop intrabar, donchian exit per-position, trailing) ──
		// Satellite positions use Donchian 55 (wide) to hold for large winners (+5R/+10R) → positive skew
		var remaining []*Position
		for _, pos := range positions {
			exit := false
			exitReason := ""
			exitPrice := bar.Close
			// per-position Donchian level — PRIOR bar's channel: the current bar's
			// own low/high is always inside its own channel, so comparing against
			// the band computed at bar i makes the close-exit unreachable.
			var donL, donH float64
			if i >= 1 {
				if pos.DonExitLen == 55 {
					donL = donExitL55[i-1]
					donH = donExitH55[i-1]
				} else {
					donL = donExitL[i-1]
					donH = donExitH[i-1]
				}
			}

			if pos.Side == 1 {
				if bar.Low <= pos.StopPrice {
					exit = true
					exitReason = "stop"
					exitPrice = pos.StopPrice
					if bar.Open < exitPrice {
						// gap through the stop: a stop-market fills near the open, not at the stop
						exitPrice = bar.Open
					}
					if eng.SlippageBps > 0 {
						slip := exitPrice * eng.SlippageBps / 10000.0
						exitPrice -= slip
						totalSlippage += slip * pos.Qty
					}
				} else if !math.IsNaN(donL) && bar.Close < donL {
					exit = true
					if pos.IsSatellite {
						exitReason = "satellite_donchian55"
					} else {
						exitReason = "donchian_exit"
					}
					exitPrice = bar.Close
				} else {
					var newStop float64
					if eng.TrailMode == "chandelier" {
						// satellite uses wider trail to let large winners run
						mult := eng.TrailATRMult
						if mult <= 0 {
							mult = 3.0
						}
						if pos.IsSatellite {
							mult += 1.0
						}
						newStop = strategy.TrailStop(ctx, i, pos.Side, mult, "chandelier")
					} else {
						newStop = donL
					}
					if !math.IsNaN(newStop) {
						pos.StopPrice = risk.TrailStopPosition(pos.StopPrice, newStop, pos.Side)
					}
				}
			} else if pos.Side == -1 {
				if bar.High >= pos.StopPrice {
					exit = true
					exitReason = "stop"
					exitPrice = pos.StopPrice
					if bar.Open > exitPrice {
						// gap through the stop: fill near the open
						exitPrice = bar.Open
					}
					if eng.SlippageBps > 0 {
						slip := exitPrice * eng.SlippageBps / 10000.0
						exitPrice += slip
						totalSlippage += slip * pos.Qty
					}
				} else if !math.IsNaN(donH) && bar.Close > donH {
					exit = true
					if pos.IsSatellite {
						exitReason = "satellite_donchian55"
					} else {
						exitReason = "donchian_exit"
					}
					exitPrice = bar.Close
				} else {
					var newStop float64
					if eng.TrailMode == "chandelier" {
						mult := eng.TrailATRMult
						if mult <= 0 {
							mult = 3.0
						}
						if pos.IsSatellite {
							mult += 1.0
						}
						newStop = strategy.TrailStop(ctx, i, pos.Side, mult, "chandelier")
					} else {
						newStop = donH
					}
					if !math.IsNaN(newStop) {
						pos.StopPrice = risk.TrailStopPosition(pos.StopPrice, newStop, pos.Side)
					}
				}
			}

			// MAE/MFE
			if pos.Side == 1 {
				if mae := (bar.Low - pos.EntryPrice) / pos.EntryPrice * 100; mae < pos.MAE {
					pos.MAE = mae
				}
				if mfe := (bar.High - pos.EntryPrice) / pos.EntryPrice * 100; mfe > pos.MFE {
					pos.MFE = mfe
				}
			} else {
				if mae := (pos.EntryPrice - bar.High) / pos.EntryPrice * 100; mae < pos.MAE {
					pos.MAE = mae
				}
				if mfe := (pos.EntryPrice - bar.Low) / pos.EntryPrice * 100; mfe > pos.MFE {
					pos.MFE = mfe
				}
			}

			if exit {
				var pnl float64
				if pos.Side == 1 {
					pnl = (exitPrice - pos.EntryPrice) * pos.Qty
				} else {
					pnl = (pos.EntryPrice - exitPrice) * pos.Qty
				}
				// entry fee was already charged at fill; charge only the exit side here
				exitFee := exitPrice * pos.Qty * eng.FeeBps / 10000.0
				fee := pos.EntryFee + exitFee
				pnlNet := pnl - fee - pos.FundingAccum
				equity += pnl - exitFee
				totalFee += exitFee
				rMult := 0.0
				if pos.RiskAmount > 0 {
					rMult = pnlNet / pos.RiskAmount
				}
				trades = append(trades, Trade{
					Symbol: eng.Symbol, Side: pos.Side,
					EntryTime: pos.EntryTime, ExitTime: bar.Time,
					EntryPrice: pos.EntryPrice, ExitPrice: exitPrice,
					Qty: pos.Qty, EntryATR: pos.EntryATR, StopPrice: pos.StopPrice,
					ExitReason: exitReason, EntryReason: pos.EntryReason, PnL: pnl, PnLNet: pnlNet, Fee: fee, FundingCost: pos.FundingAccum,
					BarsHeld: i - pos.EntryBarIdx, MAE: pos.MAE, MFE: pos.MFE,
					ReturnPct: pnlNet / (pos.EntryPrice * pos.Qty) * 100,
					RiskPct:   pos.RiskPct, Leverage: pos.Leverage, Notional: pos.Notional,
					StopDist: math.Abs(pos.EntryPrice - pos.StopPrice), RMultiple: rMult,
					SizingLog: pos.SizingLog, IsSatellite: pos.IsSatellite,
				})
				if exitReason == "stop" {
					lastStop = stopOutState{valid: true, side: pos.Side, exitBarIdx: i}
				}
			} else {
				remaining = append(remaining, pos)
			}
		}
		positions = remaining

		// ── crash brake ──
		if cfg.Portfolio.CrashBrakeDropPct > 0 && i > 0 {
			retPct := (bar.Close - bars[i-1].Close) / bars[i-1].Close * 100
			if math.Abs(retPct) >= cfg.Portfolio.CrashBrakeDropPct {
				for _, pos := range positions {
					var pnl float64
					if pos.Side == 1 {
						pnl = (bar.Close - pos.EntryPrice) * pos.Qty
					} else {
						pnl = (pos.EntryPrice - bar.Close) * pos.Qty
					}
					exitFee := bar.Close * pos.Qty * eng.FeeBps / 10000.0
					fee := pos.EntryFee + exitFee
					pnlNet := pnl - fee - pos.FundingAccum
					equity += pnl - exitFee
					totalFee += exitFee
					rMult := 0.0
					if pos.RiskAmount > 0 {
						rMult = pnlNet / pos.RiskAmount
					}
					trades = append(trades, Trade{Symbol: eng.Symbol, Side: pos.Side, EntryTime: pos.EntryTime, ExitTime: bar.Time, EntryPrice: pos.EntryPrice, ExitPrice: bar.Close, Qty: pos.Qty, EntryATR: pos.EntryATR, StopPrice: pos.StopPrice, ExitReason: "crash_brake", EntryReason: pos.EntryReason, PnL: pnl, PnLNet: pnlNet, Fee: fee, FundingCost: pos.FundingAccum, BarsHeld: i - pos.EntryBarIdx, MAE: pos.MAE, MFE: pos.MFE, RiskPct: pos.RiskPct, Leverage: pos.Leverage, Notional: pos.Notional, StopDist: math.Abs(pos.EntryPrice - pos.StopPrice), RMultiple: rMult, SizingLog: pos.SizingLog, IsSatellite: pos.IsSatellite})
				}
				positions = nil
				brakeUntil = i + 6
			}
		}

		// mark-to-market equity + drawdown for risk state
		unrealized := 0.0
		for _, pos := range positions {
			if pos.Side == 1 {
				unrealized += (bar.Close - pos.EntryPrice) * pos.Qty
			} else {
				unrealized += (pos.EntryPrice - bar.Close) * pos.Qty
			}
		}
		curEq := equity + unrealized
		if curEq > peak {
			peak = curEq
		}
		ddPct := 0.0
		if peak > 0 {
			ddPct = (curEq - peak) / peak * 100 // negative
		}

		if i < brakeUntil || curEq <= 0 {
			equityCurve = append(equityCurve, EquityPoint{Time: bar.Time, Equity: curEq, Drawdown: ddPct, Price: bar.Close, Heat: openHeat(), Leverage: openNotional(bar.Close) / math.Max(curEq, 1)})
			continue
		}

		// ── signal + RISK-BASED SIZING with DYNAMIC LEVERAGE ──
		// ── signal: intrabar (livelli da barre < i) → Next (close-mode) → re-entry ──
		var sig strategy.Signal
		intrabarFill, intrabarSlip := 0.0, 0.0
		isIntrabar := false
		if eng.EntryMode == "intrabar" && len(positions) == 0 && i >= 1 && i+1 < n {
			if lv, ok := strat.(strategy.IntrabarLevels); ok {
				levels := lv.IntrabarEntry(ctx, i)
				atrPrev := ctx.ATR[i-1]
				if levels.Enabled && !math.IsNaN(atrPrev) && atrPrev > 0 {
					longHit := !math.IsNaN(levels.LongLevel) && bar.High >= levels.LongLevel
					shortHit := !math.IsNaN(levels.ShortLevel) && bar.Low <= levels.ShortLevel
					side := 0
					var level, stopATR float64
					if longHit && !shortHit {
						side, level, stopATR = 1, levels.LongLevel, levels.LongStopATR
					} else if shortHit && !longHit {
						side, level, stopATR = -1, levels.ShortLevel, levels.ShortStopATR
					}
					if side != 0 && stopATR > 0 {
						fill := level
						if (side == 1 && bar.Open > level) || (side == -1 && bar.Open < level) {
							fill = bar.Open // gap oltre il livello: fill alla open
						}
						if eng.SlippageBps > 0 {
							intrabarSlip = fill * eng.SlippageBps / 10000.0
							if side == 1 {
								fill += intrabarSlip
							} else {
								fill -= intrabarSlip
							}
						}
						stop := fill - float64(side)*stopATR*atrPrev
						sig = strategy.Signal{Side: side, Strength: 1, StopPrice: stop, Reason: "intrabar breakout"}
						intrabarFill = fill
						isIntrabar = true
					}
				}
			}
		}
		if sig.Side == 0 {
			sig = strat.Next(ctx, i)
		}
		if sig.Side == 0 && lastStop.valid {
			if rc, ok := strat.(strategy.ReEntryChecker); ok {
				sig = rc.ReEntry(ctx, i, strategy.StopOutInfo{Side: lastStop.side, ExitBarIdx: lastStop.exitBarIdx})
			}
		}
		// with UseNextOpen a signal on the final bar can never fill: discard it
		// instead of entering at close and instantly closing EOD with phantom fees.
		// (intrabar fills happen on the current bar, so the limit does not apply)
		if sig.Side != 0 && !(eng.UseNextOpen && !isIntrabar && i+1 >= n) {
			atr := ctx.ATR[i]
			if math.IsNaN(atr) {
				atr = 0
			}

			// fill price: intrabar (già calcolato, slippage incluso) oppure next-open/close
			fillPrice := bar.Close
			fillTime := bar.Time
			slipAmt := 0.0
			if isIntrabar {
				fillPrice = intrabarFill
				slipAmt = intrabarSlip
			} else if eng.UseNextOpen && i+1 < n {
				fillPrice = bars[i+1].Open
				fillTime = bars[i+1].Time
				if eng.SlippageBps > 0 {
					slipAmt = fillPrice * eng.SlippageBps / 10000.0
					if sig.Side == 1 {
						fillPrice += slipAmt
					} else {
						fillPrice -= slipAmt
					}
				}
			} else if eng.SlippageBps > 0 {
				slipAmt = fillPrice * eng.SlippageBps / 10000.0
				if sig.Side == 1 {
					fillPrice += slipAmt
				} else {
					fillPrice -= slipAmt
				}
			}
			// slippage will be accounted against qty at order time (see below)

			// stop proposal (signal stop or 2×ATR fallback)
			stopPx := sig.StopPrice
			if math.IsNaN(stopPx) || stopPx <= 0 {
				stopPx = fillPrice - float64(sig.Side)*2*atr
				if math.IsNaN(stopPx) {
					stopPx = 0
				}
			}
			// the stop must sit on the losing side of the fill: a long stop above
			// entry (or a short stop below it) is invalid and must not be traded
			stopValid := (sig.Side == 1 && stopPx < fillPrice) || (sig.Side == -1 && stopPx > fillPrice)
			if !stopValid {
				sig.Side = 0
			}
			if sig.Side != 0 {

				// market state for risk engine — includes correlated heat for 2% limit
				// sameSideHeat computed earlier for pyramiding; recompute for fresh entry as well
				corrHeat := 0.0
				for _, p := range positions {
					if p.Side == sig.Side {
						corrHeat += p.RiskPct
					}
				}
				ms := risk.MarketState{
					Equity:                 curEq,
					Price:                  fillPrice,
					ATR:                    atr,
					StopPrice:              stopPx,
					Side:                   sig.Side,
					VolRegime:              ctx.VolRegime[i],
					ADX:                    ctx.ADX[i],
					FundingZ:               ctx.FundingZ[i],
					VolAnnualizedPct:       risk.AnnualizedVolPct(atr, fillPrice, intervalH),
					PortfolioHeatPct:       openHeat(),
					PortfolioCorrelatedPct: corrHeat,
					EquityDDPct:            -ddPct, // positive number
				}

				// ── pyramiding: count same-side units (logical entries) ──
				// When satellite splits an entry into 2 positions (core+sat), count logical entries = ceil(units/ satCount)
				// Simpler: use earliest.Units as pyramid count for its side; totalHeat for limit still sums all.
				sameSideHeat := 0.0
				var earliest *Position
				for _, p := range positions {
					if p.Side == sig.Side {
						sameSideHeat += p.RiskPct
						if earliest == nil {
							earliest = p
						}
					}
				}
				sameSideUnits := 0
				if earliest != nil {
					sameSideUnits = earliest.Units
					// If satellite split active, the total Units is ~2× logical, adjust maxUnits logic by using earliest only
				}
				if eng.PyramidingMode == "separate" {
					// unità logiche = core + gambe (satellite escluso: non partecipa al pyramid)
					sameSideUnits = 0
					for _, p := range positions {
						if p.Side == sig.Side && !p.IsSatellite {
							sameSideUnits++
						}
					}
				}
				hasSameSide := earliest != nil
				if hasSameSide {
					// ── pyramiding ──
					if risk.CanPyramid(earliest.EntryPrice, bar.Close, atr, sig.Side, sameSideUnits, eng.PyramidingMax, eng.PyramidStepATR) {
						dec := risk.Size(ms, lim)
						if dec.CappedByNotional {
							res.NotionalCapHits++
						}
						// risk_neutral: pyramid does not increase total risk (stop of existing moved to breakeven)
						if lim.PyramidingRiskNeutral && eng.PyramidingMode != "separate" {
							// keep total risk near base: pyramid risk is small, not added to heat
							dec.RiskPct = dec.RiskPct * 0.5 // pyramid at half risk when risk_neutral
							dec.RiskAmount = dec.RiskPct / 100 * ms.Equity
							dec.Qty = dec.RiskAmount / dec.StopDist
							dec.Notional = dec.Qty * fillPrice
							dec.Leverage = dec.Notional / ms.Equity
							dec.Factors = append(dec.Factors, "pyramid risk_neutral ×0.5")
						} else {
							dec.Notional = dec.Qty * fillPrice
							dec.RiskAmount = dec.Qty * dec.StopDist
							if ms.Equity > 0 {
								dec.RiskPct = dec.RiskAmount / ms.Equity * 100
								// total leverage after pyramid
								totalNotional := 0.0
								for _, p := range positions {
									if p.Side == sig.Side {
										totalNotional += p.Notional
									}
								}
								dec.Leverage = (totalNotional + dec.Notional) / ms.Equity
							}
						}
						if dec.Accept && dec.Qty > 0 {
							fee := fillPrice * dec.Qty * eng.FeeBps / 10000.0
							slipCost := slipAmt * dec.Qty
							equity -= fee
							totalFee += fee
							totalSlippage += slipCost
							if eng.PyramidingMode == "separate" {
								// gamba indipendente: stop proprio + exit wide Don55
								leg := &Position{
									Symbol: eng.Symbol, Side: sig.Side, Qty: dec.Qty,
									EntryPrice: fillPrice, EntryTime: fillTime, EntryATR: atr,
									StopPrice: stopPx, Units: 1, EntryBarIdx: i,
									RiskPct: dec.RiskPct, Leverage: dec.Leverage,
									Notional: dec.Notional, RiskAmount: dec.RiskAmount,
									SizingLog: logFactors(dec) + " | pyramid separate (wide Don55)",
									EntryFee:  fee, EntryReason: sig.Reason + " | pyramid separate",
									IsSatellite: false, DonExitLen: 55,
								}
								positions = append(positions, leg)
							} else if lim.PyramidingRiskNeutral {
								// keep total heat constant: pyramid is funded by trailing existing stop to breakeven
								earliest.EntryPrice = (earliest.EntryPrice*earliest.Qty + fillPrice*dec.Qty) / (earliest.Qty + dec.Qty)
								earliest.Qty += dec.Qty
								earliest.Units++
								earliest.Notional += dec.Notional
								earliest.EntryFee += fee
								earliest.Leverage = earliest.Notional / ms.Equity
								// risk stays same (risk_neutral) — don't add RiskPct/RiskAmount
								if !math.IsNaN(stopPx) {
									earliest.StopPrice = risk.TrailStopPosition(earliest.StopPrice, stopPx, sig.Side)
								}
								earliest.SizingLog += " | pyramid(risk_neutral): " + logFactors(dec)
							} else {
								totalQty := earliest.Qty + dec.Qty
								earliest.EntryPrice = (earliest.EntryPrice*earliest.Qty + fillPrice*dec.Qty) / totalQty
								earliest.Qty = totalQty
								earliest.Units++
								earliest.RiskPct += dec.RiskPct
								earliest.RiskAmount += dec.RiskAmount
								earliest.Notional += dec.Notional
								earliest.EntryFee += fee
								earliest.Leverage = earliest.Notional / ms.Equity
								if !math.IsNaN(stopPx) {
									earliest.StopPrice = risk.TrailStopPosition(earliest.StopPrice, stopPx, sig.Side)
								}
								earliest.SizingLog += " | pyramid: " + logFactors(dec)
							}
						}
					}
				} else if len(positions) == 0 {
					// ── fresh entry: full risk-based sizing + satellite 30% for positive skew ──
					dec := risk.Size(ms, lim)
					if dec.CappedByNotional {
						res.NotionalCapHits++
					}
					if dec.Accept && dec.Qty > 0 {
						fee := fillPrice * dec.Qty * eng.FeeBps / 10000.0
						slipCost := slipAmt * dec.Qty
						equity -= fee
						totalFee += fee
						totalSlippage += slipCost
						if lim.SatelliteEnabled && lim.SatelliteAlloc > 0 && lim.SatelliteAlloc < 1 {
							// split total qty into core (70%) and satellite (30%): satellite holds for large winners +5R/+10R
							coreQty := dec.Qty * (1 - lim.SatelliteAlloc)
							satQty := dec.Qty * lim.SatelliteAlloc
							coreRisk := dec.RiskPct * (1 - lim.SatelliteAlloc)
							satRisk := dec.RiskPct * lim.SatelliteAlloc
							coreNotional := coreQty * fillPrice
							satNotional := satQty * fillPrice
							coreLev := coreNotional / ms.Equity
							satLev := satNotional / ms.Equity
							corePos := &Position{
								Symbol: eng.Symbol, Side: sig.Side, Qty: coreQty,
								EntryPrice: fillPrice, EntryTime: fillTime, EntryATR: atr,
								StopPrice: stopPx, EntryReason: sig.Reason, Units: 1, EntryBarIdx: i,
								RiskPct: coreRisk, Leverage: coreLev,
								Notional: coreNotional, RiskAmount: coreRisk / 100 * ms.Equity,
								SizingLog:   logFactors(dec) + " | core 70%",
								EntryFee:    fee * (1 - lim.SatelliteAlloc),
								IsSatellite: false, DonExitLen: 20,
							}
							satPos := &Position{
								Symbol: eng.Symbol, Side: sig.Side, Qty: satQty,
								EntryPrice: fillPrice, EntryTime: fillTime, EntryATR: atr,
								StopPrice: stopPx, EntryReason: sig.Reason, Units: 1, EntryBarIdx: i,
								RiskPct: satRisk, Leverage: satLev,
								Notional: satNotional, RiskAmount: satRisk / 100 * ms.Equity,
								SizingLog:   logFactors(dec) + " | satellite 30% (wide Don55)",
								EntryFee:    fee * lim.SatelliteAlloc,
								IsSatellite: true, DonExitLen: 55,
							}
							positions = append(positions, corePos, satPos)
						} else {
							pos := &Position{
								Symbol: eng.Symbol, Side: sig.Side, Qty: dec.Qty,
								EntryPrice: fillPrice, EntryTime: fillTime, EntryATR: atr,
								StopPrice: stopPx, EntryReason: sig.Reason, Units: 1, EntryBarIdx: i,
								RiskPct: dec.RiskPct, Leverage: dec.Leverage,
								Notional: dec.Notional, RiskAmount: dec.RiskAmount,
								SizingLog: logFactors(dec), EntryFee: fee,
								DonExitLen: 20,
							}
							positions = append(positions, pos)
						}
					}
				}

				// intrabar same-bar stop — PESSIMISTICO: se dopo il fill anche lo stop
				// è toccabile nella stessa barra, assumiamo fill→stop (path inconoscibile)
				if isIntrabar {
					var survived []*Position
					for _, p := range positions {
						if p.EntryBarIdx != i {
							survived = append(survived, p)
							continue
						}
						stopHit := (p.Side == 1 && bar.Low <= p.StopPrice) || (p.Side == -1 && bar.High >= p.StopPrice)
						if !stopHit {
							survived = append(survived, p)
							continue
						}
						exitPrice := p.StopPrice
						if eng.SlippageBps > 0 {
							slip := exitPrice * eng.SlippageBps / 10000.0
							if p.Side == 1 {
								exitPrice -= slip
							} else {
								exitPrice += slip
							}
							totalSlippage += slip * p.Qty
						}
						// MAE/MFE della barra di entry
						if p.Side == 1 {
							if mae := (bar.Low - p.EntryPrice) / p.EntryPrice * 100; mae < p.MAE {
								p.MAE = mae
							}
							if mfe := (bar.High - p.EntryPrice) / p.EntryPrice * 100; mfe > p.MFE {
								p.MFE = mfe
							}
						} else {
							if mae := (p.EntryPrice - bar.High) / p.EntryPrice * 100; mae < p.MAE {
								p.MAE = mae
							}
							if mfe := (p.EntryPrice - bar.Low) / p.EntryPrice * 100; mfe > p.MFE {
								p.MFE = mfe
							}
						}
						recordExit(p, exitPrice, "stop_same_bar", i)
						// stop_same_bar è comunque uno stop-out: alimenta la logica
						// re-entry (ReEntryChecker) — caso d'uso core del whipsaw intrabar
						lastStop = stopOutState{valid: true, side: p.Side, exitBarIdx: i}
					}
					positions = survived
				}
			}
		}

		// equity curve point with heat + leverage audit
		unrealized = 0.0
		for _, pos := range positions {
			// a position entered with a next-open fill does not exist yet at this
			// bar's close: mark it at its fill price (unrealized = 0 at signal bar)
			markPx := bar.Close
			if eng.UseNextOpen && pos.EntryBarIdx == i {
				markPx = pos.EntryPrice
			}
			if pos.Side == 1 {
				unrealized += (markPx - pos.EntryPrice) * pos.Qty
			} else {
				unrealized += (pos.EntryPrice - markPx) * pos.Qty
			}
		}
		curEq = equity + unrealized
		if curEq > peak {
			peak = curEq
		}
		dd := 0.0
		if peak > 0 {
			dd = (curEq - peak) / peak * 100
		}
		heat := openHeat()
		equityCurve = append(equityCurve, EquityPoint{Time: bar.Time, Equity: curEq, Drawdown: dd, Price: bar.Close, Heat: heat, Leverage: openNotional(bar.Close) / math.Max(curEq, 1)})
	}

	// close remaining at last bar
	for _, pos := range positions {
		exitPrice := bars[n-1].Close
		var pnl float64
		if pos.Side == 1 {
			pnl = (exitPrice - pos.EntryPrice) * pos.Qty
		} else {
			pnl = (pos.EntryPrice - exitPrice) * pos.Qty
		}
		exitFee := exitPrice * pos.Qty * eng.FeeBps / 10000.0
		fee := pos.EntryFee + exitFee
		pnlNet := pnl - fee - pos.FundingAccum
		equity += pnl - exitFee
		totalFee += exitFee
		rMult := 0.0
		if pos.RiskAmount > 0 {
			rMult = pnlNet / pos.RiskAmount
		}
		trades = append(trades, Trade{Symbol: eng.Symbol, Side: pos.Side, EntryTime: pos.EntryTime, ExitTime: bars[n-1].Time, EntryPrice: pos.EntryPrice, ExitPrice: exitPrice, Qty: pos.Qty, EntryATR: pos.EntryATR, StopPrice: pos.StopPrice, ExitReason: "eod", EntryReason: pos.EntryReason, PnL: pnl, PnLNet: pnlNet, Fee: fee, FundingCost: pos.FundingAccum, BarsHeld: n - 1 - pos.EntryBarIdx, RiskPct: pos.RiskPct, Leverage: pos.Leverage, Notional: pos.Notional, StopDist: math.Abs(pos.EntryPrice - pos.StopPrice), RMultiple: rMult, SizingLog: pos.SizingLog, IsSatellite: pos.IsSatellite})
	}

	final := equity
	if len(equityCurve) > 0 {
		equityCurve[len(equityCurve)-1].Equity = final
	}
	gross, net := 0.0, 0.0
	maxLev, sumLev, maxRisk, sumRisk, maxHeat := 0.0, 0.0, 0.0, 0.0, 0.0
	for _, t := range trades {
		gross += t.PnL
		net += t.PnLNet
		if t.Leverage > maxLev {
			maxLev = t.Leverage
		}
		sumLev += t.Leverage
		if t.RiskPct > maxRisk {
			maxRisk = t.RiskPct
		}
		sumRisk += t.RiskPct
	}
	for _, e := range equityCurve {
		if e.Heat > maxHeat {
			maxHeat = e.Heat
		}
	}
	tn := float64(len(trades))
	if tn > 0 {
		res.AvgLeverage = sumLev / tn
		res.AvgRiskPct = sumRisk / tn
	}
	res.MaxLeverageUsed = maxLev
	res.MaxRiskPctUsed = maxRisk
	res.MaxHeatSeen = maxHeat

	res.Bars = bars
	res.Trades = trades
	res.Equity = equityCurve
	res.FinalEquity = final
	res.GrossPnL = gross
	res.NetPnL = net
	res.TotalFee = totalFee
	res.TotalFunding = totalFundingNet
	res.TotalSlippage = totalSlippage
	return res
}

func logFactors(d risk.SizingDecision) string {
	s := ""
	for i, f := range d.Factors {
		if i > 0 {
			s += "; "
		}
		s += f
	}
	return s
}

func intervalHours(interval string) float64 {
	switch interval {
	case "1m":
		return 1.0 / 60
	case "5m":
		return 5.0 / 60
	case "15m":
		return 15.0 / 60
	case "1h":
		return 1
	case "4h":
		return 4
	case "1d":
		return 24
	default:
		return 4
	}
}
