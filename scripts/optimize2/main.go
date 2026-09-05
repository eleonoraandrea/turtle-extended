// optimize v3 — grid search STAGED per H1/H4 con periodi calendar-scaled.
//
//	Nuovi gradi di libertà (rispetto a v2): donchian_entry/alt, sma_filter,
//	atr_period (per-variante), satellite_exit_len (engine).
//	Backward-compat: default config = vecchi hardcoded → risultati 4h invariati.
//
//	stage 1: grid su TRAIN 70% → rank per Sharpe → top N
//	stage 2: curve DD {(7,17),(8,20),(10,25)} sui top → selezione max CAGR con DD ≥ −maxdd
//	stage 3: TEST 30% (una lettura) + full → vincitore per TEST Calmar
//
// Uso:
//	go run ./scripts/optimize2 -symbol BTCUSDT -interval 1h -csv data/raw/BTCUSDT_1h.csv
//	go run ./scripts/optimize2 -symbol BTCUSDT -interval 4h -csv data/raw/BTCUSDT_4h.csv -maxdd 17
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
	entryLen    int // canale slow (slot Don55) — breakout principale
	altLen      int // canale fast (slot Don20) — breakout secondario
	smaLen      int // filtro trend MA
	atrPeriod   int
	atrMult     float64
	donExit     int
	satExitLen  int
	satAlloc    float64
	trailMode   string
	entryMode   string
	riskPct     float64
	ddStart     float64
	ddFlat      float64
	// mean reversion (variant M)
	mrPeriod  int
	mrDev     float64
	rsiBuy    float64
	confirm   bool
	shorts    bool
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

func buildCfg(c combo, variant string) *config.Config {
	cfg, err := config.Load("configs/default.yaml")
	if err != nil {
		panic(err)
	}
	// variant M: mean reversion — campi propri
	if variant == "M" {
		m := &cfg.VariantM
		m.MRPeriod = c.mrPeriod
		m.MRDevATR = c.mrDev
		m.TrendSMA = c.smaLen
		m.ATRPeriod = c.atrPeriod
		m.ATRStopMult = c.atrMult
		m.RSIBuy = c.rsiBuy
		m.RSISell = 100 - c.rsiBuy
		m.Confirm = c.confirm
		m.AllowShorts = c.shorts
		m.RiskPct = c.riskPct * 100
		m.Engine.TrailMode = "donchian"
		m.Engine.DonExit = c.donExit
		m.Engine.EntryMode = c.entryMode
		m.Engine.ExitMode = "reversion"
		cfg.Pyramiding.Enabled = false
		cfg.Profit.Satellite.Enabled = false
		cfg.Risk.Base = c.riskPct
		cfg.Risk.Max = c.riskPct
		cfg.Risk.MaxRiskPerTradePct = c.riskPct * 100
		cfg.Risk.DDDeleverageStart = c.ddStart
		cfg.Risk.DDFlatPct = c.ddFlat
		return cfg
	}
	// engine per-variante
	eng := &cfg.VariantA.Engine
	if variant == "B" {
		eng = &cfg.VariantB.Engine
	} else if variant == "C" {
		eng = &cfg.VariantC.Engine
	} else if variant == "D" {
		eng = &cfg.VariantD.Engine
	}
	// canali/periodi per-variante (A ha tutti i campi; B manca sma_filter — usa engine A solo per A/B con fallback)
	cfg.VariantA.DonchianEntry = c.entryLen
	cfg.VariantA.DonchianAlt = c.altLen
	cfg.VariantA.SMAFilter = c.smaLen
	cfg.VariantA.ATRPeriod = c.atrPeriod
	cfg.VariantA.ATRStopMult = c.atrMult
	cfg.VariantB.DonchianEntry = c.entryLen
	cfg.VariantB.DonchianAlt = c.altLen
	cfg.VariantB.ATRPeriod = c.atrPeriod
	cfg.VariantB.ATRStopMult = c.atrMult
	eng.TrailMode = c.trailMode
	eng.DonExit = c.donExit
	eng.SatelliteExitLen = c.satExitLen
	eng.EntryMode = c.entryMode
	cfg.Pyramiding.Enabled = false
	cfg.Profit.Satellite.Enabled = c.satAlloc > 0
	cfg.Profit.Satellite.Allocation = c.satAlloc
	cfg.Risk.Base = c.riskPct
	cfg.Risk.Max = c.riskPct
	cfg.Risk.MaxRiskPerTradePct = c.riskPct * 100
	cfg.Risk.DDDeleverageStart = c.ddStart
	cfg.Risk.DDFlatPct = c.ddFlat
	return cfg
}

