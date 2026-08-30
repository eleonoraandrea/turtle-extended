package data

import "time"

// Bar represents one OHLCV + derivatives overlay.
type Bar struct {
	Time         time.Time `json:"time"`
	Open         float64   `json:"open"`
	High         float64   `json:"high"`
	Low          float64   `json:"low"`
	Close        float64   `json:"close"`
	Volume       float64   `json:"volume"`
	QuoteVolume  float64   `json:"quote_volume,omitempty"`
	Trades       int64     `json:"trades,omitempty"`
	TakerBuyBase float64   `json:"taker_buy_base,omitempty"`
	// Derivatives overlay (aligned from funding/OI)
	FundingRate float64 `json:"funding_rate,omitempty"`
	OpenInterest float64 `json:"open_interest,omitempty"` // base qty
	SumOpenInterest float64 `json:"sum_oi,omitempty"` // notional USD
	MarkPrice    float64 `json:"mark_price,omitempty"`
}

// Bars is time-sorted ascending.
type Bars []Bar

func (b Bars) Len() int { return len(b) }
func (b Bars) Times() []time.Time {
	out := make([]time.Time, len(b))
	for i, bar := range b { out[i] = bar.Time }
	return out
}
func (b Bars) Closes() []float64 {
	out := make([]float64, len(b))
	for i, bar := range b { out[i] = bar.Close }
	return out
}
