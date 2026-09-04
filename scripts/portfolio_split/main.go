// portfolio_split: train/test/full + walk-forward del PORTFOLIO (holdout evidence).
// Split per TIMESTAMP (confine = 70% della timeline del primo simbolo in ordine alfabetico).
// Uso: go run ./scripts/portfolio_split -config configs/atps_portfolio.yaml -csvs "data/raw/{SYMBOL}_4h.csv" [-wf]
package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/atps/atps/internal/backtest"
	"github.com/atps/atps/internal/config"
	"github.com/atps/atps/internal/data"
	"github.com/atps/atps/internal/metrics"
	"github.com/atps/atps/internal/strategy"
)

func main() {
	cfgPath := flag.String("config", "configs/atps_portfolio.yaml", "")
	csvPattern := flag.String("csvs", "data/raw/{SYMBOL}_4h.csv", "")
	wf := flag.Bool("wf", false, "esegui walk-forward 8 folds")
	flag.Parse()
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}
	symbols := append([]string{}, cfg.General.Symbols...)
	sort.Strings(symbols)
	barsMap := map[string]data.Bars{}
	for _, s := range symbols {
		p := strings.ReplaceAll(*csvPattern, "{SYMBOL}", s)
		bars, err := data.LoadBarsCSV(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "csv %s: %v\n", p, err)
			os.Exit(1)
		}
		barsMap[s] = bars
	}
	run := func(m map[string]data.Bars) metrics.Stats {
		strats := map[string]strategy.Strategy{}
		for _, s := range symbols {
			strats[s] = strategy.New("A", cfg)
		}
		eng := backtest.EngineConfigFrom(cfg, "A", "PORTFOLIO")
		res := backtest.RunPortfolio(m, strats, cfg, eng)
		return metrics.Compute(res)
	}
	// split per timestamp: confine sul primo simbolo (alfabetico = BTCUSDT)
	refBars := barsMap[symbols[0]]
	boundary := refBars[int(float64(len(refBars))*0.7)].Time
	split := func(pred func(t time.Time) bool) map[string]data.Bars {
		out := map[string]data.Bars{}
		for s, bars := range barsMap {
			var sel data.Bars
			for _, b := range bars {
				if pred(b.Time) {
					sel = append(sel, b)
				}
			}
			out[s] = sel
		}
		return out
	}
	for _, name := range []string{"train", "test", "full"} {
		var m map[string]data.Bars
		switch name {
		case "train":
			m = split(func(t time.Time) bool { return t.Before(boundary) })
		case "test":
			m = split(func(t time.Time) bool { return !t.Before(boundary) })
		case "full":
			m = barsMap
		}
		st := run(m)
		fmt.Printf("%s PORTFOLIO %-5s → CAGR %6.2f%% DD %7.2f%% Sharpe %.2f Calmar %.2f trades %d (boundary %s)\n",
			*cfgPath, name, st.ReturnAnnual, st.MaxDD, st.Sharpe, st.Calmar, st.Trades, boundary.Format("2006-01"))
	}
	if !*wf {
		return
	}
	// WF 8 folds: fold k testa (k-1)/8..k/8 della timeline, allena su tutto il precedente
	var sharpes []float64
	for k := 1; k <= 8; k++ {
		lo := refBars[int(float64(len(refBars))*float64(k-1)/8)].Time
		hiIdx := int(float64(len(refBars)) * float64(k) / 8)
		if hiIdx >= len(refBars) {
			hiIdx = len(refBars) - 1
		}
		hi := refBars[hiIdx].Time
		trainM := split(func(t time.Time) bool { return t.Before(lo) })
		testM := split(func(t time.Time) bool { return !t.Before(lo) && !t.After(hi) })
		sts := run(trainM)
		ste := run(testM)
		sharpes = append(sharpes, ste.Sharpe)
		fmt.Printf("  fold %d (%s→%s): train Sharpe %.2f → test Sharpe %.2f CAGR %.2f%% DD %.2f%%\n",
			k, lo.Format("2006-01"), hi.Format("2006-01"), sts.Sharpe, ste.Sharpe, ste.ReturnAnnual, ste.MaxDD)
	}
	sort.Float64s(sharpes)
	med := (sharpes[3] + sharpes[4]) / 2
	fmt.Printf("WF mediana Sharpe: %.2f\n", med)
}
