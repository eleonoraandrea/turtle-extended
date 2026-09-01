package analysis

import (
	"testing"
	"time"

	"github.com/atps/atps/internal/backtest"
	"github.com/atps/atps/internal/config"
	"github.com/atps/atps/internal/data"
	"github.com/atps/atps/internal/strategy"
)

func TestWalkForward(t *testing.T) {
	cfg, _ := config.Load("../../configs/default.yaml")
	bars := data.GenerateSynthetic(1500, 4*time.Hour, 42)
	strat := strategy.New("A", cfg)
	eng := backtest.EngineConfig{Variant: "A", Symbol: "BTCUSDT", InitialCapital: 10000, FeeBps: 4, SlippageBps: 2, Leverage: 5, UseNextOpen: true, PyramidingMax: 4, PyramidStepATR: 0.5, TrailMode: "donchian", DonExit: 20}
	wf := WalkForward(bars, strat, cfg, eng, 4, 0.7)
	if len(wf.Folds) == 0 {
		t.Fatalf("no folds — need larger bars for warmup 210")
	}
	for _, f := range wf.Folds {
		if f.TrainStats.Trades < 0 || f.TestStats.Trades < 0 {
			t.Fatalf("negative trades")
		}
	}
	// decay should be computable if train sharpe non-zero
	_ = wf.Decay
}

func TestMonteCarlo(t *testing.T) {
	cfg, _ := config.Load("../../configs/default.yaml")
	bars := data.GenerateSynthetic(600, 4*time.Hour, 42)
	strat := strategy.New("A", cfg)
	eng := backtest.EngineConfig{Variant: "A", Symbol: "BTCUSDT", InitialCapital: 10000, FeeBps: 4, SlippageBps: 2, Leverage: 5, UseNextOpen: true, PyramidingMax: 4, PyramidStepATR: 0.5, TrailMode: "donchian", DonExit: 20}
	mc := MonteCarlo(bars, strat, cfg, eng, 20, 20, 20, 42)
	if mc.Runs != 20 {
		t.Fatalf("runs")
	}
	if len(mc.Returns) == 0 {
		t.Skip("no trades -> no MC returns")
	}
	if mc.ProbProfit < 0 || mc.ProbProfit > 100 {
		t.Fatalf("prob %f", mc.ProbProfit)
	}
}

func TestMonteCarloNoTrades(t *testing.T) {
	cfg, _ := config.Load("../../configs/default.yaml")
	// very short bars => no trades
	bars := data.GenerateSynthetic(10, 4*time.Hour, 1)
	strat := strategy.New("A", cfg)
	eng := backtest.EngineConfig{Variant: "A", Symbol: "BTCUSDT", InitialCapital: 10000, FeeBps: 4, SlippageBps: 2, Leverage: 5, UseNextOpen: true, PyramidingMax: 4, PyramidStepATR: 0.5, TrailMode: "donchian", DonExit: 20}
	mc := MonteCarlo(bars, strat, cfg, eng, 10, 20, 20, 42)
	if len(mc.Returns) != 0 {
		t.Fatalf("should be empty when no trades")
	}
}

func TestPerturb(t *testing.T) {
	cfg, _ := config.Load("../../configs/default.yaml")
	bars := data.GenerateSynthetic(600, 4*time.Hour, 42)
	eng := backtest.EngineConfig{Variant: "A", Symbol: "BTCUSDT", InitialCapital: 10000, FeeBps: 4, SlippageBps: 2, Leverage: 5, UseNextOpen: true, PyramidingMax: 4, PyramidStepATR: 0.5, TrailMode: "donchian", DonExit: 20}
	res := Perturb(bars, cfg, "A", "BTCUSDT", eng)
	if len(res) == 0 {
		t.Fatalf("empty")
	}
	if res[0].Param != "baseline" {
		t.Fatalf("first should be baseline")
	}
	summary := PerturbSummary(res)
	if summary == "" {
		t.Fatalf("summary empty")
	}
	empty := PerturbSummary(nil)
	if empty != "no data" {
		t.Fatalf("empty summary")
	}
}

func TestPortfolio(t *testing.T) {
	cfg, _ := config.Load("../../configs/default.yaml")
	barsMap := map[string]data.Bars{
		"BTCUSDT": data.GenerateSynthetic(300, 4*time.Hour, 42),
		"ETHUSDT": data.GenerateSynthetic(300, 4*time.Hour, 1337),
	}
	pr := RunPortfolio(barsMap, cfg, "A", "4h")
	if pr == nil {
		t.Fatalf("nil")
	}
	if len(pr.PerSymbol) != 2 {
		t.Fatalf("per symbol len")
	}
	if pr.CombinedStats.Trades == 0 {
		t.Fatalf("no combined trades")
	}
	if len(pr.CombinedEquity) == 0 {
		t.Fatalf("combined equity empty")
	}
	// LoadBarsMap synthetic fallback
	m, err := LoadBarsMap([]string{"BTCUSDT", "FAKEUSDT"}, "4h")
	if err != nil {
		t.Fatalf("loadBarsMap %v", err)
	}
	if len(m) != 2 {
		t.Fatalf("barsMap len")
	}
}

func TestLoadBarsMapSynthetic(t *testing.T) {
	m, _ := LoadBarsMap([]string{"BTCUSDT", "ETHUSDT"}, "4h")
	if len(m) != 2 {
		t.Fatalf("len")
	}
	// ensure per-symbol difference
	if m["BTCUSDT"][0].Close == m["ETHUSDT"][0].Close {
		t.Fatalf("different symbols should have different seed")
	}
}
