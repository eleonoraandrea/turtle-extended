package report

import (
	"encoding/json"
	"fmt"
	"html/template"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/atps/atps/internal/backtest"
	"github.com/atps/atps/internal/config"
	"github.com/atps/atps/internal/data"
	"github.com/atps/atps/internal/metrics"
)

// Report input bundles
type Input struct {
	Config      *config.Config
	Bars        data.Bars
	Result      *backtest.Result
	Stats       metrics.Stats
	Symbol      string
	Variant     string
	GeneratedAt time.Time
}

type ComparisonRow struct {
	Symbol  string
	Variant string
	Stats   metrics.Stats
}

// Generate single backtest HTML self-contained
func Generate(path string, in Input) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	// prepare data for template
	equityJSON, _ := json.Marshal(in.Result.Equity)
	tradesJSON, _ := json.Marshal(in.Result.Trades)
	barsJSON, _ := json.Marshal(in.Bars)
	// compute monthly sorted
	monthlyKeys := metrics.SortedMonthly(in.Stats.MonthlyReturns)
	yearlyKeys := metrics.SortedMonthly(in.Stats.YearlyReturns)
	// SVG equity polyline
	equitySVG := svgEquity(in.Result.Equity, 800, 220)
	drawdownSVG := svgDrawdown(in.Result.Equity, 800, 120)
	// histogram helpers
	profitHistSVG := svgTradeHist(in.Result.Trades, 500, 180)
	// Prepare trades table limited
	limit := in.Config.Report.MaxTradesInTable
	if limit == 0 {
		limit = 500
	}
	tradesShown := in.Result.Trades
	if len(tradesShown) > limit {
		tradesShown = tradesShown[len(tradesShown)-limit:]
	}
	// regime breakdown simple
	regimeTable := regimeBreakdown(in.Result, in.Bars)

	// risk & sizing aggregates
	lim := in.Result.RiskLimitsUsed
	dataMap := map[string]interface{}{
		"Title":          fmt.Sprintf("%s %s %s — %s", in.Config.Report.TitlePrefix, in.Symbol, in.Variant, in.Result.Variant),
		"Symbol":         in.Symbol,
		"Variant":        in.Variant,
		"VariantName":    in.Result.Variant,
		"GeneratedAt":    in.GeneratedAt.Format("2006-01-02 15:04 MST"),
		"Stats":          in.Stats,
		"Config":         in.Config,
		"EquityCurve":    in.Result.Equity,
		"Trades":         tradesShown,
		"AllTradesCount": len(in.Result.Trades),
		"EquitySVG":      template.HTML(equitySVG),
		"DrawdownSVG":    template.HTML(drawdownSVG),
		"TradeHistSVG":   template.HTML(profitHistSVG),
		"MonthlyKeys":    monthlyKeys,
		"YearlyKeys":     yearlyKeys,
		"RegimeTable":    regimeTable,
		"EquityJSON":     template.JS(equityJSON),
		"TradesJSON":     template.JS(tradesJSON),
		"BarsJSON":       template.JS(barsJSON),
		"ReportTheme":    in.Config.Report.Theme,
		"InitialCapital": in.Result.InitialCapital,
		"FinalEquity":    in.Result.FinalEquity,
		"Interval":       in.Config.General.Interval,
		"AvgLev":         in.Result.AvgLeverage,
		"MaxLev":         in.Result.MaxLeverageUsed,
		"LevCap":         lim.MaxLeverage,
		"AvgRisk":        in.Result.AvgRiskPct,
		"MaxRiskUsed":    in.Result.MaxRiskPctUsed,
		"MaxHeat":        in.Result.MaxHeatSeen,
		"HeatLimit":      lim.MaxHeatPct,
		"DDStart":        lim.DDDeleverageStart,
		"DDFlat":         lim.DDFlatPct,
	}
	tmpl := template.Must(template.New("report").Funcs(template.FuncMap{
		"fmtFloat": func(v float64) string {
			if math.IsNaN(v) {
				return "—"
			}
			if math.IsInf(v, 1) {
				return "∞"
			}
			if math.IsInf(v, -1) {
				return "-∞"
			}
			return fmt.Sprintf("%.2f", v)
		},
		"fmtPct": func(v float64) string {
			if math.IsNaN(v) {
				return "—"
			}
			return fmt.Sprintf("%.2f%%", v)
		},
		"fmtMoney": func(v float64) string { return fmt.Sprintf("$%.2f", v) },
		"sideStr": func(s int) string {
			if s == 1 {
				return "LONG"
			}
			if s == -1 {
				return "SHORT"
			}
			return "FLAT"
		},
		"sideColor": func(s int) string {
			if s == 1 {
				return "#22c55e"
			}
			if s == -1 {
				return "#ef4444"
			}
			return "#888"
		},
		"isPos": func(v float64) bool { return v > 0 && !math.IsNaN(v) },
		"indexFloat": func(m map[string]float64, k string) float64 {
			if m == nil {
				return math.NaN()
			}
			return m[k]
		},
	}).Parse(reportHTML))
	return tmpl.Execute(f, dataMap)
}

