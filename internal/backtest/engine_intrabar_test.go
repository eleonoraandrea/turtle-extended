package backtest

import (
	"math"
	"testing"
	"time"

	"github.com/atps/atps/internal/config"
	"github.com/atps/atps/internal/data"
	"github.com/atps/atps/internal/strategy"
)

// intrabarStrat — Next sempre flat; IntrabarEntry livelli fissi. Deterministico.
type intrabarStrat struct {
	scriptStrategy
	levels strategy.IntrabarEntryLevels
}

func (s *intrabarStrat) IntrabarEntry(_ *strategy.Context, _ int) strategy.IntrabarEntryLevels {
	return s.levels
}

// bar a prezzo p con wiggle ±w
func mkBar(t time.Time, p, w float64) data.Bar {
	return data.Bar{Time: t, Open: p, High: p + w, Low: p - w, Close: p, Volume: 100}
}

func flatBars(n int, p, w float64) data.Bars {
	out := make(data.Bars, n)
	for i := range out {
		out[i] = mkBar(time.Unix(int64(i)*14400, 0), p, w)
	}
	return out
}

func intrabarEng(cfg *config.Config, bars data.Bars, strat strategy.Strategy) *Result {
	cfg.Profit.Satellite.Enabled = false // test a posizione singola: niente split core/sat
	eng := EngineConfig{
		Variant: "A", Symbol: "TEST", InitialCapital: 10000,
		FeeBps: 4, SlippageBps: 2, UseNextOpen: true,
		PyramidingMax: 0, TrailMode: "donchian", DonExit: 20, EntryMode: "intrabar",
	}
	return Run(bars, strat, cfg, eng)
}

func TestIntrabarFillAtLevel(t *testing.T) {
	cfg, err := config.Load("../../configs/default.yaml")
	if err != nil {
		t.Fatal(err)
	}
	// wiggle 0.4: le barre piatte non toccano il livello 100.5 (High 100.4) —
	// l'entry avviene SOLO sulla breakout bar (ATR warmup 20 < 30 barre piatte)
	bars := flatBars(30, 100, 0.4)
	// breakout bar: High attraversa il livello 100.5, Low non tocca lo stop
	// (ATR_prev su barre wiggle 0.4 = 0.8 → stop = 100.52 − 2×0.8 = 98.92 < Low 99.9)
	bars = append(bars, data.Bar{Time: time.Unix(30*14400, 0), Open: 100, High: 105, Low: 99.9, Close: 104, Volume: 100})
	bars = append(bars, flatBars(10, 104, 0.5)...)
	strat := &intrabarStrat{scriptStrategy{cfg: cfg}, strategy.IntrabarEntryLevels{
		Enabled: true, LongLevel: 100.5, LongStopATR: 2, ShortLevel: math.NaN(), ShortStopATR: 2,
	}}
	res := intrabarEng(cfg, bars, strat)
	// posizione aperta → chiusa a EOD: 1 trade, fill verificato al livello + slippage
	if len(res.Trades) != 1 {
		t.Fatalf("atteso 1 trade (eod), avuti %d", len(res.Trades))
	}
	tr := res.Trades[0]
	if tr.ExitReason != "eod" {
		t.Errorf("ExitReason = %q, want eod", tr.ExitReason)
	}
	if tr.EntryReason != "intrabar breakout" {
		t.Errorf("EntryReason = %q, want 'intrabar breakout'", tr.EntryReason)
	}
	wantFill := 100.5 * (1 + 2.0/10000.0) // livello + 2bps slippage
	if math.Abs(tr.EntryPrice-wantFill) > 1e-6 {
		t.Errorf("EntryPrice = %v, want %v (fill al livello + slip)", tr.EntryPrice, wantFill)
	}
}

