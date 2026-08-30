package data

import (
	"math"
	"math/rand"
	"time"
)

// GenerateSynthetic produces deterministic OHLCV plus funding/OI for pipeline verification.
// Starts at start price 20000 with trend + volatility regime.
func GenerateSynthetic(n int, interval time.Duration, seed int64) Bars {
	r := rand.New(rand.NewSource(seed))
	out := make(Bars, n)
	start := time.Date(2020,1,1,0,0,0,0,time.UTC)
	price := 20000.0
	// synthetic ATR proxy
	for i:=0;i<n;i++ {
		t := start.Add(time.Duration(i)*interval)
		// regime switch every ~500 bars
		regime := math.Sin(float64(i)/300.0)*0.3 + 0.5
		vol := 0.015 + regime*0.02 // 1.5% to 3.5% vol
		drift := 0.00012 // slight up drift ~0.5% per 4h annualized
		if i> n/2 { drift = -0.00005 } // second half down
		ret := r.NormFloat64()*vol + drift
		// add momentum bursts for turtle breakouts
		if i% 120 == 0 { ret += 0.04 } // pop every 20 days (4h*120=20d)
		if i% 200 == 0 { ret -= 0.035 }
		old := price
		price = price * (1+ret)
		if price < 5000 { price = 5000 + math.Abs(r.NormFloat64()*500)}
		high := math.Max(old, price) * (1+ math.Abs(r.NormFloat64())*0.004)
		low := math.Min(old, price) * (1- math.Abs(r.NormFloat64())*0.004)
		open := old
		closePx := price
		volAmt := 800 + math.Abs(r.NormFloat64()*400) + regime*1000
		// funding synthetic: correlated to price momentum, mean 0.01% per 8h
		funding := r.NormFloat64()*0.0003
		if ret > 0.02 { funding += 0.0005 }
		if ret < -0.02 { funding -= 0.0004 }
		oi := 1e9 + float64(i)*1e6 + r.NormFloat64()*1e8 // growing
		if i%50==0 { oi *= 1.05 }
		out[i]=Bar{
			Time: t, Open: open, High: high, Low: low, Close: closePx, Volume: volAmt,
			FundingRate: funding, SumOpenInterest: oi, MarkPrice: closePx,
			QuoteVolume: volAmt*closePx,
			Trades: int64(1000+r.Intn(500)),
		}
	}
	return out
}
