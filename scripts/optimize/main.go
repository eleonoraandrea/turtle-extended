// optimize: grid search dei parametri sul MOTORE CORRETTO (post-fix),
// con split train/test 70/30: selezione per Sharpe sul train, conferma sul test.
// Uso: go run ./scripts/optimize -symbol BTCUSDT -csv data/raw/BTCUSDT_4h.csv -variant A
package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/atps/atps/internal/backtest"
	"github.com/atps/atps/internal/config"
	"github.com/atps/atps/internal/data"
	"github.com/atps/atps/internal/metrics"
	"github.com/atps/atps/internal/strategy"
)

type combo struct {
	atrMult      float64
	trailMode    string
	trailMult    float64
	donExit      int
	pyrOn        bool
	pyrStep      float64
	pyrAdds      int
	riskNeutral  bool
	satAlloc     float64
	riskBase     float64
	riskMax      float64
}

type result struct {
	c                 combo
	trainSharpe       float64
	testSharpe        float64
	testSortino       float64
	testReturn        float64
	testMaxDD         float64
	testCalmar        float64
	testPF            float64
	testTrades        int
	testWinRate       float64
	testExpectancyR   float64
	fullReturn        float64
	fullSharpe        float64
	fullMaxDD         float64
}

func setVariantStopMult(cfg *config.Config, variant string, m float64) {
	switch variant {
	case "A":
		cfg.VariantA.ATRStopMult = m
	case "B":
		cfg.VariantB.ATRStopMult = m
	case "C":
		cfg.VariantC.ATRStopMult = m
	case "D":
		cfg.VariantD.ATRStopMult = m
	}
}

func buildCfg(base combo, variant string) *config.Config {
	cfg, err := config.Load("configs/default.yaml")
	if err != nil {
		panic(err)
	}
	setVariantStopMult(cfg, variant, base.atrMult)
	cfg.Pyramiding.Enabled = base.pyrOn
	cfg.Pyramiding.RiskNeutral = base.riskNeutral
	cfg.Pyramiding.MaxAdditions = base.pyrAdds
	cfg.Profit.Satellite.Enabled = base.satAlloc > 0
	cfg.Profit.Satellite.Allocation = base.satAlloc
	cfg.Risk.Base = base.riskBase
	cfg.Risk.Max = base.riskMax
	cfg.Risk.MaxRiskPerTradePct = base.riskMax * 100
	return cfg
}

func engineFor(c combo, cfg *config.Config, variant, symbol string) backtest.EngineConfig {
	pyrMax := 0
	if c.pyrOn {
		pyrMax = c.pyrAdds + 1
	}
	return backtest.EngineConfig{
		Variant: variant, Symbol: symbol,
		InitialCapital: 10000,
		FeeBps:         cfg.Costs.FeeBps, SlippageBps: cfg.Costs.SlippageBps,
		UseNextOpen:    true,
		PyramidingMax:  pyrMax,
		PyramidStepATR: c.pyrStep,
		TrailATRMult:   c.trailMult,
		TrailMode:      c.trailMode,
		DonExit:        c.donExit,
	}
}

func runOnce(bars data.Bars, c combo, variant, symbol string) metrics.Stats {
	cfg := buildCfg(c, variant)
	strat := strategy.New(variant, cfg)
	res := backtest.Run(bars, strat, cfg, engineFor(c, cfg, variant, symbol))
	s := metrics.Compute(res)
	return s
}

