// optimize4 — raffinamento mirato attorno al vincitore v3 H4 (alt20 sma300 atr20×1.6 dx10 sx55 sat0.4).
// Famiglie testate una alla volta (train → selezione per train Sharpe/DD, poi UNA lettura test per la migliore).
// Uso: go run ./scripts/optimize4 -symbol BTCUSDT -csv data/raw/BTCUSDT_4h.csv
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

type mod struct {
	name   string
	apply  func(cfg *config.Config)
	family string
}

func main() {
	symbol := flag.String("symbol", "BTCUSDT", "")
	csvPath := flag.String("csv", "data/raw/BTCUSDT_4h.csv", "")
	flag.Parse()

	bars, err := data.LoadBarsCSV(*csvPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load csv: %v\n", err)
		os.Exit(1)
	}
	split := int(float64(len(bars)) * 0.7)
	train, test := bars[:split], bars[split:]

	run := func(m mod) (trainS, testS metrics.Stats) {
		cfg, err := config.Load("configs/atps_v3.yaml")
		if err != nil {
			panic(err)
		}
		m.apply(cfg)
		strat := strategy.New("A", cfg)
		eng := backtest.EngineConfigFrom(cfg, "A", *symbol)
		eng.InitialCapital = 10000
		trainS = metrics.Compute(backtest.Run(train, strat, cfg, eng))
		strat2 := strategy.New("A", cfg)
		testS = metrics.Compute(backtest.Run(test, strat2, cfg, eng))
		return
	}

	setTrail := func(mode string, mult float64) func(*config.Config) {
		return func(c *config.Config) { c.VariantA.Engine.TrailMode = mode; c.VariantA.Engine.TrailATRMult = mult }
	}
	setStop := func(m float64) func(*config.Config) {
		return func(c *config.Config) { c.VariantA.ATRStopMult = m }
	}
	setDon := func(d int) func(*config.Config) {
		return func(c *config.Config) { c.VariantA.Engine.DonExit = d }
	}
	setSat := func(a float64) func(*config.Config) {
		return func(c *config.Config) { c.Profit.Satellite.Allocation = a }
	}
	setSx := func(n int) func(*config.Config) {
		return func(c *config.Config) { c.VariantA.Engine.SatelliteExitLen = n }
	}
	setAlt := func(n int) func(*config.Config) {
		return func(c *config.Config) { c.VariantA.DonchianAlt = n }
	}
	setSMA := func(n int) func(*config.Config) {
		return func(c *config.Config) { c.VariantA.SMAFilter = n }
	}
	setPyr := func(adds int, step float64) func(*config.Config) {
		return func(c *config.Config) {
			c.Pyramiding.Enabled = true
			c.Pyramiding.MaxAdditions = adds
			c.Pyramiding.RiskNeutral = true
			c.VariantA.Engine.PyramidStepATR = step
		}
	}
	setATRP := func(n int) func(*config.Config) {
		return func(c *config.Config) { c.VariantA.ATRPeriod = n }
	}

	mods := []mod{
		{name: "BASE v3", apply: func(c *config.Config) {}, family: "base"},
		{name: "chandelier 2.5", apply: setTrail("chandelier", 2.5), family: "trail"},
		{name: "chandelier 3.0", apply: setTrail("chandelier", 3.0), family: "trail"},
		{name: "chandelier 3.5", apply: setTrail("chandelier", 3.5), family: "trail"},
		{name: "pyr 3 adds 0.5atr", apply: setPyr(3, 0.5), family: "pyr"},
		{name: "pyr 5 adds 0.5atr", apply: setPyr(5, 0.5), family: "pyr"},
		{name: "pyr 3 adds 1.0atr", apply: setPyr(3, 1.0), family: "pyr"},
		{name: "stop 1.4", apply: setStop(1.4), family: "stop"},
		{name: "stop 1.5", apply: setStop(1.5), family: "stop"},
		{name: "stop 1.7", apply: setStop(1.7), family: "stop"},
		{name: "atrP 28", apply: setATRP(28), family: "atrp"},
		{name: "atrP 40", apply: setATRP(40), family: "atrp"},
		{name: "don 6", apply: setDon(6), family: "don"},
		{name: "don 8", apply: setDon(8), family: "don"},
		{name: "don 14", apply: setDon(14), family: "don"},
		{name: "don 20", apply: setDon(20), family: "don"},
		{name: "sat 0.5", apply: setSat(0.5), family: "sat"},
		{name: "sat 0.6", apply: setSat(0.6), family: "sat"},
		{name: "sat0.5 sx84", apply: func(c *config.Config) { setSat(0.5)(c); setSx(84)(c) }, family: "sat"},
		{name: "sat0.4 sx84", apply: setSx(84), family: "sat"},
		{name: "alt 16", apply: setAlt(16), family: "alt"},
		{name: "alt 24", apply: setAlt(24), family: "alt"},
		{name: "alt 28", apply: setAlt(28), family: "alt"},
		{name: "sma 250", apply: setSMA(250), family: "sma"},
		{name: "sma 400", apply: setSMA(400), family: "sma"},
	}

	fmt.Printf("%-24s | %8s %7s %7s %6s | %8s %7s %7s %6s\n", "mod", "trCAGR", "trDD", "trSharpe", "trTrd", "teCAGR", "teDD", "teCal", "teTrd")
	for _, m := range mods {
		ts, te := run(m)
		fmt.Printf("%-24s | %8.1f %7.1f %7.2f %6d | %8.1f %7.1f %7.2f %6d  [%s]\n",
			m.name, ts.ReturnAnnual, ts.MaxDD, ts.Sharpe, ts.Trades, te.ReturnAnnual, te.MaxDD, te.Calmar, te.Trades, m.family)
	}
}
