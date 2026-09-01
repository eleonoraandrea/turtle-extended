package risk

import (
	"math"
	"testing"

	"github.com/atps/atps/internal/config"
)

func TestSizeRejectInvalid(t *testing.T) {
	lim := DefaultLimits()
	dec := Size(MarketState{Equity: 0, Price: 100, StopPrice: 90}, lim)
	if dec.Accept {
		t.Fatalf("should reject 0 equity")
	}
	dec2 := Size(MarketState{Equity: 10000, Price: 0, StopPrice: 90}, lim)
	if dec2.Accept {
		t.Fatalf("should reject 0 price")
	}
}

func TestSizeStopFallbackATR(t *testing.T) {
	lim := DefaultLimits()
	ms := MarketState{Equity: 10000, Price: 100, StopPrice: math.NaN(), ATR: 2, Side: 1}
	dec := Size(ms, lim)
	if !dec.Accept {
		t.Fatalf("should fallback to 2*ATR, got %v", dec.Factors)
	}
	if math.Abs(dec.StopDist-4.0) > 1e-9 {
		t.Fatalf("stopDist %f want 4", dec.StopDist)
	}
	// same price as stop -> distance 0 -> fallback
	ms0 := MarketState{Equity: 10000, Price: 100, StopPrice: 100, ATR: 2, Side: 1}
	dec0 := Size(ms0, lim)
	if !dec0.Accept || math.Abs(dec0.StopDist-4.0) > 1e-9 {
		t.Fatalf("zero distance fallback")
	}
	// no ATR and no stop -> reject
	ms2 := MarketState{Equity: 10000, Price: 100, StopPrice: math.NaN(), ATR: 0}
	dec2 := Size(ms2, lim)
	if dec2.Accept {
		t.Fatalf("should reject no stop nor ATR")
	}
}

func TestSizeHeatExhausted(t *testing.T) {
	lim := RiskLimits{RiskPerTradePct: 2, BaseRiskPct: 1, MinRiskPct: 0.25, MaxRiskPct: 2, MaxHeatPct: 3, MaxCorrelatedPct: 2, MaxLeverage: 5, MinLeverageCap: 0.7}
	ms := MarketState{Equity: 10000, Price: 100, StopPrice: 90, Side: 1, PortfolioHeatPct: 3.5}
	dec := Size(ms, lim)
	if dec.Accept {
		t.Fatalf("should reject heat exhausted")
	}
}

func TestSizeVolTarget(t *testing.T) {
	lim := RiskLimits{BaseRiskPct: 1, MinRiskPct: 0.25, MaxRiskPct: 2, MaxHeatPct: 10, MaxLeverage: 10, MinLeverageCap: 0.7, VolTargetPct: 50}
	// high vol 100% -> scale 0.5
	ms := MarketState{Equity: 10000, Price: 100, StopPrice: 90, Side: 1, VolAnnualizedPct: 100}
	dec := Size(ms, lim)
	if !dec.Accept {
		t.Fatalf("reject")
	}
	if dec.RiskPct > 0.6 || dec.RiskPct < 0.4 {
		t.Fatalf("vol target scaled risk %f want ~0.5", dec.RiskPct)
	}
}

func TestSizeLeverageCap(t *testing.T) {
	lim := RiskLimits{BaseRiskPct: 1, MinRiskPct: 0.25, MaxRiskPct: 2, MaxHeatPct: 10, MaxLeverage: 5, MinLeverageCap: 0.7, ADXSoftThreshold: 20}
	// low vol should boost cap to 1.3*5=6.5 capped at 5 hard
	msLow := MarketState{Equity: 10000, Price: 100, StopPrice: 99.9, ATR: 0.01, Side: 1, VolRegime: 10, ADX: 30}
	decLow := Size(msLow, lim)
	if decLow.LeverageCap != 5 {
		t.Fatalf("low vol cap capped at hard 5, got %f", decLow.LeverageCap)
	}
	// high vol regime 90 -> 0.70*5=3.5
	msHigh := MarketState{Equity: 10000, Price: 100, StopPrice: 99.9, ATR: 0.01, Side: 1, VolRegime: 90, ADX: 30}
	decHigh := Size(msHigh, lim)
	if math.Abs(decHigh.LeverageCap-3.5) > 0.01 {
		t.Fatalf("high vol cap 3.5 got %f", decHigh.LeverageCap)
	}
	// weak ADX
	msADX := MarketState{Equity: 10000, Price: 100, StopPrice: 99.9, ATR: 0.01, Side: 1, VolRegime: 50, ADX: 10}
	decADX := Size(msADX, lim)
	if math.Abs(decADX.LeverageCap-4.0) > 0.01 { // 5*0.80=4
		t.Fatalf("ADX cap 4 got %f", decADX.LeverageCap)
	}
	// funding extreme — isolate from vol/adx by setting them to NaN
	msFund := MarketState{Equity: 10000, Price: 100, StopPrice: 99.9, ATR: 0.01, Side: 1, VolRegime: math.NaN(), ADX: math.NaN(), FundingZ: 3.0}
	decFund := Size(msFund, lim)
	if math.Abs(decFund.LeverageCap-4.25) > 0.01 { // 5*0.85=4.25
		t.Fatalf("funding cap 4.25 got %f", decFund.LeverageCap)
	}
}

