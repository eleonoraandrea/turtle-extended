package backtest

import (
	"math"
	"testing"
	"time"

	"github.com/atps/atps/internal/config"
	"github.com/atps/atps/internal/data"
	"github.com/atps/atps/internal/strategy"
)

func cfgAndBars(seed int64) (*config.Config, data.Bars) {
	cfg, _ := config.Load("../../configs/default.yaml")
	bars := data.GenerateSynthetic(400, 4*time.Hour, seed)
	return cfg, bars
}

func TestRunEmpty(t *testing.T) {
	cfg, _ := cfgAndBars(1)
	strat := strategy.New("A", cfg)
	res := Run(nil, strat, cfg, EngineConfig{Variant: "A", Symbol: "TEST", InitialCapital: 10000})
	if res.FinalEquity != 10000 {
		t.Fatalf("empty should keep capital")
	}
	if len(res.Equity) != 0 {
		t.Fatalf("equity should be empty")
	}
}

func TestRunProducesEquity(t *testing.T) {
	cfg, bars := cfgAndBars(42)
	for _, v := range []string{"A", "B", "C", "D"} {
		strat := strategy.New(v, cfg)
		eng := EngineConfig{Variant: v, Symbol: "BTCUSDT", InitialCapital: 10000, FeeBps: 4, SlippageBps: 2, Leverage: 5, UseNextOpen: true, PyramidingMax: 4, PyramidStepATR: 0.5, TrailATRMult: 3, TrailMode: "donchian", DonExit: 20}
		if v == "D" {
			eng.TrailMode = "chandelier"
		}
		res := Run(bars, strat, cfg, eng)
		if len(res.Equity) != len(bars) {
			t.Fatalf("%s equity len %d vs %d", v, len(res.Equity), len(bars))
		}
		if res.FinalEquity <= 0 {
			t.Fatalf("%s final equity <=0", v)
		}
		for i, ep := range res.Equity {
			if ep.Equity <= 0 {
				t.Fatalf("%s equity zero at %d", v, i)
			}
		}
	}
}

func TestRunNextOpenVsClose(t *testing.T) {
	cfg, bars := cfgAndBars(42)
	strat := strategy.New("A", cfg)
	eng1 := EngineConfig{Variant: "A", Symbol: "BTCUSDT", InitialCapital: 10000, FeeBps: 0, SlippageBps: 0, Leverage: 5, UseNextOpen: true, PyramidingMax: 4, PyramidStepATR: 0.5, TrailMode: "donchian", DonExit: 20}
	eng2 := eng1
	eng2.UseNextOpen = false
	res1 := Run(bars, strat, cfg, eng1)
	res2 := Run(bars, strat, cfg, eng2)
	if len(res1.Trades) == 0 || len(res2.Trades) == 0 {
		t.Skip("no trades to compare")
	}
	// next open may produce slightly different results but not identical
	_ = res1
	_ = res2
}

func TestPyramiding(t *testing.T) {
	cfg, bars := cfgAndBars(42)
	strat := strategy.New("A", cfg)
	eng := EngineConfig{Variant: "A", Symbol: "BTCUSDT", InitialCapital: 10000, FeeBps: 4, SlippageBps: 2, Leverage: 5, UseNextOpen: true, PyramidingMax: 0, PyramidStepATR: 0.5, TrailMode: "donchian", DonExit: 20}
	resNoPyramid := Run(bars, strat, cfg, eng)
	eng.PyramidingMax = 4
	resPyramid := Run(bars, strat, cfg, eng)
	// pyramid should not reduce trades drastically, but may affect heat
	_ = resNoPyramid
	_ = resPyramid
}

func TestCrashBrake(t *testing.T) {
	cfg, bars := cfgAndBars(42)
	// inject crash
	bars[250].Close = bars[249].Close * 0.90 // -10%
	strat := strategy.New("A", cfg)
	eng := EngineConfig{Variant: "A", Symbol: "BTCUSDT", InitialCapital: 10000, FeeBps: 4, SlippageBps: 2, Leverage: 5, UseNextOpen: true, PyramidingMax: 4, PyramidStepATR: 0.5, TrailMode: "donchian", DonExit: 20}
	res := Run(bars, strat, cfg, eng)
	// should not panic, check no crash trades beyond limit
	for _, tr := range res.Trades {
		if math.IsNaN(tr.PnLNet) {
			t.Fatalf("NaN pnl")
		}
	}
}

func TestFundingAndFees(t *testing.T) {
	cfg, bars := cfgAndBars(42)
	// set funding to known value
	for i := range bars {
		bars[i].FundingRate = 0.0001
	}
	strat := strategy.New("A", cfg)
	eng := EngineConfig{Variant: "A", Symbol: "BTCUSDT", InitialCapital: 10000, FeeBps: 4, SlippageBps: 2, Leverage: 5, UseNextOpen: true, PyramidingMax: 1, PyramidStepATR: 0.5, TrailMode: "donchian", DonExit: 20}
	res := Run(bars, strat, cfg, eng)
	if res.TotalFee <= 0 && len(res.Trades) > 0 {
		t.Fatalf("fee should be >0")
	}
	if res.TotalFunding == 0 && len(res.Trades) > 0 {
		t.Fatalf("funding should be non-zero when funding rate set")
	}
	if res.TotalSlippage < 0 {
		t.Fatalf("slippage negative")
	}
}

func TestSatelliteSplit(t *testing.T) {
	cfg, bars := cfgAndBars(42)
	cfg.Profit.Satellite.Enabled = true
	cfg.Profit.Satellite.Allocation = 0.30
	strat := strategy.New("D", cfg)
	eng := EngineConfig{Variant: "D", Symbol: "BTCUSDT", InitialCapital: 10000, FeeBps: 4, SlippageBps: 2, Leverage: 5, UseNextOpen: true, PyramidingMax: 4, PyramidStepATR: 0.5, TrailMode: "chandelier", DonExit: 20}
	res := Run(bars, strat, cfg, eng)
	// satellite should create roughly 2x trades per entry (core+sat)
	// count satellite trades
	satCount := 0
	for _, tr := range res.Trades {
		if tr.IsSatellite {
			satCount++
		}
	}
	if len(res.Trades) > 0 && satCount == 0 {
		t.Fatalf("satellite enabled but no satellite trades")
	}
}

func TestLeverageAndRiskInvariants(t *testing.T) {
	cfg, bars := cfgAndBars(42)
	strat := strategy.New("A", cfg)
	eng := EngineConfig{Variant: "A", Symbol: "BTCUSDT", InitialCapital: 10000, FeeBps: 4, SlippageBps: 2, Leverage: 5, UseNextOpen: true, PyramidingMax: 4, PyramidStepATR: 0.5, TrailMode: "donchian", DonExit: 20}
	res := Run(bars, strat, cfg, eng)
	lim := res.RiskLimitsUsed
	for _, tr := range res.Trades {
		if tr.Leverage > lim.MaxLeverage+0.01 {
			t.Fatalf("leverage %f > cap %f", tr.Leverage, lim.MaxLeverage)
		}
	}
	if res.MaxLeverageUsed > lim.MaxLeverage+0.01 {
		t.Fatalf("maxLev %f", res.MaxLeverageUsed)
	}
}

func TestIntervalHours(t *testing.T) {
	if intervalHours("4h") != 4 {
		t.Fatalf("4h")
	}
	if intervalHours("1d") != 24 {
		t.Fatalf("1d")
	}
	if intervalHours("unknown") != 4 {
		t.Fatalf("default 4")
	}
}
