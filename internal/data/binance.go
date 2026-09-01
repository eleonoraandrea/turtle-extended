package data

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultBinanceBase = "https://fapi.binance.com"
	MaxKlinesLimit     = 1500
)

// BinanceKline raw response: [ openTime, open, high, low, close, volume, closeTime, quoteVolume, trades, takerBuyBase, takerBuyQuote, ignore ]
type BinanceClient struct {
	Base string
	HTTP *http.Client
}

func NewBinanceClient(base string) *BinanceClient {
	if base == "" {
		base = DefaultBinanceBase
	}
	return &BinanceClient{Base: strings.TrimRight(base, "/"), HTTP: &http.Client{Timeout: 30 * time.Second}}
}

// FetchKlines paginated fetch with auto pagination. Interval: 1m,5m,15m,1h,4h,1d etc
// Simple paginated loop: each request asks for up to 1500 bars starting at cur.
// This correctly handles gaps (e.g. SOL before listing) because Binance returns
// the first available bars after cur, not an empty chunk.
func (c *BinanceClient) FetchKlines(symbol, interval string, startTime, endTime time.Time) (Bars, error) {
	var all Bars
	cur := startTime
	dur := intervalToDuration(interval)
	if dur == 0 {
		dur = time.Hour
	}
	for {
		if !endTime.IsZero() && !cur.Before(endTime) {
			break
		}
		bars, err := c.fetchKlinesChunk(symbol, interval, cur, endTime)
		if err != nil {
			return nil, err
		}
		if len(bars) == 0 {
			break
		}
		all = append(all, bars...)
		if len(bars) < MaxKlinesLimit {
			break
		}
		last := bars[len(bars)-1].Time.Add(dur)
		if !last.After(cur) {
			break
		}
		cur = last
		if !endTime.IsZero() && (cur.After(endTime) || cur.Equal(endTime)) {
			break
		}
		time.Sleep(120 * time.Millisecond)
		if len(all) > 200000 {
			break
		}
	}
	return all, nil
}