func TestAnnualizedVol(t *testing.T) {
	v := AnnualizedVolPct(2, 100, 4)
	// perYear 2190 sqrt ~46.8 *0.02 =93.6
	if math.Abs(v-93.6) > 1.0 {
		t.Fatalf("annualized vol %f", v)
	}
	if AnnualizedVolPct(0, 100, 4) != 0 {
		t.Fatalf("zero atr")
	}
}

func TestCanPyramid(t *testing.T) {
	if CanPyramid(100, 101, 1, 1, 0, 4, 0.5) != true {
		t.Fatalf("should pyramid 0.5 ATR up")
	}
	if CanPyramid(100, 100.3, 1, 1, 0, 4, 0.5) != false {
		t.Fatalf("not enough move")
	}
	if CanPyramid(100, 99, 1, -1, 0, 4, 0.5) != true {
		t.Fatalf("short pyramid")
	}
	if CanPyramid(100, 101, 1, 1, 4, 4, 0.5) != false {
		t.Fatalf("max units reached")
	}
	if CanPyramid(100, 101, 0, 1, 0, 4, 0.5) != false {
		t.Fatalf("zero atr")
	}
}

func TestTrailStop(t *testing.T) {
	if TrailStopPosition(90, 92, 1) != 92 {
		t.Fatalf("long trail up")
	}
	if TrailStopPosition(90, 88, 1) != 90 {
		t.Fatalf("long not down")
	}
	if TrailStopPosition(110, 108, -1) != 108 {
		t.Fatalf("short trail down")
	}
	if TrailStopPosition(110, 112, -1) != 110 {
		t.Fatalf("short not up")
	}
	if !math.IsNaN(TrailStopPosition(90, math.NaN(), 1)) && TrailStopPosition(90, math.NaN(), 1) != 90 {
		t.Fatalf("NaN new stop")
	}
}

func TestValidateLimits(t *testing.T) {
	lim := DefaultLimits()
	if err := ValidateLimitInvariants(lim); err != nil {
		t.Fatalf("default invalid %v", err)
	}
	lim.RiskPerTradePct = 0
	if err := ValidateLimitInvariants(lim); err == nil {
		t.Fatalf("should fail 0 risk")
	}
	lim = DefaultLimits()
	lim.MaxHeatPct = 0.5
	lim.RiskPerTradePct = 2
	if err := ValidateLimitInvariants(lim); err == nil {
		t.Fatalf("heat < risk should fail")
	}
}

func TestLimitsFromConfig(t *testing.T) {
	cfg, err := config.Load("../../configs/default.yaml")
	if err != nil {
		t.Fatalf("load %v", err)
	}
	limA := LimitsFromConfig(cfg, "A")
	if limA.MaxRiskPct != cfg.VariantA.RiskPct {
		t.Fatalf("variant A max risk %f != cfg %f", limA.MaxRiskPct, cfg.VariantA.RiskPct)
	}
	limD := LimitsFromConfig(cfg, "D")
	if limD.MaxRiskPct != cfg.VariantD.RiskPct {
		t.Fatalf("D max %f", limD.MaxRiskPct)
	}
	// B should disable vol target
	limB := LimitsFromConfig(cfg, "B")
	if limB.VolTargetPct != 0 {
		t.Fatalf("B vol target should be 0, got %f", limB.VolTargetPct)
	}
	// satellite enabled per config true
	if !limD.SatelliteEnabled {
		t.Fatalf("D satellite should be enabled per config")
	}
	// nil cfg returns defaults
	limN := LimitsFromConfig(nil, "A")
	if limN.MaxLeverage != 5 {
		t.Fatalf("nil cfg default")
	}
}

func TestIsCrashBar(t *testing.T) {
	if !IsCrashBar(8.5, 8.0) {
		t.Fatalf("crash")
	}
	if IsCrashBar(7.9, 8.0) {
		t.Fatalf("not crash")
	}
	if !IsCrashBar(-9, 8) {
		t.Fatalf("abs")
	}
}
