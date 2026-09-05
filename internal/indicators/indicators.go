package indicators

import "math"

// SMA simple moving average, NaN for insufficient
func SMA(src []float64, period int) []float64 {
	n := len(src)
	out := make([]float64, n)
	for i := range out {
		out[i] = math.NaN()
	}
	if period <= 0 {
		return out
	}
	sum := 0.0
	for i := 0; i < n; i++ {
		sum += src[i]
		if i >= period {
			sum -= src[i-period]
		}
		if i >= period-1 {
			out[i] = sum / float64(period)
		}
	}
	return out
}

// EMA exponential, Wilder alpha=1/period for ATR variant, standard EMA alpha=2/(p+1)
func EMA(src []float64, period int) []float64 {
	n := len(src)
	out := make([]float64, n)
	for i := range out {
		out[i] = math.NaN()
	}
	if period <= 0 || n == 0 {
		return out
	}
	alpha := 2.0 / float64(period+1)
	// seed SMA
	sma := SMA(src, period)
	for i := 0; i < n; i++ {
		if math.IsNaN(sma[i]) {
			continue
		}
		if i == period-1 {
			out[i] = sma[i]
		} else if i >= period {
			out[i] = src[i]*alpha + out[i-1]*(1-alpha)
		}
	}
	return out
}

// RMA Wilder smoothing (alpha=1/period) — robust NaN handling for ADX/ATR
func RMA(src []float64, period int) []float64 {
	n := len(src)
	out := make([]float64, n)
	for i := range out {
		out[i] = math.NaN()
	}
	if period <= 0 || n == 0 {
		return out
	}
	sum := 0.0
	validCount := 0
	var lastValid float64
	hasLast := false
	for i := 0; i < n; i++ {
		if math.IsNaN(src[i]) {
			// gap resets running window but preserves lastValid for Wilder continuity
			sum = 0
			validCount = 0
			continue
		}
		if !hasLast {
			sum += src[i]
			validCount++
			if validCount == period {
				out[i] = sum / float64(period)
				lastValid = out[i]
				hasLast = true
			}
		} else {
			// Wilder recursion: last*(period-1)+src / period
			if math.IsNaN(lastValid) {
				// need fresh SMA window when previous NaN
				sum = 0
				validCount = 0
				// recompute window ending at i
				for k := i - period + 1; k <= i; k++ {
					if k >= 0 && !math.IsNaN(src[k]) {
						sum += src[k]
						validCount++
					} else {
						validCount = 0
						sum = 0
						break
					}
				}
				if validCount == period {
					out[i] = sum / float64(period)
					lastValid = out[i]
				} else {
					out[i] = math.NaN()
					hasLast = false
					sum = 0
					validCount = 0
				}
			} else {
				out[i] = (lastValid*float64(period-1) + src[i]) / float64(period)
				lastValid = out[i]
			}
		}
	}
	return out
}

// TR true range
func TrueRange(high, low, close []float64) []float64 {
	n := len(high)
	out := make([]float64, n)
	for i := range out {
		out[i] = math.NaN()
	}
	for i := 0; i < n; i++ {
		if i == 0 {
			out[i] = high[i] - low[i]
		} else {
			tr1 := high[i] - low[i]
			tr2 := math.Abs(high[i] - close[i-1])
			tr3 := math.Abs(low[i] - close[i-1])
			out[i] = math.Max(tr1, math.Max(tr2, tr3))
		}
	}
	return out
}

// ATR Wilder RMA on TR
func ATR(high, low, close []float64, period int) []float64 {
	tr := TrueRange(high, low, close)
	return RMA(tr, period)
}

// Donchian channels
func DonchianHigh(high []float64, period int) []float64 {
	n := len(high)
	out := make([]float64, n)
	for i := range out {
		out[i] = math.NaN()
	}
	for i := 0; i < n; i++ {
		if i >= period-1 {
			mx := high[i]
			for j := i - period + 1; j <= i; j++ {
				if high[j] > mx {
					mx = high[j]
				}
			}
			out[i] = mx
		}
	}
	return out
}
func DonchianLow(low []float64, period int) []float64 {
	n := len(low)
	out := make([]float64, n)
	for i := range out {
		out[i] = math.NaN()
	}
	for i := 0; i < n; i++ {
		if i >= period-1 {
			mn := low[i]
			for j := i - period + 1; j <= i; j++ {
				if low[j] < mn {
					mn = low[j]
				}
			}
			out[i] = mn
		}
	}
	return out
}

// ADX Wilder: returns adx, plusDI, minusDI, dx
func ADX(high, low, close []float64, period int) (adx, plusDI, minusDI []float64) {
	n := len(high)
	adx = make([]float64, n)
	plusDI = make([]float64, n)
	minusDI = make([]float64, n)
	for i := range adx {
		adx[i] = math.NaN()
		plusDI[i] = math.NaN()
		minusDI[i] = math.NaN()
	}
	if n < period+1 {
		return
	}
	plusDM := make([]float64, n)
	minusDM := make([]float64, n)
	tr := TrueRange(high, low, close)
	for i := 1; i < n; i++ {
		up := high[i] - high[i-1]
		dn := low[i-1] - low[i]
		if up > dn && up > 0 {
			plusDM[i] = up
		} else {
			plusDM[i] = 0
		}
		if dn > up && dn > 0 {
			minusDM[i] = dn
		} else {
			minusDM[i] = 0
		}
	}
	sTR := RMA(tr, period)
	sPlus := RMA(plusDM, period)
	sMinus := RMA(minusDM, period)
	dx := make([]float64, n)
	for i := range dx {
		dx[i] = math.NaN()
	}
	for i := 0; i < n; i++ {
		if math.IsNaN(sTR[i]) || sTR[i] == 0 {
			continue
		}
		plusDI[i] = 100 * sPlus[i] / sTR[i]
		minusDI[i] = 100 * sMinus[i] / sTR[i]
		sum := plusDI[i] + minusDI[i]
		if sum != 0 {
			dx[i] = 100 * math.Abs(plusDI[i]-minusDI[i]) / sum
		} else {
			dx[i] = 0
		}
	}
	adxR := RMA(dx, period)
	copy(adx, adxR)
	return
}

