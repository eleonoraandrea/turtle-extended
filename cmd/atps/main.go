package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/atps/atps/internal/analysis"
	"github.com/atps/atps/internal/backtest"
	"github.com/atps/atps/internal/config"
	"github.com/atps/atps/internal/data"
	"github.com/atps/atps/internal/metrics"
	"github.com/atps/atps/internal/report"
	"github.com/atps/atps/internal/strategy"
)

var cfgPath string

func main() {
	root := &cobra.Command{
		Use:   "atps",
		Short: "ATPS — Adaptive Turtle Perpetual System (Go) — Binance data, Orderly execution",
		Long: `ATPS Go: backtest Turtle A/B/C/D con funding/OI, fee/slippage, walk-forward, MonteCarlo, report HTML MT5-style.
Binance per dati, Orderly per live (isolato con -tags live).`,
	}
	root.PersistentFlags().StringVar(&cfgPath, "config", "configs/default.yaml", "path to config yaml")
	root.AddCommand(cmdDownload(), cmdBacktest(), cmdCompare(), cmdWalkForward(), cmdMonteCarlo(), cmdPerturb(), cmdPortfolio(), cmdPortfolioBacktest(), cmdGenerateDemo(), cmdReportDemo(), cmdTUI(), cmdLive())

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

// loadCfg carica la config; se esplicita (--config passato) e non valida,
// termina con errore invece di degradare silenziosamente al default.
func loadCfg(explicit bool) *config.Config {
	c, err := config.Load(cfgPath)
	if err != nil {
		if explicit {
			fmt.Fprintf(os.Stderr, "config load failed (%s): %v\n", cfgPath, err)
			os.Exit(1)
		}
		c2, err2 := config.Load(config.DefaultPath())
		if err2 != nil {
			fmt.Fprintf(os.Stderr, "config load failed %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "warn: config %s non valida (%v) → uso %s\n", cfgPath, err, config.DefaultPath())
		return c2
	}
	return c
}

func cmdDownload() *cobra.Command {
	var symbol, interval, start, end, out string
	var withFunding, withOI bool
	cmd := &cobra.Command{
		Use:   "download",
		Short: "Scarica OHLCV (+ funding + OI) da Binance e salva CSV allineato",
		Run: func(cmd *cobra.Command, args []string) {
			cfg := loadCfg(cmd.Flags().Changed("config"))
			base := cfg.Data.BinanceBase
			if base == "" {
				base = data.DefaultBinanceBase
			}
			client := data.NewBinanceClient(base)
			if symbol == "" {
				symbol = cfg.General.Symbols[0]
			}
			if interval == "" {
				interval = cfg.General.Interval
			}
			if out == "" {
				out = fmt.Sprintf("data/raw/%s_%s.csv", symbol, interval)
			}
			st := parseTime(start, cfg.General.StartTime)
			et := parseTime(end, cfg.General.EndTime)
			if et.IsZero() {
				et = time.Now().UTC()
			}
			fmt.Printf("download %s %s %s -> %s\n", symbol, interval, st.Format("2006-01-02"), et.Format("2006-01-02"))
			bars, err := client.FetchKlines(symbol, interval, st, et)
			if err != nil {
				fmt.Fprintf(os.Stderr, "klines error %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("klines %d\n", len(bars))
			var fundings []data.FundingRate
			var ois []data.OIHist
			if withFunding {
				f, err := client.FetchFundingRate(symbol, st, et, 1000)
				if err != nil {
					fmt.Printf("funding warn %v\n", err)
				} else {
					fundings = f
					fmt.Printf("funding %d\n", len(fundings))
				}
			}
			if withOI {
				oiPeriod := cfg.Data.OIPeriod
				if oiPeriod == "" {
					oiPeriod = "4h"
				}
				o, err := client.FetchOpenInterestHist(symbol, oiPeriod, st, et, 500)
				if err != nil {
					fmt.Printf("oi warn %v\n", err)
				} else {
					ois = o
					fmt.Printf("oi %d\n", len(ois))
				}
				// need paginate for full range: naive loop if >500
				// simplified: if len==500, iterate
				if len(ois) == 500 {
					// try additional pages by advancing start
					cur := ois[len(ois)-1].Timestamp
					for {
						more, err := client.FetchOpenInterestHist(symbol, oiPeriod, time.UnixMilli(cur+1), et, 500)
						if err != nil || len(more) == 0 {
							break
						}
						ois = append(ois, more...)
						cur = more[len(more)-1].Timestamp
						if len(more) < 500 {
							break
						}
						fmt.Printf("oi paginated %d total\n", len(ois))
						if len(ois) > 5000 {
							break
						}
					}
				}
			}
			if cfg.Data.AlignFunding || cfg.Data.AlignOI {
				bars = data.AlignDerivatives(bars, fundings, ois)
			}
			if err := data.SaveBarsCSV(out, bars); err != nil {
				fmt.Fprintf(os.Stderr, "save %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("saved %s (%d bars) with funding/OI aligned=%v\n", out, len(bars), cfg.Data.AlignFunding)
			// also save fundings json for audit
			if len(fundings) > 0 {
				b, _ := json.MarshalIndent(fundings, "", "  ")
				os.WriteFile(strings.Replace(out, ".csv", "_funding.json", 1), b, 0644)
			}
		},
	}
	cmd.Flags().StringVar(&symbol, "symbol", "", "symbol e.g. BTCUSDT")
	cmd.Flags().StringVar(&interval, "interval", "", "1m 5m 15m 1h 4h 1d")
	cmd.Flags().StringVar(&start, "start", "", "YYYY-MM-DD")
	cmd.Flags().StringVar(&end, "end", "", "YYYY-MM-DD")
	cmd.Flags().StringVar(&out, "out", "", "output csv path")
	cmd.Flags().BoolVar(&withFunding, "funding", true, "fetch funding")
	cmd.Flags().BoolVar(&withOI, "oi", true, "fetch OI")
	return cmd
}

func cmdBacktest() *cobra.Command {
	var symbol, variant, csvPath, outHTML string
	var interval string
	cmd := &cobra.Command{
		Use:   "backtest",
		Short: "Esegui backtest su CSV (o demo sintetico) e genera report HTML",
		Run: func(cmd *cobra.Command, args []string) {
			cfg := loadCfg(cmd.Flags().Changed("config"))
			if interval != "" {
				cfg.General.Interval = interval // propaga l'interval al motore (funding scaling + report)
			}
			csvExplicit := cmd.Flags().Changed("csv")
			if symbol == "" {
				symbol = cfg.General.Symbols[0]
			}
			if variant == "" {
				variant = "D"
			}
			variant = strings.ToUpper(variant)
			if csvPath == "" {
				csvPath = fmt.Sprintf("data/raw/%s_%s.csv", symbol, interval)
				if interval == "" {
					interval = cfg.General.Interval
					csvPath = fmt.Sprintf("data/raw/%s_%s.csv", symbol, interval)
				}
			}
			var bars data.Bars
			var err error
			if _, e := os.Stat(csvPath); e == nil {
				bars, err = data.LoadBarsCSV(csvPath)
				if err != nil {
					fmt.Fprintf(os.Stderr, "load csv %v\n", err)
					os.Exit(1)
				}
				fmt.Printf("loaded %d bars from %s\n", len(bars), csvPath)
			} else if csvExplicit {
				// --csv esplicito mancante: ERRORE, non sostituire in silenzio
				// con dati sintetici (risultati plausibili ma falsi)
				fmt.Fprintf(os.Stderr, "csv %s non trovato (--csv esplicito)\n", csvPath)
				os.Exit(1)
			} else {
				seedMap := map[string]int64{"BTCUSDT": 42, "ETHUSDT": 1337, "SOLUSDT": 9999}
				seed := seedMap[symbol]
				if seed == 0 {
					seed = 42
				}
				fmt.Printf("csv %s not found → using synthetic demo (%s %s seed %d)\n", csvPath, symbol, variant, seed)
				bars = data.GenerateSynthetic(3500, intervalDuration(cfg.General.Interval), seed)
			}
			strat := strategy.New(variant, cfg)
			eng := engineFromCfg(cfg, variant, symbol)
			res := backtest.Run(bars, strat, cfg, eng)
			stats := metrics.Compute(res)
			fmt.Printf("%s %s: Return %.2f%% CAGR %.2f%% Sharpe %.2f Sortino %.2f MaxDD %.2f%% PF %.2f Trades %d Fee $%.2f Funding $%.2f\n",
				symbol, variant, stats.ReturnPct, stats.ReturnAnnual, stats.Sharpe, stats.Sortino, stats.MaxDD, stats.ProfitFactor, stats.Trades, stats.TotalFee, stats.TotalFunding)
			fmt.Printf("scaling ceiling: %.2f%% (%s lega)\n", res.ScalingCeilingPct, res.ScalingBinding)
			for _, w := range res.Warnings {
				fmt.Printf("warn: %s\n", w)
			}
			if res.NotionalCapHits > 0 {
				fmt.Printf("notional cap: %d entry ridotte dal cap nozionali\n", res.NotionalCapHits)
			}
			if outHTML == "" {
				outHTML = fmt.Sprintf("reports/%s_%s_%s.html", symbol, variant, time.Now().Format("20060102_1504"))
			}
			in := report.Input{Config: cfg, Bars: bars, Result: res, Stats: stats, Symbol: symbol, Variant: variant, GeneratedAt: time.Now()}
			if err := report.Generate(outHTML, in); err != nil {
				fmt.Fprintf(os.Stderr, "report %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("report %s\n", outHTML)
			// save trades json — TrimSuffix evita di sovrascrivere il report
			// quando outHTML non termina con .html
			jpath := strings.TrimSuffix(outHTML, ".html") + "_trades.json"
			b, _ := json.MarshalIndent(res.Trades, "", "  ")
			if err := os.WriteFile(jpath, b, 0644); err != nil {
				fmt.Fprintf(os.Stderr, "trades json %s: %v\n", jpath, err)
			}
			// walk-forward hint
			fmt.Printf("next: atps walk-forward --symbol %s --variant %s --csv %s\n", symbol, variant, csvPath)
		},
	}
	cmd.Flags().StringVar(&symbol, "symbol", "", "BTCUSDT etc")
	cmd.Flags().StringVar(&variant, "variant", "", "A/B/C/D")
	cmd.Flags().StringVar(&csvPath, "csv", "", "path to bars csv")
	cmd.Flags().StringVar(&outHTML, "out", "", "output html path")
	cmd.Flags().StringVar(&interval, "interval", "", "override interval")
	return cmd
}

func cmdCompare() *cobra.Command {
	var symbolsCSV, variantsCSV, out string
	cmd := &cobra.Command{
		Use:   "compare",
		Short: "Confronto A/B/C/D su uno o più simboli → report comparison HTML",
		Run: func(cmd *cobra.Command, args []string) {
			cfg := loadCfg(cmd.Flags().Changed("config"))
			syms := strings.Split(symbolsCSV, ",")
			if symbolsCSV == "" {
				syms = cfg.Compare.Symbols
			}
			if len(syms) == 0 || (len(syms) == 1 && syms[0] == "") {
				syms = cfg.General.Symbols
			}
			vars := strings.Split(variantsCSV, ",")
			if variantsCSV == "" {
				vars = cfg.Compare.Variants
			}
			if len(vars) == 0 || vars[0] == "" {
				vars = []string{"A", "B", "C", "D"}
			}
			var rows []report.ComparisonRow
			for _, sym := range syms {
				sym = strings.TrimSpace(sym)
				csvPath := fmt.Sprintf("data/raw/%s_%s.csv", sym, cfg.General.Interval)
				var bars data.Bars
				if _, e := os.Stat(csvPath); e == nil {
					b, _ := data.LoadBarsCSV(csvPath)
					bars = b
					fmt.Printf("loaded %s %d bars\n", sym, len(bars))
				} else {
					seed := map[string]int64{"BTCUSDT": 42, "ETHUSDT": 1337, "SOLUSDT": 9999, "BTC": 42, "ETH": 1337, "SOL": 9999}[sym]
					if seed == 0 {
						seed = 42
					}
					for _, c := range sym {
						seed += int64(c) * 3
					}
					bars = data.GenerateSynthetic(3000, 4*time.Hour, seed)
					fmt.Printf("synthetic %s %d bars (seed %d)\n", sym, len(bars), seed)
				}
				for _, v := range vars {
					v = strings.TrimSpace(strings.ToUpper(v))
					strat := strategy.New(v, cfg)
					eng := engineFromCfg(cfg, v, sym)
					res := backtest.Run(bars, strat, cfg, eng)
					stats := metrics.Compute(res)
					rows = append(rows, report.ComparisonRow{Symbol: sym, Variant: v, Stats: stats})
					fmt.Printf("%s %s: R %.1f%% Sharpe %.2f DD %.1f%% PF %.2f Trades %d\n", sym, v, stats.ReturnPct, stats.Sharpe, stats.MaxDD, stats.ProfitFactor, stats.Trades)
					// also generate individual html for each
					outHTML := fmt.Sprintf("reports/%s_%s.html", sym, v)
					in := report.Input{Config: cfg, Bars: bars, Result: res, Stats: stats, Symbol: sym, Variant: v, GeneratedAt: time.Now()}
					report.Generate(outHTML, in)
				}
			}
			if out == "" {
				out = "reports/comparison.html"
			}
			if err := report.GenerateComparison(out, rows, cfg); err != nil {
				fmt.Fprintf(os.Stderr, "cmp report %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("comparison %s\n", out)
		},
	}
	cmd.Flags().StringVar(&symbolsCSV, "symbols", "", "comma separated BTCUSDT,ETHUSDT")
	cmd.Flags().StringVar(&variantsCSV, "variants", "", "A,B,C,D")
	cmd.Flags().StringVar(&out, "out", "", "output html")
	return cmd
}

func cmdWalkForward() *cobra.Command {
	var symbol, variant, csvPath, out string
	var folds int
	cmd := &cobra.Command{
		Use:   "walk-forward",
		Short: "Walk-forward analysis su folds",
		Run: func(cmd *cobra.Command, args []string) {
			cfg := loadCfg(cmd.Flags().Changed("config"))
			if symbol == "" {
				symbol = cfg.General.Symbols[0]
			}
			if variant == "" {
				variant = "D"
			}
			variant = strings.ToUpper(variant)
			if folds == 0 {
				folds = cfg.WalkForward.Folds
			}
			if csvPath == "" {
				csvPath = fmt.Sprintf("data/raw/%s_%s.csv", symbol, cfg.General.Interval)
			}
			var bars data.Bars
			if _, e := os.Stat(csvPath); e == nil {
				b, _ := data.LoadBarsCSV(csvPath)
				bars = b
			} else {
				seedMap := map[string]int64{"BTCUSDT": 42, "ETHUSDT": 1337, "SOLUSDT": 9999}
				seed := seedMap[symbol]
				if seed == 0 {
					seed = 42
				}
				bars = data.GenerateSynthetic(3500, 4*time.Hour, seed)
			}
			strat := strategy.New(variant, cfg)
			eng := engineFromCfg(cfg, variant, symbol)
			wf := analysis.WalkForward(bars, strat, cfg, eng, folds, cfg.WalkForward.TrainRatio)
			fmt.Printf("WF %s %s folds %d train %.0f%% avgTrainSharpe %.2f avgTestSharpe %.2f decay %.2f OOS %.2f%%\n", symbol, variant, len(wf.Folds), cfg.WalkForward.TrainRatio*100, wf.AvgTrainSharpe, wf.AvgTestSharpe, wf.Decay, wf.OOSReturn)
			for _, f := range wf.Folds {
				fmt.Printf(" fold %d train %.1f%%/Sharpe %.2f → test %.1f%%/Sharpe %.2f\n", f.Index, f.TrainStats.ReturnPct, f.TrainStats.Sharpe, f.TestStats.ReturnPct, f.TestStats.Sharpe)
			}
			if out == "" {
				out = fmt.Sprintf("reports/%s_%s_WF.json", symbol, variant)
			}
			os.MkdirAll(filepath.Dir(out), 0755)
			b, _ := json.MarshalIndent(wf, "", "  ")
			os.WriteFile(out, b, 0644)
			fmt.Printf("saved %s\n", out)
		},
	}
	cmd.Flags().StringVar(&symbol, "symbol", "", "")
	cmd.Flags().StringVar(&variant, "variant", "", "")
	cmd.Flags().StringVar(&csvPath, "csv", "", "")
	cmd.Flags().IntVar(&folds, "folds", 0, "")
	cmd.Flags().StringVar(&out, "out", "", "")
	return cmd
}

func cmdMonteCarlo() *cobra.Command {
	var symbol, variant, csvPath, out string
	var runs int
	cmd := &cobra.Command{
		Use:   "montecarlo",
		Short: "MonteCarlo bootstrap (block) sul trade list",
		Run: func(cmd *cobra.Command, args []string) {
			cfg := loadCfg(cmd.Flags().Changed("config"))
			if symbol == "" {
				symbol = cfg.General.Symbols[0]
			}
			if variant == "" {
				variant = "D"
			}
			variant = strings.ToUpper(variant)
			if runs == 0 {
				runs = cfg.MonteCarlo.Runs
			}
			if csvPath == "" {
				csvPath = fmt.Sprintf("data/raw/%s_%s.csv", symbol, cfg.General.Interval)
			}
			var bars data.Bars
			if _, e := os.Stat(csvPath); e == nil {
				b, _ := data.LoadBarsCSV(csvPath)
				bars = b
			} else {
				seedMap := map[string]int64{"BTCUSDT": 42, "ETHUSDT": 1337, "SOLUSDT": 9999}
				seed := seedMap[symbol]
				if seed == 0 {
					seed = 42
				}
				bars = data.GenerateSynthetic(3500, 4*time.Hour, seed)
			}
			strat := strategy.New(variant, cfg)
			eng := engineFromCfg(cfg, variant, symbol)
			mc := analysis.MonteCarlo(bars, strat, cfg, eng, runs, cfg.MonteCarlo.PerturbationPct, cfg.MonteCarlo.BlockSize, cfg.MonteCarlo.Seed)
			fmt.Printf("MC %s %s runs %d median %.1f%% p5 %.1f%% p95 %.1f%% probProfit %.1f%%\n", symbol, variant, mc.Runs, mc.MedianReturn, mc.P5Return, mc.P95Return, mc.ProbProfit)
			if out == "" {
				out = fmt.Sprintf("reports/%s_%s_MC.json", symbol, variant)
			}
			os.MkdirAll(filepath.Dir(out), 0755)
			b, _ := json.MarshalIndent(mc, "", "  ")
			os.WriteFile(out, b, 0644)
			fmt.Printf("saved %s\n", out)
		},
	}
	cmd.Flags().StringVar(&symbol, "symbol", "", "")
	cmd.Flags().StringVar(&variant, "variant", "", "")
	cmd.Flags().StringVar(&csvPath, "csv", "", "")
	cmd.Flags().IntVar(&runs, "runs", 0, "")
	cmd.Flags().StringVar(&out, "out", "", "")
	return cmd
}

func cmdPerturb() *cobra.Command {
	var symbol, variant, csvPath, out string
	cmd := &cobra.Command{
		Use:   "perturb",
		Short: "Parameter perturbation ±20% — test robustezza (no overfit)",
		Run: func(cmd *cobra.Command, args []string) {
			cfg := loadCfg(cmd.Flags().Changed("config"))
			if symbol == "" {
				symbol = cfg.General.Symbols[0]
			}
			if variant == "" {
				variant = "D"
			}
			variant = strings.ToUpper(variant)
			if csvPath == "" {
				csvPath = fmt.Sprintf("data/raw/%s_%s.csv", symbol, cfg.General.Interval)
			}
			var bars data.Bars
			if _, e := os.Stat(csvPath); e == nil {
				b, _ := data.LoadBarsCSV(csvPath)
				bars = b
			} else {
				seedMap := map[string]int64{"BTCUSDT": 42, "ETHUSDT": 1337, "SOLUSDT": 9999}
				seed := seedMap[symbol]
				if seed == 0 {
					seed = 42
				}
				bars = data.GenerateSynthetic(3500, 4*time.Hour, seed)
			}
			eng := engineFromCfg(cfg, variant, symbol)
			results := analysis.Perturb(bars, cfg, variant, symbol, eng)
			fmt.Print(analysis.PerturbSummary(results))
			if out == "" {
				out = fmt.Sprintf("reports/%s_%s_PERTURB.json", symbol, variant)
			}
			os.MkdirAll(filepath.Dir(out), 0755)
			b, _ := json.MarshalIndent(results, "", "  ")
			os.WriteFile(out, b, 0644)
			fmt.Printf("saved %s\n", out)
		},
	}
	cmd.Flags().StringVar(&symbol, "symbol", "", "")
	cmd.Flags().StringVar(&variant, "variant", "", "")
	cmd.Flags().StringVar(&csvPath, "csv", "", "")
	cmd.Flags().StringVar(&out, "out", "", "")
	return cmd
}

func cmdPortfolio() *cobra.Command {
	var variant, out string
	cmd := &cobra.Command{
		Use:   "portfolio",
		Short: "Portfolio test — BTC/ETH/SOL con heat condiviso (max_open_risk 3%)",
		Run: func(cmd *cobra.Command, args []string) {
			cfg := loadCfg(cmd.Flags().Changed("config"))
			if variant == "" {
				variant = "D"
			}
			variant = strings.ToUpper(variant)
			barsMap, _ := analysis.LoadBarsMap(cfg.General.Symbols, cfg.General.Interval)
			pr := analysis.RunPortfolio(barsMap, cfg, variant, cfg.General.Interval)
			fmt.Printf("PORTFOLIO %s (%s) — Symbols %v\n", variant, cfg.General.Interval, pr.Symbols)
			for sym, st := range pr.PerSymbolStats {
				fmt.Printf("  %s: %.1f%% CAGR %.1f%% Sharpe %.2f PF %.2f R%.2f Skew %.2f Trades %d\n", sym, st.ReturnPct, st.ReturnAnnual, st.Sharpe, st.ProfitFactor, st.ExpectancyR, st.SkewR, st.Trades)
			}
			fmt.Printf("COMBINED: %.1f%% (%.1f%% CAGR) Sharpe %.2f PF %.2f ExpectancyR %.2f SkewR %.2f Trades %d MaxHeat %.1f%%\n",
				pr.CombinedStats.ReturnPct, pr.CombinedStats.ReturnAnnual, pr.CombinedStats.Sharpe, pr.CombinedStats.ProfitFactor, pr.CombinedStats.ExpectancyR, pr.CombinedStats.SkewR, pr.TotalTrades, pr.MaxHeatSeen)
			if out == "" {
				out = fmt.Sprintf("reports/PORTFOLIO_%s.html", variant)
			}
			// Generate simple portfolio HTML via comparison of per-symbol + combined
			// Reuse comparison report with portfolio stats as extra row
			var rows []report.ComparisonRow
			for sym, st := range pr.PerSymbolStats {
				rows = append(rows, report.ComparisonRow{Symbol: sym, Variant: variant, Stats: st})
			}
			rows = append(rows, report.ComparisonRow{Symbol: "PORTFOLIO", Variant: variant, Stats: pr.CombinedStats})
			report.GenerateComparison(out, rows, cfg)
			fmt.Printf("portfolio report %s\n", out)
			// also save JSON
			jpath := strings.Replace(out, ".html", ".json", 1)
			b, _ := json.MarshalIndent(pr, "", "  ")
			os.WriteFile(jpath, b, 0644)
		},
	}
	cmd.Flags().StringVar(&variant, "variant", "", "A/B/C/D")
	cmd.Flags().StringVar(&out, "out", "", "")
	return cmd
}

func cmdPortfolioBacktest() *cobra.Command {
	var cfgPath, csvPattern, outHTML string
	cmd := &cobra.Command{
		Use:   "portfolio-backtest",
		Short: "Backtest PORTFOLIO multi-simbolo (equity+heat condivisi)",
		Run: func(cmd *cobra.Command, args []string) {
			cfg, err := config.Load(cfgPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "config load failed (%s): %v\n", cfgPath, err)
				os.Exit(1)
			}
			symbols := cfg.General.Symbols
			if len(symbols) == 0 {
				fmt.Fprintf(os.Stderr, "general.symbols vuoto\n")
				os.Exit(1)
			}
			barsMap := map[string]data.Bars{}
			strats := map[string]strategy.Strategy{}
			for _, s := range symbols {
				p := strings.ReplaceAll(csvPattern, "{SYMBOL}", s)
				bars, err := data.LoadBarsCSV(p)
				if err != nil {
					fmt.Fprintf(os.Stderr, "csv %s: %v\n", p, err)
					os.Exit(1)
				}
				barsMap[s] = bars
				strats[s] = strategy.New("A", cfg)
			}
			eng := backtest.EngineConfigFrom(cfg, "A", "PORTFOLIO")
			res := backtest.RunPortfolio(barsMap, strats, cfg, eng)
			stats := metrics.Compute(res)
			fmt.Printf("PORTFOLIO %s: Return %.2f%% CAGR %.2f%% Sharpe %.2f Sortino %.2f MaxDD %.2f%% PF %.2f Trades %d Fee $%.2f Funding $%.2f\n",
				strings.Join(symbols, "+"), stats.ReturnPct, stats.ReturnAnnual, stats.Sharpe, stats.Sortino, stats.MaxDD, stats.ProfitFactor, stats.Trades, stats.TotalFee, stats.TotalFunding)
			fmt.Printf("scaling ceiling: %.2f%% (%s lega)\n", res.ScalingCeilingPct, res.ScalingBinding)
			for _, w := range res.Warnings {
				fmt.Printf("warn: %s\n", w)
			}
			// breakdown per-simbolo (da trade list)
			type symAgg struct {
				trades, winners int
				pnl             float64
			}
			aggs := map[string]*symAgg{}
			for _, tr := range res.Trades {
				a := aggs[tr.Symbol]
				if a == nil {
					a = &symAgg{}
					aggs[tr.Symbol] = a
				}
				a.trades++
				a.pnl += tr.PnLNet
				if tr.PnLNet > 0 {
					a.winners++
				}
			}
			fmt.Println("per-symbol (da trade list):")
			for _, s := range symbols {
				if a, ok := aggs[s]; ok {
					fmt.Printf("  %s: trades %d, win %d (%.0f%%), PnL netto $%.2f\n", s, a.trades, a.winners, float64(a.winners)/float64(a.trades)*100, a.pnl)
				}
			}
			if outHTML == "" {
				outHTML = fmt.Sprintf("reports/PORTFOLIO_%s.html", time.Now().Format("20060102_1504"))
			}
			if err := report.Generate(outHTML, report.Input{Config: cfg, Bars: barsMap[symbols[0]], Result: res, Stats: stats, Symbol: "PORTFOLIO", Variant: "A", GeneratedAt: time.Now()}); err != nil {
				fmt.Fprintf(os.Stderr, "report %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("report %s\n", outHTML)
		},
	}
	cmd.Flags().StringVar(&cfgPath, "config", config.DefaultPath(), "path config yaml")
	cmd.Flags().StringVar(&csvPattern, "csvs", "data/raw/{SYMBOL}_4h.csv", "pattern CSV per simbolo ({SYMBOL} sostituito)")
	cmd.Flags().StringVar(&outHTML, "out", "", "output html path")
	return cmd
}

func cmdGenerateDemo() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "generate-demo",
		Short: "Genera dataset sintetico demo e salva CSV (per verificare pipeline)",
		Run: func(cmd *cobra.Command, args []string) {
			cfg := loadCfg(cmd.Flags().Changed("config"))
			seedMap := map[string]int64{"BTCUSDT": 42, "ETHUSDT": 1337, "SOLUSDT": 9999}
			for _, sym := range cfg.General.Symbols {
				seed := seedMap[sym]
				if seed == 0 {
					seed = 42
				}
				bars := data.GenerateSynthetic(3000, 4*time.Hour, seed)
				path := fmt.Sprintf("data/raw/%s_%s.csv", sym, cfg.General.Interval)
				data.SaveBarsCSV(path, bars)
				fmt.Printf("demo %s %d bars seed %d -> %s\n", sym, len(bars), seed, path)
			}
		},
	}
	return cmd
}
func cmdReportDemo() *cobra.Command {
	// alias per quick demo: generate + compare
	cmd := &cobra.Command{
		Use:   "demo",
		Short: "Run full demo (synthetic) A/B/C/D + reports — verifica pipeline",
		Run: func(cmd *cobra.Command, args []string) {
			cfg := loadCfg(cmd.Flags().Changed("config"))
			seedMap := map[string]int64{"BTCUSDT": 42, "ETHUSDT": 1337, "SOLUSDT": 9999}
			for _, sym := range cfg.General.Symbols {
				seed := seedMap[sym]
				if seed == 0 {
					seed = 42
				}
				bars := data.GenerateSynthetic(3000, 4*time.Hour, seed)
				// reuse compare logic inline
				for _, v := range []string{"A", "B", "C", "D"} {
					strat := strategy.New(v, cfg)
					eng := engineFromCfg(cfg, v, sym)
					res := backtest.Run(bars, strat, cfg, eng)
					stats := metrics.Compute(res)
					out := fmt.Sprintf("reports/demo_%s_%s.html", sym, v)
					report.Generate(out, report.Input{Config: cfg, Bars: bars, Result: res, Stats: stats, Symbol: sym, Variant: v, GeneratedAt: time.Now()})
					fmt.Printf("demo %s %s %.1f%% Sharpe %.2f -> %s\n", sym, v, stats.ReturnPct, stats.Sharpe, out)
				}
			}
		},
	}
	return cmd
}

func engineFromCfg(cfg *config.Config, variant, symbol string) backtest.EngineConfig {
	eng := backtest.EngineConfigFrom(cfg, variant, symbol)
	// regime BTC filter: carica la serie BTC se il filtro è attivo (spec regime.btc_filter).
	// Se il CSV non esiste il filtro resta silenziosamente inattivo (engine: len==0).
	if cfg.Regime.BtcFilter {
		path := fmt.Sprintf("data/raw/BTCUSDT_%s.csv", cfg.General.Interval)
		if symbol != "BTCUSDT" {
			if bars, err := data.LoadBarsCSV(path); err == nil {
				eng.RegimeBars = bars
				eng.RegimeSMALen = cfg.Regime.SMALen
			}
		}
	}
	return eng
}

// intervalDuration converte l'interval Binance in time.Duration.
func intervalDuration(s string) time.Duration {
	switch s {
	case "1m":
		return time.Minute
	case "3m":
		return 3 * time.Minute
	case "5m":
		return 5 * time.Minute
	case "15m":
		return 15 * time.Minute
	case "30m":
		return 30 * time.Minute
	case "1h":
		return time.Hour
	case "2h":
		return 2 * time.Hour
	case "4h":
		return 4 * time.Hour
	case "6h":
		return 6 * time.Hour
	case "8h":
		return 8 * time.Hour
	case "12h":
		return 12 * time.Hour
	case "1d":
		return 24 * time.Hour
	default:
		return 4 * time.Hour
	}
}

func parseTime(s, fallback string) time.Time {
	if s == "" {
		s = fallback
	}
	if s == "" {
		return time.Time{}
	}
	layouts := []string{"2006-01-02", "2006-01-02 15:04", "2006-01-02T15:04:05Z07:00", time.RFC3339}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t.UTC()
		}
	}
	// try fallback
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.UTC()
	}
	return time.Time{}
}
