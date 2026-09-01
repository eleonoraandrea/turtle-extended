package analysis

import (
	"fmt"
	"math"

	"github.com/atps/atps/internal/backtest"
	"github.com/atps/atps/internal/config"
	"github.com/atps/atps/internal/data"
	"github.com/atps/atps/internal/metrics"
	"github.com/atps/atps/internal/strategy"
)

type PerturbResult struct {
	Param        string  `json:"param"`
	Value        float64 `json:"value"`
	ReturnPct    float64 `json:"return_pct"`
	Sharpe       float64 `json:"sharpe"`
	MaxDD        float64 `json:"max_dd"`
	ProfitFactor float64 `json:"profit_factor"`
	Trades       int     `json:"trades"`
	ExpectancyR  float64 `json:"expectancy_r"`
	SkewR        float64 `json:"skew_r"`
}

func Perturb(bars data.Bars, baseCfg *config.Config, variant, symbol string, eng backtest.EngineConfig) []PerturbResult {
	var out []PerturbResult
	// baseline
	strat := strategy.New(variant, baseCfg)
	res := backtest.Run(bars, strat, baseCfg, eng)
	baseStats := metrics.Compute(res)
	out = append(out, PerturbResult{Param: "baseline", Value: 0, ReturnPct: baseStats.ReturnPct, Sharpe: baseStats.Sharpe, MaxDD: baseStats.MaxDD, ProfitFactor: baseStats.ProfitFactor, Trades: baseStats.Trades, ExpectancyR: baseStats.ExpectancyR, SkewR: baseStats.SkewR})

	// perturbations: donchian entry, atr stop, risk base
	perturbations := []struct {
		name  string
		apply func(*config.Config)
		orig  float64
		delta float64
	}{
		{"donchian_entry", func(c *config.Config) {
			c.VariantA.DonchianEntry = int(float64(c.VariantA.DonchianEntry) * 1.2)
			c.VariantB.DonchianEntry = int(float64(c.VariantB.DonchianEntry) * 1.2)
			c.VariantD.DonchianFast = int(float64(c.VariantD.DonchianFast) * 1.2)
		}, 55, 11},
		{"donchian_entry -20%", func(c *config.Config) {
			c.VariantA.DonchianEntry = int(float64(c.VariantA.DonchianEntry) * 0.8)
			c.VariantB.DonchianEntry = int(float64(c.VariantB.DonchianEntry) * 0.8)
			c.VariantD.DonchianFast = int(float64(c.VariantD.DonchianFast) * 0.8)
		}, 55, -11},
		{"atr_stop +20%", func(c *config.Config) {
			c.VariantA.ATRStopMult *= 1.2
			c.VariantB.ATRStopMult *= 1.2
			c.VariantD.ATRStopMult *= 1.2
			c.ATRConf.InitialStop *= 1.2
		}, 1.8, 0.36},
		{"atr_stop -20%", func(c *config.Config) {
			c.VariantA.ATRStopMult *= 0.8
			c.VariantB.ATRStopMult *= 0.8
			c.VariantD.ATRStopMult *= 0.8
			c.ATRConf.InitialStop *= 0.8
		}, 1.8, -0.36},
		{"risk base +0.5%", func(c *config.Config) { c.Risk.Base += 0.005; c.Risk.Max += 0.005 }, 0.01, 0.005},
		{"risk base -0.5%", func(c *config.Config) {
			c.Risk.Base -= 0.005
			if c.Risk.Base < 0.0025 {
				c.Risk.Base = 0.0025
			}
			c.Risk.Max -= 0.005
		}, 0.01, -0.005},
	}

	for _, p := range perturbations {
		cfgCopy := *baseCfg
		// deep copy variants? shallow is ok for our fields (no pointers)
		p.apply(&cfgCopy)
		s := strategy.New(variant, &cfgCopy)
		r := backtest.Run(bars, s, &cfgCopy, eng)
		st := metrics.Compute(r)
		out = append(out, PerturbResult{Param: p.name, Value: p.orig + p.delta, ReturnPct: st.ReturnPct, Sharpe: st.Sharpe, MaxDD: st.MaxDD, ProfitFactor: st.ProfitFactor, Trades: st.Trades, ExpectancyR: st.ExpectancyR, SkewR: st.SkewR})
	}
	return out
}

func PerturbSummary(results []PerturbResult) string {
	if len(results) == 0 {
		return "no data"
	}
	base := results[0]
	var sb string
	sb += fmt.Sprintf("Baseline: %.1f%% R%.2f Sharpe %.2f PF %.2f SkewR %.2f\n", base.ReturnPct, base.ExpectancyR, base.Sharpe, base.ProfitFactor, base.SkewR)
	minRet := math.MaxFloat64
	maxRet := -math.MaxFloat64
	for _, r := range results[1:] {
		if r.ReturnPct < minRet {
			minRet = r.ReturnPct
		}
		if r.ReturnPct > maxRet {
			maxRet = r.ReturnPct
		}
		delta := r.ReturnPct - base.ReturnPct
		sb += fmt.Sprintf("  %s: %.1f%% (Δ%.1f%%) Sharpe %.2f PF %.2f R%.2f Skew %.2f\n", r.Param, r.ReturnPct, delta, r.Sharpe, r.ProfitFactor, r.ExpectancyR, r.SkewR)
	}
	sb += fmt.Sprintf("Robustness: range %.1f%% to %.1f%% (Δ%.1f%%), all %d perturbs %s\n", minRet, maxRet, maxRet-minRet, len(results)-1, map[bool]string{true: "PROFITTEVOLI", false: "FRAGILE"}[minRet > 0 && base.ReturnPct > 0])
	return sb
}