// Rolling std (population) — skips windows with NaN
func RollingStd(src []float64, period int) []float64 {
	n := len(src)
	out := make([]float64, n)
	for i := range out {
		out[i] = math.NaN()
	}
	if period <= 0 {
		return out
	}
	for i := period - 1; i < n; i++ {
		hasNaN := false
		mean := 0.0
		for j := i - period + 1; j <= i; j++ {
			if math.IsNaN(src[j]) {
				hasNaN = true
				break
			}
			mean += src[j]
		}
		if hasNaN {
			continue
		}
		mean /= float64(period)
		var sum float64
		for j := i - period + 1; j <= i; j++ {
			d := src[j] - mean
			sum += d * d
		}
		out[i] = math.Sqrt(sum / float64(period))
	}
	return out
}

// Percentile rank of current value within lookback window (0-100) — NaN aware
func PercentileRank(src []float64, lookback int) []float64 {
	n := len(src)
	out := make([]float64, n)
	for i := range out {
		out[i] = math.NaN()
	}
	if lookback <= 0 {
		return out
	}
	for i := lookback - 1; i < n; i++ {
		if math.IsNaN(src[i]) {
			continue
		}
		cnt := 0
		valid := 0
		for j := i - lookback + 1; j <= i; j++ {
			if math.IsNaN(src[j]) {
				continue
			}
			valid++
			if src[j] <= src[i] {
				cnt++
			}
		}
		if valid == 0 {
			continue
		}
		out[i] = float64(cnt) / float64(valid) * 100
	}
	return out
}

// ZScore rolling
func ZScore(src []float64, period int) []float64 {
	n := len(src)
	out := make([]float64, n)
	for i := range out {
		out[i] = math.NaN()
	}
	mean := SMA(src, period)
	std := RollingStd(src, period)
	for i := 0; i < n; i++ {
		if math.IsNaN(mean[i]) || math.IsNaN(std[i]) || std[i] == 0 {
			continue
		}
		out[i] = (src[i] - mean[i]) / std[i]
	}
	return out
}

// Returns log returns
func LogReturns(close []float64) []float64 {
	n := len(close)
	out := make([]float64, n)
	for i := range out {
		out[i] = math.NaN()
	}
	for i := 1; i < n; i++ {
		if close[i-1] <= 0 || close[i] <= 0 {
			continue
		}
		out[i] = math.Log(close[i] / close[i-1])
	}
	return out
}

// Volatility regime: ATR percentile or realized vol percentile
func VolRegime(atr []float64, lookback int) []float64 {
	return PercentileRank(atr, lookback)
}

// Chandelier trailing: highest high - atr*mult for longs, lowest low + atr*mult for shorts
func ChandelierLong(high []float64, atr []float64, period int, mult float64) []float64 {
	// rolling max high then minus atr*mult
	dh := DonchianHigh(high, period)
	n := len(high)
	out := make([]float64, n)
	for i := range out {
		out[i] = math.NaN()
	}
	for i := 0; i < n; i++ {
		if math.IsNaN(dh[i]) || math.IsNaN(atr[i]) {
			continue
		}
		out[i] = dh[i] - mult*atr[i]
	}
	return out
}
func ChandelierShort(low []float64, atr []float64, period int, mult float64) []float64 {
	dl := DonchianLow(low, period)
	n := len(low)
	out := make([]float64, n)
	for i := range out {
		out[i] = math.NaN()
	}
	for i := 0; i < n; i++ {
		if math.IsNaN(dl[i]) || math.IsNaN(atr[i]) {
			continue
		}
		out[i] = dl[i] + mult*atr[i]
	}
	return out
}

// RSI — Wilder smoothing (RMA di gain/loss). NaN in warmup.
func RSI(close []float64, period int) []float64 {
	n := len(close)
	out := make([]float64, n)
	for i := range out {
		out[i] = math.NaN()
	}
	if period <= 0 || n < period+1 {
		return out
	}
	gains := make([]float64, n)
	losses := make([]float64, n)
	for i := 1; i < n; i++ {
		d := close[i] - close[i-1]
		if d > 0 {
			gains[i] = d
		} else {
			losses[i] = -d
		}
	}
	ag := RMA(gains, period)
	al := RMA(losses, period)
	for i := 0; i < n; i++ {
		if math.IsNaN(ag[i]) || math.IsNaN(al[i]) {
			continue
		}
		if al[i] == 0 {
			out[i] = 100
			continue
		}
		rs := ag[i] / al[i]
		out[i] = 100 - 100/(1+rs)
	}
	return out
}
