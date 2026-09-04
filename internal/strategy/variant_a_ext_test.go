package strategy

import (
	"math"
	"testing"

	"github.com/atps/atps/internal/config"
)

func ctxFor(n int) *Context {
	ctx := &Context{
		Close:  make([]float64, n),
		High:   make([]float64, n),
		Low:    make([]float64, n),
		ATR:    make([]float64, n),
		SMA200: make([]float64, n),
	}
	for i := 0; i < n; i++ {
		ctx.Close[i] = 100
		ctx.High[i] = 101
		ctx.Low[i] = 99
		ctx.ATR[i] = 2
		ctx.SMA200[i] = 90
	}
	return ctx
}

func testCfgA(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.Load("../../configs/default.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestVariantAIntrabarEntry(t *testing.T) {
	cfg := testCfgA(t)
	s := NewA(cfg)
	n := 250
	ctx := ctxFor(n)
	ctx.Don20H = make([]float64, n)
	ctx.Don20L = make([]float64, n)
	for i := 0; i < n; i++ {
		ctx.Don20H[i] = 102
		ctx.Don20L[i] = 98
	}
	// trend up: close 100 > sma 90 → long abilitato, short no
	lv := s.IntrabarEntry(ctx, n-1)
	if !lv.Enabled {
		t.Fatalf("Enabled = false, want true")
	}
	if lv.LongLevel != 102 {
		t.Errorf("LongLevel = %v, want 102 (Don20H[i-1])", lv.LongLevel)
	}
	if !math.IsNaN(lv.ShortLevel) {
		t.Errorf("ShortLevel deve essere NaN (filtro SMA: close>sma)")
	}
	if lv.LongStopATR != cfg.VariantA.ATRStopMult {
		t.Errorf("LongStopATR = %v, want %v", lv.LongStopATR, cfg.VariantA.ATRStopMult)
	}
	// trend down: close < sma → short abilitato
	for i := 0; i < n; i++ {
		ctx.SMA200[i] = 110
	}
	lv = s.IntrabarEntry(ctx, n-1)
	if !math.IsNaN(lv.LongLevel) || lv.ShortLevel != 98 {
		t.Errorf("trend down: Long NaN e Short 98 attesi, avuti %v/%v", lv.LongLevel, lv.ShortLevel)
	}
	// warmup
	if s.IntrabarEntry(ctx, 100).Enabled {
		t.Errorf("warmup (< 200): Enabled deve essere false")
	}
	// SMA NaN (non calcolabile): entrambi i lati abilitati (comportamento Next attuale)
	for i := 0; i < n; i++ {
		ctx.SMA200[i] = math.NaN()
	}
	lv = s.IntrabarEntry(ctx, n-1)
	if math.IsNaN(lv.LongLevel) || math.IsNaN(lv.ShortLevel) {
		t.Errorf("SMA NaN: entrambi i lati devono restare abilitati")
	}
}

func reentryCtx(n int) *Context {
	ctx := ctxFor(n)
	ctx.Don20H = make([]float64, n)
	ctx.Don20L = make([]float64, n)
	for i := 0; i < n; i++ {
		ctx.Don20H[i] = 102
		ctx.Don20L[i] = 98
	}
	return ctx
}

func TestVariantAReEntry(t *testing.T) {
	cfg := testCfgA(t)
	cfg.VariantA.ReEntry = config.ReEntryCfg{Enabled: true, Lookback: 10, WithinBars: 20}
	s := NewA(cfg)
	n := 250
	ctx := reentryCtx(n)

	// scenario: stop-out long a bar 230; a bar 235 close 100>sma 90 e High[235]=103
	// nuovo high 10-barre (massimo High[225..234] = 101) → re-entry long
	for i := 225; i <= 234; i++ {
		ctx.High[i] = 101
	}
	ctx.High[235] = 103
	ctx.Close[235] = 102.5
	sig := s.ReEntry(ctx, 235, StopOutInfo{Side: 1, ExitBarIdx: 230})
	if sig.Side != 1 {
		t.Fatalf("atteso re-entry long, avuto side %d (%s)", sig.Side, sig.Reason)
	}
	wantStop := 102.5 - cfg.VariantA.ATRStopMult*2.0
	if math.Abs(sig.StopPrice-wantStop) > 1e-9 {
		t.Errorf("StopPrice = %v, want %v", sig.StopPrice, wantStop)
	}

	// finestra scaduta: stop-out a 210, i=235 → 25 > within 20 → niente
	if s2 := s.ReEntry(ctx, 235, StopOutInfo{Side: 1, ExitBarIdx: 210}); s2.Side != 0 {
		t.Errorf("finestra scaduta: side %d, want 0", s2.Side)
	}

	// trend contrario: sma sopra il close → niente re-entry long
	for i := 0; i < n; i++ {
		ctx.SMA200[i] = 110
	}
	if s3 := s.ReEntry(ctx, 235, StopOutInfo{Side: 1, ExitBarIdx: 230}); s3.Side != 0 {
		t.Errorf("trend contrario: side %d, want 0", s3.Side)
	}

	// disabled
	cfg.VariantA.ReEntry.Enabled = false
	if s4 := s.ReEntry(ctx, 235, StopOutInfo{Side: 1, ExitBarIdx: 230}); s4.Side != 0 {
		t.Errorf("disabled: side %d, want 0", s4.Side)
	}
}
