package backtest

import (
	"testing"
	"time"

	"github.com/atps/atps/internal/config"
	"github.com/atps/atps/internal/data"
	"github.com/atps/atps/internal/strategy"
)

// sepStrat — segnali long scriptati.
type sepStrat struct {
	scriptStrategy
}

func sepCfg(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.Load("../../configs/default.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Profit.Satellite.Enabled = false
	return cfg
}

func sepEng() EngineConfig {
	return EngineConfig{
		Variant: "A", Symbol: "TEST", InitialCapital: 10000,
		FeeBps: 4, SlippageBps: 2, UseNextOpen: true,
		PyramidingMax: 4, PyramidStepATR: 0.5, PyramidingMode: "separate",
		TrailMode: "donchian", DonExit: 10, EntryMode: "close",
	}
}

// NOTA PIANO-BUG (documentato): lo script originale usava flatBars(30) con
// segnali alle barre 2/5 e pump da barra 4. Così:
//   1. flatBars(30) + loop fino a 40 = panic index-out-of-range (fix: 40 barre);
//   2. ATR(20) è NaN fino a barra 19 → CanPyramid(atr=0) sempre false,
//      nessun add né in merged né in separate (serve pump dopo warmup ATR);
//   3. pump piatto a 101.5-low con trailing donchian si auto-stoppa appena la
//      finestra Don10 esce dal flat (low == stop trailed → stop).
// Fix: barre post-warmup (core segnale 25/fill 26, add 28/fill 29) + pump in
// leggero uptrend così low resta sopra lo stop trailed. Spirito invariato.

// sepPump scrive un pump in uptrend da from (incluso) a to (escluso):
// base sale di drift/bar così il trailing donchian non raggiunge mai low.
func sepPump(bars data.Bars, from, to int, base0, drift float64) {
	for i := from; i < to; i++ {
		base := base0 + drift*float64(i-from)
		bars[i] = data.Bar{Time: time.Unix(int64(i)*14400, 0), Open: base, High: base + 1.5, Low: base - 0.5, Close: base + 1, Volume: 100}
	}
}

func TestSeparatePyramidCreatesIndependentLeg(t *testing.T) {
	cfg := sepCfg(t)
	bars := flatBars(40, 100, 0.5)
	sepPump(bars, 26, 40, 102, 0.5)
	signals := map[int]strategy.Signal{
		25: {Side: 1, Strength: 1, StopPrice: 98, Reason: "script core"},
		28: {Side: 1, Strength: 1, StopPrice: 97, Reason: "script add"},
	}
	strat := &sepStrat{scriptStrategy{cfg: cfg, signals: signals}}
	res := Run(bars, strat, cfg, sepEng())
	if len(res.Trades) != 2 {
		t.Fatalf("attesi 2 trades a EOD (core + gamba), avuti %d", len(res.Trades))
	}
	var core, leg *Trade
	for i := range res.Trades {
		if res.Trades[i].EntryReason == "script core" {
			core = &res.Trades[i]
		} else {
			leg = &res.Trades[i]
		}
	}
	if core == nil || leg == nil {
		t.Fatalf("trade core/leg non trovati: %+v", res.Trades)
	}
	if core.Qty == leg.Qty && core.EntryPrice == leg.EntryPrice {
		t.Errorf("core e leg sembrano la stessa posizione fusa")
	}
	if leg.EntryReason != "script add | pyramid separate" {
		t.Errorf("leg EntryReason = %q", leg.EntryReason)
	}
	if leg.StopPrice != 97 {
		t.Errorf("leg StopPrice = %v, want 97 (stop proprio)", leg.StopPrice)
	}
}

func TestSeparatePyramidWideExitSurvivesCoreExit(t *testing.T) {
	cfg := sepCfg(t)
	bars := flatBars(40, 100, 0.5)
	sepPump(bars, 26, 30, 102, 0.5)
	for i := 30; i < 40; i++ {
		bars[i] = data.Bar{Time: time.Unix(int64(i)*14400, 0), Open: 99, High: 99.5, Low: 98.8, Close: 99, Volume: 100}
	}
	signals := map[int]strategy.Signal{
		25: {Side: 1, Strength: 1, StopPrice: 98, Reason: "script core"},
		28: {Side: 1, Strength: 1, StopPrice: 97, Reason: "script add"},
	}
	strat := &sepStrat{scriptStrategy{cfg: cfg, signals: signals}}
	res := Run(bars, strat, cfg, sepEng())
	if len(res.Trades) != 2 {
		t.Fatalf("attesi 2 trades (core exit + leg eod), avuti %d", len(res.Trades))
	}
	// PIANO-BUG: il piano attendeva donchian_exit per il core, ma con trailing
	// donchian lo stop == DonLow e low <= close < DonLow è impossibile senza
	// toccare prima lo stop (stop ha priorità nel loop exit). Con barre
	// post-warmup (trailing attivo) il core esce quindi in "stop" — lo spirito
	// (core esce, gamba wide-Don55 sopravvive) è invariato.
	if res.Trades[0].EntryReason != "script core" || res.Trades[0].ExitReason != "stop" {
		t.Errorf("trade[0] = %s/%s, want script core/stop", res.Trades[0].EntryReason, res.Trades[0].ExitReason)
	}
	if res.Trades[1].EntryReason != "script add | pyramid separate" {
		t.Errorf("trade[1] EntryReason = %q", res.Trades[1].EntryReason)
	}
	if res.Trades[1].ExitReason != "eod" {
		t.Errorf("trade[1] ExitReason = %q, want eod (leg sopravvive con exit Don55)", res.Trades[1].ExitReason)
	}
	if res.Trades[1].StopPrice != 97 {
		t.Errorf("leg StopPrice = %v, want 97 (stop proprio del segnale)", res.Trades[1].StopPrice)
	}
}

func TestSeparatePyramidRespectsMaxUnits(t *testing.T) {
	cfg := sepCfg(t)
	cfg.Portfolio.MaxOpenRisk = 0.10
	cfg.Portfolio.MaxCorrelatedRisk = 0.10
	bars := flatBars(40, 100, 0.5)
	sepPump(bars, 26, 40, 102, 0.5)
	signals := map[int]strategy.Signal{}
	for b := 25; b <= 33; b++ {
		signals[b] = strategy.Signal{Side: 1, Strength: 1, StopPrice: 98, Reason: "script"}
	}
	strat := &sepStrat{scriptStrategy{cfg: cfg, signals: signals}}
	res := Run(bars, strat, cfg, sepEng())
	if len(res.Trades) != 4 {
		t.Fatalf("attesi 4 trades (1 core + 3 add, max 4 unità), avuti %d", len(res.Trades))
	}
}