func (c *BinanceClient) fetchKlinesChunk(symbol, interval string, start, end time.Time) (Bars, error) {
	u := fmt.Sprintf("%s/fapi/v1/klines?symbol=%s&interval=%s&limit=%d", c.Base, url.QueryEscape(symbol), url.QueryEscape(interval), MaxKlinesLimit)
	if !start.IsZero() {
		u += fmt.Sprintf("&startTime=%d", start.UnixMilli())
	}
	if !end.IsZero() {
		u += fmt.Sprintf("&endTime=%d", end.UnixMilli())
	}
	resp, err := c.HTTP.Get(u)
	if err != nil {
		return nil, fmt.Errorf("binance klines GET %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("binance klines %d %s", resp.StatusCode, string(b))
	}
	var raw [][]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	out := make(Bars, 0, len(raw))
	for _, r := range raw {
		if len(r) < 11 {
			continue
		}
		ot := toInt64(r[0])
		o := toFloat(r[1])
		h := toFloat(r[2])
		l := toFloat(r[3])
		cl := toFloat(r[4])
		vol := toFloat(r[5])
		ct := toInt64(r[6])
		_ = ct
		qv := toFloat(r[7])
		tr := toInt64(r[8])
		tbBase := toFloat(r[9])
		out = append(out, Bar{
			Time: time.UnixMilli(ot).UTC(),
			Open: o, High: h, Low: l, Close: cl,
			Volume: vol, QuoteVolume: qv, Trades: tr, TakerBuyBase: tbBase,
		})
	}
	return out, nil
}

// FundingRate entry
type FundingRate struct {
	Symbol      string  `json:"symbol"`
	FundingRate float64 `json:"fundingRate,string"`
	FundingTime int64   `json:"fundingTime"`
	MarkPrice   float64 `json:"markPrice,string"`
}

func (c *BinanceClient) FetchFundingRate(symbol string, start, end time.Time, limit int) ([]FundingRate, error) {
	if limit == 0 || limit > 1000 {
		limit = 1000
	}
	var all []FundingRate
	curStart := start
	for {
		u := fmt.Sprintf("%s/fapi/v1/fundingRate?symbol=%s&limit=%d", c.Base, url.QueryEscape(symbol), limit)
		if !curStart.IsZero() {
			u += fmt.Sprintf("&startTime=%d", curStart.UnixMilli())
		}
		if !end.IsZero() {
			u += fmt.Sprintf("&endTime=%d", end.UnixMilli())
		}
		resp, err := c.HTTP.Get(u)
		if err != nil {
			return nil, err
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("funding %d %s", resp.StatusCode, string(b))
		}
		var raw []struct {
			Symbol      string `json:"symbol"`
			FundingRate string `json:"fundingRate"`
			FundingTime int64  `json:"fundingTime"`
			MarkPrice   string `json:"markPrice"`
		}
		if err := json.Unmarshal(b, &raw); err != nil {
			return nil, err
		}
		if len(raw) == 0 {
			break
		}
		var chunk []FundingRate
		for _, r := range raw {
			fr, _ := strconv.ParseFloat(r.FundingRate, 64)
			mp, _ := strconv.ParseFloat(r.MarkPrice, 64)
			chunk = append(chunk, FundingRate{Symbol: r.Symbol, FundingRate: fr, FundingTime: r.FundingTime, MarkPrice: mp})
		}
		// filter by end if needed (Binance may return beyond)
		for _, fr := range chunk {
			if !end.IsZero() && fr.FundingTime > end.UnixMilli() {
				continue
			}
			all = append(all, fr)
		}
		if len(chunk) < limit {
			break
		}
		last := chunk[len(chunk)-1].FundingTime
		nextStart := time.UnixMilli(last + 1)
		if !end.IsZero() && nextStart.After(end) {
			break
		}
		// prevent infinite loop if last doesn't advance (Binance quirk)
		if !curStart.IsZero() && nextStart.Equal(curStart) {
			break
		}
		curStart = nextStart
		if len(all) > 20000 {
			break
		}
		// single shot when no time bounds: Binance returns latest 1000 only, no pagination needed
		if start.IsZero() && end.IsZero() {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	return all, nil
}

// OpenInterestHist entry
type OIHist struct {
	Symbol               string  `json:"symbol"`
	SumOpenInterest      float64 `json:"sumOpenInterest,string"`
	SumOpenInterestValue float64 `json:"sumOpenInterestValue,string"` // alternative field
	CMCSumOpenInterest   float64 `json:"CMCSumOpenInterest,string"`
	Timestamp            int64   `json:"timestamp"`
}

func (c *BinanceClient) FetchOpenInterestHist(symbol, period string, start, end time.Time, limit int) ([]OIHist, error) {
	if limit == 0 || limit > 500 {
		limit = 500
	}
	if period == "" {
		period = "4h"
	}
	var all []OIHist
	curStart := start
	for {
		u := fmt.Sprintf("%s/futures/data/openInterestHist?symbol=%s&period=%s&limit=%d", c.Base, url.QueryEscape(symbol), url.QueryEscape(period), limit)
		if !curStart.IsZero() {
			u += fmt.Sprintf("&startTime=%d", curStart.UnixMilli())
		}
		if !end.IsZero() {
			u += fmt.Sprintf("&endTime=%d", end.UnixMilli())
		}
		resp, err := c.HTTP.Get(u)
		if err != nil {
			return nil, err
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("oiHist %d %s", resp.StatusCode, string(b))
		}
		var raw []map[string]interface{}
		if err := json.Unmarshal(b, &raw); err != nil {
			return nil, fmt.Errorf("oi json %w body %s", err, string(b))
		}
		if len(raw) == 0 {
			break
		}
		for _, m := range raw {
			s, _ := m["symbol"].(string)
			ts := toInt64(m["timestamp"])
			var soi float64
			if v, ok := m["sumOpenInterest"]; ok {
				soi = toFloat(v)
			}
			if soi == 0 {
				if v, ok := m["sumOpenInterestValue"]; ok {
					soi = toFloat(v)
				}
			}
			all = append(all, OIHist{Symbol: s, SumOpenInterest: soi, Timestamp: ts})
		}
		if len(raw) < limit {
			break
		}
		lastTs := toInt64(raw[len(raw)-1]["timestamp"])
		nextStart := time.UnixMilli(lastTs + 1)
		if !end.IsZero() && nextStart.After(end) {
			break
		}
		if nextStart.Equal(curStart) {
			break
		}
		curStart = nextStart
		if len(all) > 10000 {
			break
		}
		if start.IsZero() && end.IsZero() {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	return all, nil
}

// Storage helpers

func SaveBarsCSV(path string, bars Bars) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	w.Write([]string{"time", "open", "high", "low", "close", "volume", "quote_volume", "trades", "taker_buy_base", "funding_rate", "open_interest", "sum_oi", "mark_price"})
	for _, b := range bars {
		w.Write([]string{
			b.Time.Format(time.RFC3339),
			fmt.Sprintf("%.8f", b.Open),
			fmt.Sprintf("%.8f", b.High),
			fmt.Sprintf("%.8f", b.Low),
			fmt.Sprintf("%.8f", b.Close),
			fmt.Sprintf("%.8f", b.Volume),
			fmt.Sprintf("%.8f", b.QuoteVolume),
			strconv.FormatInt(b.Trades, 10),
			fmt.Sprintf("%.8f", b.TakerBuyBase),
			strconv.FormatFloat(b.FundingRate, 'f', 8, 64),
			strconv.FormatFloat(b.OpenInterest, 'f', 6, 64),
			strconv.FormatFloat(b.SumOpenInterest, 'f', 2, 64),
			strconv.FormatFloat(b.MarkPrice, 'f', 6, 64),
		})
	}
	w.Flush()
	return w.Error()
}

func LoadBarsCSV(path string) (Bars, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	header, err := r.Read()
	if err != nil {
		return nil, err
	}
	_ = header
	var out Bars
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(rec) < 5 {
			continue
		}
		t, _ := time.Parse(time.RFC3339, rec[0])
		o, _ := strconv.ParseFloat(rec[1], 64)
		h, _ := strconv.ParseFloat(rec[2], 64)
		l, _ := strconv.ParseFloat(rec[3], 64)
		c, _ := strconv.ParseFloat(rec[4], 64)
		vol := 0.0
		if len(rec) > 5 {
			vol, _ = strconv.ParseFloat(rec[5], 64)
		}
		qv := 0.0
		if len(rec) > 6 {
			qv, _ = strconv.ParseFloat(rec[6], 64)
		}
		tr := int64(0)
		if len(rec) > 7 {
			tr, _ = strconv.ParseInt(rec[7], 10, 64)
		}
		tb := 0.0
		if len(rec) > 8 {
			tb, _ = strconv.ParseFloat(rec[8], 64)
		}
		fr := 0.0
		if len(rec) > 9 {
			fr, _ = strconv.ParseFloat(rec[9], 64)
		}
		oi := 0.0
		if len(rec) > 10 {
			oi, _ = strconv.ParseFloat(rec[10], 64)
		}
		soi := 0.0
		if len(rec) > 11 {
			soi, _ = strconv.ParseFloat(rec[11], 64)
		}
		mp := 0.0
		if len(rec) > 12 {
			mp, _ = strconv.ParseFloat(rec[12], 64)
		}
		out = append(out, Bar{Time: t, Open: o, High: h, Low: l, Close: c, Volume: vol, QuoteVolume: qv, Trades: tr, TakerBuyBase: tb, FundingRate: fr, OpenInterest: oi, SumOpenInterest: soi, MarkPrice: mp})
	}
	return out, nil
}

// Align funding and OI onto bars timeline (forward fill last known before bar time)
func AlignDerivatives(bars Bars, fundings []FundingRate, ois []OIHist) Bars {
	if len(bars) == 0 {
		return bars
	}
	// sort fundings by time
	// fundingRate applies to period ending at fundingTime; we assign to bar whose time >= fundingTime
	// simple: for each bar, find latest fundingTime <= bar.Time+interval
	out := make(Bars, len(bars))
	copy(out, bars)
	// OI map
	oiIdx := 0
	var lastOI float64
	// assume ois sorted by timestamp
	for i, bar := range out {
		for oiIdx < len(ois) && ois[oiIdx].Timestamp <= bar.Time.UnixMilli() {
			lastOI = ois[oiIdx].SumOpenInterest
			oiIdx++
		}
		out[i].SumOpenInterest = lastOI
		// if OI entry exactly at bar time, use it else carry
	}
	// funding
	fIdx := 0
	var lastFR float64
	var lastMP float64
	for i, bar := range out {
		for fIdx < len(fundings) && fundings[fIdx].FundingTime <= bar.Time.UnixMilli() {
			lastFR = fundings[fIdx].FundingRate
			lastMP = fundings[fIdx].MarkPrice
			fIdx++
		}
		out[i].FundingRate = lastFR
		if lastMP != 0 {
			out[i].MarkPrice = lastMP
		}
	}
	return out
}

// Helpers

func intervalToDuration(s string) time.Duration {
	switch s {
	case "1m":
		return time.Minute
	case "3m":
		return 3 * time.Minute
	case "5m":
		return 5 * time.Minute
	case "15m":
		return 15 * time.Minute
	case "30m":
		return 30 * time.Minute
	case "1h":
		return time.Hour
	case "2h":
		return 2 * time.Hour
	case "4h":
		return 4 * time.Hour
	case "6h":
		return 6 * time.Hour
	case "8h":
		return 8 * time.Hour
	case "12h":
		return 12 * time.Hour
	case "1d":
		return 24 * time.Hour
	case "3d":
		return 72 * time.Hour
	case "1w":
		return 7 * 24 * time.Hour
	}
	return 0
}
func toFloat(v interface{}) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case string:
		f, _ := strconv.ParseFloat(x, 64)
		return f
	case json.Number:
		f, _ := x.Float64()
		return f
	case int:
		return float64(x)
	case int64:
		return float64(x)
	}
	// try string conv via fmt
	s := fmt.Sprintf("%v", v)
	if f, err := strconv.ParseFloat(s, 64); err == nil && !math.IsNaN(f) && !math.IsInf(f, 0) {
		return f
	}
	return 0
}
func toInt64(v interface{}) int64 {
	switch x := v.(type) {
	case float64:
		return int64(x)
	case int:
		return int64(x)
	case int64:
		return x
	case string:
		i, _ := strconv.ParseInt(x, 10, 64)
		if i != 0 {
			return i
		}
		f, _ := strconv.ParseFloat(x, 64)
		return int64(f)
	case json.Number:
		i, _ := x.Int64()
		return i
	}
	s := fmt.Sprintf("%v", v)
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return int64(f)
	}
	return 0
}
