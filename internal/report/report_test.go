package report

import (
	"os"
	"testing"
	"time"

	"github.com/atps/atps/internal/backtest"
	"github.com/atps/atps/internal/config"
	"github.com/atps/atps/internal/data"
	"github.com/atps/atps/internal/metrics"
	"github.com/atps/atps/internal/strategy"
)

func TestGenerate(t *testing.T) {
	cfg, _ := config.Load("../../configs/default.yaml")
	bars := data.GenerateSynthetic(300, 4*time.Hour, 42)
	strat := strategy.New("A", cfg)
	eng := backtest.EngineConfig{Variant: "A", Symbol: "BTCUSDT", InitialCapital: 10000, FeeBps: 4, SlippageBps: 2, Leverage: 5, UseNextOpen: true, PyramidingMax: 4, PyramidStepATR: 0.5, TrailMode: "donchian", DonExit: 20}
	res := backtest.Run(bars, strat, cfg, eng)
	stats := metrics.Compute(res)
	path := t.TempDir() + "/report.html"
	in := Input{Config: cfg, Bars: bars, Result: res, Stats: stats, Symbol: "BTCUSDT", Variant: "A", GeneratedAt: time.Now()}
	if err := Generate(path, in); err != nil {
		t.Fatalf("generate %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("report not exists")
	}
	// check file contains expected markers
	b, _ := os.ReadFile(path)
	if len(b) == 0 {
		t.Fatalf("empty report")
	}
	// comparison
	rows := []ComparisonRow{{Symbol: "BTCUSDT", Variant: "A", Stats: stats}}
	cmpPath := t.TempDir() + "/cmp.html"
	if err := GenerateComparison(cmpPath, rows, cfg); err != nil {
		t.Fatalf("cmp %v", err)
	}
}
