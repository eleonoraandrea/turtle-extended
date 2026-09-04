package strategy

import (
	"math"

	"github.com/atps/atps/internal/config"
	"github.com/atps/atps/internal/data"
)

type VariantA struct{ cfg *config.Config }

func NewA(cfg *config.Config) *VariantA             { return &VariantA{cfg: cfg} }
func (s *VariantA) Name() string                    { return s.cfg.VariantA.Name }
func (s *VariantA) Variant() string                 { return "A" }
func (s *VariantA) Warmup() int                     { return 200 }
func (s *VariantA) Prepare(bars data.Bars) *Context { return PrepareCommon(bars, s.cfg, "A") }

// Classic Turtle: HH20 breakout + SMA200 filter + 2ATR stop + LL10 exit handled by engine.
func (s *VariantA) Next(ctx *Context, i int) Signal {
	if i < s.Warmup() || i == 0 {
		return Signal{Side: 0, Reason: "warmup"}
	}
	c := s.cfg.VariantA
	closePx := ctx.Close[i]
	prevClose := ctx.Close[i-1]
	atr := ctx.ATR[i]
	if math.IsNaN(atr) || atr == 0 {
		return Signal{Side: 0}
	}
	// Donchian breakout: close > HH20[-1] (prior bar's channel)
	hh20Prev := ctx.Don20H[i-1]
	ll20Prev := ctx.Don20L[i-1]
	hh55Prev := ctx.Don55H[i-1]
	ll55Prev := ctx.Don55L[i-1]
	sma200 := ctx.SMA200[i]

	// Long entry: breakout 20 or 55 if close> sma200
	if !math.IsNaN(hh20Prev) && closePx > hh20Prev && prevClose <= hh20Prev {
		if math.IsNaN(sma200) || closePx > sma200 {
			stop := closePx - c.ATRStopMult*atr
			return Signal{Side: 1, Strength: 1, StopPrice: stop, Reason: "A HH20 long", Meta: map[string]float64{"atr": atr, "hh": hh20Prev}}
		}
	}
	if !math.IsNaN(hh55Prev) && closePx > hh55Prev && prevClose <= hh55Prev {
		if math.IsNaN(sma200) || closePx > sma200 {
			stop := closePx - c.ATRStopMult*atr
			return Signal{Side: 1, Strength: 1, StopPrice: stop, Reason: "A HH55 long"}
		}
	}
	// Short entry: mirror
	if !math.IsNaN(ll20Prev) && closePx < ll20Prev && prevClose >= ll20Prev {
		if math.IsNaN(sma200) || closePx < sma200 {
			stop := closePx + c.ATRStopMult*atr
			return Signal{Side: -1, Strength: 1, StopPrice: stop, Reason: "A LL20 short"}
		}
	}
	if !math.IsNaN(ll55Prev) && closePx < ll55Prev && prevClose >= ll55Prev {
		if math.IsNaN(sma200) || closePx < sma200 {
			stop := closePx + c.ATRStopMult*atr
			return Signal{Side: -1, Strength: 1, StopPrice: stop, Reason: "A LL55 short"}
		}
	}
	return Signal{Side: 0, Reason: "no breakout"}
}

// IntrabarEntry — livelli entry stop-order (modalità intrabar engine).
// Livelli da barre < i SOLO (no lookahead); filtro SMA deciso sul close di i-1.
func (s *VariantA) IntrabarEntry(ctx *Context, i int) IntrabarEntryLevels {
	c := s.cfg.VariantA
	l := IntrabarEntryLevels{
		Enabled:      true,
		LongLevel:    math.NaN(),
		ShortLevel:   math.NaN(),
		LongStopATR:  c.ATRStopMult,
		ShortStopATR: c.ATRStopMult,
	}
	if i < s.Warmup() || i < 1 {
		l.Enabled = false
		return l
	}
	hh := ctx.Don20H[i-1]
	ll := ctx.Don20L[i-1]
	sma := ctx.SMA200[i-1]
	if math.IsNaN(hh) || math.IsNaN(ll) || math.IsNaN(ctx.ATR[i-1]) || ctx.ATR[i-1] <= 0 {
		l.Enabled = false
		return l
	}
	// close di i-1 > SMA200 → long; il canale 20 è sempre ≥ close (include la propria barra)
	if math.IsNaN(sma) || ctx.Close[i-1] > sma {
		l.LongLevel = hh
	}
	if math.IsNaN(sma) || ctx.Close[i-1] < sma {
		l.ShortLevel = ll
	}
	return l
}

// ReEntry — dopo stop-out: se il trend filter regge e la barra corrente fa un
// nuovo high/low sulle Lookback barre precedenti, entro WithinBars dallo stop.
func (s *VariantA) ReEntry(ctx *Context, i int, last StopOutInfo) Signal {
	r := s.cfg.VariantA.ReEntry
	zero := Signal{Side: 0, Reason: "no reentry"}
	if !r.Enabled || i < s.Warmup() || last.ExitBarIdx <= 0 {
		return zero
	}
	if i-last.ExitBarIdx > r.WithinBars {
		return zero
	}
	lo := i - r.Lookback
	if lo < 1 {
		lo = 1
	}
	if i-lo < 2 {
		return zero
	}
	atr := ctx.ATR[i]
	if math.IsNaN(atr) || atr <= 0 {
		return zero
	}
	sma := ctx.SMA200[i]
	closePx := ctx.Close[i]
	nh, nl := ctx.High[lo], ctx.Low[lo]
	for j := lo + 1; j < i; j++ {
		if ctx.High[j] > nh {
			nh = ctx.High[j]
		}
		if ctx.Low[j] < nl {
			nl = ctx.Low[j]
		}
	}
	mult := s.cfg.VariantA.ATRStopMult
	if last.Side == 1 && (math.IsNaN(sma) || closePx > sma) && ctx.High[i] > nh {
		return Signal{Side: 1, Strength: 1, StopPrice: closePx - mult*atr, Reason: "A reentry long (nuovo high)"}
	}
	if last.Side == -1 && (math.IsNaN(sma) || closePx < sma) && ctx.Low[i] < nl {
		return Signal{Side: -1, Strength: 1, StopPrice: closePx + mult*atr, Reason: "A reentry short (nuovo low)"}
	}
	return zero
}
