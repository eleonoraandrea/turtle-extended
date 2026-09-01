package strategy

import (
	"math"

	"github.com/atps/atps/internal/config"
	"github.com/atps/atps/internal/data"
)

type VariantB struct{ cfg *config.Config }

func NewB(cfg *config.Config) *VariantB             { return &VariantB{cfg: cfg} }
func (s *VariantB) Name() string                    { return s.cfg.VariantB.Name }
func (s *VariantB) Variant() string                 { return "B" }
func (s *VariantB) Warmup() int                     { return 200 }
func (s *VariantB) Prepare(bars data.Bars) *Context { return PrepareCommon(bars, s.cfg, "B") }

func (s *VariantB) Next(ctx *Context, i int) Signal {
	if i < s.Warmup() || i == 0 {
		return Signal{Side: 0, Reason: "warmup"}
	}
	c := s.cfg.VariantB
	closePx := ctx.Close[i]
	prevClose := ctx.Close[i-1]
	atr := ctx.ATR[i]
	adx := ctx.ADX[i]
	ema50 := ctx.EMA50[i]
	ema200 := ctx.EMA200[i]
	volReg := ctx.VolRegime[i]
	if math.IsNaN(atr) || atr == 0 {
		return Signal{Side: 0}
	}
	if !math.IsNaN(adx) && adx < c.ADXThreshold {
		return Signal{Side: 0, Reason: "B adx filter"}
	}
	// EMA trend filter
	trendLong := true
	trendShort := true
	if !math.IsNaN(ema50) && !math.IsNaN(ema200) {
		trendLong = ema50 > ema200 && closePx > ema200
		trendShort = ema50 < ema200 && closePx < ema200
	}
	_ = volReg // reserved for future vol regime veto (B uses ADX+EMA only by design, no vol block)
	hh20Prev := ctx.Don20H[i-1]
	ll20Prev := ctx.Don20L[i-1]
	hh55Prev := ctx.Don55H[i-1]
	ll55Prev := ctx.Don55L[i-1]

	if trendLong {
		if !math.IsNaN(hh20Prev) && closePx > hh20Prev && prevClose <= hh20Prev {
			stop := closePx - c.ATRStopMult*atr
			return Signal{Side: 1, Strength: 1, StopPrice: stop, Reason: "B HH20 long regime ok", Meta: map[string]float64{"adx": adx, "volReg": volReg}}
		}
		if !math.IsNaN(hh55Prev) && closePx > hh55Prev && prevClose <= hh55Prev {
			stop := closePx - c.ATRStopMult*atr
			return Signal{Side: 1, Strength: 1, StopPrice: stop, Reason: "B HH55 long"}
		}
	}
	if trendShort {
		if !math.IsNaN(ll20Prev) && closePx < ll20Prev && prevClose >= ll20Prev {
			stop := closePx + c.ATRStopMult*atr
			return Signal{Side: -1, Strength: 1, StopPrice: stop, Reason: "B LL20 short regime ok"}
		}
		if !math.IsNaN(ll55Prev) && closePx < ll55Prev && prevClose >= ll55Prev {
			stop := closePx + c.ATRStopMult*atr
			return Signal{Side: -1, Strength: 1, StopPrice: stop, Reason: "B LL55 short"}
		}
	}
	return Signal{Side: 0, Reason: "B no signal"}
}
