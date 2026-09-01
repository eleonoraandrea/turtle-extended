package data

import (
	"math"
	"testing"
	"time"
)

func TestGenerateSynthetic(t *testing.T) {
	bars := GenerateSynthetic(100, 4*time.Hour, 42)
	if len(bars) != 100 {
		t.Fatalf("len")
	}
	for i, b := range bars {
		if b.High < b.Low {
			t.Fatalf("high<low at %d", i)
		}
		if b.High < math.Max(b.Open, b.Close)-1e-9 {
			t.Fatalf("high < max(open,close) %d", i)
		}
		if b.Low > math.Min(b.Open, b.Close)+1e-9 {
			t.Fatalf("low > min %d", i)
		}
		if b.Volume <= 0 {
			t.Fatalf("volume")
		}
		if b.Time.IsZero() {
			t.Fatalf("time zero")
		}
		if i > 0 && !bars[i].Time.After(bars[i-1].Time) {
			t.Fatalf("not sorted")
		}
	}
	// deterministic: same seed same first close
	bars2 := GenerateSynthetic(100, 4*time.Hour, 42)
	if bars[0].Close != bars2[0].Close {
		t.Fatalf("deterministic")
	}
	bars3 := GenerateSynthetic(100, 4*time.Hour, 43)
	if bars[0].Close == bars3[0].Close {
		t.Fatalf("different seed should differ")
	}
	// OI positive
	for _, b := range bars {
		if b.SumOpenInterest <= 0 {
			t.Fatalf("OI <=0")
		}
	}
}

func TestSaveLoadRoundTripWithDerivatives(t *testing.T) {
	bars := GenerateSynthetic(50, time.Hour, 7)
	// tweak funding/OI
	bars[5].FundingRate = 0.000123
	bars[5].SumOpenInterest = 1.23e9
	path := t.TempDir() + "/test.csv"
	if err := SaveBarsCSV(path, bars); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadBarsCSV(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != len(bars) {
		t.Fatalf("len %d vs %d", len(loaded), len(bars))
	}
	for i := range bars {
		if math.Abs(loaded[i].Close-bars[i].Close) > 1e-6 {
			t.Fatalf("close mismatch %d %.8f vs %.8f", i, loaded[i].Close, bars[i].Close)
		}
		if math.Abs(loaded[i].FundingRate-bars[i].FundingRate) > 1e-8 {
			t.Fatalf("funding mismatch %d %.9f vs %.9f", i, loaded[i].FundingRate, bars[i].FundingRate)
		}
	}
}

func TestAlignDerivatives(t *testing.T) {
	bars := GenerateSynthetic(10, 4*time.Hour, 1)
	// clear existing funding
	for i := range bars {
		bars[i].FundingRate = 0
		bars[i].SumOpenInterest = 0
	}
	fundings := []FundingRate{
		{Symbol: "BTCUSDT", FundingRate: 0.0001, FundingTime: bars[2].Time.UnixMilli()},
		{Symbol: "BTCUSDT", FundingRate: 0.0002, FundingTime: bars[5].Time.UnixMilli()},
	}
	ois := []OIHist{
		{Symbol: "BTCUSDT", SumOpenInterest: 1e9, Timestamp: bars[1].Time.UnixMilli()},
		{Symbol: "BTCUSDT", SumOpenInterest: 2e9, Timestamp: bars[6].Time.UnixMilli()},
	}
	aligned := AlignDerivatives(bars, fundings, ois)
	// before funding time, rate 0
	if aligned[0].FundingRate != 0 || aligned[1].FundingRate != 0 {
		t.Fatalf("early funding should be 0")
	}
	if aligned[2].FundingRate != 0.0001 {
		t.Fatalf("funding at 2")
	}
	if aligned[5].FundingRate != 0.0002 {
		t.Fatalf("funding at 5")
	}
	if aligned[0].SumOpenInterest != 0 {
		t.Fatalf("OI before first ts 0")
	}
	if aligned[1].SumOpenInterest != 1e9 {
		t.Fatalf("OI at 1")
	}
	if aligned[6].SumOpenInterest != 2e9 {
		t.Fatalf("OI at 6")
	}
	// empty bars
	if len(AlignDerivatives(nil, fundings, ois)) != 0 {
		t.Fatalf("nil")
	}
}

func TestIntervalToDuration(t *testing.T) {
	cases := map[string]time.Duration{
		"1m":  time.Minute,
		"5m":  5 * time.Minute,
		"15m": 15 * time.Minute,
		"1h":  time.Hour,
		"4h":  4 * time.Hour,
		"1d":  24 * time.Hour,
	}
	for k, want := range cases {
		if got := intervalToDuration(k); got != want {
			t.Fatalf("%s got %v want %v", k, got, want)
		}
	}
	if intervalToDuration("unknown") != 0 {
		t.Fatalf("unknown should be 0")
	}
}

func TestToFloatAndInt(t *testing.T) {
	if toFloat("123.45") != 123.45 {
		t.Fatalf("toFloat string")
	}
	if toFloat(float64(1.5)) != 1.5 {
		t.Fatalf("float")
	}
	if toInt64("123") != 123 {
		t.Fatalf("toInt string")
	}
	if toInt64(float64(42.7)) != 42 {
		t.Fatalf("float int")
	}
}