// GenerateComparison HTML for A/B/C/D
func GenerateComparison(path string, rows []ComparisonRow, cfg *config.Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	// sort by Sharpe desc
	sort.Slice(rows, func(i, j int) bool { return rows[i].Stats.Sharpe > rows[j].Stats.Sharpe })
	dataMap := map[string]interface{}{
		"Title":       "ATPS Comparison — A/B/C/D",
		"Rows":        rows,
		"GeneratedAt": time.Now().Format("2006-01-02 15:04 MST"),
		"Config":      cfg,
	}
	tmpl := template.Must(template.New("cmp").Funcs(template.FuncMap{
		"fmtFloat": func(v float64) string {
			if math.IsNaN(v) {
				return "—"
			}
			return fmt.Sprintf("%.2f", v)
		},
		"fmtPct": func(v float64) string {
			if math.IsNaN(v) {
				return "—"
			}
			return fmt.Sprintf("%.2f%%", v)
		},
	}).Parse(comparisonHTML))
	return tmpl.Execute(f, dataMap)
}

// SVG helpers

func svgEquity(equity []backtest.EquityPoint, w, h int) string {
	if len(equity) == 0 {
		return "<div>no data</div>"
	}
	minE, maxE := equity[0].Equity, equity[0].Equity
	for _, e := range equity {
		if e.Equity < minE {
			minE = e.Equity
		}
		if e.Equity > maxE {
			maxE = e.Equity
		}
	}
	pad := (maxE - minE) * 0.1
	if pad == 0 {
		pad = maxE * 0.02
	}
	minE -= pad
	maxE += pad
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`<svg viewBox="0 0 %d %d" width="100%%" height="%d" style="background:#111827;border-radius:8px">`, w, h, h))
	// grid
	for i := 0; i <= 4; i++ {
		y := float64(h-20)*float64(i)/4 + 10
		sb.WriteString(fmt.Sprintf(`<line x1="40" y1="%.1f" x2="%d" y2="%.1f" stroke="#1f2937" stroke-width="1"/>`, y, w-10, y))
	}
	// polyline equity
	sb.WriteString(`<polyline fill="none" stroke="#60a5fa" stroke-width="2" points="`)
	for i, e := range equity {
		x := 40 + float64(w-50)*float64(i)/float64(len(equity)-1)
		y := 10 + (maxE-e.Equity)/(maxE-minE)*float64(h-30)
		if i > 0 {
			sb.WriteString(" ")
		}
		sb.WriteString(fmt.Sprintf("%.1f,%.1f", x, y))
	}
	sb.WriteString(`"/>`)
	// bh line? simplified no
	sb.WriteString(fmt.Sprintf(`<text x="10" y="15" fill="#9ca3af" font-size="10">%.0f</text>`, maxE))
	sb.WriteString(fmt.Sprintf(`<text x="10" y="%d" fill="#9ca3af" font-size="10">%.0f</text>`, h-5, minE))
	sb.WriteString(`</svg>`)
	return sb.String()
}
func svgDrawdown(equity []backtest.EquityPoint, w, h int) string {
	if len(equity) == 0 {
		return ""
	}
	minDD := 0.0
	maxDD := 0.0
	for _, e := range equity {
		if e.Drawdown < minDD {
			minDD = e.Drawdown
		}
		if e.Drawdown > maxDD {
			maxDD = e.Drawdown
		}
	}
	if minDD == 0 {
		minDD = -1
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`<svg viewBox="0 0 %d %d" width="100%%" height="%d" style="background:#1f0f0f;border-radius:8px">`, w, h, h))
	// baseline 0 at top
	sb.WriteString(fmt.Sprintf(`<line x1="40" y1="10" x2="%d" y2="10" stroke="#374151" stroke-width="1"/>`, w-10))
	// drawdown as bars
	barW := float64(w-50) / float64(len(equity))
	for i, e := range equity {
		x := 40 + float64(i)*barW
		dd := e.Drawdown
		height := math.Abs(dd) / math.Abs(minDD) * float64(h-20)
		y := 10.0
		if dd < 0 {
			sb.WriteString(fmt.Sprintf(`<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="#f87171" opacity="0.85"/>`, x, y, math.Max(1, barW-0.5), height))
		}
	}
	sb.WriteString(fmt.Sprintf(`<text x="10" y="15" fill="#fca5a5" font-size="10">0%%</text>`))
	sb.WriteString(fmt.Sprintf(`<text x="10" y="%d" fill="#fca5a5" font-size="10">%.1f%%</text>`, h-5, minDD))
	sb.WriteString(`</svg>`)
	return sb.String()
}
func svgTradeHist(trades []backtest.Trade, w, h int) string {
	if len(trades) == 0 {
		return "<div style='color:#9ca3af'>no trades</div>"
	}
	// histogram of PnL
	minP, maxP := trades[0].PnLNet, trades[0].PnLNet
	for _, t := range trades {
		if t.PnLNet < minP {
			minP = t.PnLNet
		}
		if t.PnLNet > maxP {
			maxP = t.PnLNet
		}
	}
	bins := 20
	if len(trades) < 20 {
		bins = 10
	}
	counts := make([]int, bins)
	rangeW := (maxP - minP)
	if rangeW == 0 {
		rangeW = 1
	}
	for _, t := range trades {
		idx := int((t.PnLNet - minP) / rangeW * float64(bins-1))
		if idx < 0 {
			idx = 0
		}
		if idx >= bins {
			idx = bins - 1
		}
		counts[idx]++
	}
	maxC := 0
	for _, c := range counts {
		if c > maxC {
			maxC = c
		}
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`<svg viewBox="0 0 %d %d" width="100%%" height="%d" style="background:#111827;border-radius:8px">`, w, h, h))
	barW := float64(w-40) / float64(bins)
	for i, c := range counts {
		height := float64(c) / float64(maxC) * float64(h-30)
		x := 20 + float64(i)*barW
		y := float64(h-15) - height
		color := "#60a5fa"
		if i < bins/2 {
			color = "#f87171"
		} else {
			color = "#34d399"
		}
		sb.WriteString(fmt.Sprintf(`<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s"/>`, x, y, barW-2, height, color))
	}
	sb.WriteString(`</svg>`)
	return sb.String()
}