func runOnce(bars data.Bars, c combo, variant, symbol string) metrics.Stats {
	cfg := buildCfg(c, variant)
	strat := strategy.New(variant, cfg)
	eng := backtest.EngineConfigFrom(cfg, variant, symbol)
	eng.InitialCapital = 10000
	res := backtest.Run(bars, strat, cfg, eng)
	return metrics.Compute(res)
}

func comboName(c combo) string {
	if c.mrPeriod > 0 {
		return fmt.Sprintf("mr%d dev%.1f sma%d atr%d×%.1f dx%d rsi%.0f conf%v sh%v r%.3f dd(%.0f/%.0f)",
			c.mrPeriod, c.mrDev, c.smaLen, c.atrPeriod, c.atrMult, c.donExit, c.rsiBuy, c.confirm, c.shorts, c.riskPct, c.ddStart, c.ddFlat)
	}
	return fmt.Sprintf("e%d a%d sma%d atr%d×%.1f dx%d sx%d sat%.1f %s %s r%.3f dd(%.0f/%.0f)",
		c.entryLen, c.altLen, c.smaLen, c.atrPeriod, c.atrMult, c.donExit, c.satExitLen, c.satAlloc,
		c.trailMode[:3], c.entryMode, c.riskPct, c.ddStart, c.ddFlat)
}

