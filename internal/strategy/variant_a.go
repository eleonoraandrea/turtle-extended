package strategy

import (
	"math"

	"github.com/atps/atps/internal/config"
	"github.com/atps/atps/internal/data"
)

type VariantA struct { cfg *config.Config }
func NewA(cfg *config.Config) *VariantA { return &VariantA{cfg:cfg} }
func (s *VariantA) Name() string { return s.cfg.VariantA.Name }
func (s *VariantA) Variant() string { return "A" }
func (s *VariantA) Warmup() int { return 200 }
func (s *VariantA) Prepare(bars data.Bars) *Context { return PrepareCommon(bars, s.cfg, "A")}

// Classic Turtle: HH20 breakout + SMA200 filter + 2ATR stop + LL10 exit handled by engine.
func (s *VariantA) Next(ctx *Context, i int) Signal {
	if i < s.Warmup() || i==0 { return Signal{Side:0, Reason:"warmup"} }
	c:=s.cfg.VariantA
	closePx:=ctx.Close[i]
	prevClose:=ctx.Close[i-1]
	atr:=ctx.ATR[i]
	if math.IsNaN(atr) || atr==0 { return Signal{Side:0} }
	// SMA filter
	if c.SMAFilter>0 && !math.IsNaN(ctx.SMA200[i]) {
		// need trend filter only for entry direction? Use SMA200 as trend
	}
	// Donchian breakout: close > HH20[-1] (prior bar's channel)
	hh20Prev:=ctx.Don20H[i-1]
	ll20Prev:=ctx.Don20L[i-1]
	hh55Prev:=ctx.Don55H[i-1]
	ll55Prev:=ctx.Don55L[i-1]
	sma200:=ctx.SMA200[i]

	// Long entry: breakout 20 or 55 if close> sma200
	if !math.IsNaN(hh20Prev) && closePx > hh20Prev && prevClose <= hh20Prev {
		if math.IsNaN(sma200) || closePx > sma200 {
			stop:= closePx - c.ATRStopMult*atr
			return Signal{Side:1, Strength:1, StopPrice: stop, Reason:"A HH20 long", Meta: map[string]float64{"atr":atr, "hh":hh20Prev}}
		}
	}
	if !math.IsNaN(hh55Prev) && closePx > hh55Prev && prevClose <= hh55Prev {
		if math.IsNaN(sma200) || closePx > sma200 {
			stop:= closePx - c.ATRStopMult*atr
			return Signal{Side:1, Strength:1, StopPrice: stop, Reason:"A HH55 long"}
		}
	}
	// Short entry: mirror
	if !math.IsNaN(ll20Prev) && closePx < ll20Prev && prevClose >= ll20Prev {
		if math.IsNaN(sma200) || closePx < sma200 {
			stop:= closePx + c.ATRStopMult*atr
			return Signal{Side:-1, Strength:1, StopPrice: stop, Reason:"A LL20 short"}
		}
	}
	if !math.IsNaN(ll55Prev) && closePx < ll55Prev && prevClose >= ll55Prev {
		if math.IsNaN(sma200) || closePx < sma200 {
			stop:= closePx + c.ATRStopMult*atr
			return Signal{Side:-1, Strength:1, StopPrice: stop, Reason:"A LL55 short"}
		}
	}
	// Exit signal detection: if price crosses opposite Donchian; engine also handles trailing but we emit flat signal
	// For simplicity, engine will close on LL10; we provide exit hint if needed
	if !math.IsNaN(ctx.Don20L[i-1]) {
		// engine handles stop; but signal 0 keeps position
	}
	return Signal{Side:0, Reason:"no breakout"}
}
