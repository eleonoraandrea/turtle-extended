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
	adxPeriod := 14
	adx, _plus, _minus := indicators.ADX(high, low, closeP, adxPeriod)
	ema50 := indicators.EMA(closeP, 50)
	ema200 := indicators.EMA(closeP, 200)
	sma200 := indicators.SMA(closeP, 200)
	don20h := indicators.DonchianHigh(high, 20)
	don20l := indicators.DonchianLow(low, 20)
	don55h := indicators.DonchianHigh(high, 55)
	don55l := indicators.DonchianLow(low, 55)
	don100h := indicators.DonchianHigh(high, 100)
	don100l := indicators.DonchianLow(low, 100)
	volRegime := indicators.VolRegime(atr, 100)
	fundingZ := indicators.ZScore(funding, 30)
	volSMA := indicators.SMA(vol, 20)
	chLong := indicators.ChandelierLong(high, atr, 22, 3.0)
	chShort := indicators.ChandelierShort(low, atr, 22, 3.0)
	return &Context{
		Bars: bars, Close: closeP, High: high, Low: low, Volume: vol,
		ATR: atr, ADX: adx, PlusDI: _plus, MinusDI: _minus,
		EMA50: ema50, EMA200: ema200, SMA200: sma200,
		Don20H: don20h, Don20L: don20l, Don55H: don55h, Don55L: don55l, Don100H: don100h, Don100L: don100l,
		VolRegime: volRegime, FundingZ: fundingZ, VolumeSMA: volSMA, OI: oi, Funding: funding,
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
