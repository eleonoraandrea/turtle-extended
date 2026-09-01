package indicators

import (
	"math"
	"testing"
)

func approxEqual(a, b, eps float64) bool {
	if math.IsNaN(a) && math.IsNaN(b) {
		return true
	}
	return math.Abs(a-b) <= eps
}

func TestSMA(t *testing.T) {
	src := []float64{1, 2, 3, 4, 5}
	sma := SMA(src, 3)
	if !math.IsNaN(sma[0]) || !math.IsNaN(sma[1]) {
		t.Fatalf("warmup should be NaN")
	}
	if !approxEqual(sma[2], 2.0, 1e-9) {
		t.Fatalf("sma[2]=%f want 2.0", sma[2])
	}
	if !approxEqual(sma[4], 4.0, 1e-9) {
		t.Fatalf("sma[4]=%f want 4.0", sma[4])
	}
	// period 0 returns all NaN
	sma0 := SMA(src, 0)
	for i, v := range sma0 {
		if !math.IsNaN(v) {
			t.Fatalf("sma0[%d] should be NaN", i)
		}
	}
}

func TestEMA(t *testing.T) {
	src := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	ema := EMA(src, 3)
	// first 2 NaN, third seeded SMA(1,2,3)=2
	if !math.IsNaN(ema[0]) || !math.IsNaN(ema[1]) {
		t.Fatalf("warmup NaN")
	}
	if !approxEqual(ema[2], 2.0, 1e-9) {
		t.Fatalf("ema[2]=%f want 2.0", ema[2])
	}
	// EMA should be monotonic increasing for increasing src
	for i := 3; i < len(ema); i++ {
		if ema[i] <= ema[i-1] {
			t.Fatalf("ema not increasing at %d %.4f <= %.4f", i, ema[i], ema[i-1])
		}
	}
}

func TestRMA(t *testing.T) {
	src := []float64{1, 2, 3, 4, 5, 6, 7}
	rma := RMA(src, 3)
	if !math.IsNaN(rma[0]) || !math.IsNaN(rma[1]) {
		t.Fatalf("warmup NaN")
	}
	if !approxEqual(rma[2], 2.0, 1e-9) {
		t.Fatalf("rma[2]=%f want 2", rma[2])
	}
	// Wilder: rma[3]=(2*2+4)/3=8/3≈2.666, rma[4]=(2.666*2+5)/3≈3.444
	if !approxEqual(rma[3], (2.0*2+4)/3, 1e-9) {
		t.Fatalf("rma[3]=%f", rma[3])
	}
	// with NaN gap
	src2 := []float64{1, 2, 3, math.NaN(), 4, 5, 6}
	rma2 := RMA(src2, 3)
	if !math.IsNaN(rma2[3]) {
		t.Fatalf("NaN src should produce NaN out")
	}
	// after NaN, should recover not panic
	_ = rma2
}

func TestTrueRangeAndATR(t *testing.T) {
	high := []float64{10, 11, 12}
	low := []float64{9, 9.5, 10}
	close := []float64{9.5, 10.5, 11}
	tr := TrueRange(high, low, close)
	if !approxEqual(tr[0], 1.0, 1e-9) {
		t.Fatalf("tr0 %f", tr[0])
	}
	if !approxEqual(tr[1], 1.5, 1e-9) {
		t.Fatalf("tr1 %f", tr[1])
	}
	atr := ATR(high, low, close, 2)
	if math.IsNaN(atr[1]) {
		t.Fatalf("atr should be computed")
	}
}

func TestDonchian(t *testing.T) {
	high := []float64{1, 3, 2, 5, 4}
	dh := DonchianHigh(high, 3)
	if !math.IsNaN(dh[0]) || !math.IsNaN(dh[1]) {
		t.Fatalf("warmup NaN")
	}
	if !approxEqual(dh[2], 3, 1e-9) {
		t.Fatalf("dh2 %f", dh[2])
	}
	if !approxEqual(dh[3], 5, 1e-9) {
		t.Fatalf("dh3 %f", dh[3])
	}
	low := []float64{5, 4, 6, 2, 3}
	dl := DonchianLow(low, 3)
	if !approxEqual(dl[2], 4, 1e-9) {
		t.Fatalf("dl2 %f", dl[2])
	}
}

