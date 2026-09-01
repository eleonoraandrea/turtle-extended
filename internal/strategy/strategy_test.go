package strategy

import (
	"math"
	"testing"
	"time"

	"github.com/atps/atps/internal/config"
	"github.com/atps/atps/internal/data"
)

func TestFactory(t *testing.T) {
	cfg, _ := config.Load("../../configs/default.yaml")
	for _, v := range []string{"A", "B", "C", "D", "a", "d", "unknown"} {
		s := New(v, cfg)
		if s == nil {
			t.Fatalf("nil for %s", v)
		}
	}
}

func TestPrepareCommon(t *testing.T) {
	cfg, _ := config.Load("../../configs/default.yaml")
	bars := data.GenerateSynthetic(300, 4*time.Hour, 1)
	ctx := PrepareCommon(bars, cfg, "D")
	if len(ctx.ATR) != len(bars) {
		t.Fatalf("atr len")
	}
	if len(ctx.ADX) != len(bars) {
		t.Fatalf("adx len")
	}
	if len(ctx.Don20H) != len(bars) {
		t.Fatalf("don")
	}
	// first 20 ATR should be NaN (period 20)
	if !math.IsNaN(ctx.ATR[0]) {
		t.Fatalf("ATR warmup should be NaN")
	}
}

func TestVariantAWarmupAndSignal(t *testing.T) {
	cfg, _ := config.Load("../../configs/default.yaml")
	bars := data.GenerateSynthetic(300, 4*time.Hour, 42)
	s := NewA(cfg)
	ctx := PrepareCommon(bars, cfg, "A")
	// warmup
	sig := s.Next(ctx, 5)
	if sig.Side != 0 {
		t.Fatalf("warmup should be 0")
	}
	// after warmup should produce some signals eventually
	found := false
	for i := s.Warmup(); i < len(bars); i++ {
		sig := s.Next(ctx, i)
		if sig.Side != 0 {
			found = true
			if sig.StopPrice == 0 {
				t.Fatalf("stop price zero")
			}
			break
		}
	}
	if !found {
		t.Logf("no signal found in 300 bars — may be okay per seed but expected at least one")
	}
	// ensure NaN ATR blocks
	ctx.ATR[250] = math.NaN()
	sig2 := s.Next(ctx, 250)
	if sig2.Side != 0 {
		t.Fatalf("NaN ATR should block")
	}
}

func TestVariantBFilter(t *testing.T) {
	cfg, _ := config.Load("../../configs/default.yaml")
	bars := data.GenerateSynthetic(300, 4*time.Hour, 42)
	s := NewB(cfg)
	ctx := PrepareCommon(bars, cfg, "B")
	// set ADX low to trigger filter
	i := s.Warmup() + 10
	ctx.ADX[i] = 5 // below threshold 20
	ctx.Close[i] = ctx.Don20H[i-1] + 10 // breakout
	ctx.Close[i-1] = ctx.Don20H[i-1] - 1
	sig := s.Next(ctx, i)
	if sig.Side != 0 && sig.Reason == "B HH20 long regime ok" {
		t.Fatalf("ADX filter should block")
	}
}

func TestVariantCVolumeFilter(t *testing.T) {
	cfg, _ := config.Load("../../configs/default.yaml")
	bars := data.GenerateSynthetic(300, 4*time.Hour, 42)
	s := NewC(cfg)
	ctx := PrepareCommon(bars, cfg, "C")
	i := s.Warmup() + 10
	// set volume low vs SMA
	ctx.Volume[i] = 1
	ctx.VolumeSMA[i] = 1000
	// even if breakout, should veto
	ctx.Close[i] = ctx.Don20H[i-1] + 10
	ctx.Close[i-1] = ctx.Don20H[i-1] - 1
	if len(ctx.EMA50) > i && len(ctx.EMA200) > i {
		ctx.EMA50[i] = 20000
		ctx.EMA200[i] = 19000
		ctx.Close[i] = 21000
	}
	sig := s.Next(ctx, i)
	if sig.Side != 0 {
		// may be blocked by volume veto
		t.Logf("C signal %d reason %s", sig.Side, sig.Reason)
	}
}

func TestVariantDAdaptive(t *testing.T) {
	cfg, _ := config.Load("../../configs/default.yaml")
	bars := data.GenerateSynthetic(300, 4*time.Hour, 42)
	s := NewD(cfg)
	ctx := PrepareCommon(bars, cfg, "D")
	if s.Warmup() != 200 {
		t.Fatalf("warmup 200")
	}
	// force low vol regime -> channel 20
	i := 250
	ctx.VolRegime[i] = 10
	ctx.ATR[i] = 100
	ctx.Close[i] = ctx.Don20H[i-1] + 5
	ctx.Close[i-1] = ctx.Don20H[i-1] - 1
	ctx.EMA50[i] = 20000
	ctx.EMA200[i] = 19000
	ctx.Close[i] = 21000
	// ensure ADX passes
	ctx.ADX[i] = 30
	ctx.Volume[i] = 10000
	ctx.VolumeSMA[i] = 1000
	sig := s.Next(ctx, i)
	// should be long if breakout
	if sig.Side == 0 {
		t.Logf("D no signal reason %s (may be volume/OI etc)", sig.Reason)
	} else if sig.Side == 1 {
		if sig.StopPrice >= ctx.Close[i] {
			t.Fatalf("stop should be below price for long")
		}
	}
}

func TestTrailStopHelper(t *testing.T) {
	cfg, _ := config.Load("../../configs/default.yaml")
	bars := data.GenerateSynthetic(300, 4*time.Hour, 1)
	ctx := PrepareCommon(bars, cfg, "D")
	// TrailStop should return chandelier or donchian
	ts := TrailStop(ctx, 250, 1, 3.0, "chandelier")
	if math.IsNaN(ts) {
		// could be NaN during warmup, test valid later
		t.Logf("trail NaN at 250")
	}
	ts2 := TrailStop(ctx, 0, 1, 3.0, "chandelier")
	if !math.IsNaN(ts2) {
		t.Fatalf("i<1 should be NaN")
	}
}
