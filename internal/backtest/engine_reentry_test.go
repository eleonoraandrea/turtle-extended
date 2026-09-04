package backtest

import (
	"testing"
	"time"

	"github.com/atps/atps/internal/config"
	"github.com/atps/atps/internal/data"
	"github.com/atps/atps/internal/strategy"
)

// reentryStrat — entry long scriptata alla bar 2 (stop 95); ReEntry long quando
// last.ExitBarIdx == reentryAfterBar. Deterministico per test engine.
type reentryStrat struct {
	scriptStrategy
	reentryAfterBar int
}

func (s *reentryStrat) ReEntry(_ *strategy.Context, _ int, last strategy.StopOutInfo) strategy.Signal {
	if last.ExitBarIdx == s.reentryAfterBar {
		return strategy.Signal{Side: 1, Strength: 1, StopPrice: 95, Reason: "script reentry long"}
	}
	return strategy.Signal{Side: 0}
}

func TestEngineReEntryAfterStopOut(t *testing.T) {
	cfg, err := config.Load("../../configs/default.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Profit.Satellite.Enabled = false // test a posizione singola
	// 40 barre: segnale long alla bar 2 (stop 95, fill open bar 3 = 100);
	// discesa alla bar 5 (low 93 → stop colpito alla bar 5, exitBarIdx=5):
	// l'engine processa l'exit PRIMA del signal nella stessa barra → ReEntry
	// scatta alla bar 5 e fila alla open della bar 6. Il recovery dalla bar 6
	// è IN SALITA (lows crescenti): un recovery piatto ratcheterebbe la trail
	// donchian fino ai lows stessi → self-stop. Con lows crescenti lo stop
	// ratchetato (donL = min low 20 barre) resta sempre sotto il low corrente
	// → re-entry sopravvive fino a EOD.
	bars := flatBars(40, 100, 0.5)
	bars[5] = data.Bar{Time: time.Unix(5*14400, 0), Open: 97, High: 97.5, Low: 93, Close: 94, Volume: 100}
	for i := 6; i < 40; i++ {
		c := 96 + float64(i-6)*0.35
		bars[i] = data.Bar{Time: time.Unix(int64(i)*14400, 0), Open: c - 0.1, High: c + 0.4, Low: c - 0.6, Close: c, Volume: 100}
	}
	signals := map[int]strategy.Signal{2: {Side: 1, Strength: 1, StopPrice: 95, Reason: "script long"}}
	strat := &reentryStrat{scriptStrategy{cfg: cfg, signals: signals}, 5} // re-entry dopo stop alla bar 5
	eng := EngineConfig{
		Variant: "A", Symbol: "TEST", InitialCapital: 10000,
		FeeBps: 4, SlippageBps: 2, UseNextOpen: true,
		PyramidingMax: 0, TrailMode: "donchian", DonExit: 20, EntryMode: "close",
	}
	res := Run(bars, strat, cfg, eng)
	if len(res.Trades) != 2 {
		t.Fatalf("attesi 2 trades (entry→stop, re-entry→eod), avuti %d", len(res.Trades))
	}
	if res.Trades[0].ExitReason != "stop" {
		t.Errorf("trade[0].ExitReason = %q, want stop", res.Trades[0].ExitReason)
	}
	if res.Trades[1].EntryReason != "script reentry long" {
		t.Errorf("trade[1].EntryReason = %q, want 'script reentry long'", res.Trades[1].EntryReason)
	}
	if res.Trades[1].ExitReason != "eod" {
		t.Errorf("trade[1].ExitReason = %q, want eod", res.Trades[1].ExitReason)
	}
}