func TestADX(t *testing.T) {
	// trending data should produce valid ADX after warmup
	n := 50
	high := make([]float64, n)
	low := make([]float64, n)
	close := make([]float64, n)
	for i := 0; i < n; i++ {
		high[i] = 100 + float64(i)*0.5 + 0.3
		low[i] = 100 + float64(i)*0.5 - 0.3
		close[i] = 100 + float64(i)*0.5
	}
	adx, plus, minus := ADX(high, low, close, 14)
	if len(adx) != n {
		t.Fatalf("len")
	}
	// after period*2 warmup ADX should be numeric
	found := false
	for i := 28; i < n; i++ {
		if !math.IsNaN(adx[i]) {
			found = true
			if adx[i] < 0 || adx[i] > 100 {
				t.Fatalf("adx out of range %f", adx[i])
			}
			if math.IsNaN(plus[i]) || math.IsNaN(minus[i]) {
				t.Fatalf("DI NaN when ADX valid")
			}
			break
		}
	}
	if !found {
		t.Fatalf("no valid ADX found")
	}
}

func TestRollingStd(t *testing.T) {
	src := []float64{1, 1, 1, 1, 1}
	std := RollingStd(src, 3)
	if !approxEqual(std[2], 0, 1e-9) {
		t.Fatalf("std flat %f", std[2])
	}
	src2 := []float64{1, 2, 3, 4, 5}
	std2 := RollingStd(src2, 3)
	if std2[2] <= 0 {
		t.Fatalf("std should be >0")
	}
	// NaN window should stay NaN
	src3 := []float64{1, math.NaN(), 3, 4, 5}
	std3 := RollingStd(src3, 3)
	if !math.IsNaN(std3[2]) {
		t.Fatalf("NaN window should be NaN")
	}
}

func TestPercentileRank(t *testing.T) {
	src := []float64{1, 2, 3, 4, 5}
	pr := PercentileRank(src, 5)
	if !approxEqual(pr[4], 100, 1e-9) {
		t.Fatalf("pr max 100 got %f", pr[4])
	}
	src2 := []float64{5, 4, 3, 2, 1}
	pr2 := PercentileRank(src2, 5)
	if !approxEqual(pr2[4], 20, 1e-9) { // only 1 of 5 <=1 => 20%
		t.Fatalf("pr min got %f", pr2[4])
	}
}

func TestZScore(t *testing.T) {
	src := []float64{1, 1, 1, 10, 1, 1, 1}
	z := ZScore(src, 3)
	// peak at i=3 should have high Z
	if math.IsNaN(z[3]) {
		t.Fatalf("z[3] NaN")
	}
	if z[3] <= 1.0 {
		t.Fatalf("peak z should be >1 got %f", z[3])
	}
}

func TestVolRegimeAndChandelier(t *testing.T) {
	atr := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	vr := VolRegime(atr, 5)
	if math.IsNaN(vr[4]) {
		t.Fatalf("vr 4 not NaN")
	}
	high := []float64{10, 11, 12, 13, 14, 15, 16}
	low := []float64{9, 9.5, 10, 10.5, 11, 11.5, 12}
	atr2 := []float64{1, 1, 1, 1, 1, 1, 1}
	cl := ChandelierLong(high, atr2, 3, 2.0)
	cs := ChandelierShort(low, atr2, 3, 2.0)
	if math.IsNaN(cl[2]) || math.IsNaN(cs[2]) {
		t.Fatalf("chandelier NaN unexpected")
	}
	// long = donHigh - mult*atr
	if !approxEqual(cl[2], 12-2*1, 1e-9) {
		t.Fatalf("chandelier long %f", cl[2])
	}
}

func TestLogReturns(t *testing.T) {
	close := []float64{100, 110, 99}
	lr := LogReturns(close)
	if math.IsNaN(lr[1]) || lr[1] <= 0 {
		t.Fatalf("positive return")
	}
	if lr[2] >= 0 {
		t.Fatalf("negative return")
	}
	// zero price should be NaN
	close2 := []float64{0, 100}
	lr2 := LogReturns(close2)
	if !math.IsNaN(lr2[1]) {
		t.Fatalf("zero price should be NaN")
	}
}
