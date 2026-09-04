// optimize v2 — grid search a STADI con selezione DD-constrained, sul percorso
// config unificato (backtest.EngineConfigFrom — identico a CLI e bot live):
//
//	stage 1: grid base su TRAIN 70% → rank per Sharpe → top 30
//	stage 2: varianti curva DD {(7,17),(8,20),(10,25)} sui top 30 su TRAIN
//	         → selezione max CAGR con MaxDD ≥ −maxdd → top 10
//	stage 3: TEST 30% (una sola lettura) + full → vincitore
//
// Uso: go run ./scripts/optimize -symbol BTCUSDT -csv data/raw/BTCUSDT_4h.csv -variant A -maxdd 15
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
	atrMult   float64
	trailMode string
	trailMult float64
	donExit   int
	pyrOn     bool
	pyrAdds   int
	satAlloc  float64
	riskPct   float64 // base = max
	entryMode string
	reentry   bool
	ddStart   float64
	ddFlat    float64
}

type result struct {
	c             combo
	trainSharpe   float64
	trainCAGR     float64
	trainDD       float64
	testSharpe    float64
	testCAGR      float64
	testDD        float64
	testCalmar    float64
	testTrades    int
	fullCAGR      float64
	fullSharpe    float64
	fullDD        float64
	fullReturnPct float64
}

func engineCfgPtr(cfg *config.Config, variant string) *config.EngineCfg {
	switch variant {
	case "A":
		return &cfg.VariantA.Engine
	case "B":
		return &cfg.VariantB.Engine
	case "C":
		return &cfg.VariantC.Engine
	default:
		return &cfg.VariantD.Engine
	}
}

func reentryCfgPtr(cfg *config.Config, variant string) *config.ReEntryCfg {
	switch variant {
	case "A":
		return &cfg.VariantA.ReEntry
	case "B":
		return &cfg.VariantB.ReEntry
	case "C":
		return &cfg.VariantC.ReEntry
	default:
		return &cfg.VariantD.ReEntry
	}
}

func setVariantStopMult(cfg *config.Config, variant string, m float64) {
	switch variant {
	case "A":
		cfg.VariantA.ATRStopMult = m
	case "B":
		cfg.VariantB.ATRStopMult = m
	case "C":
		cfg.VariantC.ATRStopMult = m
	default:
		cfg.VariantD.ATRStopMult = m
	}
}

func buildCfg(c combo, variant string) *config.Config {
	cfg, err := config.Load("configs/default.yaml")
	if err != nil {
		panic(err)
	}
	setVariantStopMult(cfg, variant, c.atrMult)
	eng := engineCfgPtr(cfg, variant)
	eng.TrailMode = c.trailMode
	eng.TrailATRMult = c.trailMult
	eng.DonExit = c.donExit
	eng.EntryMode = c.entryMode
	eng.PyramidingUnits = 0 // usa la sezione pyramiding: globale qui sotto
	cfg.Pyramiding.Enabled = c.pyrOn
	cfg.Pyramiding.MaxAdditions = c.pyrAdds
	cfg.Pyramiding.RiskNeutral = true
	cfg.Profit.Satellite.Enabled = c.satAlloc > 0
	cfg.Profit.Satellite.Allocation = c.satAlloc
	cfg.Risk.Base = c.riskPct
	cfg.Risk.Max = c.riskPct
	cfg.Risk.MaxRiskPerTradePct = c.riskPct * 100
	cfg.Risk.DDDeleverageStart = c.ddStart
	cfg.Risk.DDFlatPct = c.ddFlat
	re := reentryCfgPtr(cfg, variant)
	re.Enabled = c.reentry
	if c.reentry {
		re.Lookback = 10
		re.WithinBars = 20
	}
	return cfg
}

func runOnce(bars data.Bars, c combo, variant, symbol string) metrics.Stats {
	cfg := buildCfg(c, variant)
	strat := strategy.New(variant, cfg)
	eng := backtest.EngineConfigFrom(cfg, variant, symbol) // STESSO percorso di CLI/bot
	eng.InitialCapital = 10000
	res := backtest.Run(bars, strat, cfg, eng)
	return metrics.Compute(res)
}

func comboName(c combo) string {
	return fmt.Sprintf("atr%.1f %s%.1f don%d pyr(%v,a%d) sat%.1f r%.3f entry:%s reentry:%v dd(%.0f/%.0f)",
		c.atrMult, c.trailMode[:3], c.trailMult, c.donExit, c.pyrOn, c.pyrAdds, c.satAlloc, c.riskPct, c.entryMode, c.reentry, c.ddStart, c.ddFlat)
}