type RegimeRow struct {
	Regime  string
	Trades  int
	WinRate float64
	AvgPnL  float64
}

func regimeBreakdown(res *backtest.Result, bars data.Bars) []RegimeRow {
	// simple: split by month or by volatility? Here split by long/short and by quarter
	longWins, longCnt, shortWins, shortCnt := 0, 0, 0, 0
	var longPnL, shortPnL float64
	for _, t := range res.Trades {
		if t.Side == 1 {
			longCnt++
			longPnL += t.PnLNet
			if t.PnLNet > 0 {
				longWins++
			}
		} else if t.Side == -1 {
			shortCnt++
			shortPnL += t.PnLNet
			if t.PnLNet > 0 {
				shortWins++
			}
		}
	}
	var out []RegimeRow
	if longCnt > 0 {
		out = append(out, RegimeRow{Regime: "LONG", Trades: longCnt, WinRate: float64(longWins) / float64(longCnt) * 100, AvgPnL: longPnL / float64(longCnt)})
	}
	if shortCnt > 0 {
		out = append(out, RegimeRow{Regime: "SHORT", Trades: shortCnt, WinRate: float64(shortWins) / float64(shortCnt) * 100, AvgPnL: shortPnL / float64(shortCnt)})
	}
	// by year
	yMap := map[string][]backtest.Trade{}
	for _, t := range res.Trades {
		y := t.EntryTime.Format("2006")
		yMap[y] = append(yMap[y], t)
	}
	for y, ts := range yMap {
		w := 0
		sum := 0.0
		for _, t := range ts {
			sum += t.PnLNet
			if t.PnLNet > 0 {
				w++
			}
		}
		out = append(out, RegimeRow{Regime: "Year " + y, Trades: len(ts), WinRate: float64(w) / float64(len(ts)) * 100, AvgPnL: sum / float64(len(ts))})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Regime < out[j].Regime })
	return out
}

const reportHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{.Title}}</title>
<style>
:root{--bg:#0b0e14;--card:#111827;--card2:#1f2937;--text:#e5e7eb;--muted:#9ca3af;--accent:#60a5fa;--green:#22c55e;--red:#ef4444;--yellow:#eab308}
*{box-sizing:border-box}body{margin:0;font-family:Inter,ui-sans-serif,system-ui,Segoe UI,Roboto,sans-serif;background:var(--bg);color:var(--text);line-height:1.45}
header{padding:22px 24px;border-bottom:1px solid #1f2937;background:linear-gradient(180deg,#0f172a,#0b0e14)}
h1{margin:0;font-size:22px;font-weight:700;letter-spacing:-0.02em}
.sub{color:var(--muted);font-size:13px;margin-top:6px}
.wrap{max-width:1280px;margin:0 auto;padding:18px}
.grid{display:grid;grid-template-columns:repeat(12,1fr);gap:14px}
.card{background:var(--card);border:1px solid #1f2937;border-radius:12px;padding:16px}
.kpi{grid-column:span 3;text-align:center}
.kpi .label{font-size:11px;color:var(--muted);text-transform:uppercase;letter-spacing:0.08em}
.kpi .value{font-size:24px;font-weight:800;margin:6px 0}
.kpi .delta{font-size:12px}
.pill{display:inline-block;padding:3px 8px;border-radius:999px;font-size:11px;font-weight:600;border:1px solid #334155}
table{width:100%;border-collapse:collapse;font-size:13px}
th,td{padding:8px 10px;border-bottom:1px solid #1f2937;text-align:left}
th{color:var(--muted);font-size:11px;text-transform:uppercase;letter-spacing:0.06em;background:#0f172a;position:sticky;top:0}
tr:hover td{background:#0f172a}
.badge{padding:2px 6px;border-radius:6px;font-size:11px;font-weight:700}
.badge-long{background:rgba(34,197,94,0.15);color:#4ade80;border:1px solid rgba(34,197,94,0.3)}
.badge-short{background:rgba(239,68,68,0.15);color:#f87171;border:1px solid rgba(239,68,68,0.3)}
.heat{display:grid;grid-template-columns:repeat(13,1fr);gap:4px}
.heat div{height:28px;border-radius:6px;display:flex;align-items:center;justify-content:center;font-size:11px;font-weight:700}
.section-title{font-size:13px;font-weight:700;letter-spacing:0.04em;text-transform:uppercase;color:#cbd5e1;margin:0 0 10px 0}
.small{font-size:12px;color:var(--muted)}
.svgbox{overflow:hidden}
.footer{padding:18px;color:var(--muted);font-size:12px;border-top:1px solid #1f2937;margin-top:18px}
code{background:#0f172a;padding:2px 6px;border-radius:6px;font-size:12px;border:1px solid #1f2937}
@media(max-width:900px){.kpi{grid-column:span 6}.grid{grid-template-columns:repeat(6,1fr)}}
</style>
<script src="https://unpkg.com/lightweight-charts@4.1.3/dist/lightweight-charts.standalone.production.js"></script>
</head>
<body>
<header>
  <h1>{{.Title}}</h1>
  <div class="sub">Generated {{.GeneratedAt}} • Interval {{.Interval}} • Initial ${{.InitialCapital}} → Final ${{.FinalEquity}} • Variant {{.Variant}} • Bars {{len .EquityCurve}} • Trades {{.AllTradesCount}}</div>
  <div style="margin-top:10px;display:flex;gap:8px;flex-wrap:wrap">
    <span class="pill">Symbol: {{.Symbol}}</span>
    <span class="pill">Variant: {{.Variant}}</span>
    <span class="pill">Fee: {{.Config.Costs.FeeBps}} bps</span>
    <span class="pill">Slippage: {{.Config.Costs.SlippageBps}} bps</span>
    <span class="pill">Leverage: {{.Config.Costs.Leverage}}x</span>
    <span class="pill">Pyramiding: {{.Config.Backtest.PyramidingMaxUnits}}×</span>
  </div>
</header>
<div class="wrap">
  <!-- KPI row -->
  <div class="grid" style="margin-bottom:14px">
    <div class="card kpi"><div class="label">Return netto</div><div class="value" style="color:{{if isPos .Stats.ReturnPct}}var(--green){{else}}var(--red){{end}}">{{fmtPct .Stats.ReturnPct}}</div><div class="delta small">CAGR {{fmtPct .Stats.ReturnAnnual}} • BH {{fmtPct .Stats.BuyHoldReturn}}</div></div>
    <div class="card kpi"><div class="label">Sharpe / Sortino / Calmar</div><div class="value">{{fmtFloat .Stats.Sharpe}} / {{fmtFloat .Stats.Sortino}} / {{fmtFloat .Stats.Calmar}}</div><div class="delta small">Vol ann {{fmtPct .Stats.VolatilityAnn}} • SQN {{fmtFloat .Stats.SQN}}</div></div>
    <div class="card kpi"><div class="label">Max DD</div><div class="value" style="color:var(--red)">{{fmtPct .Stats.MaxDD}}</div><div class="delta small">{{.Stats.MaxDDDurationBars}} bars underwater • Ulcer {{fmtFloat .Stats.UlcerIndex}}</div></div>
    <div class="card kpi"><div class="label">WinRate • PF • Trades</div><div class="value">{{fmtPct .Stats.WinRate}} • {{fmtFloat .Stats.ProfitFactor}}</div><div class="delta small">{{.Stats.Trades}} trades • {{fmtFloat .Stats.AnnualTrades}}/y • PF {{fmtFloat .Stats.PayoffRatio}}</div></div>
  </div>

  <div class="grid">
    <div class="card" style="grid-column:span 8">
      <div class="section-title">Equity curve (net) — price overlay in lightweight-charts below</div>
      <div class="svgbox">{{.EquitySVG}}</div>
      <div id="lwc-equity" style="height:280px;margin-top:12px;border-radius:8px;overflow:hidden;background:#111827"></div>
      <div class="small" style="margin-top:8px">Equity net includes fee, slippage, funding. Lightweight-Charts 4.1 renders OHLC + equity + volume + drawdown.</div>
    </div>
    <div class="card" style="grid-column:span 4">
      <div class="section-title">Drawdown %</div>
      <div class="svgbox">{{.DrawdownSVG}}</div>
      <div style="margin-top:14px" class="section-title">Trade PnL distribution</div>
      {{.TradeHistSVG}}
      <div style="margin-top:12px;display:grid;grid-template-columns:1fr 1fr;gap:8px">
        <div style="background:#0f172a;padding:10px;border-radius:8px;border:1px solid #1f2937"><div class="small">Best / Worst</div><div style="font-weight:800;color:var(--green)">${{fmtFloat .Stats.BestTrade}} / <span style="color:var(--red)">${{fmtFloat .Stats.WorstTrade}}</span></div></div>
        <div style="background:#0f172a;padding:10px;border-radius:8px;border:1px solid #1f2937"><div class="small">Avg Win / Loss</div><div style="font-weight:800">${{fmtFloat .Stats.AvgWin}} / ${{fmtFloat .Stats.AvgLoss}}</div></div>
        <div style="background:#0f172a;padding:10px;border-radius:8px;border:1px solid #1f2937"><div class="small">Expectancy</div><div style="font-weight:800">${{fmtFloat .Stats.Expectancy}}</div></div>
        <div style="background:#0f172a;padding:10px;border-radius:8px;border:1px solid #1f2937"><div class="small">Exposure</div><div style="font-weight:800">{{fmtPct .Stats.ExposurePct}}</div></div>
      </div>
    </div>
  </div>

  <div class="grid" style="margin-top:14px">
    <div class="card" style="grid-column:span 7">
      <div class="section-title">Prestazioni complete — 32 metriche</div>
      <div style="max-height:420px;overflow:auto;border:1px solid #1f2937;border-radius:8px">
      <table>
        <tr><th>Metrica</th><th>Valore</th><th>Metrica</th><th>Valore</th></tr>
        <tr><td>Return netto</td><td><b>{{fmtPct .Stats.ReturnPct}}</b></td><td>CAGR</td><td>{{fmtPct .Stats.ReturnAnnual}}</td></tr>
        <tr><td>Buy&Hold</td><td>{{fmtPct .Stats.BuyHoldReturn}}</td><td>Alpha (approx)</td><td>{{fmtFloat .Stats.Alpha}}</td></tr>
        <tr><td>Sharpe</td><td>{{fmtFloat .Stats.Sharpe}}</td><td>Sortino</td><td>{{fmtFloat .Stats.Sortino}}</td></tr>
        <tr><td>Calmar</td><td>{{fmtFloat .Stats.Calmar}}</td><td>Vol ann</td><td>{{fmtPct .Stats.VolatilityAnn}}</td></tr>
        <tr><td>Max DD</td><td style="color:var(--red)">{{fmtPct .Stats.MaxDD}}</td><td>Duration DD</td><td>{{.Stats.MaxDDDurationBars}} bars</td></tr>
        <tr><td>Ulcer Index</td><td>{{fmtFloat .Stats.UlcerIndex}}</td><td>Beta</td><td>{{fmtFloat .Stats.Beta}}</td></tr>
        <tr><td>WinRate</td><td>{{fmtPct .Stats.WinRate}} ({{.Stats.Winners}}W / {{.Stats.Losers}}L)</td><td>Profit Factor</td><td>{{fmtFloat .Stats.ProfitFactor}}</td></tr>
        <tr><td>Payoff</td><td>{{fmtFloat .Stats.PayoffRatio}}</td><td>Expectancy</td><td>${{fmtFloat .Stats.Expectancy}} ({{fmtFloat .Stats.ExpectancyR}}R)</td></tr>
        <tr><td>Avg Trade</td><td>${{fmtFloat .Stats.AvgTrade}}</td><td>Avg Bars Held</td><td>{{fmtFloat .Stats.AvgBarsHeld}}</td></tr>
        <tr><td>Best / Worst</td><td>${{fmtFloat .Stats.BestTrade}} / ${{fmtFloat .Stats.WorstTrade}}</td><td>SQN</td><td>{{fmtFloat .Stats.SQN}}</td></tr>
        <tr><td>Kelly %</td><td>{{fmtPct .Stats.KellyPct}}</td><td>Exposure</td><td>{{fmtPct .Stats.ExposurePct}}</td></tr>
        <tr style="background:rgba(99,102,241,0.12)"><td><b>Skew (trades)</b></td><td><b>{{fmtFloat .Stats.Skew}}</b></td><td><b>Skew R</b></td><td><b>{{fmtFloat .Stats.SkewR}}</b> {{if isPos .Stats.SkewR}}<span style="color:#4ade80">✓ positive</span>{{else}}<span style="color:#f87171">✗ negative</span>{{end}}</td></tr>
        <tr style="background:rgba(34,197,94,0.12)"><td><b>Expectancy R</b></td><td><b>{{fmtFloat .Stats.ExpectancyR}}R</b></td><td><b>PosSkewScore</b></td><td><b>{{fmtFloat .Stats.PosSkewScore}}</b> <span class="small">E×Skew×(1+WR)</span></td></tr>
        <tr><td>Median R</td><td>{{fmtFloat .Stats.MedianR}}R</td><td>Tail Ratio (p95/|p05|)</td><td>{{fmtFloat .Stats.TailRatio}}</td></tr>
        <tr><td>Gross PnL</td><td>${{fmtFloat .Stats.GrossPnL}}</td><td>Net PnL</td><td>${{fmtFloat .Stats.NetPnL}}</td></tr>
        <tr><td>Fee</td><td>${{fmtFloat .Stats.TotalFee}} ({{fmtPct .Stats.FeeDragPct}} drag)</td><td>Funding</td><td>${{fmtFloat .Stats.TotalFunding}} ({{fmtPct .Stats.FundingDragPct}} drag)</td></tr>
        <tr><td>Trades / anno</td><td>{{fmtFloat .Stats.AnnualTrades}}</td><td>Duration</td><td>{{fmtFloat .Stats.DurationDays}} gg</td></tr>
      </table>
      </div>
    </div>
    <div class="card" style="grid-column:span 5">
      <div class="section-title">Breakdown regime & direzione</div>
      <table>
        <tr><th>Regime</th><th>Trades</th><th>WinRate</th><th>Avg PnL</th></tr>
        {{range .RegimeTable}}<tr><td>{{.Regime}}</td><td>{{.Trades}}</td><td>{{fmtPct .WinRate}}</td><td>${{fmtFloat .AvgPnL}}</td></tr>{{end}}
      </table>
      <div style="margin-top:14px" class="section-title">Costi</div>
      <div style="display:grid;grid-template-columns:1fr 1fr;gap:8px">
        <div class="card" style="background:#0f172a"><div class="small">Fee totali</div><div style="font-size:18px;font-weight:800">${{fmtFloat .Stats.TotalFee}}</div><div class="small">{{fmtPct .Stats.FeeDragPct}} del gross</div></div>
        <div class="card" style="background:#0f172a"><div class="small">Funding</div><div style="font-size:18px;font-weight:800">${{fmtFloat .Stats.TotalFunding}}</div><div class="small">{{fmtPct .Stats.FundingDragPct}} del gross</div></div>
      </div>
      <div class="small" style="margin-top:8px">Funding 8h scalato su interval {{.Interval}}. Long paga se rate>0.</div>
    </div>
  </div>

  <div class="card" style="margin-top:14px">
    <div class="section-title">Monthly returns % — heatmap</div>
    <div style="overflow:auto">
      <table>
        <tr><th>Mese</th><th>Ritorno</th></tr>
        {{range .MonthlyKeys}}<tr><td>{{.}}</td><td style="font-weight:700;color:{{if isPos (indexFloat $.Stats.MonthlyReturns .)}}#4ade80{{else}}#f87171{{end}}">{{fmtPct (index $.Stats.MonthlyReturns .)}}</td></tr>{{end}}
      </table>
    </div>
    <div style="margin-top:10px" class="section-title">Yearly</div>
    <div style="display:flex;gap:6px;flex-wrap:wrap">
      {{range .YearlyKeys}}<span class="pill" style="background:{{if isPos (indexFloat $.Stats.YearlyReturns .)}}rgba(34,197,94,0.2){{else}}rgba(239,68,68,0.2){{end}}">{{.}}: {{fmtPct (index $.Stats.YearlyReturns .)}}</span>{{end}}
    </div>
  </div>

  <div class="card" style="margin-top:14px">
    <div class="section-title">Risk &amp; Sizing — leva dinamica, rischio per trade (max {{fmtFloat .MaxRiskUsed}}%)</div>
    <div style="display:grid;grid-template-columns:repeat(6,1fr);gap:8px">
      <div style="background:#0f172a;padding:10px;border-radius:8px;border:1px solid #1f2937"><div class="small">Leva media</div><div style="font-size:18px;font-weight:800">{{fmtFloat .AvgLev}}×</div></div>
      <div style="background:#0f172a;padding:10px;border-radius:8px;border:1px solid #1f2937"><div class="small">Leva max usata</div><div style="font-size:18px;font-weight:800;color:var(--yellow)">{{fmtFloat .MaxLev}}×</div></div>
      <div style="background:#0f172a;padding:10px;border-radius:8px;border:1px solid #1f2937"><div class="small">Hard cap leva</div><div style="font-size:18px;font-weight:800">{{fmtFloat .LevCap}}×</div></div>
      <div style="background:#0f172a;padding:10px;border-radius:8px;border:1px solid #1f2937"><div class="small">Rischio medio/trade</div><div style="font-size:18px;font-weight:800">{{fmtPct .AvgRisk}}</div></div>
      <div style="background:#0f172a;padding:10px;border-radius:8px;border:1px solid #1f2937"><div class="small">Rischio max/trade</div><div style="font-size:18px;font-weight:800;color:var(--yellow)">{{fmtPct .MaxRiskUsed}}</div></div>
      <div style="background:#0f172a;padding:10px;border-radius:8px;border:1px solid #1f2937"><div class="small">Heat max (portafoglio)</div><div style="font-size:18px;font-weight:800">{{fmtPct .MaxHeat}}</div></div>
    </div>
    <div class="small" style="margin-top:8px">Sizing: <code>qty = (equity × risk%) / |entry − stop|</code> — leva <b>derivata</b> (notional/equity), mai fissa. Cap dinamico: vol regime ×0.50/0.75/1.20, ADX&lt;18 ×0.60, |funding z|&gt;2 ×0.70, drawdown de-leverage da {{fmtFloat .DDStart}}% a flat {{fmtFloat .DDFlat}}%. Heat budget {{fmtPct .HeatLimit}}.</div>
  </div>

  <div class="card" style="margin-top:14px">
    <div class="section-title">Trades — dettagliato (ultimi {{len .Trades}} di {{.AllTradesCount}}) — MT5 style</div>
    <div style="max-height:420px;overflow:auto;border:1px solid #1f2937;border-radius:8px">
    <table>
      <tr><th>#</th><th>Entry</th><th>Exit</th><th>Side</th><th>Entry</th><th>Exit</th><th>Qty</th><th>Lev</th><th>Risk%</th><th>Bars</th><th>MAE</th><th>MFE</th><th>Fee</th><th>Funding</th><th>PnL net</th><th>R</th><th>Reason</th></tr>
      {{range $i, $t := .Trades}}<tr>
        <td>{{$i}}</td>
        <td>{{$t.EntryTime.Format "2006-01-02 15:04"}}</td>
        <td>{{$t.ExitTime.Format "2006-01-02 15:04"}}</td>
        <td><span class="badge {{if eq $t.Side 1}}badge-long{{else}}badge-short{{end}}">{{sideStr $t.Side}}</span></td>
        <td>{{fmtFloat $t.EntryPrice}}</td>
        <td>{{fmtFloat $t.ExitPrice}}</td>
        <td>{{fmtFloat $t.Qty}}</td>
        <td style="color:var(--yellow)">{{fmtFloat $t.Leverage}}×</td>
        <td style="color:var(--accent)">{{fmtPct $t.RiskPct}}</td>
        <td>{{$t.BarsHeld}}</td>
        <td style="color:var(--red)">{{fmtPct $t.MAE}}</td>
        <td style="color:var(--green)">{{fmtPct $t.MFE}}</td>
        <td>${{fmtFloat $t.Fee}}</td>
        <td>${{fmtFloat $t.FundingCost}}</td>
        <td style="font-weight:800;color:{{if isPos $t.PnLNet}}var(--green){{else}}var(--red){{end}}">${{fmtFloat $t.PnLNet}}</td>
        <td style="font-weight:700">{{fmtFloat $t.RMultiple}}R</td>
        <td class="small">{{$t.ExitReason}}</td>
      </tr>{{end}}
    </table>
    </div>
  </div>

  <div class="card" style="margin-top:14px">
    <div class="section-title">Dati grezzi & config (reproducibilità)</div>
    <details><summary class="small" style="cursor:pointer">Mostra barre JSON ({{len .EquityCurve}})</summary><pre style="max-height:220px;overflow:auto;background:#0f172a;padding:12px;border-radius:8px;font-size:11px"><code id="bars-json">{{.BarsJSON}}</code></pre></details>
    <details><summary class="small" style="cursor:pointer">Mostra equity JSON</summary><pre style="max-height:220px;overflow:auto;background:#0f172a;padding:12px;border-radius:8px;font-size:11px"><code>{{.EquityJSON}}</code></pre></details>
    <details open><summary class="small" style="cursor:pointer">Config YAML</summary><pre style="background:#0f172a;padding:12px;border-radius:8px;font-size:11px;white-space:pre-wrap">{{.Config}}</pre></details>
  </div>

  <div class="footer">ATPS Adaptive Turtle Perpetual System (Go) • Binance dati (fapi + OI) • Orderly esecuzione isolata • Report self-contained. Not financial advice. Dati sintetici se demo.</div>
</div>

<script>
// Lightweight charts equity + price dual pane
const equity = {{.EquityJSON}};
const barsRaw = {{.BarsJSON}};
// map bars to lightweight format
const candles = barsRaw.map(b=>({time: Math.floor(new Date(b.time).getTime()/1000), open:b.open, high:b.high, low:b.low, close:b.close}));
const vols = barsRaw.map(b=>({time: Math.floor(new Date(b.time).getTime()/1000), value:b.volume, color: b.close>=b.open?'rgba(34,197,94,0.5)':'rgba(239,68,68,0.5)'}));
const eqSeries = equity.map(e=>({time: Math.floor(new Date(e.time).getTime()/1000), value:e.equity}));
const ddSeries = equity.map(e=>({time: Math.floor(new Date(e.time).getTime()/1000), value:e.drawdown, color: e.drawdown<0?'#f87171':'#22c55e'}));
try{
  const el=document.getElementById('lwc-equity');
  if(el && window.LightweightCharts && candles.length>5){
    const chart=LightweightCharts.createChart(el,{width: el.clientWidth, height:280, layout:{background:{color:'#111827'},textColor:'#9ca3af'}, grid:{vertLines:{color:'#1f2937'},horzLines:{color:'#1f2937'}}, timeScale:{borderColor:'#334155'}});
    const candleSeries=chart.addCandlestickSeries({upColor:'#22c55e',downColor:'#ef4444',borderVisible:false,wickUpColor:'#22c55e',wickDownColor:'#ef4444'});
    candleSeries.setData(candles);
    // overlay equity on same chart via line series (right price scale)
    const eqLine=chart.addLineSeries({color:'#60a5fa',lineWidth:2, priceScaleId:'right'});
    eqLine.setData(eqSeries);
    chart.priceScale('right').applyOptions({borderColor:'#334155'});
    chart.timeScale().fitContent();
    window.addEventListener('resize',()=>chart.applyOptions({width: el.clientWidth}));
  }
}catch(e){console.log('LWC error',e)}
</script>
</body>
</html>`

const comparisonHTML = `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>{{.Title}}</title>
<style>body{margin:0;font-family:Inter,system-ui,sans-serif;background:#0b0e14;color:#e5e7eb}header{padding:22px;border-bottom:1px solid #1f2937;background:#0f172a}h1{margin:0} table{width:100%;border-collapse:collapse;font-size:13px}th,td{padding:8px 10px;border-bottom:1px solid #1f2937}th{color:#9ca3af;text-transform:uppercase;font-size:11px;background:#111827} .pill{display:inline-block;padding:3px 8px;border-radius:999px;border:1px solid #334155;font-size:11px} .wrap{max-width:1280px;margin:0 auto;padding:18px} .card{background:#111827;border:1px solid #1f2937;border-radius:12px;padding:16px} .best{background:rgba(34,197,94,0.12)} </style>
</head><body>
<header><h1>{{.Title}}</h1><div style="color:#9ca3af;font-size:13px">{{.GeneratedAt}} — Ordinato per Sharpe desc • Fee {{.Config.Costs.FeeBps}} bps • Funding incluso</div></header>
<div class="wrap">
<div class="card">
<table>
<tr><th>Rank</th><th>Variant</th><th>Symbol</th><th>Return</th><th>CAGR</th><th>Sharpe</th><th>Sortino</th><th>Calmar</th><th>MaxDD</th><th>WinRate</th><th>PF</th><th>Trades</th><th>Exposure</th><th>Fee drag</th><th>Funding</th></tr>
{{range $i, $r := .Rows}}<tr class="{{if eq $i 0}}best{{end}}">
<td>{{$i}}</td><td><b>{{$r.Variant}}</b> — {{$r.Stats.Variant}}</td><td>{{$r.Symbol}}</td>
<td>{{fmtPct $r.Stats.ReturnPct}}</td><td>{{fmtPct $r.Stats.ReturnAnnual}}</td>
<td>{{fmtFloat $r.Stats.Sharpe}}</td><td>{{fmtFloat $r.Stats.Sortino}}</td><td>{{fmtFloat $r.Stats.Calmar}}</td>
<td style="color:#f87171">{{fmtPct $r.Stats.MaxDD}}</td>
<td>{{fmtPct $r.Stats.WinRate}}</td><td>{{fmtFloat $r.Stats.ProfitFactor}}</td><td>{{$r.Stats.Trades}}</td>
<td>{{fmtPct $r.Stats.ExposurePct}}</td><td>{{fmtPct $r.Stats.FeeDragPct}}</td><td>${{fmtFloat $r.Stats.TotalFunding}}</td>
</tr>{{end}}
</table>
</div>
<div class="card" style="margin-top:14px"><div style="font-size:13px;font-weight:700;text-transform:uppercase;color:#cbd5e1">Note</div><div style="color:#9ca3af;font-size:12px;margin-top:6px">Verifica: su dati sintetici la gerarchia A/B/C/D dipende dal seed. Su dati reali BTC/ETH/SOL atteso D > C > B > A se i filtri OI/funding hanno edge. Walk-forward e MonteCarlo vanno lanciati per validare.</div></div>
</div>
</body></html>`
