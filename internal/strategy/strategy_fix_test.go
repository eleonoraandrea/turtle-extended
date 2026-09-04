package strategy

import (
	"math"
	"testing"
	"time"

	"github.com/atps/atps/internal/config"
	"github.com/atps/atps/internal/data"
)

// TrailStop must honor atrMult: chandelier = highestHigh(N) − mult×ATR.
func TestTrailStopHonorsATRMult(t *testing.T) {
	cfg, _ := config.Load("../../configs/default.yaml")
	bars := data.GenerateSynthetic(300, 4*time.Hour, 7)
	ctx := PrepareCommon(bars, cfg, "D")
	i := 250
	atr := ctx.ATR[i]
	if math.IsNaN(atr) || atr <= 0 {
		t.Fatalf("ATR not available at %d", i)
	}
	stop2 := TrailStop(ctx, i, 1, 2.0, "chandelier")
	stop4 := TrailStop(ctx, i, 1, 4.0, "chandelier")
	if math.IsNaN(stop2) || math.IsNaN(stop4) {
		t.Fatalf("stops NaN: %.4f %.4f", stop2, stop4)
	}
	if math.Abs((stop2-stop4)-2.0*atr) > 1e-9 {
		t.Fatalf("TrailStop ignores atrMult: stop(2)=%.4f stop(4)=%.4f diff=%.4f want=%.4f",
			stop2, stop4, stop4-stop2, -2.0*atr)
	}
	// short side symmetric
	stop2s := TrailStop(ctx, i, -1, 2.0, "chandelier")
	stop4s := TrailStop(ctx, i, -1, 4.0, "chandelier")
	if math.IsNaN(stop2s) || math.IsNaN(stop4s) {
		t.Fatalf("short stops NaN")
	}
	if math.Abs((stop4s-stop2s)-2.0*atr) > 1e-9 {
		t.Fatalf("TrailStop short ignores atrMult: diff=%.4f want=%.4f", stop4s-stop2s, 2.0*atr)
	}
}

// Variant D crash brake must veto the crash bar itself and the cooldown window.
func TestVariantDCrashBrakeVetoWindow(t *testing.T) {
	cfg, _ := config.Load("../../configs/default.yaml")
	bars := data.GenerateSynthetic(400, 4*time.Hour, 42)
	s := NewD(cfg)
	ctx := PrepareCommon(bars, cfg, "D")

	// craft a passing breakout context at bar i, then test crash scenarios
	makePassing := func(ctx *Context, i int) {
		px := ctx.Close[i]
		ctx.ADX[i] = 30
		ctx.EMA50[i] = px + 10
		ctx.EMA200[i] = px - 10
		ctx.VolumeSMA[i] = 100
		ctx.Volume[i] = 500 // volConfirm ok
		ctx.OI[i] = 0
		ctx.Don20H[i-1] = px - 1
		ctx.Don20L[i-1] = px + 1
		ctx.Close[i-1] = px - 2 // prevClose <= hhPrev
		ctx.ATR[i] = 1
	}

	// scenario A: the crash IS the current bar (drop 9%) — must veto
	i := 300
	makePassing(ctx, i)
	px := ctx.Close[i]
	ctx.Close[i-1] = px / 0.91 // +? previous close such that current return = -9%
	// keep breakout valid: prevClose <= Don20H[i-1]
	ctx.Don20H[i-1] = ctx.Close[i-1] + 1
	sig := s.Next(ctx, i)
	if sig.Side != 0 {
		t.Fatalf("crash bar must veto entry, got side %d (%s)", sig.Side, sig.Reason)
	}

	// scenario B: crash happened 2 bars ago, current bar calm — still inside cooldown → veto
	ctx2 := PrepareCommon(bars, cfg, "D")
	j := 300
	makePassing(ctx2, j)
	pj := ctx2.Close[j]
	ctx2.Close[j-2] = pj / 1.09 // bar j-2 close such that return from j-3.. wait: crash at j-2 means ret of bar j-2
	// ret of bar j-2 = (close[j-2]-close[j-3])/close[j-3]; set close[j-3] higher
	ctx2.Close[j-3] = ctx2.Close[j-2] / (1 - 0.09)
	sig2 := s.Next(ctx2, j)
	if sig2.Side != 0 {
		t.Fatalf("cooldown bar must veto entry (crash 2 bars ago), got side %d (%s)", sig2.Side, sig2.Reason)
	}
}