func main() {
	symbol := flag.String("symbol", "BTCUSDT", "")
	csvPath := flag.String("csv", "data/raw/BTCUSDT_4h.csv", "")
	variant := flag.String("variant", "A", "A/B/C/D")
	maxDD := flag.Float64("maxdd", 15.0, "vincolo DD train (%) per selezione CAGR")
	flag.Parse()

	bars, err := data.LoadBarsCSV(*csvPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load csv: %v\n", err)
		os.Exit(1)
	}
	split := int(float64(len(bars)) * 0.7)
	train, test := bars[:split], bars[split:]
	fmt.Printf("optimize v2 %s %s — %d bars, train %d (%s→%s), test %d (%s→%s), vincolo DD %.0f%%\n",
		*symbol, *variant, len(bars), len(train),
		train[0].Time.Format("2006-01"), train[len(train)-1].Time.Format("2006-01"),
		len(test), test[0].Time.Format("2006-01"), test[len(test)-1].Time.Format("2006-01"), *maxDD)

	// ── stage 1: grid base su train ──
	// Nota pruning vs spec §2: pyr adds {3,6} (il 4 è interpolativo tra 3 e 6);
	// satellite {0, 0.3, 0.4} completo. 2592 combos ≈ 2.9h a ~4s/run.
	// Per run più rapide: ridurre risk a {0.02, 0.025} → 1296 (~1.5h), documentandolo.
	var base []combo
	trailOpts := []struct {
		mode string
		mult float64
	}{{"donchian", 0}, {"chandelier", 2.5}, {"chandelier", 3.0}}
	for _, atr := range []float64{1.4, 1.6, 1.8} {
		for _, tr := range trailOpts {
			for _, de := range []int{10, 20} {
				for _, pyr := range []struct {
					on   bool
					adds int
				}{{false, 0}, {true, 3}, {true, 6}} {
					for _, risk := range []float64{0.015, 0.02, 0.025, 0.03} {
						for _, sat := range []float64{0, 0.3, 0.4} {
							for _, entry := range []string{"close", "intrabar"} {
								for _, re := range []bool{false, true} {
									base = append(base, combo{
										atrMult: atr, trailMode: tr.mode, trailMult: tr.mult,
										donExit: de, pyrOn: pyr.on, pyrAdds: pyr.adds,
										satAlloc: sat, riskPct: risk, entryMode: entry, reentry: re,
										ddStart: 10, ddFlat: 25, // curva attuale nella stage 1
									})
								}
							}
						}
					}
				}
			}
		}
	}
	fmt.Printf("stage 1: %d combos su train (≈ %.0f min)\n", len(base), float64(len(base))*4.0/60.0)

	type s1 struct {
		c  combo
		sh float64
	}
	var stage1 []s1
	t0 := time.Now()
	for i, c := range base {
		s := runOnce(train, c, *variant, *symbol)
		stage1 = append(stage1, s1{c, s.Sharpe})
		if (i+1)%200 == 0 {
			fmt.Printf("  s1 %d/%d (%.0fs)\n", i+1, len(base), time.Since(t0).Seconds())
		}
	}
	sort.Slice(stage1, func(a, b int) bool { return stage1[a].sh > stage1[b].sh })
	if len(stage1) > 30 {
		stage1 = stage1[:30]
	}

	// ── stage 2: curve DD sui top-30 ──
	ddOpts := [][2]float64{{7, 17}, {8, 20}, {10, 25}}
	type s2 struct {
		c    combo
		cagr float64
		dd   float64
		cal  float64
	}
	var stage2 []s2
	for _, x := range stage1 {
		for _, dd := range ddOpts {
			c := x.c
			c.ddStart, c.ddFlat = dd[0], dd[1]
			s := runOnce(train, c, *variant, *symbol)
			stage2 = append(stage2, s2{c, s.ReturnAnnual, s.MaxDD, s.Calmar})
		}
	}
	// selezione: max CAGR tra chi rispetta DD; se nessuno, max Calmar
	var feasible []s2
	for _, x := range stage2 {
		if x.dd >= -*maxDD {
			feasible = append(feasible, x)
		}
	}
	if len(feasible) > 0 {
		sort.Slice(feasible, func(a, b int) bool { return feasible[a].cagr > feasible[b].cagr })
	} else {
		fmt.Printf("NESSUNA combo rispetta DD %.0f%% sul train — fallback a max Calmar\n", *maxDD)
		feasible = stage2
		sort.Slice(feasible, func(a, b int) bool { return feasible[a].cal > feasible[b].cal })
	}
	if len(feasible) > 10 {
		feasible = feasible[:10]
	}

	// ── stage 3: test (una sola lettura) + full ──
	var results []result
	for _, x := range feasible {
		st := runOnce(test, x.c, *variant, *symbol)
		sf := runOnce(bars, x.c, *variant, *symbol)
		results = append(results, result{
			c: x.c, trainCAGR: x.cagr, trainDD: x.dd,
			testSharpe: st.Sharpe, testCAGR: st.ReturnAnnual, testDD: st.MaxDD,
			testCalmar: st.Calmar, testTrades: st.Trades,
			fullCAGR: sf.ReturnAnnual, fullSharpe: sf.Sharpe, fullDD: sf.MaxDD,
			fullReturnPct: sf.ReturnPct,
		})
	}
	sort.Slice(results, func(a, b int) bool { return results[a].testCalmar > results[b].testCalmar })

	fmt.Println("\n=== FINAL — top per TEST Calmar (train CAGR/DD, test, full) ===")
	fmt.Printf("%-72s %8s %7s | %8s %7s %7s %6s | %8s %7s %7s\n",
		"combo", "trCAGR%", "trDD%", "teCAGR%", "teDD%", "teCal", "teTrd", "fullCAGR%", "fullDD%", "fullRet%")
	for i, r := range results {
		if i >= 10 {
			break
		}
		fmt.Printf("%-72s %8.1f %7.1f | %8.1f %7.1f %7.2f %6d | %8.1f %7.1f %7.0f\n",
			comboName(r.c), r.trainCAGR, r.trainDD, r.testCAGR, r.testDD, r.testCalmar, r.testTrades, r.fullCAGR, r.fullDD, r.fullReturnPct)
	}

	fmt.Printf(`
PROTOCOLLO (spec §4) — prossimi passi manuali:
  1. gate test:     degrado CAGR train→test < 1/3 e Calmar test > 0
  2. walk-forward:  ./atps walk-forward --config configs/atps_v2.yaml --symbol %s --variant %s --csv %s --folds 8
  3. perturbazione: ./atps perturb --config configs/atps_v2.yaml --symbol %s --variant %s --csv %s
  4. ETH/SOL:       backtest con atps_v2.yaml, confronto vs btc_opt.yaml — degrado < 20%%
`, *symbol, *variant, *csvPath, *symbol, *variant, *csvPath)
}
