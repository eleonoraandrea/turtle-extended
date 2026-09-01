package analysis

import (
	"math"
	"math/rand"
	"sort"

	"github.com/atps/atps/internal/backtest"
	"github.com/atps/atps/internal/config"
	"github.com/atps/atps/internal/data"
	"github.com/atps/atps/internal/strategy"
)

type MCResult struct {
	Symbol       string    `json:"symbol"`
	Variant      string    `json:"variant"`
	Runs         int       `json:"runs"`
	Returns      []float64 `json:"returns"`
	Sharpe       []float64 `json:"sharpe"`
	MaxDD        []float64 `json:"max_dd"`
	MedianReturn float64   `json:"median_return"`
	P5Return     float64   `json:"p5_return"`
	P95Return    float64   `json:"p95_return"`
	MedianSharpe float64   `json:"median_sharpe"`
	ProbProfit   float64   `json:"prob_profit"`
}

func MonteCarlo(bars data.Bars, strat strategy.Strategy, cfg *config.Config, eng backtest.EngineConfig, runs int, perturbPct float64, blockSize int, seed int64) MCResult {
	if runs <= 0 {
		runs = 1000
	}
	if blockSize <= 0 {
		blockSize = 20
	}
	rnd := rand.New(rand.NewSource(seed))
	// precompute original trade returns distribution? Instead we bootstrap bars returns with block bootstrap
	// Simpler: for each run, perturb close prices by random +/- perturb% and rerun backtest fast? Instead we bootstrap trades: resample trades with replacement
	// Use trade-level bootstrap from original result
	orig := backtest.Run(bars, strat, cfg, eng)
	trades := orig.Trades
	if len(trades) == 0 {
		return MCResult{Symbol: eng.Symbol, Variant: eng.Variant, Runs: runs}
	}
	var returns, sharpes, dds []float64
	for i := 0; i < runs; i++ {
		// block bootstrap trades: sample with replacement preserving sequence blocks
		var sample []backtest.Trade
		for len(sample) < len(trades) {
			start := rnd.Intn(len(trades))
			blockLen := blockSize
			if rnd.Float64() < 0.3 {
				blockLen = 1 + rnd.Intn(blockSize)
			} // variable
			for b := 0; b < blockLen && start+b < len(trades) && len(sample) < len(trades); b++ {
				t := trades[start+b]
				// perturb PnL by +/- perturbPct
				pert := 1 + (rnd.Float64()*2-1)*perturbPct/100.0
				t.PnLNet *= pert
				sample = append(sample, t)
			}
		}
		// compute equity curve from sampled trades: start equity, add each pnl
		eq := eng.InitialCapital
		peak := eq
		maxDD := 0.0
		for _, t := range sample {
			eq += t.PnLNet
			if eq > peak {
				peak = eq
			}
			dd := 0.0
			if peak > 0 {
				dd = (eq - peak) / peak * 100
			}
			if dd < maxDD {
				maxDD = dd
			}
		}
		ret := (eq - eng.InitialCapital) / eng.InitialCapital * 100
		// sharpe approx from trade returns
		var rets []float64
		for _, t := range sample {
			rets = append(rets, t.PnLNet)
		}
		sh := tradeSharpe(rets)
		returns = append(returns, ret)
		sharpes = append(sharpes, sh)
		dds = append(dds, maxDD)
	}
	sort.Float64s(returns)
	sort.Float64s(sharpes)
	sortedDD := append([]float64(nil), dds...)
	sort.Float64s(sortedDD)
	probProfit := 0.0
	for _, r := range returns {
		if r > 0 {
			probProfit++
		}
	}
	if len(returns) > 0 {
		probProfit = probProfit / float64(len(returns)) * 100
	}
	return MCResult{Symbol: eng.Symbol, Variant: eng.Variant, Runs: runs, Returns: returns, Sharpe: sharpes, MaxDD: dds,
		MedianReturn: percentile(returns, 50), P5Return: percentile(returns, 5), P95Return: percentile(returns, 95),
		MedianSharpe: percentile(sharpes, 50), ProbProfit: probProfit}
}
func tradeSharpe(rets []float64) float64 {
	if len(rets) < 2 {
		return 0
	}
	m := meanRets(rets)
	sd := stdRets(rets, m)
	if sd == 0 {
		return 0
	}
	// annualized approx trades per year ~ 50? Use sqrt(252) proxy: sharpe ~ mean/std * sqrt(N)
	return m / sd * math.Sqrt(float64(len(rets)))
}
func meanRets(a []float64) float64 {
	s := 0.0
	for _, v := range a {
		s += v
	}
	return s / float64(len(a))
}
func stdRets(a []float64, m float64) float64 {
	s := 0.0
	for _, v := range a {
		d := v - m
		s += d * d
	}
	return math.Sqrt(s / float64(len(a)-1))
}
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(p/100*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
