package strategy

import (
	"math"

	"github.com/atps/atps/internal/config"
	"github.com/atps/atps/internal/data"
	"github.com/atps/atps/internal/indicators"
)

// Signal emitted per bar
type Signal struct {
	Side      int     // 1 long, -1 short, 0 flat/no signal
	Strength  float64 // 0-1
	StopPrice float64 // proposed stop
	TakePrice float64 // optional
	Reason    string
	Meta      map[string]float64
}

type Context struct {
	Index int
	Bar   data.Bar
	Bars  data.Bars
	// Precomputed indicators slices (full length)
	Close     []float64
	High      []float64
	Low       []float64
	Volume    []float64
	ATR       []float64
	ADX       []float64
	PlusDI    []float64
	MinusDI   []float64
	EMA50     []float64
	EMA200    []float64
	SMA200    []float64
	Don20H    []float64
	Don20L    []float64
	Don55H    []float64
	Don55L    []float64
	Don100H   []float64
	Don100L   []float64
	RSI        []float64 // RSI (M: rsi_period, default 14) — mean reversion
	SMAShort   []float64 // SMA breve (M: mr_period) — media di riferimento reversion
	VolRegime []float64
	FundingZ  []float64
	VolumeSMA []float64
	OI        []float64
	Funding   []float64
	// For trailing etc
	ChandelierLong  []float64
	ChandelierShort []float64
}

// Strategy interface single symbol
type Strategy interface {
	Name() string
	Variant() string
	Warmup() int
	Prepare(bars data.Bars) *Context
	Next(ctx *Context, i int) Signal
}

// IntrabarEntryLevels — livelli entry per modalità intrabar (stop-entry a canale).
// I livelli DEVONO essere calcolati solo da barre < i (no lookahead).
type IntrabarEntryLevels struct {
	Enabled      bool
	LongLevel    float64 // NaN = disabilitato/filtrato
	LongStopATR  float64
	ShortLevel   float64 // NaN = disabilitato/filtrato
	ShortStopATR float64
}

type IntrabarLevels interface {
	IntrabarEntry(ctx *Context, i int) IntrabarEntryLevels
}

// StopOutInfo — ultimo stop-out, per logica re-entry
type StopOutInfo struct {
	Side       int
	ExitBarIdx int
}

type ReEntryChecker interface {
	ReEntry(ctx *Context, i int, last StopOutInfo) Signal
}

// variantChannels — risolve le lunghezze dei canali/MA per variante dalla config.
// Backward-compatible: valore 0/assente → default storici (20/55/100, SMA 200),
// così i risultati 4h esistenti sono immutati. Su 1h si configurano periodi
// calendar-equivalenti (es. entry 220 = 55×4h).
func variantChannels(cfg *config.Config, variant string) (fastLen, slowLen, midLen, smaLen int) {
	fastLen, slowLen, midLen, smaLen = 20, 55, 100, 200
	switch variant {
	case "A":
		if cfg.VariantA.DonchianAlt > 0 {
			fastLen = cfg.VariantA.DonchianAlt
		}
		if cfg.VariantA.DonchianEntry > 0 {
			slowLen = cfg.VariantA.DonchianEntry
		}
		if cfg.VariantA.SMAFilter > 0 {
			smaLen = cfg.VariantA.SMAFilter
		}
	case "B":
		if cfg.VariantB.DonchianAlt > 0 {
			fastLen = cfg.VariantB.DonchianAlt
		}
		if cfg.VariantB.DonchianEntry > 0 {
			slowLen = cfg.VariantB.DonchianEntry
		}
	case "D":
		if cfg.VariantD.DonchianFast > 0 {
			fastLen = cfg.VariantD.DonchianFast
		}
		if cfg.VariantD.DonchianMid > 0 {
			slowLen = cfg.VariantD.DonchianMid
		}
		if cfg.VariantD.DonchianSlow > 0 {
			midLen = cfg.VariantD.DonchianSlow
		}
	case "M":
		// M non usa i canali donchian per l'entry: il filtro trend usa lo slot SMA
		if cfg.VariantM.TrendSMA > 0 {
			smaLen = cfg.VariantM.TrendSMA
		}
	}
	return
}

// VariantWarmup — warmup dinamico: copre il periodo più lungo tra i canali e il
// filtro MA della variante (default storici → 200, identico al comportamento fisso).
func VariantWarmup(cfg *config.Config, variant string) int {
	fastLen, slowLen, midLen, smaLen := variantChannels(cfg, variant)
	w := fastLen
	if slowLen > w {
		w = slowLen
	}
	if midLen > w {
		w = midLen
	}
	if smaLen > w {
		w = smaLen
	}
	if w < 200 {
		w = 200
	}
	return w
}

