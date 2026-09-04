package strategy

import (
	"math"

	"github.com/atps/atps/internal/config"
	"github.com/atps/atps/internal/data"
)

type VariantC struct{ cfg *config.Config }

func NewC(cfg *config.Config) *VariantC             { return &VariantC{cfg: cfg} }
func (s *VariantC) Name() string                    { return s.cfg.VariantC.Name }
func (s *VariantC) Variant() string                 { return "C" }
func (s *VariantC) Warmup() int                     { return 200 }
func (s *VariantC) Prepare(bars data.Bars) *Context { return PrepareCommon(bars, s.cfg, "C") }

func (s *VariantC) Next(ctx *Context, i int) Signal {
	if i < s.Warmup() || i == 0 {
		return Signal{Side: 0, Reason: "warmup"}
	}
	c := s.cfg.VariantC
	closePx := ctx.Close[i]
	prevClose := ctx.Close[i-1]
	atr := ctx.ATR[i]
	adx := ctx.ADX[i]
	ema50 := ctx.EMA50[i]
	ema200 := ctx.EMA200[i]
	if math.IsNaN(atr) || atr == 0 {
		return Signal{Side: 0}
	}
	if !math.IsNaN(adx) && adx < c.ADXThreshold {
		return Signal{Side: 0, Reason: "C adx filter"}
	}
	trendLong := true
	trendShort := true
	if !math.IsNaN(ema50) && !math.IsNaN(ema200) {
		trendLong = ema50 > ema200 && closePx > ema200
		trendShort = ema50 < ema200 && closePx < ema200
	}
	fundingZ := ctx.FundingZ[i] // funding solo costo, non veto
	// Volume confirmation
	vol := ctx.Volume[i]
	volSMA := ctx.VolumeSMA[i]
	if !volConfirm(vol, volSMA, c.VolumeMult) {
		return Signal{Side: 0, Reason: "C volume veto"}
	}
	// OI confirmation: for long need OI up > threshold, for short OI up as well (trend support). Compute delta
	var oiDelta float64
	if i > 0 && ctx.OI[i-1] != 0 && !math.IsNaN(ctx.OI[i]) && !math.IsNaN(ctx.OI[i-1]) {
		oiDelta = (ctx.OI[i] - ctx.OI[i-1]) / ctx.OI[i-1]
	}
	// open_interest.filter=false disattiva TUTTI i veto OI (funding/OI solo costo/informazione)
	oiFilterOn := s.cfg.OpenInterest.Filter
	// For C we require oiDelta > threshold for either direction (confirm participation) else veto
	hasOIData := ctx.OI[i] != 0 && !math.IsNaN(ctx.OI[i])
	oiOk := !oiFilterOn || !hasOIData || oiDelta >= -c.OIDeltaThreshold // allow slight drop but not big drop
	if !oiOk {
		return Signal{Side: 0, Reason: "C OI veto"}
	}
	hh20Prev := ctx.Don20H[i-1]
	ll20Prev := ctx.Don20L[i-1]
	if trendLong && !math.IsNaN(hh20Prev) && closePx > hh20Prev && prevClose <= hh20Prev {
		if oiFilterOn && hasOIData && oiDelta < c.OIDeltaThreshold {
			// weak OI, reduce strength but still allow? For C require
			return Signal{Side: 0, Reason: "C OI weak long"}
		}
		stop := closePx - c.ATRStopMult*atr
		return Signal{Side: 1, Strength: 1, StopPrice: stop, Reason: "C HH20 long OI+vol ok", Meta: map[string]float64{"adx": adx, "fundingZ": fundingZ, "oiDelta": oiDelta}}
	}
	if trendShort && !math.IsNaN(ll20Prev) && closePx < ll20Prev && prevClose >= ll20Prev {
		if oiFilterOn && hasOIData && oiDelta < c.OIDeltaThreshold {
			return Signal{Side: 0, Reason: "C OI weak short"}
		}
		stop := closePx + c.ATRStopMult*atr
		return Signal{Side: -1, Strength: 1, StopPrice: stop, Reason: "C LL20 short OI+vol ok"}
	}
	return Signal{Side: 0, Reason: "C no signal"}
}
