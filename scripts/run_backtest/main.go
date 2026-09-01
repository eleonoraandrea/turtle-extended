package main

import (
	"fmt"
	"time"

	"github.com/atps/atps/internal/backtest"
	"github.com/atps/atps/internal/config"
	"github.com/atps/atps/internal/data"
	"github.com/atps/atps/internal/metrics"
	"github.com/atps/atps/internal/report"
	"github.com/atps/atps/internal/strategy"
)

func main() {
	cfg, _ := config.Load("configs/default.yaml")
	for _, sym := range cfg.General.Symbols {
		for _, v := range []string{"A", "B", "C", "D"} {
			path := fmt.Sprintf("data/raw/%s_%s.csv", sym, cfg.General.Interval)
			bars, _ := data.LoadBarsCSV(path)
			if len(bars) == 0 {
				bars = data.GenerateSynthetic(3000, 4*time.Hour, 42)
			}
			strat := strategy.New(v, cfg)
			eng := backtest.EngineConfig{Variant: v, Symbol: sym, InitialCapital: cfg.General.InitialCapital, FeeBps: cfg.Costs.FeeBps, SlippageBps: cfg.Costs.SlippageBps, Leverage: cfg.Costs.Leverage, UseNextOpen: true, PyramidingMax: cfg.Backtest.PyramidingMaxUnits, PyramidStepATR: cfg.Backtest.PyramidStepATR, TrailATRMult: cfg.Backtest.TrailATRMult, TrailMode: "chandelier", DonExit: 20}
			if v != "D" {
				eng.TrailMode = "donchian"
			}
			res := backtest.Run(bars, strat, cfg, eng)
			stats := metrics.Compute(res)
			out := fmt.Sprintf("reports/%s_%s.html", sym, v)
			report.Generate(out, report.Input{Config: cfg, Bars: bars, Result: res, Stats: stats, Symbol: sym, Variant: v, GeneratedAt: time.Now()})
			fmt.Printf("%s %s %.2f%% Sharpe %.2f -> %s\n", sym, v, stats.ReturnPct, stats.Sharpe, out)
		}
	}
}