func TestIntrabarGapOpenFillsAtOpen(t *testing.T) {
	cfg, err := config.Load("../../configs/default.yaml")
	if err != nil {
		t.Fatal(err)
	}
	bars := flatBars(30, 100, 0.4)
	// gap: open già sopra il livello → fill alla open (101), non al livello
	bars = append(bars, data.Bar{Time: time.Unix(30*14400, 0), Open: 101, High: 106, Low: 100.8, Close: 105, Volume: 100})
	bars = append(bars, flatBars(10, 105, 0.5)...)
	strat := &intrabarStrat{scriptStrategy{cfg: cfg}, strategy.IntrabarEntryLevels{
		Enabled: true, LongLevel: 100.5, LongStopATR: 2, ShortLevel: math.NaN(), ShortStopATR: 2,
	}}
	res := intrabarEng(cfg, bars, strat)
	if len(res.Trades) != 1 {
		t.Fatalf("atteso 1 trade (eod): avuti %d", len(res.Trades))
	}
	// gap: open 101 > livello → fill alla open + slippage, non al livello
	wantFill := 101.0 * (1 + 2.0/10000.0)
	if math.Abs(res.Trades[0].EntryPrice-wantFill) > 1e-6 {
		t.Errorf("EntryPrice = %v, want %v (fill alla open per gap + slip)", res.Trades[0].EntryPrice, wantFill)
	}
}

func TestIntrabarSameBarStopPessimistic(t *testing.T) {
	cfg, err := config.Load("../../configs/default.yaml")
	if err != nil {
		t.Fatal(err)
	}
	bars := flatBars(30, 100, 0.4)
	// ATR_prev = 0.8 (wiggle 0.4) → stop = 100.52 − 2×0.8 = 98.92; Low 98 ≤ stop → pessimistico
	bars = append(bars, data.Bar{Time: time.Unix(30*14400, 0), Open: 100, High: 105, Low: 98, Close: 99, Volume: 100})
	bars = append(bars, flatBars(10, 99, 0.1)...)
	strat := &intrabarStrat{scriptStrategy{cfg: cfg}, strategy.IntrabarEntryLevels{
		Enabled: true, LongLevel: 100.5, LongStopATR: 2, ShortLevel: math.NaN(), ShortStopATR: 2,
	}}
	res := intrabarEng(cfg, bars, strat)
	if len(res.Trades) != 1 {
		t.Fatalf("atteso 1 trade (fill→stop stessa barra, pessimistico), avuti %d", len(res.Trades))
	}
	tr := res.Trades[0]
	if tr.ExitReason != "stop_same_bar" {
		t.Errorf("ExitReason = %q, want stop_same_bar", tr.ExitReason)
	}
	if tr.BarsHeld != 0 {
		t.Errorf("BarsHeld = %d, want 0", tr.BarsHeld)
	}
	if tr.PnLNet >= 0 {
		t.Errorf("same-bar stop deve essere una perdita netta, avuto %.2f", tr.PnLNet)
	}
	if tr.RMultiple > 0 {
		t.Errorf("R-multiple deve essere negativo (≤ −1R circa), avuto %.2f", tr.RMultiple)
	}
}

func TestIntrabarBothSidesHitNoEntry(t *testing.T) {
	cfg, err := config.Load("../../configs/default.yaml")
	if err != nil {
		t.Fatal(err)
	}
	bars := flatBars(30, 100, 0.4)
	// huge range bar: tocca sia livello long che short → path inconoscibile → niente entry
	bars = append(bars, data.Bar{Time: time.Unix(30*14400, 0), Open: 100, High: 110, Low: 90, Close: 100, Volume: 100})
	bars = append(bars, flatBars(10, 100, 0.5)...)
	strat := &intrabarStrat{scriptStrategy{cfg: cfg}, strategy.IntrabarEntryLevels{
		Enabled: true, LongLevel: 100.5, LongStopATR: 2, ShortLevel: 99.5, ShortStopATR: 2,
	}}
	res := intrabarEng(cfg, bars, strat)
	if len(res.Trades) != 0 || res.MaxLeverageUsed > 0 {
		t.Errorf("both-sides-hit: attesa nessuna entry, trades %d lev %.3f", len(res.Trades), res.MaxLeverageUsed)
	}
}

// reentryProbeStrat — registra l'ultima StopOutInfo ricevuta da ReEntry (probe)
type reentryProbeStrat struct {
	scriptStrategy
	levels  strategy.IntrabarEntryLevels
	gotLast strategy.StopOutInfo
	called  bool
}

func (s *reentryProbeStrat) IntrabarEntry(_ *strategy.Context, _ int) strategy.IntrabarEntryLevels {
	return s.levels
}

func (s *reentryProbeStrat) ReEntry(_ *strategy.Context, _ int, last strategy.StopOutInfo) strategy.Signal {
	s.called = true
	s.gotLast = last
	return strategy.Signal{Side: 0}
}

