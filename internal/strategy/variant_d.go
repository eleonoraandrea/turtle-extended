package strategy

import (
	"math"

	"github.com/atps/atps/internal/config"
	"github.com/atps/atps/internal/data"
)

type VariantD struct{ cfg *config.Config }

func NewD(cfg *config.Config) *VariantD             { return &VariantD{cfg: cfg} }
func (s *VariantD) Name() string                    { return s.cfg.VariantD.Name }
func (s *VariantD) Variant() string                 { return "D" }
func (s *VariantD) Warmup() int                     { return 200 }
func (s *VariantD) Prepare(bars data.Bars) *Context { return PrepareCommon(bars, s.cfg, "D") }

func (s *VariantD) Next(ctx *Context, i int) Signal {
	if i < s.Warmup() || i == 0 {
		return Signal{Side: 0, Reason: "warmup"}
	}
	c := s.cfg.VariantD
	closePx := ctx.Close[i]
	prevClose := ctx.Close[i-1]
	atr := ctx.ATR[i]
	adx := ctx.ADX[i]
	volReg := ctx.VolRegime[i]
	if math.IsNaN(atr) || atr == 0 {
		return Signal{Side: 0}
	}
	if !math.IsNaN(adx) && adx < c.ADXThreshold {
		// allow only if vol regime high and breakout big?
		return Signal{Side: 0, Reason: "D adx filter"}
	}
	// crash brake: veto entries on the crash bar itself and for the cooldown
	// window (6 bars) after any bar whose return exceeded the threshold
	if c.UseCrashBrake && i >= 2 {
		for k := i - 5; k <= i; k++ {
			if k < 1 {
				continue
			}
			r2 := (ctx.Close[k] - ctx.Close[k-1]) / ctx.Close[k-1] * 100
			if math.Abs(r2) >= s.cfg.Portfolio.CrashBrakeDropPct {
				return Signal{Side: 0, Reason: "D crash brake"}
			}
		}
	}
	ema50 := ctx.EMA50[i]
	ema200 := ctx.EMA200[i]
	trendLong := true
	trendShort := true
	if !math.IsNaN(ema50) && !math.IsNaN(ema200) {
		trendLong = ema50 > ema200 && closePx > ema200
		trendShort = ema50 < ema200 && closePx < ema200
	}
	// FUNDING RIMOSSO su richiesta utente — non blocca più l'entry (solo costo in backtest)
	// fundingZ:=ctx.FundingZ[i] // ora solo info per report, non veto
	vol := ctx.Volume[i]
	volSMA := ctx.VolumeSMA[i]
	if !volConfirm(vol, volSMA, c.VolumeMult) {
		return Signal{Side: 0, Reason: "D vol veto"}
	}
	var oiDelta float64
	hasOI := ctx.OI[i] != 0 && !math.IsNaN(ctx.OI[i]) && !math.IsNaN(ctx.OI[i-1]) && ctx.OI[i-1] != 0
	if hasOI {
		oiDelta = (ctx.OI[i] - ctx.OI[i-1]) / ctx.OI[i-1]
	}
	// open_interest.filter=false disattiva i veto OI (resta solo informativo)
	oiFilterOn := s.cfg.OpenInterest.Filter
	fundingZ := ctx.FundingZ[i] // solo per Meta, non per veto
	// adaptive channel selection based on volRegime
	var hhPrev, llPrev float64
	var channel string
	if !math.IsNaN(volReg) && c.AdaptiveChannel {
		if volReg < 30 {
			// low vol -> tighter 20
			hhPrev = ctx.Don20H[i-1]
			llPrev = ctx.Don20L[i-1]
			channel = "20"
		} else if volReg < 70 {
			hhPrev = ctx.Don55H[i-1]
			llPrev = ctx.Don55L[i-1]
			channel = "55"
		} else {
			hhPrev = ctx.Don100H[i-1]
			llPrev = ctx.Don100L[i-1]
			channel = "100"
		}
	} else {
		hhPrev = ctx.Don20H[i-1]
		llPrev = ctx.Don20L[i-1]
		channel = "20"
	}
	// also consider alternative confirmations depending on variant: if primary not break but secondary big breakout? Keep simple: only one adaptive
	// ATR stop adaptive BIG IMPROVE: tighter in high vol (era 3.0 → 2.5)
	atrMult := c.ATRStopMult // base 1.8
	if !math.IsNaN(volReg) {
		if volReg > 80 {
			atrMult = 2.5
		} else if volReg > 60 {
			atrMult = 2.0
		} else if volReg < 20 {
			atrMult = 1.5
		}
	}
	// long — funding non blocca più (solo costo)
	if trendLong && !math.IsNaN(hhPrev) && closePx > hhPrev && prevClose <= hhPrev {
		if oiFilterOn && hasOI && oiDelta < c.OIDeltaThreshold {
			return Signal{Side: 0, Reason: "D OI weak long"}
		}
		stop := closePx - atrMult*atr
		return Signal{Side: 1, Strength: 1, StopPrice: stop, Reason: "D " + channel + " long adaptive", Meta: map[string]float64{"atrMult": atrMult, "adx": adx, "volReg": volReg, "oiDelta": oiDelta, "fundingZ": fundingZ}}
	}
	if trendShort && !math.IsNaN(llPrev) && closePx < llPrev && prevClose >= llPrev {
		if oiFilterOn && hasOI && oiDelta < c.OIDeltaThreshold {
			return Signal{Side: 0, Reason: "D OI weak short"}
		}
		stop := closePx + atrMult*atr
		return Signal{Side: -1, Strength: 1, StopPrice: stop, Reason: "D " + channel + " short adaptive", Meta: map[string]float64{"atrMult": atrMult, "adx": adx, "volReg": volReg}}
	}
	return Signal{Side: 0, Reason: "D no signal"}
}

// Trail logic helper for engine: compute chandelier or donchian trailing stop.
// The chandelier is computed on the fly so the caller's atrMult is honored:
// long = highestHigh(22) − mult×ATR, short = lowestLow(22) + mult×ATR.
func TrailStop(ctx *Context, i int, side int, atrMult float64, mode string) float64 {
	if i < 1 {
		return math.NaN()
	}
	if atrMult <= 0 {
		atrMult = 3.0
	}
	if mode == "chandelier" {
		atr := ctx.ATR[i]
		if !math.IsNaN(atr) && atr > 0 {
			const lookback = 22
			lo := i - lookback + 1
			if lo < 0 {
				lo = 0
			}
			if side == 1 {
				hh := math.Inf(-1)
				for k := lo; k <= i; k++ {
					if ctx.High[k] > hh {
						hh = ctx.High[k]
					}
				}
				return hh - atrMult*atr
			}
			if side == -1 {
				ll := math.Inf(1)
				for k := lo; k <= i; k++ {
					if ctx.Low[k] < ll {
						ll = ctx.Low[k]
					}
				}
				return ll + atrMult*atr
			}
		}
		// ATR unavailable → fall through to the donchian fallback below
	}
	if side == 1 {
		// donchian low fallback
		if !math.IsNaN(ctx.Don20L[i]) {
			return ctx.Don20L[i]
		}
	}
	if side == -1 {
		if !math.IsNaN(ctx.Don20H[i]) {
			return ctx.Don20H[i]
		}
	}
	return math.NaN()
}