func main() {
	symbol := flag.String("symbol", "BTCUSDT", "")
	csvPath := flag.String("csv", "data/raw/BTCUSDT_4h.csv", "")
	variant := flag.String("variant", "A", "A/B/C/D")
	topN := flag.Int("top", 30, "combos from train phase promoted to test")
	flag.Parse()

	bars, err := data.LoadBarsCSV(*csvPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load csv: %v\n", err)
		os.Exit(1)
	}
	split := int(float64(len(bars)) * 0.7)
	train, test := bars[:split], bars[split:]
	fmt.Printf("optimize %s %s — %d bars, train %d (%s→%s), test %d (%s→%s)\n",
		*symbol, *variant, len(bars), len(train),
		train[0].Time.Format("2006-01"), train[len(train)-1].Time.Format("2006-01"),
		len(test), test[0].Time.Format("2006-01"), test[len(test)-1].Time.Format("2006-01"))

	// grid
	var combos []combo
	trailOpts := []struct {
		mode string
		mult float64
	}{{"donchian", 0}, {"chandelier", 2.5}, {"chandelier", 3.0}, {"chandelier", 3.5}}
	pyrOpts := []struct {
		on    bool
		step  float64
		adds  int
		neut  bool
	}{
		{false, 0, 0, false},
		{true, 0.5, 3, true},
		{true, 0.75, 3, true},
		{true, 1.0, 3, true},
		{true, 0.5, 5, true},
		{true, 0.5, 3, false},
	}
	riskOpts := []struct{ base, max float64 }{{0.01, 0.02}, {0.015, 0.025}}
	satOpts := []float64{0, 0.3}

	for _, atr := range []float64{1.4, 1.6, 1.8, 2.0, 2.2} {
		for _, tr := range trailOpts {
			for _, de := range []int{10, 20, 55} {
				for _, py := range pyrOpts {
					for _, rk := range riskOpts {
						for _, sat := range satOpts {
							combos = append(combos, combo{
								atrMult: atr, trailMode: tr.mode, trailMult: tr.mult,
								donExit: de, pyrOn: py.on, pyrStep: py.step, pyrAdds: py.adds,
								riskNeutral: py.neut, satAlloc: sat, riskBase: rk.base, riskMax: rk.max,
							})
						}
					}
				}
			}
		}
	}
	fmt.Printf("grid: %d combos\n", len(combos))

	// fase 1: train
	type trainRes struct {
		c    combo
		sh   float64
	}
	var trs []trainRes
	t0 := time.Now()
	for i, c := range combos {
		s := runOnce(train, c, *variant, *symbol)
		trs = append(trs, trainRes{c, s.Sharpe})
		if (i+1)%200 == 0 {
			fmt.Printf("  train %d/%d (%.0fs)\n", i+1, len(combos), time.Since(t0).Seconds())
		}
	}
	sort.Slice(trs, func(a, b int) bool { return trs[a].sh > trs[b].sh })
	if len(trs) > *topN {
		trs = trs[:*topN]
	}

	// fase 2: test sui top train + full period
	var results []result
	for _, tr := range trs {
		st := runOnce(test, tr.c, *variant, *symbol)
		sf := runOnce(bars, tr.c, *variant, *symbol)
		results = append(results, result{
			c: tr.c, trainSharpe: tr.sh,
			testSharpe: st.Sharpe, testSortino: st.Sortino, testReturn: st.ReturnPct,
			testMaxDD: st.MaxDD, testCalmar: st.Calmar, testPF: st.ProfitFactor,
			testTrades: st.Trades, testWinRate: st.WinRate, testExpectancyR: st.ExpectancyR,
			fullReturn: sf.ReturnPct, fullSharpe: sf.Sharpe, fullMaxDD: sf.MaxDD,
		})
	}
	sort.Slice(results, func(a, b int) bool { return results[a].testSharpe > results[b].testSharpe })

	fmt.Println("\n=== TOP per TEST Sharpe (train Sharpe in colonna) ===")
	fmt.Printf("%-38s %6s %6s %9s %8s %7s %6s %5s %10s %9s %8s\n",
		"combo", "trSh", "teSh", "teRet%", "teDD%", "teCal", "tePF", "trW%", "fullRet%", "fullSh", "fullDD%")
	for i, r := range results {
		if i >= 15 {
			break
		}
		c := r.c
		name := fmt.Sprintf("atr%.1f %s%.1f don%d pyr(%v,s%.2f,a%d,n%v) sat%.1f r%.2f/%.2f",
			c.atrMult, c.trailMode[:3], c.trailMult, c.donExit, c.pyrOn, c.pyrStep, c.pyrAdds, c.riskNeutral, c.satAlloc, c.riskBase, c.riskMax)
		fmt.Printf("%-38s %6.2f %6.2f %9.1f %8.1f %7.1f %6.2f %5.1f %10.1f %9.2f %8.1f\n",
			name, r.trainSharpe, r.testSharpe, r.testReturn, r.testMaxDD, r.testCalmar, r.testPF,
			r.testWinRate, r.fullReturn, r.fullSharpe, r.fullMaxDD)
	}

	best := results[0]
	fmt.Printf("\nBEST: %+v\n", best.c)
	fmt.Printf("test: Sharpe %.2f Sortino %.2f Ret %.1f%% DD %.1f%% PF %.2f trades %d | full: Ret %.1f%% Sharpe %.2f DD %.1f%%\n",
		best.testSharpe, best.testSortino, best.testReturn, best.testMaxDD, best.testPF, best.testTrades,
		best.fullReturn, best.fullSharpe, best.fullMaxDD)
}