func main() {
	symbol := flag.String("symbol", "BTCUSDT", "")
	csvPath := flag.String("csv", "", "csv path (required)")
	variant := flag.String("variant", "A", "A/B/C/D")
	interval := flag.String("interval", "1h", "1h|4h — seleziona la griglia")
	maxDD := flag.Float64("maxdd", 17.0, "vincolo DD train (%) per selezione CAGR")
	flag.Parse()
	if *csvPath == "" {
		*csvPath = "data/raw/" + *symbol + "_" + *interval + ".csv"
	}

	bars, err := data.LoadBarsCSV(*csvPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load csv: %v\n", err)
		os.Exit(1)
	}
	split := int(float64(len(bars)) * 0.7)
	train, test := bars[:split], bars[split:]
	fmt.Printf("optimize v3 %s %s interval=%s — %d bars, train %d (%s→%s), test %d (%s→%s), vincolo DD %.0f%%\n",
		*symbol, *variant, *interval, len(bars), len(train),
		train[0].Time.Format("2006-01"), train[len(train)-1].Time.Format("2006-01"),
		len(test), test[0].Time.Format("2006-01"), test[len(test)-1].Time.Format("2006-01"), *maxDD)

	// ── stage 1 grid ──
	var base []combo
	if *variant == "M" {
		// mean reversion H1 — griglia MR
		for _, mrP := range []int{24, 48, 96} {
			for _, dev := range []float64{2.0, 2.5, 3.0, 3.5} {
				for _, sma := range []int{480, 840} {
					for _, atrM := range []float64{2.0, 3.0} {
						for _, de := range []int{8, 12, 24} {
							for _, rsi := range []float64{0, 30} {
								for _, conf := range []bool{false, true} {
									for _, sh := range []bool{true, false} {
										base = append(base, combo{
											mrPeriod: mrP, mrDev: dev, smaLen: sma, atrPeriod: 14,
											atrMult: atrM, donExit: de, rsiBuy: rsi, confirm: conf, shorts: sh,
											trailMode: "donchian", entryMode: "close", riskPct: 0.02,
											ddStart: 10, ddFlat: 25,
										})
									}
								}
							}
						}
					}
				}
			}
		}
	} else if *interval == "1h" {
		// calendar-scaled: 4h→1h ≈ ×4 (55→220, 20→80≈96, SMA200→800≈840, ATR20→80≈56)
		for _, entry := range []int{120, 168, 220, 336} {
			for _, alt := range []int{24, 48, 96} {
				for _, sma := range []int{480, 840} {
					for _, atrP := range []int{24, 56} {
						for _, atrM := range []float64{2.0, 2.5, 3.5, 4.5} {
							for _, de := range []int{16, 24, 48} {
								for _, sx := range []int{120, 220} {
									for _, sat := range []float64{0.3, 0.4} {
										base = append(base, combo{
											entryLen: entry, altLen: alt, smaLen: sma, atrPeriod: atrP,
											atrMult: atrM, donExit: de, satExitLen: sx, satAlloc: sat,
											trailMode: "donchian", entryMode: "close", riskPct: 0.02,
											ddStart: 10, ddFlat: 25,
										})
									}
								}
							}
						}
					}
				}
			}
		}
	} else {
		// H4: neighborhood del vincitore v2 (e55 a20 sma200 atr20×1.8 dx10 sx55 sat0.4)
		// + estensioni ora possibili: entry più lunghi (84/100), sma 300, ATR 28, satExit 84
		for _, entry := range []int{55, 84, 100} {
			for _, alt := range []int{20, 28} {
				for _, sma := range []int{200, 300} {
					for _, atrP := range []int{20, 28} {
						for _, atrM := range []float64{1.6, 1.8, 2.2} {
							for _, de := range []int{10, 14} {
								for _, sx := range []int{55, 84} {
									for _, sat := range []float64{0.4, 0.5} {
										base = append(base, combo{
											entryLen: entry, altLen: alt, smaLen: sma, atrPeriod: atrP,
											atrMult: atrM, donExit: de, satExitLen: sx, satAlloc: sat,
											trailMode: "donchian", entryMode: "close", riskPct: 0.02,
											ddStart: 10, ddFlat: 25,
										})
									}
								}
							}
						}
					}
				}
			}
		}
	}
	fmt.Printf("stage 1: %d combos su train\n", len(base))

	type s1 struct {
		c  combo
		sh float64
	}
	var stage1 []s1
	t0 := time.Now()
	for i, c := range base {
		s := runOnce(train, c, *variant, *symbol)
		stage1 = append(stage1, s1{c, s.Sharpe})
		if (i+1)%100 == 0 {
			el := time.Since(t0).Seconds()
			fmt.Printf("  s1 %d/%d (%.0fs, ETA %.0fs)\n", i+1, len(base), el, el/float64(i+1)*float64(len(base)-i-1))
		}
	}
	sort.Slice(stage1, func(a, b int) bool { return stage1[a].sh > stage1[b].sh })
	if len(stage1) > 25 {
		stage1 = stage1[:25]
	}

	// ── stage 2: curve DD ──
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

	// ── stage 3: test + full ──
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

	fmt.Println("\n=== FINAL — top per TEST Calmar (train, test, full) ===")
	fmt.Printf("%-78s %8s %7s | %8s %7s %7s %6s | %8s %7s %7s\n",
		"combo", "trCAGR%", "trDD%", "teCAGR%", "teDD%", "teCal", "teTrd", "fullCAGR%", "fullDD%", "fullRet%")
	for i, r := range results {
		if i >= 10 {
			break
		}
		fmt.Printf("%-78s %8.1f %7.1f | %8.1f %7.1f %7.2f %6d | %8.1f %7.1f %7.0f\n",
			comboName(r.c), r.trainCAGR, r.trainDD, r.testCAGR, r.testDD, r.testCalmar, r.testTrades, r.fullCAGR, r.fullDD, r.fullReturnPct)
	}
	fmt.Printf("\nbaseline v2 (train): confrontare teCAGR/teCal vs v2 (BTC 4h: te 12.4%%/Cal 0.73)\n")
}
