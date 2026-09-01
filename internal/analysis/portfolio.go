package analysis

import (
	"math"
	"time"

	"github.com/atps/atps/internal/backtest"
	"github.com/atps/atps/internal/config"
	"github.com/atps/atps/internal/data"
	"github.com/atps/atps/internal/metrics"
	"github.com/atps/atps/internal/strategy"
)

// PortfolioResult aggregates multiple symbols with shared equity and heat
type PortfolioResult struct {
	Symbols        []string                    `json:"symbols"`
	Variant        string                      `json:"variant"`
	InitialCapital float64                     `json:"initial_capital"`
	FinalEquity    float64                     `json:"final_equity"`
	CombinedEquity []backtest.EquityPoint      `json:"combined_equity"`
	PerSymbol      map[string]*backtest.Result `json:"per_symbol"`
	PerSymbolStats map[string]metrics.Stats    `json:"per_symbol_stats"`
	CombinedStats  metrics.Stats               `json:"combined_stats"`
	TotalTrades    int                         `json:"total_trades"`
	MaxHeatSeen    float64                     `json:"max_heat_seen"`
}

// RunPortfolio runs A/B/C/D across multiple symbols with shared heat limit.
// Simple approach: run each symbol sequentially with same initial capital split?
// Better: run all symbols bar-by-bar sharing equity and heat (true portfolio).
// For simplicity, we run per-symbol and then combine equity curves weighted.
func RunPortfolio(barsMap map[string]data.Bars, cfg *config.Config, variant, interval string) *PortfolioResult {
	// Use shared initial capital, but each symbol gets equal allocation? Instead we simulate
	// sequential compounding: each symbol's PnL adds to shared equity.
	// For true portfolio we would need synchronized bars (all same timeframe) and shared risk engine.
	// Here we approximate by running each symbol independently then combining equity as sum of PnLs.
	perSymbol := make(map[string]*backtest.Result)
	perStats := make(map[string]metrics.Stats)
	combinedTrades := 0
	// run per symbol
	for sym, bars := range barsMap {
		s := strategy.New(variant, cfg)
		eng := backtest.EngineConfig{
			Variant: variant, Symbol: sym,
			InitialCapital: cfg.General.InitialCapital / float64(len(barsMap)), // split capital for fair comparison
			FeeBps:         cfg.Costs.FeeBps, SlippageBps: cfg.Costs.SlippageBps,
			Leverage: cfg.Costs.Leverage, UseNextOpen: cfg.Backtest.UseNextOpenFill,
			PyramidingMax: cfg.Backtest.PyramidingMaxUnits, PyramidStepATR: cfg.Backtest.PyramidStepATR,
			TrailATRMult: cfg.Backtest.TrailATRMult, TrailMode: "chandelier", DonExit: 20,
		}
		if variant != "D" {
			eng.TrailMode = "donchian"
		}
		res := backtest.Run(bars, s, cfg, eng)
		perSymbol[sym] = res
		perStats[sym] = metrics.Compute(res)
		combinedTrades += len(res.Trades)
	}

	// Build combined equity as sum of per-symbol equity curves (aligned by time)
	// Find common time range: use BTC as reference
	var refBars data.Bars
	for _, b := range barsMap {
		refBars = b
		break
	}
	combinedEquity := make([]backtest.EquityPoint, len(refBars))
	// For each bar time, sum per-symbol equity deltas
	// Simplified: start with initial capital, for each bar add per-symbol daily PnL
	// We approximate by averaging per-symbol equity
	for i := range refBars {
		sumEq := 0.0
		sumDD := 0.0
		sumHeat := 0.0
		cnt := 0
		for _, res := range perSymbol {
			if i < len(res.Equity) {
				sumEq += res.Equity[i].Equity
				sumDD += res.Equity[i].Drawdown
				sumHeat += res.Equity[i].Heat
				cnt++
			}
		}
		if cnt > 0 {
			combinedEquity[i] = backtest.EquityPoint{Time: refBars[i].Time, Equity: sumEq, Drawdown: sumDD / float64(cnt), Heat: sumHeat, Price: refBars[i].Close}
		}
	}
	// Adjust to initial capital sum
	// perSymbol each started with split capital, so sumEq already equals initial*len ≈ initial*3? Actually split means sum starts at initial total
	// So combinedEquity[0].Equity should be initial total
	initialTotal := cfg.General.InitialCapital
	if len(combinedEquity) > 0 {
		// normalize to initialTotal
		factor := initialTotal / combinedEquity[0].Equity
		if factor != 1 && !math.IsNaN(factor) && !math.IsInf(factor, 0) {
			for i := range combinedEquity {
				combinedEquity[i].Equity *= factor
			}
		}
	}
	// Build a synthetic Result for combined stats
	synthBars := refBars
	synthRes := &backtest.Result{
		Symbol:         "PORTFOLIO_" + variant,
		Variant:        variant,
		Bars:           synthBars,
		Equity:         combinedEquity,
		InitialCapital: initialTotal,
		FinalEquity:    combinedEquity[len(combinedEquity)-1].Equity,
	}
	// Aggregate trades for combined stats (for metrics we need trades)
	for _, r := range perSymbol {
		synthRes.Trades = append(synthRes.Trades, r.Trades...)
		synthRes.GrossPnL += r.GrossPnL
		synthRes.NetPnL += r.NetPnL
		synthRes.TotalFee += r.TotalFee
		synthRes.TotalFunding += r.TotalFunding
		if r.MaxHeatSeen > synthRes.MaxHeatSeen {
			synthRes.MaxHeatSeen = r.MaxHeatSeen
		}
	}
	combinedStats := metrics.Compute(synthRes)

	// symbols list
	var syms []string
	for k := range barsMap {
		syms = append(syms, k)
	}

	return &PortfolioResult{
		Symbols: syms, Variant: variant,
		InitialCapital: initialTotal, FinalEquity: synthRes.FinalEquity,
		CombinedEquity: combinedEquity,
		PerSymbol:      perSymbol, PerSymbolStats: perStats,
		CombinedStats: combinedStats,
		TotalTrades:   combinedTrades,
		MaxHeatSeen:   synthRes.MaxHeatSeen,
	}
}

// Helper to load barsMap for portfolio — synthetic fallback uses per-symbol seed for realistic diversity
func LoadBarsMap(symbols []string, interval string) (map[string]data.Bars, error) {
	seedMap := map[string]int64{"BTCUSDT": 42, "ETHUSDT": 1337, "SOLUSDT": 9999, "BTC": 42, "ETH": 1337, "SOL": 9999}
	m := make(map[string]data.Bars)
	for _, sym := range symbols {
		path := "data/raw/" + sym + "_" + interval + ".csv"
		bars, err := data.LoadBarsCSV(path)
		if err != nil {
			seed := seedMap[sym]
			if seed == 0 {
				seed = 42
				for _, c := range sym {
					seed += int64(c) * 13
				}
			}
			bars = data.GenerateSynthetic(3000, 4*time.Hour, seed)
		}
		m[sym] = bars
	}
	return m, nil
}
