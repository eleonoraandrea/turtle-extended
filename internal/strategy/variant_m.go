package strategy

import (
	"math"

	"github.com/atps/atps/internal/config"
	"github.com/atps/atps/internal/data"
)

// VariantM — H1 Mean Reversion: compra i dip sotto la mean in regime rialzista
// (short speculare in ribassista), esce al ritocco della mean (engine exit_mode
// "reversion") o su stop ATR. Complemento del trend-following A: edge decorrelato.
type VariantM struct{ cfg *config.Config }

func NewM(cfg *config.Config) *VariantM             { return &VariantM{cfg: cfg} }
func (s *VariantM) Name() string                    { return s.cfg.VariantM.Name }
func (s *VariantM) Variant() string                 { return "M" }
func (s *VariantM) Warmup() int                     { return VariantWarmup(s.cfg, "M") }
func (s *VariantM) Prepare(bars data.Bars) *Context { return PrepareCommon(bars, s.cfg, "M") }

func (s *VariantM) Next(ctx *Context, i int) Signal {
	if i < s.Warmup() || i == 0 {
		return Signal{Side: 0, Reason: "warmup"}
	}
	c := s.cfg.VariantM
	if c.MRPeriod <= 0 || c.MRDevATR <= 0 {
		return Signal{Side: 0}
	}
	closePx := ctx.Close[i]
	prevClose := ctx.Close[i-1]
	atr := ctx.ATR[i]
	smaMR := ctx.SMAShort[i]
	smaT := ctx.SMA200[i] // slot trend (variantChannels: M → TrendSMA)
	if math.IsNaN(atr) || atr <= 0 || math.IsNaN(smaMR) || math.IsNaN(smaT) {
		return Signal{Side: 0}
	}
	rsi := ctx.RSI[i]
	dev := c.MRDevATR * atr
	mult := c.ATRStopMult
	if mult <= 0 {
		mult = 2.0
	}
	rsiOn := !math.IsNaN(rsi)
	// LONG: regime rialzista + dislocazione sotto la mean (± conferma rimbalzo/RSI)
	if closePx > smaT && closePx <= smaMR-dev {
		if c.Confirm && closePx <= prevClose {
			return Signal{Side: 0, Reason: "M dip long, attesa rimbalzo"}
		}
		if rsiOn && c.RSIBuy > 0 && rsi > c.RSIBuy {
			return Signal{Side: 0, Reason: "M RSI non oversold"}
		}
		return Signal{Side: 1, Strength: 1, StopPrice: closePx - mult*atr,
			Reason: "M dip long", Meta: map[string]float64{"rsi": rsi, "dev": dev}}
	}
	// SHORT: speculare in regime ribassista (± funding raccolto)
	if c.AllowShorts && closePx < smaT && closePx >= smaMR+dev {
		if c.Confirm && closePx >= prevClose {
			return Signal{Side: 0, Reason: "M rip short, attesa reazione"}
		}
		if rsiOn && c.RSISell > 0 && rsi < c.RSISell {
			return Signal{Side: 0, Reason: "M RSI non overbought"}
		}
		return Signal{Side: -1, Strength: 1, StopPrice: closePx + mult*atr,
			Reason: "M rip short", Meta: map[string]float64{"rsi": rsi, "dev": dev}}
	}
	return Signal{Side: 0, Reason: "M no dislocazione"}
}
