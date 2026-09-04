package backtest

import (
	"math"
	"sort"
	"testing"
	"time"

	"github.com/atps/atps/internal/config"
	"github.com/atps/atps/internal/data"
	"github.com/atps/atps/internal/strategy"
)

// Test invariante: RunPortfolio con UN simbolo deve riprodurre engine.Run
func TestRunPortfolioSingleSymbolInvariantBTC(t *testing.T) {
	cfg, err := config.Load("../../configs/atps_v2.yaml")
	if err != nil {
		t.Fatal(err)
	}
	bars, err := data.LoadBarsCSV("../../data/raw/BTCUSDT_4h.csv")
	if err != nil {
		t.Fatal(err)
	}
	strat := strategy.New("A", cfg)
	eng := EngineConfigFrom(cfg, "A", "BTCUSDT")
	single := Run(bars, strat, cfg, eng)
	port := RunPortfolio(map[string]data.Bars{"BTCUSDT": bars}, map[string]strategy.Strategy{"BTCUSDT": strat}, cfg, eng)
	if len(port.Trades) != len(single.Trades) {
		t.Fatalf("trades %d != %d", len(port.Trades), len(single.Trades))
	}
	if math.Abs(port.FinalEquity-single.FinalEquity) > 1e-6 {
		t.Errorf("FinalEquity %.6f != %.6f", port.FinalEquity, single.FinalEquity)
	}
	for i := range single.Trades {
		a, b := &single.Trades[i], &port.Trades[i]
		if a.Symbol != b.Symbol || a.EntryTime != b.EntryTime || a.ExitTime != b.ExitTime ||
			math.Abs(a.PnLNet-b.PnLNet) > 1e-9 || a.ExitReason != b.ExitReason {
			t.Errorf("trade[%d] diverge: %+v vs %+v", i, a, b)
		}
	}
	if len(port.Equity) != len(single.Equity) {
		t.Errorf("equity points %d != %d", len(port.Equity), len(single.Equity))
	}
}

func TestRunPortfolioSingleSymbolInvariantETH(t *testing.T) {
	cfg, err := config.Load("../../configs/atps_v2.yaml")
	if err != nil {
		t.Fatal(err)
	}
	bars, err := data.LoadBarsCSV("../../data/raw/ETHUSDT_4h.csv")
	if err != nil {
		t.Fatal(err)
	}
	strat := strategy.New("A", cfg)
	eng := EngineConfigFrom(cfg, "A", "ETHUSDT")
	single := Run(bars, strat, cfg, eng)
	port := RunPortfolio(map[string]data.Bars{"ETHUSDT": bars}, map[string]strategy.Strategy{"ETHUSDT": strat}, cfg, eng)
	if len(port.Trades) != len(single.Trades) || math.Abs(port.FinalEquity-single.FinalEquity) > 1e-6 {
		t.Fatalf("invariante ETH rotta: trades %d/%d equity %.4f/%.4f",
			len(port.Trades), len(single.Trades), port.FinalEquity, single.FinalEquity)
	}
}

