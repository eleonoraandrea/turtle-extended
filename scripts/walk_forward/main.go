package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/atps/atps/internal/analysis"
	"github.com/atps/atps/internal/backtest"
	"github.com/atps/atps/internal/config"
	"github.com/atps/atps/internal/data"
	"github.com/atps/atps/internal/strategy"
)

func main() {
	cfg, _ := config.Load("configs/default.yaml")
	sym := "BTCUSDT"
	bars, _ := data.LoadBarsCSV(fmt.Sprintf("data/raw/%s_%s.csv", sym, cfg.General.Interval))
	if len(bars) == 0 {
		panic("no data")
	}
	strat := strategy.New("D", cfg)
	eng := backtest.EngineConfig{Variant: "D", Symbol: sym, InitialCapital: cfg.General.InitialCapital, FeeBps: cfg.Costs.FeeBps, SlippageBps: cfg.Costs.SlippageBps, Leverage: cfg.Costs.Leverage, UseNextOpen: true, PyramidingMax: 4, PyramidStepATR: 0.5, TrailATRMult: 3.0, TrailMode: "chandelier", DonExit: 20}
	wf := analysis.WalkForward(bars, strat, cfg, eng, cfg.WalkForward.Folds, cfg.WalkForward.TrainRatio)
	b, _ := json.MarshalIndent(wf, "", "  ")
	os.WriteFile("reports/walk_forward.json", b, 0644)
	fmt.Printf("WF OOS %.2f%% decay %.2f\n", wf.OOSReturn, wf.Decay)
}
