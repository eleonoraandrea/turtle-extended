// baseline_split: stampa train/test/full metriche di una config su un CSV
// (evidenza holdout per i gate di validazione — vedi reports/V2_VALIDATION.md).
// Uso: go run ./scripts/baseline_split -config configs/btc_opt.yaml -csv data/raw/BTCUSDT_4h.csv -variant A
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/atps/atps/internal/backtest"
	"github.com/atps/atps/internal/config"
	"github.com/atps/atps/internal/data"
	"github.com/atps/atps/internal/metrics"
	"github.com/atps/atps/internal/strategy"
)

func main() {
	cfgPath := flag.String("config", "configs/btc_opt.yaml", "")
	csvPath := flag.String("csv", "data/raw/BTCUSDT_4h.csv", "")
	variant := flag.String("variant", "A", "")
	flag.Parse()

	bars, err := data.LoadBarsCSV(*csvPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load csv: %v\n", err)
		os.Exit(1)
	}
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}
	split := int(float64(len(bars)) * 0.7)
	for name, slice := range map[string]data.Bars{"train": bars[:split], "test": bars[split:], "full": bars} {
		strat := strategy.New(*variant, cfg)
		eng := backtest.EngineConfigFrom(cfg, *variant, "SPLIT")
		eng.InitialCapital = 10000
		res := backtest.Run(slice, strat, cfg, eng)
		s := metrics.Compute(res)
		fmt.Printf("%s %s %s %-5s → CAGR %6.2f%% DD %7.2f%% Sharpe %.2f Calmar %.2f trades %d\n",
			*cfgPath, *csvPath, *variant, name, s.ReturnAnnual, s.MaxDD, s.Sharpe, s.Calmar, s.Trades)
	}
}