// fixture portfolio: simboli sintetici sfasati, heat condiviso 3% blocca la 4ª entry
func portfolioTestCfg(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.Load("../../configs/default.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Profit.Satellite.Enabled = false
	cfg.Pyramiding.Enabled = false
	// risk fisso 1% per trade, heat 3% → max 3 posizioni contemporanee
	cfg.Risk.Base = 0.01
	cfg.Risk.Max = 0.01
	cfg.Risk.MaxRiskPerTradePct = 1.0
	cfg.Risk.KellyCapPct = 1.0
	cfg.VariantA.RiskPct = 1.0
	cfg.Portfolio.MaxOpenRisk = 0.03
	cfg.Portfolio.MaxCorrelatedRisk = 0.03
	cfg.Portfolio.CrashBrakeDropPct = 0 // off per test
	return cfg
}

func portfolioEng() EngineConfig {
	return EngineConfig{
		Variant: "A", Symbol: "PORTFOLIO", InitialCapital: 10000,
		FeeBps: 4, SlippageBps: 2, UseNextOpen: true,
		PyramidingMax: 0, TrailMode: "donchian", DonExit: 20, EntryMode: "close",
	}
}

func portStrat(cfg *config.Config, atBars ...int) strategy.Strategy {
	signals := map[int]strategy.Signal{}
	for _, b := range atBars {
		signals[b] = strategy.Signal{Side: 1, Strength: 1, StopPrice: 80, Reason: "script long"}
	}
	return &reentryStrat{scriptStrategy{cfg: cfg, signals: signals}, -1} // ReEntry mai (ExitBarIdx -1)
}

// pumpBars — warmup a volatilità DECLINANTE con coda piatta: l'ATR Wilder
// converge a ~1.0 (realVol ~48% < vol target 50% → niente vol-target scaling)
// restando il minimo stretto della finestra percentile 100 (percentile ~1 →
// niente vol-adaptive scaling); un warmup tutto piatto dà ATR costante →
// percentile 100 per i tie → risk scalato a 0.475%). Poi pump crescente
// (evita lo stop donchian-trailing che sulle barre piatte ratchet-a lo stop
// sul low e chiude subito le posizioni)
func pumpBars(n, from int) data.Bars {
	bars := make(data.Bars, n)
	decl := from - 60 // barre di discesa; ultime 60 piatte a wiggle 0.5
	for i := 0; i < from; i++ {
		w := 0.5
		if i < decl {
			w = 5.0 - 4.5*float64(i)/float64(decl) // wiggle 5.0 → 0.5
		}
		bars[i] = data.Bar{Time: time.Unix(int64(i)*14400, 0), Open: 100, High: 100 + w, Low: 100 - w, Close: 100, Volume: 100}
	}
	for i := from; i < n; i++ {
		c := 100 + float64(i-from)*0.3
		bars[i] = data.Bar{Time: time.Unix(int64(i)*14400, 0), Open: c - 0.3, High: c + 0.2, Low: c - 0.5, Close: c, Volume: 100}
	}
	return bars
}

func shiftBars(bars data.Bars, offset time.Duration) data.Bars {
	out := make(data.Bars, len(bars))
	copy(out, bars)
	for i := range out {
		out[i].Time = out[i].Time.Add(offset)
	}
	return out
}

func TestPortfolioSharedHeatCap(t *testing.T) {
	cfg := portfolioTestCfg(t)
	// pump dal 206: entry (segnale 205 → fill 206) che restano aperte
	// (low crescenti > stop donchian ratchettato).
	// NOTA scaffolding: con PyramidingMax=0 un simbolo può tenere UNA sola
	// posizione (un segnale same-side con posizione aperta passa dal branch
	// pyramiding e CanPyramid rigetta) → servono 4 candidate per osservare
	// il cap: 4 simboli sfasati (interleaving), risk 1%, heat condiviso 3%
	// → le prime 3 aprono, la 4ª è rifiutata dal heat cap condiviso.
	barsMap := map[string]data.Bars{
		"A": pumpBars(300, 206),
		"B": shiftBars(pumpBars(300, 206), 2*time.Hour),
		"C": shiftBars(pumpBars(300, 206), 4*time.Hour),
		"D": shiftBars(pumpBars(300, 206), 6*time.Hour),
	}
	strats := map[string]strategy.Strategy{}
	for _, s := range []string{"A", "B", "C", "D"} {
		strats[s] = portStrat(cfg, 205)
	}
	res := RunPortfolio(barsMap, strats, cfg, portfolioEng())
	open := 0
	for _, tr := range res.Trades {
		if tr.ExitReason == "eod" {
			open++
		}
	}
	if open != 3 {
		t.Errorf("posizioni eod %d != 3: heat cap condiviso non applicato correttamente", open)
	}
	// il 4° simbolo deve essere rifiutato dal heat cap condiviso
	syms := map[string]bool{}
	for _, tr := range res.Trades {
		syms[tr.Symbol] = true
	}
	if len(syms) != 3 {
		t.Errorf("attesi trade di 3 simboli (4° bloccato dal heat cap), avuti %v", syms)
	}
	if res.MaxHeatSeen > 3.0001 {
		t.Errorf("heat massimo %.3f%% > 3%%: cap heat condiviso violato", res.MaxHeatSeen)
	}
}

func TestPortfolioCorrelatedCapClipsSecondSymbol(t *testing.T) {
	cfg := portfolioTestCfg(t)
	cfg.Risk.Base = 0.015
	cfg.Risk.Max = 0.015
	cfg.Risk.MaxRiskPerTradePct = 1.5
	cfg.Risk.KellyCapPct = 1.5
	cfg.VariantA.RiskPct = 1.5
	cfg.Portfolio.MaxOpenRisk = 0.09
	cfg.Portfolio.MaxCorrelatedRisk = 0.02 // correlati: 1.5% + max 0.5% residuo
	strats := map[string]strategy.Strategy{
		"A": portStrat(cfg, 205),
		"B": portStrat(cfg, 205),
	}
	res := RunPortfolio(map[string]data.Bars{
		"A": pumpBars(300, 206), "B": shiftBars(pumpBars(300, 206), 2*time.Hour),
	}, strats, cfg, portfolioEng())
	var riskPcts []float64
	for _, tr := range res.Trades {
		if tr.ExitReason == "eod" {
			riskPcts = append(riskPcts, tr.RiskPct)
		}
	}
	sort.Float64s(riskPcts)
	if len(riskPcts) != 2 {
		t.Fatalf("attese 2 posizioni eod, avute %d", len(riskPcts))
	}
	if riskPcts[0] > 0.51 || riskPcts[0] < 0.49 {
		t.Errorf("seconda entry same-side deve essere clippata a ~0.5%% (corr cap), avuto %.3f%%", riskPcts[0])
	}
	if riskPcts[1] < 1.49 {
		t.Errorf("prima entry deve restare ~1.5%%, avuto %.3f%%", riskPcts[1])
	}
}

func TestPortfolioSharedEquityGrowsFromBothSymbols(t *testing.T) {
	cfg := portfolioTestCfg(t)
	strats := map[string]strategy.Strategy{
		"A": portStrat(cfg, 205),
		"B": portStrat(cfg, 205),
	}
	res := RunPortfolio(map[string]data.Bars{
		"A": pumpBars(300, 206), "B": shiftBars(pumpBars(300, 206), 2*time.Hour),
	}, strats, cfg, portfolioEng())
	if res.FinalEquity <= 10000 {
		t.Errorf("equity condivisa deve crescere con PnL di entrambi i simboli, avuta %.2f", res.FinalEquity)
	}
	syms := map[string]bool{}
	for _, tr := range res.Trades {
		syms[tr.Symbol] = true
	}
	if len(syms) != 2 {
		t.Errorf("attesi trade di 2 simboli, avuti %v", syms)
	}
}