func TestIntrabarSameBarStopSetsLastStop(t *testing.T) {
	cfg, err := config.Load("../../configs/default.yaml")
	if err != nil {
		t.Fatal(err)
	}
	bars := flatBars(30, 100, 0.4)
	// stop = 100.52 − 2×0.8 = 98.92; Low 98 ≤ stop → same-bar stop
	bars = append(bars, data.Bar{Time: time.Unix(30*14400, 0), Open: 100, High: 105, Low: 98, Close: 99, Volume: 100})
	bars = append(bars, flatBars(10, 99, 0.1)...)
	strat := &reentryProbeStrat{scriptStrategy: scriptStrategy{cfg: cfg}, levels: strategy.IntrabarEntryLevels{
		Enabled: true, LongLevel: 100.5, LongStopATR: 2, ShortLevel: math.NaN(), ShortStopATR: 2,
	}}
	res := intrabarEng(cfg, bars, strat)
	if len(res.Trades) != 1 || res.Trades[0].ExitReason != "stop_same_bar" {
		t.Fatalf("precondizione: atteso 1 trade stop_same_bar, avuti %+v", res.Trades)
	}
	if !strat.called {
		t.Fatal("ReEntry mai chiamata dopo stop_same_bar — lastStop non settato")
	}
	if strat.gotLast.Side != 1 || strat.gotLast.ExitBarIdx != 30 {
		t.Errorf("lastStop = %+v, want Side 1 ExitBarIdx 30", strat.gotLast)
	}
}

func TestIntrabarShortFillAndSlippage(t *testing.T) {
	cfg, err := config.Load("../../configs/default.yaml")
	if err != nil {
		t.Fatal(err)
	}
	bars := flatBars(30, 100, 0.4)
	// breakdown bar: Low attraversa livello short 99.5 → fill al livello − slippage
	bars = append(bars, data.Bar{Time: time.Unix(30*14400, 0), Open: 100, High: 100.4, Low: 95, Close: 96, Volume: 100})
	bars = append(bars, flatBars(10, 96, 0.3)...)
	strat := &intrabarStrat{scriptStrategy{cfg: cfg}, strategy.IntrabarEntryLevels{
		Enabled: true, LongLevel: math.NaN(), LongStopATR: 2, ShortLevel: 99.5, ShortStopATR: 2,
	}}
	res := intrabarEng(cfg, bars, strat)
	// stop short = fill + 2×0.8 = 99.496 + 1.6 = 101.1; High della breakdown bar 100.4 < stop → sopravvive
	if len(res.Trades) != 1 {
		t.Fatalf("atteso 1 trade (eod), avuti %d", len(res.Trades))
	}
	tr := res.Trades[0]
	if tr.Side != -1 {
		t.Errorf("Side = %d, want -1", tr.Side)
	}
	wantFill := 99.5 * (1 - 2.0/10000.0) // livello − 2bps slippage per short
	if math.Abs(tr.EntryPrice-wantFill) > 1e-6 {
		t.Errorf("EntryPrice = %v, want %v (short: fill al livello − slip)", tr.EntryPrice, wantFill)
	}
	if tr.PnLNet <= 0 {
		t.Errorf("short profittevole atteso (entry 99.5, eod 96), avuto %.2f", tr.PnLNet)
	}
}

func TestIntrabarDisabledByDefault(t *testing.T) {
	cfg, err := config.Load("../../configs/default.yaml")
	if err != nil {
		t.Fatal(err)
	}
	bars := flatBars(30, 100, 0.5)
	bars = append(bars, data.Bar{Time: time.Unix(30*14400, 0), Open: 100, High: 105, Low: 99.9, Close: 104, Volume: 100})
	strat := &intrabarStrat{scriptStrategy{cfg: cfg}, strategy.IntrabarEntryLevels{
		Enabled: true, LongLevel: 100.5, LongStopATR: 2, ShortLevel: math.NaN(), ShortStopATR: 2,
	}}
	eng := EngineConfig{
		Variant: "A", Symbol: "TEST", InitialCapital: 10000,
		FeeBps: 4, SlippageBps: 2, UseNextOpen: true,
		PyramidingMax: 0, TrailMode: "donchian", DonExit: 20, // EntryMode "" → close
	}
	res := Run(bars, strat, cfg, eng)
	if len(res.Trades) != 0 || res.MaxLeverageUsed > 0 {
		t.Errorf("entry_mode close default: strategy Next è flat → nessuna entry, avute trades %d", len(res.Trades))
	}
}
