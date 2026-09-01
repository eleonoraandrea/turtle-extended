package metrics

import (
	"math"
	"testing"
	"time"

	"github.com/atps/atps/internal/backtest"
	"github.com/atps/atps/internal/config"
	"github.com/atps/atps/internal/data"
	"github.com/atps/atps/internal/strategy"
)

func syntheticResult(variant string) (*backtest.Result, *config.Config) {
	cfg, _ := config.Load("../../configs/default.yaml")
	bars := data.GenerateSynthetic(500, 4*time.Hour, 42)
	strat := strategy.New(variant, cfg)
	eng := backtest.EngineConfig{Variant: variant, Symbol: "BTCUSDT", InitialCapital: 10000, FeeBps: 4, SlippageBps: 2, Leverage: 5, UseNextOpen: true, PyramidingMax: 4, PyramidStepATR: 0.5, TrailATRMult: 3, TrailMode: "donchian", DonExit: 20}
	if variant == "D" {
		eng.TrailMode = "chandelier"
	}
	res := backtest.Run(bars, strat, cfg, eng)
	return res, cfg
}

func TestComputeBasics(t *testing.T) {
	res, _ := syntheticResult("A")
	stats := Compute(res)
	if stats.Trades == 0 {
		t.Fatalf("no trades")
	}
	if stats.ReturnPct == 0 {
		t.Logf("return 0 maybe but trades %d", stats.Trades)
	}
	if math.IsNaN(stats.Sharpe) && stats.Trades > 5 {
		t.Fatalf("sharpe NaN")
	}
	if stats.MaxDD > 0 {
		t.Fatalf("maxDD should be negative %f", stats.MaxDD)
	}
	if stats.ProfitFactor < 0 {
		t.Fatalf("PF negative")
	}
	if stats.ExposurePct < 0 || stats.ExposurePct > 100 {
		t.Fatalf("exposure")
	}
}

func TestComputeEmpty(t *testing.T) {
	res := &backtest.Result{Symbol: "TEST", InitialCapital: 10000, FinalEquity: 10000}
	stats := Compute(res)
	if stats.ReturnPct != 0 {
		t.Fatalf("empty return")
	}
}

func TestSkewAndExpectancy(t *testing.T) {
	res, _ := syntheticResult("A")
	stats := Compute(res)
	if len(res.Trades) >= 3 {
		if math.IsNaN(stats.Skew) {
			t.Fatalf("skew NaN")
		}
		// expectancyR should be finite
		if math.IsNaN(stats.ExpectancyR) {
			t.Fatalf("expectancyR NaN")
		}
	}
}

func TestMonthlyYearly(t *testing.T) {
	res, _ := syntheticResult("A")
	stats := Compute(res)
	if len(stats.MonthlyReturns) == 0 {
		t.Fatalf("monthly empty")
	}
	if len(stats.YearlyReturns) == 0 {
		t.Fatalf("yearly empty")
	}
	for _, v := range stats.MonthlyReturns {
		if math.IsNaN(v) {
			t.Fatalf("monthly NaN")
		}
	}
}

func TestSortedMonthly(t *testing.T) {
	m := map[string]float64{"2020-02": 1, "2020-01": 2}
	ks := SortedMonthly(m)
	if ks[0] != "2020-01" || ks[1] != "2020-02" {
		t.Fatalf("sorted %v", ks)
	}
}

func TestDownsideAndCalmar(t *testing.T) {
	res, _ := syntheticResult("B")
	stats := Compute(res)
	// Calmar may be NaN if MaxDD 0, but not inf
	if math.IsInf(stats.Calmar, 0) {
		t.Fatalf("calmar inf")
	}
}