// Helpers to convert bars to slices
func PrepareCommon(bars data.Bars, cfg *config.Config, variant string) *Context {
	n := len(bars)
	closeP := make([]float64, n)
	high := make([]float64, n)
	low := make([]float64, n)
	vol := make([]float64, n)
	oi := make([]float64, n)
	funding := make([]float64, n)
	for i, b := range bars {
		closeP[i] = b.Close
		high[i] = b.High
		low[i] = b.Low
		vol[i] = b.Volume
		oi[i] = b.SumOpenInterest
		funding[i] = b.FundingRate
	}
	// pick ATR period depending variant
	atrPeriod := 20
	if variant == "D" {
		atrPeriod = cfg.VariantD.ATRPeriod
	} else if variant == "C" {
		atrPeriod = cfg.VariantC.ATRPeriod
	} else if variant == "B" {
		atrPeriod = cfg.VariantB.ATRPeriod
	} else {
		atrPeriod = cfg.VariantA.ATRPeriod
	}
	atr := indicators.ATR(high, low, closeP, atrPeriod)
	// periodi indicatori wired alla config della variante (default = valori storici)
	adxPeriod, emaFast, emaSlow := 14, 50, 200
	volLookback, fundLookback, volSMALen := 100, 30, 20
	switch variant {
	case "B":
		if cfg.VariantB.ADXPeriod > 0 {
			adxPeriod = cfg.VariantB.ADXPeriod
		}
		if cfg.VariantB.EMAFast > 0 {
			emaFast = cfg.VariantB.EMAFast
		}
		if cfg.VariantB.EMASlow > 0 {
			emaSlow = cfg.VariantB.EMASlow
		}
		if cfg.VariantB.VolLookback > 0 {
			volLookback = cfg.VariantB.VolLookback
		}
	case "C":
		if cfg.VariantC.ADXPeriod > 0 {
			adxPeriod = cfg.VariantC.ADXPeriod
		}
		if cfg.VariantC.EMAFast > 0 {
			emaFast = cfg.VariantC.EMAFast
		}
		if cfg.VariantC.EMASlow > 0 {
			emaSlow = cfg.VariantC.EMASlow
		}
		if cfg.VariantC.FundingZLookback > 0 {
			fundLookback = cfg.VariantC.FundingZLookback
		}
		if cfg.VariantC.VolumeSMA > 0 {
			volSMALen = cfg.VariantC.VolumeSMA
		}
	case "D":
		if cfg.VariantD.ADXPeriod > 0 {
			adxPeriod = cfg.VariantD.ADXPeriod
		}
		if cfg.VariantD.EMAFast > 0 {
			emaFast = cfg.VariantD.EMAFast
		}
		if cfg.VariantD.EMASlow > 0 {
			emaSlow = cfg.VariantD.EMASlow
		}
		if cfg.VariantD.VolLookback > 0 {
			volLookback = cfg.VariantD.VolLookback
		}
		if cfg.VariantD.FundingZLookback > 0 {
			fundLookback = cfg.VariantD.FundingZLookback
		}
		if cfg.VariantD.VolumeSMA > 0 {
			volSMALen = cfg.VariantD.VolumeSMA
		}
	}
	adx, _plus, _minus := indicators.ADX(high, low, closeP, adxPeriod)
	ema50 := indicators.EMA(closeP, emaFast)
	ema200 := indicators.EMA(closeP, emaSlow)
	// canali configurabili per variante (0 → default storici) — chiave per H1:
	// su 1h i canali vengono scalati in barre per mantenere la stessa finestra
	// calendar del 4h (es. entry 55×4h → 220×1h)
	fastLen, slowLen, midLen, smaLen := variantChannels(cfg, variant)
	sma200 := indicators.SMA(closeP, smaLen)
	don20h := indicators.DonchianHigh(high, fastLen)
	don20l := indicators.DonchianLow(low, fastLen)
	don55h := indicators.DonchianHigh(high, slowLen)
	don55l := indicators.DonchianLow(low, slowLen)
	don100h := indicators.DonchianHigh(high, midLen)
	don100l := indicators.DonchianLow(low, midLen)
	// mean-reversion extras: RSI + SMA breve (M: mr_period, altri: 20)
	rsiPeriod := 14
	if variant == "M" && cfg.VariantM.RSIPeriod > 0 {
		rsiPeriod = cfg.VariantM.RSIPeriod
	}
	rsi := indicators.RSI(closeP, rsiPeriod)
	smaShortLen := 20
	if variant == "M" && cfg.VariantM.MRPeriod > 0 {
		smaShortLen = cfg.VariantM.MRPeriod
	}
	smaShort := indicators.SMA(closeP, smaShortLen)
	volRegime := indicators.VolRegime(atr, volLookback)
	fundingZ := indicators.ZScore(funding, fundLookback)
	volSMA := indicators.SMA(vol, volSMALen)
	chLong := indicators.ChandelierLong(high, atr, 22, 3.0)
	chShort := indicators.ChandelierShort(low, atr, 22, 3.0)
	return &Context{
		Bars: bars, Close: closeP, High: high, Low: low, Volume: vol,
		ATR: atr, ADX: adx, PlusDI: _plus, MinusDI: _minus,
		EMA50: ema50, EMA200: ema200, SMA200: sma200,
		Don20H: don20h, Don20L: don20l, Don55H: don55h, Don55L: don55l, Don100H: don100h, Don100L: don100l,
		VolRegime: volRegime, FundingZ: fundingZ, VolumeSMA: volSMA, OI: oi, Funding: funding,
		RSI: rsi, SMAShort: smaShort,
		ChandelierLong: chLong, ChandelierShort: chShort,
	}
}

func isNaN(f float64) bool { return math.IsNaN(f) }

// Volume confirmation helper
func volConfirm(vol, volSMA float64, mult float64) bool {
	if isNaN(volSMA) || volSMA == 0 {
		return true
	} // warmup pass
	return vol >= volSMA*mult
}
