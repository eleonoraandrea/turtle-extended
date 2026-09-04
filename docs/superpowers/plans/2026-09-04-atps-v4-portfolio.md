# ATPS v4 — Portfolio engine vero (Implementation Plan)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Engine multi-simbolo BTC+ETH+SOL con equity e heat condivisi, validato col protocollo (baseline atps_v2 BTC 34.31%/-17.01, holdout 12.4/Cal 0.73), con risk re-scaling condizionato (2.5%/3% solo se DD headroom) e README aggiornato (incl. link morti post-cancellazione report).

**Architecture:** (1) `RunPortfolio` in nuovo file che adatta il loop di `engine.Run` a timeline-union multi-simbolo con UNA equity, heat/correlati condivisi (la cap logic esiste già in `risk.Size`), per-simbolo strategie/posizioni/brake; invariante forte: single-symbol == engine.Run; (2) CLI `portfolio-backtest` + `scripts/portfolio_split` (holdout + WF 8 folds); (3) validazione stage-1 risk 2% e stage-2 condizionale; (4) README.

**Tech Stack:** Go, yaml.v3, cobra (cmd), `go test`. Spec: `docs/superpowers/specs/2026-09-04-atps-v4-portfolio-design.md`.

**Convenzioni:** repo root, branch main autorizzato, `gofmt -l .` vuoto + `go vet ./...` + `go test ./...` verdi + commit per task. Baseline numeri: v2 BTC 34.31/-17.01/1.50/2.14/416 (test-window 12.4/-16.9/Cal 0.73); v2 ETH 20.66/-16.14; v2 SOL 6.01/-17.13.

---

### Task 1: `RunPortfolio` — engine multi-simbolo (invariante + test sintetici)

**Files:**
- Create: `internal/backtest/engine_portfolio.go`
- Test: `internal/backtest/engine_portfolio_test.go` (create)

Contesto obbligatorio prima di scrivere: LEGGERE `internal/backtest/engine.go` per intero (loop `Run`: funding → exits → crash brake → mark-to-market/peak/dd → guard brake → intrabar/Next/ReEntry signals → fill → sizing → pyramiding/fresh-entry → same-bar stop → equity point; chiusure EOD; aggregati). Il portfolio replica QUESTA semantica per-simbolo con equity/peak condivisi.

- [ ] **Step 1: Scrivi i test falliti**

Crea `internal/backtest/engine_portfolio_test.go`:

```go
package backtest

import (
	"math"
	"sort"
	"testing"
	"time"

	"github.com/atps/atps/internal/config"
	"github.com/atps/atps/internal/data"
	"github.com/atps/atps/internal/strategy"
)

// Test invariante: RunPortfolio con UN simbolo deve riprodurre engine.Run
func TestRunPortfolioSingleSymbolInvariantBTC(t *testing.T) {
	cfg, err := config.Load("../../configs/atps_v2.yaml")
	if err != nil {
		t.Fatal(err)
	}
	bars, err := data.LoadBarsCSV("../../data/raw/BTCUSDT_4h.csv")
	if err != nil {
		t.Fatal(err)
	}
	strat := strategy.New("A", cfg)
	eng := EngineConfigFrom(cfg, "A", "BTCUSDT")
	single := Run(bars, strat, cfg, eng)
	port := RunPortfolio(map[string]data.Bars{"BTCUSDT": bars}, map[string]strategy.Strategy{"BTCUSDT": strat}, cfg, eng)
	if len(port.Trades) != len(single.Trades) {
		t.Fatalf("trades %d != %d", len(port.Trades), len(single.Trades))
	}
	if math.Abs(port.FinalEquity-single.FinalEquity) > 1e-6 {
		t.Errorf("FinalEquity %.6f != %.6f", port.FinalEquity, single.FinalEquity)
	}
	for i := range single.Trades {
		a, b := &single.Trades[i], &port.Trades[i]
		if a.Symbol != b.Symbol || a.EntryTime != b.EntryTime || a.ExitTime != b.ExitTime ||
			math.Abs(a.PnLNet-b.PnLNet) > 1e-9 || a.ExitReason != b.ExitReason {
			t.Errorf("trade[%d] diverge: %+v vs %+v", i, a, b)
		}
	}
	if len(port.Equity) != len(single.Equity) {
		t.Errorf("equity points %d != %d", len(port.Equity), len(single.Equity))
	}
}

func TestRunPortfolioSingleSymbolInvariantETH(t *testing.T) {
	cfg, err := config.Load("../../configs/atps_v2.yaml")
	if err != nil {
		t.Fatal(err)
	}
	bars, err := data.LoadBarsCSV("../../data/raw/ETHUSDT_4h.csv")
	if err != nil {
		t.Fatal(err)
	}
	strat := strategy.New("A", cfg)
	eng := EngineConfigFrom(cfg, "A", "ETHUSDT")
	single := Run(bars, strat, cfg, eng)
	port := RunPortfolio(map[string]data.Bars{"ETHUSDT": bars}, map[string]strategy.Strategy{"ETHUSDT": strat}, cfg, eng)
	if len(port.Trades) != len(single.Trades) || math.Abs(port.FinalEquity-single.FinalEquity) > 1e-6 {
		t.Fatalf("invariante ETH rotta: trades %d/%d equity %.4f/%.4f",
			len(port.Trades), len(single.Trades), port.FinalEquity, single.FinalEquity)
	}
}

// due simboli identici sfasati: heat condiviso blocca la 4ª entry a 3% heat
func portfolioTestCfg(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.Load("../../configs/default.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Profit.Satellite.Enabled = false
	cfg.Pyramiding.Enabled = false
	// risk fisso 1% per trade, heat 3% → max 3 posizioni contemporanee
	cfg.Risk.Base = 0.01
	cfg.Risk.Max = 0.01
	cfg.Risk.MaxRiskPerTradePct = 1.0
	cfg.Risk.KellyCapPct = 1.0
	cfg.VariantA.RiskPct = 1.0
	cfg.Portfolio.MaxOpenRisk = 0.03
	cfg.Portfolio.MaxCorrelatedRisk = 0.03
	cfg.Portfolio.CrashBrakeDropPct = 0 // off per test
	return cfg
}

func portfolioEng() EngineConfig {
	return EngineConfig{
		Variant: "A", Symbol: "PORTFOLIO", InitialCapital: 10000,
		FeeBps: 4, SlippageBps: 2, UseNextOpen: true,
		PyramidingMax: 0, TrailMode: "donchian", DonExit: 20, EntryMode: "close",
	}
}

func portStrat(cfg *config.Config, atBars ...int) strategy.Strategy {
	signals := map[int]strategy.Signal{}
	for _, b := range atBars {
		signals[b] = strategy.Signal{Side: 1, Strength: 1, StopPrice: 80, Reason: "script long"}
	}
	return &reentryStrat{scriptStrategy{cfg: cfg, signals: signals}, -1} // ReEntry mai (ExitBarIdx -1)
}

// pumpBars — flat per warmup poi pump crescente (evita lo stop donchian-trailing
// che sulle barre piatte ratchet-a lo stop sul low e chiude subito le posizioni)
func pumpBars(n, from int) data.Bars {
	bars := flatBars(n, 100, 0.5)
	for i := from; i < n; i++ {
		c := 100 + float64(i-from)*0.3
		bars[i] = data.Bar{Time: time.Unix(int64(i)*14400, 0), Open: c - 0.3, High: c + 0.2, Low: c - 0.5, Close: c, Volume: 100}
	}
	return bars
}

func shiftBars(bars data.Bars, offset time.Duration) data.Bars {
	out := make(data.Bars, len(bars))
	copy(out, bars)
	for i := range out {
		out[i].Time = out[i].Time.Add(offset)
	}
	return out
}

func TestPortfolioSharedHeatCap(t *testing.T) {
	cfg := portfolioTestCfg(t)
	// pump dal 206: entry (segnale 205 → fill 206) e le successive restano aperte
	// (low crescenti > stop donchian ratchettato)
	barsA := pumpBars(300, 206)
	barsB := shiftBars(pumpBars(300, 206), 2*time.Hour) // timestamp diversi → interleaving
	// 3 segnali per simbolo → 6 entry candidate, heat 3% (risk 1%) → esattamente 3 aperte
	strats := map[string]strategy.Strategy{
		"A": portStrat(cfg, 205, 210, 215),
		"B": portStrat(cfg, 205, 210, 215),
	}
	res := RunPortfolio(map[string]data.Bars{"A": barsA, "B": barsB}, strats, cfg, portfolioEng())
	open := 0
	for _, tr := range res.Trades {
		if tr.ExitReason == "eod" {
			open++
		}
	}
	if open != 3 {
		t.Errorf("posizioni eod %d != 3: heat cap condiviso non applicato correttamente", open)
	}
}

func TestPortfolioCorrelatedCapClipsSecondSymbol(t *testing.T) {
	cfg := portfolioTestCfg(t)
	cfg.Risk.Base = 0.015
	cfg.Risk.Max = 0.015
	cfg.Risk.MaxRiskPerTradePct = 1.5
	cfg.Risk.KellyCapPct = 1.5
	cfg.VariantA.RiskPct = 1.5
	cfg.Portfolio.MaxOpenRisk = 0.09
	cfg.Portfolio.MaxCorrelatedRisk = 0.02 // correlati: 1.5% + max 0.5% residuo
	strats := map[string]strategy.Strategy{
		"A": portStrat(cfg, 205),
		"B": portStrat(cfg, 205),
	}
	res := RunPortfolio(map[string]data.Bars{
		"A": pumpBars(300, 206), "B": shiftBars(pumpBars(300, 206), 2*time.Hour),
	}, strats, cfg, portfolioEng())
	var riskPcts []float64
	for _, tr := range res.Trades {
		if tr.ExitReason == "eod" {
			riskPcts = append(riskPcts, tr.RiskPct)
		}
	}
	sort.Float64s(riskPcts)
	if len(riskPcts) != 2 {
		t.Fatalf("attese 2 posizioni eod, avute %d", len(riskPcts))
	}
	if riskPcts[0] > 0.51 || riskPcts[0] < 0.49 {
		t.Errorf("seconda entry same-side deve essere clippata a ~0.5%% (corr cap), avuto %.3f%%", riskPcts[0])
	}
	if riskPcts[1] < 1.49 {
		t.Errorf("prima entry deve restare ~1.5%%, avuto %.3f%%", riskPcts[1])
	}
}

func TestPortfolioSharedEquityGrowsFromBothSymbols(t *testing.T) {
	cfg := portfolioTestCfg(t)
	strats := map[string]strategy.Strategy{
		"A": portStrat(cfg, 205),
		"B": portStrat(cfg, 205),
	}
	res := RunPortfolio(map[string]data.Bars{
		"A": pumpBars(300, 206), "B": shiftBars(pumpBars(300, 206), 2*time.Hour),
	}, strats, cfg, portfolioEng())
	if res.FinalEquity <= 10000 {
		t.Errorf("equity condivisa deve crescere con PnL di entrambi i simboli, avuta %.2f", res.FinalEquity)
	}
	syms := map[string]bool{}
	for _, tr := range res.Trades {
		syms[tr.Symbol] = true
	}
	if len(syms) != 2 {
		t.Errorf("attesi trade di 2 simboli, avuti %v", syms)
	}
}
```

NOTE per l'implementazione dei test (verifica empirica, adatta le BARRE non lo spirito):
- `reentryStrat` (definito in engine_reentry_test.go) con `reentryAfterBar: -1` non emette mai re-entry: riusalo come portStrat.
- Stop 80 molto lontano: nessuno stop nei test; uscita eod.
- Nel test heat: 6 entry candidate ma il cap heat 3% con risk 1% lascia max 3 POSIZIONI CONTEMPORANEE — se le entry dei due simboli avvengono a timestamp diversi, le prime 3 aprono e le altre vengono rifiutate (heat cap) → eod ≤ 3. Verifica che il sizing log contenga "heat cap".
- Nel test correlated: A e B aprono same-side long; A (primo in ordine) prende 1.5%, B viene clippato da `MaxCorrelatedPct` 2% − 1.5% = 0.5%.

- [ ] **Step 2: Verifica che fallisca**

Run: `go test ./internal/backtest/ -run "TestRunPortfolio|TestPortfolio" -v`
Expected: FAIL compile — `undefined: RunPortfolio`.

- [ ] **Step 3: Implementa `internal/backtest/engine_portfolio.go`**

Struttura completa (adattata da engine.Run — copiane la semantica riga per riga dove marcata "identico"):

```go
package backtest

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/atps/atps/internal/config"
	"github.com/atps/atps/internal/data"
	"github.com/atps/atps/internal/indicators"
	"github.com/atps/atps/internal/risk"
	"github.com/atps/atps/internal/strategy"
)

// symState — stato per-simbolo del portfolio engine
type symState struct {
	symbol     string
	bars       data.Bars
	strat      strategy.Strategy
	ctx        *strategy.Context
	donExitH   []float64
	donExitL   []float64
	donExitH55 []float64
	donExitL55 []float64
	positions  []*Position
	cursor     int
	brakeUntil int
	lastClose  float64
	lastStop   struct {
		valid      bool
		side       int
		exitBarIdx int
	}
}

// RunPortfolio — engine multi-simbolo con UNA equity e heat condivisi.
// Timeline: union dei timestamp; ogni simbolo processa la propria barra quando
// il timestamp combacia. Invariante: con un solo simbolo riproduce engine.Run.
func RunPortfolio(barsMap map[string]data.Bars, strats map[string]strategy.Strategy, cfg *config.Config, eng EngineConfig) *Result {
	res := &Result{Symbol: "PORTFOLIO", Variant: eng.Variant, InitialCapital: eng.InitialCapital, FinalEquity: eng.InitialCapital}
	symbols := make([]string, 0, len(barsMap))
	for s := range barsMap {
		symbols = append(symbols, s)
	}
	sort.Strings(symbols) // ordine deterministico

	// risk limits + guardrail scaling — identico a Run
	lim := risk.LimitsFromConfig(cfg, eng.Variant)
	if lim.MaxLeverage == 0 {
		lim.MaxLeverage = eng.Leverage
		if lim.MaxLeverage == 0 {
			lim.MaxLeverage = 3
		}
	}
	if lim.MaxNotional == 0 && cfg != nil && cfg.Costs.MaxNotionalPerTrade > 0 {
		lim.MaxNotional = cfg.Costs.MaxNotionalPerTrade
	}
	res.RiskLimitsUsed = lim
	res.ScalingCeilingPct, res.ScalingBinding = risk.ScalingCeiling(lim)
	if res.ScalingCeilingPct < lim.MaxRiskPct {
		res.Warnings = append(res.Warnings, fmt.Sprintf("scaling: risk richiesto %.2f%% → tetto effettivo %.2f%% (%s lega)",
			lim.MaxRiskPct, res.ScalingCeilingPct, res.ScalingBinding))
	}
	if eng.PyramidingMode == "separate" && cfg != nil && cfg.Profit.Satellite.Enabled {
		res.Warnings = append(res.Warnings, "pyramiding.mode=separate disabilita satellite (incompatibile: le gambe usano già exit wide)")
	}
	if eng.PyramidingMode == "separate" {
		lim.SatelliteEnabled = false
		lim.SatelliteAlloc = 0
	}

	intervalH := intervalHours(cfg.General.Interval)
	if intervalH == 0 {
		intervalH = 4
	}

	exitLen := eng.DonExit
	if exitLen == 0 {
		exitLen = 20
	}

	// stato per-simbolo (Prepare + canali donchian)
	states := make([]*symState, 0, len(symbols))
	for _, s := range symbols {
		bars := barsMap[s]
		high := make([]float64, len(bars))
		low := make([]float64, len(bars))
		for i, b := range bars {
			high[i] = b.High
			low[i] = b.Low
		}
		st := &symState{
			symbol: s, bars: bars, strat: strats[s],
			ctx: strats[s].Prepare(bars), brakeUntil: -1,
			donExitH: indicators.DonchianHigh(high, exitLen), donExitL: indicators.DonchianLow(low, exitLen),
			donExitH55: indicators.DonchianHigh(high, 55), donExitL55: indicators.DonchianLow(low, 55),
		}
		if len(bars) > 0 {
			st.lastClose = bars[len(bars)-1].Close
		}
		states = append(states, st)
	}

	// timeline: union ordinata dei timestamp
	seen := map[time.Time]bool{}
	var timeline []time.Time
	for _, st := range states {
		for _, b := range st.bars {
			if !seen[b.Time] {
				seen[b.Time] = true
				timeline = append(timeline, b.Time)
			}
		}
	}
	sort.Slice(timeline, func(a, b int) bool { return timeline[a].Before(timeline[b]) })

	// stato condiviso
	equity := eng.InitialCapital
	peak := equity
	var trades []Trade
	var equityCurve []EquityPoint
	var totalFee, totalFundingNet, totalSlippage float64

	openHeatAll := func() float64 {
		sum := 0.0
		for _, st := range states {
			for _, p := range st.positions {
				sum += p.RiskPct
			}
		}
		return sum
	}
	unrealizedAll := func() float64 {
		sum := 0.0
		for _, st := range states {
			for _, p := range st.positions {
				markPx := st.lastClose
				// come Run: posizione fillata next-open nella barra appena processata
				// → marcata a prezzo di entry (unrealized 0 sulla barra di segnale)
				if eng.UseNextOpen && p.EntryBarIdx == st.cursor-1 {
					markPx = p.EntryPrice
				}
				if p.Side == 1 {
					sum += (markPx - p.EntryPrice) * p.Qty
				} else {
					sum += (p.EntryPrice - markPx) * p.Qty
				}
			}
		}
		return sum
	}
	openNotionalAll := func() float64 {
		sum := 0.0
		for _, st := range states {
			for _, p := range st.positions {
				sum += p.Qty * st.lastClose
			}
		}
		return sum
	}

	// recordExit portfolio — identico a Run ma con equity/trades condivisi
	recordExitP := func(st *symState, pos *Position, exitPrice float64, reason string, barIdx int) {
		var pnl float64
		if pos.Side == 1 {
			pnl = (exitPrice - pos.EntryPrice) * pos.Qty
		} else {
			pnl = (pos.EntryPrice - exitPrice) * pos.Qty
		}
		exitFee := exitPrice * pos.Qty * eng.FeeBps / 10000.0
		fee := pos.EntryFee + exitFee
		pnlNet := pnl - fee - pos.FundingAccum
		equity += pnl - exitFee
		totalFee += exitFee
		rMult := 0.0
		if pos.RiskAmount > 0 {
			rMult = pnlNet / pos.RiskAmount
		}
		trades = append(trades, Trade{
			Symbol: st.symbol, Side: pos.Side,
			EntryTime: pos.EntryTime, ExitTime: st.bars[barIdx].Time,
			EntryPrice: pos.EntryPrice, ExitPrice: exitPrice,
			Qty: pos.Qty, EntryATR: pos.EntryATR, StopPrice: pos.StopPrice,
			EntryReason: pos.EntryReason, ExitReason: reason, DonExitLen: pos.DonExitLen,
			PnL: pnl, PnLNet: pnlNet, Fee: fee, FundingCost: pos.FundingAccum,
			BarsHeld: barIdx - pos.EntryBarIdx, MAE: pos.MAE, MFE: pos.MFE,
			ReturnPct: pnlNet / (pos.EntryPrice * pos.Qty) * 100,
			RiskPct:   pos.RiskPct, Leverage: pos.Leverage, Notional: pos.Notional,
			StopDist: math.Abs(pos.EntryPrice - pos.StopPrice), RMultiple: rMult,
			SizingLog: pos.SizingLog, IsSatellite: pos.IsSatellite,
		})
	}

	for _, ts := range timeline {
		for _, st := range states {
			if st.cursor >= len(st.bars) || !st.bars[st.cursor].Time.Equal(ts) {
				continue
			}
			i := st.cursor
			bar := st.bars[i]
			st.cursor++
			n := len(st.bars)

			// ── funding — identico a Run ──
			for _, pos := range st.positions {
				if bar.FundingRate != 0 {
					scale := intervalH / 8.0
					notional := pos.Qty * bar.Close
					pay := notional * bar.FundingRate * scale
					if pos.Side == 1 {
						equity -= pay
						pos.FundingAccum += pay
						totalFundingNet += pay
					} else {
						equity += pay
						pos.FundingAccum -= pay
						totalFundingNet -= pay
					}
				}
			}

			// ── exits — identico a Run (per-simbolo, canali propri) ──
			var remaining []*Position
			for _, pos := range st.positions {
				exit := false
				exitReason := ""
				exitPrice := bar.Close
				var donL, donH float64
				if i >= 1 {
					if pos.DonExitLen == 55 {
						donL = st.donExitL55[i-1]
						donH = st.donExitH55[i-1]
					} else {
						donL = st.donExitL[i-1]
						donH = st.donExitH[i-1]
					}
				}
				if pos.Side == 1 {
					if bar.Low <= pos.StopPrice {
						exit = true
						exitReason = "stop"
						exitPrice = pos.StopPrice
						if bar.Open < exitPrice {
							exitPrice = bar.Open
						}
						if eng.SlippageBps > 0 {
							slip := exitPrice * eng.SlippageBps / 10000.0
							exitPrice -= slip
							totalSlippage += slip * pos.Qty
						}
					} else if !math.IsNaN(donL) && bar.Close < donL {
						exit = true
						if pos.IsSatellite {
							exitReason = "satellite_donchian55"
						} else {
							exitReason = "donchian_exit"
						}
						exitPrice = bar.Close
					} else {
						var newStop float64
						if eng.TrailMode == "chandelier" {
							mult := eng.TrailATRMult
							if mult <= 0 {
								mult = 3.0
							}
							if pos.IsSatellite {
								mult += 1.0
							}
							newStop = strategy.TrailStop(st.ctx, i, pos.Side, mult, "chandelier")
						} else {
							newStop = donL
						}
						if !math.IsNaN(newStop) {
							pos.StopPrice = risk.TrailStopPosition(pos.StopPrice, newStop, pos.Side)
						}
					}
				} else if pos.Side == -1 {
					if bar.High >= pos.StopPrice {
						exit = true
						exitReason = "stop"
						exitPrice = pos.StopPrice
						if bar.Open > exitPrice {
							exitPrice = bar.Open
						}
						if eng.SlippageBps > 0 {
							slip := exitPrice * eng.SlippageBps / 10000.0
							exitPrice += slip
							totalSlippage += slip * pos.Qty
						}
					} else if !math.IsNaN(donH) && bar.Close > donH {
						exit = true
						if pos.IsSatellite {
							exitReason = "satellite_donchian55"
						} else {
							exitReason = "donchian_exit"
						}
						exitPrice = bar.Close
					} else {
						var newStop float64
						if eng.TrailMode == "chandelier" {
							mult := eng.TrailATRMult
							if mult <= 0 {
								mult = 3.0
							}
							if pos.IsSatellite {
								mult += 1.0
							}
							newStop = strategy.TrailStop(st.ctx, i, pos.Side, mult, "chandelier")
						} else {
							newStop = donH
						}
						if !math.IsNaN(newStop) {
							pos.StopPrice = risk.TrailStopPosition(pos.StopPrice, newStop, pos.Side)
						}
					}
				}
				// MAE/MFE — identico a Run
				if pos.Side == 1 {
					if mae := (bar.Low - pos.EntryPrice) / pos.EntryPrice * 100; mae < pos.MAE {
						pos.MAE = mae
					}
					if mfe := (bar.High - pos.EntryPrice) / pos.EntryPrice * 100; mfe > pos.MFE {
						pos.MFE = mfe
					}
				} else {
					if mae := (pos.EntryPrice - bar.High) / pos.EntryPrice * 100; mae < pos.MAE {
						pos.MAE = mae
					}
					if mfe := (pos.EntryPrice - bar.Low) / pos.EntryPrice * 100; mfe > pos.MFE {
						pos.MFE = mfe
					}
				}
				if exit {
					recordExitP(st, pos, exitPrice, exitReason, i)
					if exitReason == "stop" {
						st.lastStop.valid = true
						st.lastStop.side = pos.Side
						st.lastStop.exitBarIdx = i
					}
				} else {
					remaining = append(remaining, pos)
				}
			}
			st.positions = remaining

			// ── crash brake per-simbolo — identico a Run (chiude SOLO questo simbolo) ──
			if cfg.Portfolio.CrashBrakeDropPct > 0 && i > 0 {
				retPct := (bar.Close - st.bars[i-1].Close) / st.bars[i-1].Close * 100
				if math.Abs(retPct) >= cfg.Portfolio.CrashBrakeDropPct {
					for _, pos := range st.positions {
						recordExitP(st, pos, bar.Close, "crash_brake", i)
					}
					st.positions = nil
					st.brakeUntil = i + 6
				}
			}

			st.lastClose = bar.Close

			// mark-to-market condiviso + peak/dd (come Run pre-signal)
			unrealized := 0.0
			for _, st2 := range states {
				for _, p := range st2.positions {
					if p.Side == 1 {
						unrealized += (st2.lastClose - p.EntryPrice) * p.Qty
					} else {
						unrealized += (p.EntryPrice - st2.lastClose) * p.Qty
					}
				}
			}
			curEq := equity + unrealized
			if curEq > peak {
				peak = curEq
			}
			ddPct := 0.0
			if peak > 0 {
				ddPct = (curEq - peak) / peak * 100
			}

			if i < st.brakeUntil || curEq <= 0 {
				continue // niente segnali per questo simbolo (curve point a fine timestamp)
			}

			// ── signal: intrabar → Next → re-entry — identico a Run, per-simbolo ──
			var sig strategy.Signal
			intrabarFill, intrabarSlip := 0.0, 0.0
			isIntrabar := false
			if eng.EntryMode == "intrabar" && len(st.positions) == 0 && i >= 1 && i+1 < n {
				if lv, ok := st.strat.(strategy.IntrabarLevels); ok {
					levels := lv.IntrabarEntry(st.ctx, i)
					atrPrev := st.ctx.ATR[i-1]
					if levels.Enabled && !math.IsNaN(atrPrev) && atrPrev > 0 {
						longHit := !math.IsNaN(levels.LongLevel) && bar.High >= levels.LongLevel
						shortHit := !math.IsNaN(levels.ShortLevel) && bar.Low <= levels.ShortLevel
						side := 0
						var level, stopATR float64
						if longHit && !shortHit {
							side, level, stopATR = 1, levels.LongLevel, levels.LongStopATR
						} else if shortHit && !longHit {
							side, level, stopATR = -1, levels.ShortLevel, levels.ShortStopATR
						}
						if side != 0 && stopATR > 0 {
							fill := level
							if (side == 1 && bar.Open > level) || (side == -1 && bar.Open < level) {
								fill = bar.Open
							}
							if eng.SlippageBps > 0 {
								intrabarSlip = fill * eng.SlippageBps / 10000.0
								if side == 1 {
									fill += intrabarSlip
								} else {
									fill -= intrabarSlip
								}
							}
							stop := fill - float64(side)*stopATR*atrPrev
							sig = strategy.Signal{Side: side, Strength: 1, StopPrice: stop, Reason: "intrabar breakout"}
							intrabarFill = fill
							isIntrabar = true
						}
					}
				}
			}
			if sig.Side == 0 {
				sig = st.strat.Next(st.ctx, i)
			}
			if sig.Side == 0 && st.lastStop.valid {
				if rc, ok := st.strat.(strategy.ReEntryChecker); ok {
					sig = rc.ReEntry(st.ctx, i, strategy.StopOutInfo{Side: st.lastStop.side, ExitBarIdx: st.lastStop.exitBarIdx})
				}
			}

			if sig.Side != 0 && !(eng.UseNextOpen && !isIntrabar && i+1 >= n) {
				atr := st.ctx.ATR[i]
				if math.IsNaN(atr) {
					atr = 0
				}
				fillPrice := bar.Close
				fillTime := bar.Time
				slipAmt := 0.0
				if isIntrabar {
					fillPrice = intrabarFill
					slipAmt = intrabarSlip
				} else if eng.UseNextOpen && i+1 < n {
					fillPrice = st.bars[i+1].Open
					fillTime = st.bars[i+1].Time
					if eng.SlippageBps > 0 {
						slipAmt = fillPrice * eng.SlippageBps / 10000.0
						if sig.Side == 1 {
							fillPrice += slipAmt
						} else {
							fillPrice -= slipAmt
						}
					}
				} else if eng.SlippageBps > 0 {
					slipAmt = fillPrice * eng.SlippageBps / 10000.0
					if sig.Side == 1 {
						fillPrice += slipAmt
					} else {
						fillPrice -= slipAmt
					}
				}
				stopPx := sig.StopPrice
				if math.IsNaN(stopPx) || stopPx <= 0 {
					stopPx = fillPrice - float64(sig.Side)*2*atr
					if math.IsNaN(stopPx) {
						stopPx = 0
					}
				}
				stopValid := (sig.Side == 1 && stopPx < fillPrice) || (sig.Side == -1 && stopPx > fillPrice)
				if !stopValid {
					sig.Side = 0
				}
				if sig.Side != 0 {
					// heat condiviso: totale = tutte le posizioni; correlato = same-side TUTTI simboli
					corrHeat := 0.0
					for _, st2 := range states {
						for _, p := range st2.positions {
							if p.Side == sig.Side {
								corrHeat += p.RiskPct
							}
						}
					}
					ms := risk.MarketState{
						Equity:                 curEq,
						Price:                  fillPrice,
						ATR:                    atr,
						StopPrice:              stopPx,
						Side:                   sig.Side,
						VolRegime:              st.ctx.VolRegime[i],
						ADX:                    st.ctx.ADX[i],
						FundingZ:               st.ctx.FundingZ[i],
						VolAnnualizedPct:       risk.AnnualizedVolPct(atr, fillPrice, intervalH),
						PortfolioHeatPct:       openHeatAll(),
						PortfolioCorrelatedPct: corrHeat,
						EquityDDPct:            -ddPct,
					}

					sameSideHeat := 0.0
					var earliest *Position
					for _, p := range st.positions {
						if p.Side == sig.Side {
							sameSideHeat += p.RiskPct
							if earliest == nil {
								earliest = p
							}
						}
					}
					sameSideUnits := 0
					if earliest != nil {
						sameSideUnits = earliest.Units
					}
					if eng.PyramidingMode == "separate" {
						sameSideUnits = 0
						for _, p := range st.positions {
							if p.Side == sig.Side && !p.IsSatellite {
								sameSideUnits++
							}
						}
					}
					hasSameSide := earliest != nil
					if hasSameSide {
						if risk.CanPyramid(earliest.EntryPrice, bar.Close, atr, sig.Side, sameSideUnits, eng.PyramidingMax, eng.PyramidStepATR) {
							dec := risk.Size(ms, lim)
							if dec.CappedByNotional {
								res.NotionalCapHits++
							}
							if lim.PyramidingRiskNeutral && eng.PyramidingMode != "separate" {
								dec.RiskPct = dec.RiskPct * 0.5
								dec.RiskAmount = dec.RiskPct / 100 * ms.Equity
								dec.Qty = dec.RiskAmount / dec.StopDist
								dec.Notional = dec.Qty * fillPrice
								dec.Leverage = dec.Notional / ms.Equity
								dec.Factors = append(dec.Factors, "pyramid risk_neutral ×0.5")
							} else if eng.PyramidingMode != "separate" {
								dec.Notional = dec.Qty * fillPrice
								dec.RiskAmount = dec.Qty * dec.StopDist
								if ms.Equity > 0 {
									dec.RiskPct = dec.RiskAmount / ms.Equity * 100
									totalNotional := 0.0
									for _, p := range st.positions {
										if p.Side == sig.Side {
											totalNotional += p.Notional
										}
									}
									dec.Leverage = (totalNotional + dec.Notional) / ms.Equity
								}
							}
							if dec.Accept && dec.Qty > 0 {
								fee := fillPrice * dec.Qty * eng.FeeBps / 10000.0
								slipCost := slipAmt * dec.Qty
								equity -= fee
								totalFee += fee
								totalSlippage += slipCost
								if eng.PyramidingMode == "separate" {
									leg := &Position{
										Symbol: st.symbol, Side: sig.Side, Qty: dec.Qty,
										EntryPrice: fillPrice, EntryTime: fillTime, EntryATR: atr,
										StopPrice: stopPx, Units: 1, EntryBarIdx: i,
										RiskPct: dec.RiskPct, Leverage: dec.Leverage,
										Notional: dec.Notional, RiskAmount: dec.RiskAmount,
										SizingLog:   logFactors(dec) + " | pyramid separate (wide Don55)",
										EntryFee:    fee, EntryReason: sig.Reason + " | pyramid separate",
										IsSatellite: false, DonExitLen: 55,
									}
									st.positions = append(st.positions, leg)
								} else if lim.PyramidingRiskNeutral {
									earliest.EntryPrice = (earliest.EntryPrice*earliest.Qty + fillPrice*dec.Qty) / (earliest.Qty + dec.Qty)
									earliest.Qty += dec.Qty
									earliest.Units++
									earliest.Notional += dec.Notional
									earliest.EntryFee += fee
									earliest.Leverage = earliest.Notional / ms.Equity
									if !math.IsNaN(stopPx) {
										earliest.StopPrice = risk.TrailStopPosition(earliest.StopPrice, stopPx, sig.Side)
									}
									earliest.SizingLog += " | pyramid(risk_neutral): " + logFactors(dec)
								} else {
									totalQty := earliest.Qty + dec.Qty
									earliest.EntryPrice = (earliest.EntryPrice*earliest.Qty + fillPrice*dec.Qty) / totalQty
									earliest.Qty = totalQty
									earliest.Units++
									earliest.RiskPct += dec.RiskPct
									earliest.RiskAmount += dec.RiskAmount
									earliest.Notional += dec.Notional
									earliest.EntryFee += fee
									earliest.Leverage = earliest.Notional / ms.Equity
									if !math.IsNaN(stopPx) {
										earliest.StopPrice = risk.TrailStopPosition(earliest.StopPrice, stopPx, sig.Side)
									}
									earliest.SizingLog += " | pyramid: " + logFactors(dec)
								}
							}
						}
					} else if len(st.positions) == 0 {
						dec := risk.Size(ms, lim)
						if dec.CappedByNotional {
							res.NotionalCapHits++
						}
						if dec.Accept && dec.Qty > 0 {
							fee := fillPrice * dec.Qty * eng.FeeBps / 10000.0
							slipCost := slipAmt * dec.Qty
							equity -= fee
							totalFee += fee
							totalSlippage += slipCost
							if lim.SatelliteEnabled && lim.SatelliteAlloc > 0 && lim.SatelliteAlloc < 1 {
								coreQty := dec.Qty * (1 - lim.SatelliteAlloc)
								satQty := dec.Qty * lim.SatelliteAlloc
								coreRisk := dec.RiskPct * (1 - lim.SatelliteAlloc)
								satRisk := dec.RiskPct * lim.SatelliteAlloc
								coreNotional := coreQty * fillPrice
								satNotional := satQty * fillPrice
								corePos := &Position{
									Symbol: st.symbol, Side: sig.Side, Qty: coreQty,
									EntryPrice: fillPrice, EntryTime: fillTime, EntryATR: atr,
									StopPrice: stopPx, Units: 1, EntryBarIdx: i,
									RiskPct: coreRisk, Leverage: coreNotional / ms.Equity,
									Notional: coreNotional, RiskAmount: coreRisk / 100 * ms.Equity,
									SizingLog:   logFactors(dec) + " | core 70%",
									EntryFee:    fee * (1 - lim.SatelliteAlloc),
									EntryReason: sig.Reason, IsSatellite: false, DonExitLen: 20,
								}
								satPos := &Position{
									Symbol: st.symbol, Side: sig.Side, Qty: satQty,
									EntryPrice: fillPrice, EntryTime: fillTime, EntryATR: atr,
									StopPrice: stopPx, Units: 1, EntryBarIdx: i,
									RiskPct: satRisk, Leverage: satNotional / ms.Equity,
									Notional: satNotional, RiskAmount: satRisk / 100 * ms.Equity,
									SizingLog:   logFactors(dec) + " | satellite 30% (wide Don55)",
									EntryFee:    fee * lim.SatelliteAlloc,
									EntryReason: sig.Reason, IsSatellite: true, DonExitLen: 55,
								}
								st.positions = append(st.positions, corePos, satPos)
							} else {
								pos := &Position{
									Symbol: st.symbol, Side: sig.Side, Qty: dec.Qty,
									EntryPrice: fillPrice, EntryTime: fillTime, EntryATR: atr,
									StopPrice: stopPx, Units: 1, EntryBarIdx: i,
									RiskPct: dec.RiskPct, Leverage: dec.Leverage,
									Notional: dec.Notional, RiskAmount: dec.RiskAmount,
									SizingLog: logFactors(dec), EntryFee: fee,
									EntryReason: sig.Reason, DonExitLen: 20,
								}
								st.positions = append(st.positions, pos)
							}
						}
					}

					// ── intrabar same-bar stop — identico a Run ──
					if isIntrabar {
						var survived []*Position
						for _, p := range st.positions {
							if p.EntryBarIdx != i {
								survived = append(survived, p)
								continue
							}
							stopHit := (p.Side == 1 && bar.Low <= p.StopPrice) || (p.Side == -1 && bar.High >= p.StopPrice)
							if !stopHit {
								survived = append(survived, p)
								continue
							}
							exitPrice := p.StopPrice
							if eng.SlippageBps > 0 {
								slip := exitPrice * eng.SlippageBps / 10000.0
								if p.Side == 1 {
									exitPrice -= slip
								} else {
									exitPrice += slip
								}
								totalSlippage += slip * p.Qty
							}
							if p.Side == 1 {
								if mae := (bar.Low - p.EntryPrice) / p.EntryPrice * 100; mae < p.MAE {
									p.MAE = mae
								}
								if mfe := (bar.High - p.EntryPrice) / p.EntryPrice * 100; mfe > p.MFE {
									p.MFE = mfe
								}
							} else {
								if mae := (p.EntryPrice - bar.High) / p.EntryPrice * 100; mae < p.MAE {
									p.MAE = mae
								}
								if mfe := (p.EntryPrice - bar.Low) / p.EntryPrice * 100; mfe > p.MFE {
									p.MFE = mfe
								}
							}
							st.lastStop.valid = true
							st.lastStop.side = p.Side
							st.lastStop.exitBarIdx = i
							recordExitP(st, p, exitPrice, "stop_same_bar", i)
						}
						st.positions = survived
					}
				}
			}
		}

		// ── equity point per timestamp (fine barra, tutti i simboli processati) ──
		curEq := equity + unrealizedAll()
		if curEq > peak {
			peak = curEq
		}
		dd := 0.0
		if peak > 0 {
			dd = (curEq - peak) / peak * 100
		}
		price := 0.0
		for _, st := range states {
			if st.cursor > 0 && st.bars[st.cursor-1].Time.Equal(ts) {
				price = st.bars[st.cursor-1].Close
				break
			}
		}
		equityCurve = append(equityCurve, EquityPoint{
			Time: ts, Equity: curEq, Drawdown: dd, Price: price,
			Heat: openHeatAll(), Leverage: openNotionalAll() / math.Max(curEq, 1),
		})
	}

	// ── chiusure EOD per-simbolo — identico a Run ──
	for _, st := range states {
		if len(st.bars) == 0 {
			continue
		}
		n := len(st.bars)
		for _, pos := range st.positions {
			recordExitP(st, pos, st.bars[n-1].Close, "eod", n-1)
		}
		st.positions = nil
	}

	final := equity
	if len(equityCurve) > 0 {
		equityCurve[len(equityCurve)-1].Equity = final
	}
	gross, net := 0.0, 0.0
	maxLev, sumLev, maxRisk, sumRisk, maxHeat := 0.0, 0.0, 0.0, 0.0, 0.0
	for _, t := range trades {
		gross += t.PnL
		net += t.PnLNet
		if t.Leverage > maxLev {
			maxLev = t.Leverage
		}
		sumLev += t.Leverage
		if t.RiskPct > maxRisk {
			maxRisk = t.RiskPct
		}
		sumRisk += t.RiskPct
	}
	for _, e := range equityCurve {
		if e.Heat > maxHeat {
			maxHeat = e.Heat
		}
	}
	tn := float64(len(trades))
	if tn > 0 {
		res.AvgLeverage = sumLev / tn
		res.AvgRiskPct = sumRisk / tn
	}
	res.MaxLeverageUsed = maxLev
	res.MaxRiskPctUsed = maxRisk
	res.MaxHeatSeen = maxHeat
	res.Trades = trades
	res.Equity = equityCurve
	res.FinalEquity = final
	res.GrossPnL = gross
	res.NetPnL = net
	res.TotalFee = totalFee
	res.TotalFunding = totalFundingNet
	res.TotalSlippage = totalSlippage
	return res
}
```

IMPORTANTE confronto con engine.Run durante la trascrizione: ogni blocco marcato "identico" va copiato ADATTANDO solo i riferimenti (positions → st.positions, bars → st.bars, ctx → st.ctx, equity/trades condivisi). Se una riga di Run usa `res.` direttamente va tenuta uguale. La funzione `intervalHours` e `logFactors` esistono già in engine.go (stesso package).

- [ ] **Step 4: Verifica invariante e test**

Run: `go test ./internal/backtest/ -run "TestRunPortfolio|TestPortfolio" -v -timeout 300s`
Expected: PASS tutti. SE l'invariante fallisce: confronta il primo trade divergente (indice, simbolo, tempo) e verifica la sequenza equity-mutazioni — NON aggiustare i numeri a mano, trova la divergenza logica (ordine funding/exits/mark/signals deve rispettare Run).

Run: `go test ./... -count=1` (nessuna regressione).

- [ ] **Step 5: Commit**

```bash
git add internal/backtest/engine_portfolio.go internal/backtest/engine_portfolio_test.go
git commit -m "feat(portfolio): RunPortfolio multi-simbolo — equity/heat/correlati condivisi, invariante single-symbol == engine.Run"
```

---

### Task 2: CLI `portfolio-backtest`

**Files:**
- Modify: `cmd/atps/main.go` (nuovo comando + registrazione in root)

- [ ] **Step 1: Aggiungi il comando** (segui il pattern di cmdBacktest/cmdCompare — cobra)

In `cmd/atps/main.go`, aggiungi:

```go
func cmdPortfolioBacktest() *cobra.Command {
	var cfgPath, csvPattern, outHTML string
	cmd := &cobra.Command{
		Use:   "portfolio-backtest",
		Short: "Backtest PORTFOLIO multi-simbolo (equity+heat condivisi)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return err
			}
			symbols := cfg.General.Symbols
			if len(symbols) == 0 {
				return fmt.Errorf("general.symbols vuoto")
			}
			barsMap := map[string]data.Bars{}
			strats := map[string]strategy.Strategy{}
			for _, s := range symbols {
				p := strings.ReplaceAll(csvPattern, "{SYMBOL}", s)
				bars, err := data.LoadBarsCSV(p)
				if err != nil {
					return fmt.Errorf("csv %s: %w", p, err)
				}
				barsMap[s] = bars
				strats[s] = strategy.New("A", cfg)
			}
			eng := backtest.EngineConfigFrom(cfg, "A", "PORTFOLIO")
			res := backtest.RunPortfolio(barsMap, strats, cfg, eng)
			stats := metrics.Compute(res)
			fmt.Printf("PORTFOLIO %s: Return %.2f%% CAGR %.2f%% Sharpe %.2f Sortino %.2f MaxDD %.2f%% PF %.2f Trades %d Fee $%.2f Funding $%.2f\n",
				strings.Join(symbols, "+"), stats.ReturnPct, stats.ReturnAnnual, stats.Sharpe, stats.Sortino, stats.MaxDD, stats.ProfitFactor, stats.Trades, stats.TotalFee, stats.TotalFunding)
			fmt.Printf("scaling ceiling: %.2f%% (%s lega)\n", res.ScalingCeilingPct, res.ScalingBinding)
			for _, w := range res.Warnings {
				fmt.Printf("warn: %s\n", w)
			}
			// breakdown per-simbolo (da trade list)
			type symAgg struct{ trades, winners int; pnl float64 }
			aggs := map[string]*symAgg{}
			for _, tr := range res.Trades {
				a := aggs[tr.Symbol]
				if a == nil {
					a = &symAgg{}
					aggs[tr.Symbol] = a
				}
				a.trades++
				a.pnl += tr.PnLNet
				if tr.PnLNet > 0 {
					a.winners++
				}
			}
			fmt.Println("per-symbol (da trade list):")
			for _, s := range symbols {
				if a, ok := aggs[s]; ok {
					fmt.Printf("  %s: trades %d, win %d (%.0f%%), PnL netto $%.2f\n", s, a.trades, a.winners, float64(a.winners)/float64(a.trades)*100, a.pnl)
				}
			}
			if outHTML == "" {
				outHTML = fmt.Sprintf("reports/PORTFOLIO_%s.html", time.Now().Format("20060102_1504"))
			}
			return report.Generate(outHTML, report.Input{Config: cfg, Bars: barsMap[symbols[0]], Result: res, Stats: stats, Symbol: "PORTFOLIO", Variant: "A", GeneratedAt: time.Now()})
		},
	}
	cmd.Flags().StringVar(&cfgPath, "config", config.DefaultPath(), "path config yaml")
	cmd.Flags().StringVar(&csvPattern, "csvs", "data/raw/{SYMBOL}_4h.csv", "pattern CSV per simbolo ({SYMBOL} sostituito)")
	cmd.Flags().StringVar(&outHTML, "out", "", "output html path")
	return cmd
}
```

Registra nel root command (riga con `root.AddCommand(...)`): aggiungi `cmdPortfolioBacktest()`.

Verifica import necessari (`strings`, `time`, `metrics`, `report`, ecc. — molti già presenti).

- [ ] **Step 2: Smoke su dati reali**

```bash
go build -o atps ./cmd/atps
cp configs/atps_v2.yaml configs/atps_portfolio.yaml
# (tempaneo per lo smoke: symbols 3 — la config definitiva arriva nel Task 4)
python3 - <<'EOF'
import re
p='configs/atps_portfolio.yaml'; s=open(p).read()
s=s.replace('symbols: ["BTCUSDT"]','symbols: ["BTCUSDT","ETHUSDT","SOLUSDT"]')
s=s.replace('orderly_symbols: ["PERP_BTC_USDC"]','orderly_symbols: ["PERP_BTC_USDC","PERP_ETH_USDC","PERP_SOL_USDC"]')
open(p,'w').write(s)
EOF
./atps portfolio-backtest --config configs/atps_portfolio.yaml --out reports/PORTFOLIO_smoke.html
```

Expected: summary PORTFOLIO BTCUSDT+ETHUSDT+SOLUSDT + breakdown 3 simboli + html generato, senza panic. Annota i numeri (servono al Task 4 come previsione stage-1).

- [ ] **Step 3: Commit**

```bash
git add cmd/atps/main.go
git commit -m "feat(cli): portfolio-backtest — summary combinato + breakdown per-simbolo + report HTML"
```

---

### Task 3: `scripts/portfolio_split` (holdout + walk-forward)

**Files:**
- Create: `scripts/portfolio_split/main.go`

- [ ] **Step 1: Implementa** (pattern di scripts/baseline_split)

```go
// portfolio_split: train/test/full + walk-forward del PORTFOLIO (holdout evidence).
// Split per TIMESTAMP (confine = 70% della timeline del primo simbolo in ordine alfabetico).
// Uso: go run ./scripts/portfolio_split -config configs/atps_portfolio.yaml -csvs "data/raw/{SYMBOL}_4h.csv" [-wf]
package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/atps/atps/internal/backtest"
	"github.com/atps/atps/internal/config"
	"github.com/atps/atps/internal/data"
	"github.com/atps/atps/internal/metrics"
	"github.com/atps/atps/internal/strategy"
)

func main() {
	cfgPath := flag.String("config", "configs/atps_portfolio.yaml", "")
	csvPattern := flag.String("csvs", "data/raw/{SYMBOL}_4h.csv", "")
	wf := flag.Bool("wf", false, "esegui walk-forward 8 folds")
	flag.Parse()
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}
	symbols := append([]string{}, cfg.General.Symbols...)
	sort.Strings(symbols)
	barsMap := map[string]data.Bars{}
	for _, s := range symbols {
		bars, err := data.LoadBarsCSV(os.ExpandEnv(fmt.Sprintf(*csvPattern, s, s)))
		if err != nil {
			// pattern manuale: sostituisci {SYMBOL}
			p := *csvPattern
			for i := 0; i < len(p)-9; i++ {
				if p[i:i+9] == "{SYMBOL}" {
					p = p[:i] + s + p[i+9:]
				}
			}
			bars, err = data.LoadBarsCSV(p)
			if err != nil {
				fmt.Fprintf(os.Stderr, "csv %s: %v\n", s, err)
				os.Exit(1)
			}
		}
		barsMap[s] = bars
	}
	run := func(m map[string]data.Bars) *metrics.Stats {
		strats := map[string]strategy.Strategy{}
		for _, s := range symbols {
			strats[s] = strategy.New("A", cfg)
		}
		eng := backtest.EngineConfigFrom(cfg, "A", "PORTFOLIO")
		res := backtest.RunPortfolio(m, strats, cfg, eng)
		return metrics.Compute(res)
	}
	// split per timestamp: confine sul primo simbolo (alfabetico = BTCUSDT)
	refBars := barsMap[symbols[0]]
	boundary := refBars[int(float64(len(refBars))*0.7)].Time
	split := func(pred func(t time.Time) bool) map[string]data.Bars {
		out := map[string]data.Bars{}
		for s, bars := range barsMap {
			var sel data.Bars
			for _, b := range bars {
				if pred(b.Time) {
					sel = append(sel, b)
				}
			}
			out[s] = sel
		}
		return out
	}
	for name, m := range map[string]map[string]data.Bars{
		"train": split(func(t time.Time) bool { return t.Before(boundary) }),
		"test":  split(func(t time.Time) bool { return !t.Before(boundary) }),
		"full":  barsMap,
	} {
		st := run(m)
		fmt.Printf("%s %s PORTFOLIO %-5s → CAGR %6.2f%% DD %7.2f%% Sharpe %.2f Calmar %.2f trades %d (boundary %s)\n",
			*cfgPath, *csvPattern, name, st.ReturnAnnual, st.MaxDD, st.Sharpe, st.Calmar, st.Trades, boundary.Format("2006-01"))
	}
	if !*wf {
		return
	}
	// WF 8 folds: fold k testa (k/8, (k+1)/8] della timeline, allena su tutto il precedente
	var sharpes []float64
	for k := 1; k <= 8; k++ {
		lo := refBars[int(float64(len(refBars))*float64(k-1)/8)].Time
		hiIdx := int(float64(len(refBars)) * float64(k) / 8)
		if hiIdx >= len(refBars) {
			hiIdx = len(refBars) - 1
		}
		hi := refBars[hiIdx].Time
		trainM := split(func(t time.Time) bool { return t.Before(lo) })
		testM := split(func(t time.Time) bool { return !t.Before(lo) && !t.After(hi) })
		sts := run(trainM)
		ste := run(testM)
		sharpes = append(sharpes, ste.Sharpe)
		fmt.Printf("  fold %d (%s→%s): train Sharpe %.2f → test Sharpe %.2f CAGR %.2f%% DD %.2f%%\n",
			k, lo.Format("2006-01"), hi.Format("2006-01"), sts.Sharpe, ste.Sharpe, ste.ReturnAnnual, ste.MaxDD)
	}
	sort.Float64s(sharpes)
	med := (sharpes[3] + sharpes[4]) / 2
	fmt.Printf("WF mediana Sharpe: %.2f\n", med)
}
```

NOTA: la prima LoadBarsCSV con `fmt.Sprintf(*csvPattern, s, s)` è solo un tentativo ingenuo — SEMPLIFICA: rimuovila e fai DIRETTAMENTE la sostituzione stringa di `{SYMBOL}` (il blocco `for i := 0; i < len(p)-9; i++` → usa `strings.ReplaceAll(p, "{SYMBOL}", s)` con import `strings`). Trascrivi pulito.

- [ ] **Step 2: Verifica**

```bash
go vet ./scripts/portfolio_split/ && gofmt -l scripts/portfolio_split/
go run ./scripts/portfolio_split -config configs/atps_portfolio.yaml | tee reports/V4_HOLDOUT_PORTFOLIO.txt
go run ./scripts/portfolio_split -config configs/atps_portfolio.yaml -wf | tee reports/V4_WF_PORTFOLIO.txt
```

Expected: train/test/full plausibili (train ≈ periodo 2020-01→2024-09), WF 8 folds + mediana.

- [ ] **Step 3: Commit**

```bash
git add scripts/portfolio_split/
git commit -m "feat(scripts): portfolio_split — holdout per-timestamp + walk-forward 8 folds del portfolio"
```

---

### Task 4: Validazione v4 (stage-1 risk 2%, stage-2 condizionale) + config finale

**Files:**
- Modify: `configs/atps_portfolio.yaml` ( già creata nel Task 2 — finalizza name/commenti)
- Create: `reports/V4_VALIDATION.md` + `reports/V4_*.txt`
- Temporanee (mai committare se bocciate): `configs/atps_portfolio_r25.yaml`, `configs/atps_portfolio_r30.yaml`

- [ ] **Step 1: Finalizza atps_portfolio.yaml**

`variant_a.name`: `"Variant A — ATPS v4 portfolio BTC+ETH+SOL (risk 2%, vedi reports/V4_VALIDATION.md)"`. Verifica: le uniche differenze vs atps_v2.yaml sono `general.symbols`, `orderly.symbols` (mappa completa già presente) e il name.

- [ ] **Step 2: Stage-1 gates (risk 2%)**

```bash
go build -o atps ./cmd/atps
./atps portfolio-backtest --config configs/atps_portfolio.yaml --out reports/V4_PORTFOLIO.html | tee reports/V4_PORTFOLIO.txt
./atps montecarlo --config configs/atps_portfolio.yaml --symbol BTCUSDT --variant A --csv data/raw/BTCUSDT_4h.csv --runs 2000 --out reports/V4_MC_BTCCOMPONENT.json | tail -1   # informativo, componente BTC
go run ./scripts/portfolio_split -config configs/atps_portfolio.yaml | tee reports/V4_HOLDOUT_PORTFOLIO.txt
go run ./scripts/portfolio_split -config configs/atps_portfolio.yaml -wf | tee reports/V4_WF_PORTFOLIO.txt
```

Gate stage-1 (baseline v2 BTC): **portfolio teCAGR ≥ 12.4 AND teCal ≥ 0.73** (da V4_HOLDOUT_PORTFOLIO.txt); **DD full ≥ -18%**; WF mediana Sharpe > 0.
Perturb (documentato BTC-domina): `./atps perturb --config configs/atps_portfolio.yaml --symbol BTCUSDT --variant A --csv data/raw/BTCUSDT_4h.csv --out reports/V4_PERTURB_BTCCOMPONENT.json | tee reports/V4_PERTURB_BTCCOMPONENT.txt` — degrado CAGR < 30%.

- [ ] **Step 3: Stage-2 SOLO se DD full ≥ -15.0%** (headroom ≥ 3pt vs budget -18%)

Crea `configs/atps_portfolio_r25.yaml` (copia con risk base=max=0.025, legacy 2.5, kelly 2.5, variant risk_pct 2.5, heat 0.045, corr 0.025) e `..._r30.yaml` (0.03 / 3.0 / 0.054 / 0.03). Run holdout + full per entrambe:
```bash
go run ./scripts/portfolio_split -config configs/atps_portfolio_r25.yaml | tee reports/V4_HOLDOUT_R25.txt
go run ./scripts/portfolio_split -config configs/atps_portfolio_r30.yaml | tee reports/V4_HOLDOUT_R30.txt
./atps portfolio-backtest --config configs/atps_portfolio_r25.yaml --out reports/V4_PORTFOLIO_R25.html | tee reports/V4_PORTFOLIO_R25.txt
./atps portfolio-backtest --config configs/atps_portfolio_r30.yaml --out reports/V4_PORTFOLIO_R30.html | tee reports/V4_PORTFOLIO_R30.txt
```
Gate: stessi criteri stage-1 (teCAGR ≥ 12.4, teCal ≥ 0.73, DD ≥ -18%). Promuovi il risk più alto che passa; a parità passa il più prudente.

- [ ] **Step 4: Scrivi `reports/V4_VALIDATION.md`**

Struttura (numeri REALI, zero segnaposto):

```markdown
# ATPS v4 — Portfolio validation (2026-09-04)

Baseline: atps_v2 BTC (34.31/-17.01; holdout 12.4/Cal 0.73). Budget DD full: ≥ -18%.
Metodo: RunPortfolio equity+heat condivisi; split per timestamp (confine 2024-09);
WF 8 folds per-timestamp; perturb su componente BTC (dominante).

## Stage 1 — portfolio risk 2%
| Gate | Criterio | Valore | Esito |
|---|---|---|---|
| Full | informativo | CAGR <> DD <> Sharpe <> trades <> | INFO |
| Holdout | teCAGR ≥ 12.4 AND teCal ≥ 0.73 | <> / <> | <> |
| DD full | ≥ -18% | <> | <> |
| WF 8f | mediana Sharpe > 0 | <> | <> |
| Perturb (BTC comp) | degrado < 30% | <> | <> |

## Stage 2 — risk re-scaling (SOLO se DD full ≥ -15%)
| Config | Full CAGR/DD | teCAGR/teCal | DD ≥ -18% | Esito |
|---|---|---|---|---|
| r2.5% | <> | <> | <> | <> |
| r3.0% | <> | <> | <> | <> |

## Decisione
- [ ] PORTFOLIO PROMOSSO (config: atps_portfolio.yaml [risk <>%])
- [ ] BOCCIATO — resta atps_v2 (motivo: <>)

## Breakdown per-simbolo (da reports/V4_PORTFOLIO*.txt)
<tabella: symbol × trades/win%/PnL per la config promossa>

## Appendice — config bocciate (se presenti)
<diff esatto vs atps_portfolio.yaml per ogni config temporanea eliminata>
```

Se stage-1 fallisce: niente stage-2, `rm` config temporanee, decisione BOCCIATO.
Se promosso con risk ≠ 2%: incorpora i valori di risk VINCENTI in atps_portfolio.yaml (le r25/r30 vanno comunque eliminate, con diff in appendice).

- [ ] **Step 5: Commit**

```bash
git add configs/atps_portfolio.yaml reports/V4_*
git commit -m "feat(v4): portfolio BTC+ETH+SOL validato (equity+heat condivisi) — decisione documentata"
```

---

### Task 5: README (fix link morti + portfolio) + verifica finale

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Fix link morti**

I report V2/V3_VALIDATION.md sono stati cancellati su richiesta (commit 0729664) — i README link puntano a file inesistenti. Sostituisci OGNI riferimento a `reports/V2_VALIDATION.md` e `reports/V3_VALIDATION.md` con: `reports/V2_VALIDATION.md (rimosso dai report su richiesta utente 2026-09-04; reperibile in git history al commit precedente a 0729664)`. Cerca anche riferimenti ad altri file V2_/V3_ cancellati (`grep -n "V2_\|V3_" README.md`) e uniforma.

- [ ] **Step 2: Sezione portfolio v4**

Dopo la tabella "Tentativi di scala v3", aggiorna "Configurazione finale del sistema" in base alla Decisione v4:
- PROMOSSO: nuova riga `**atps_portfolio (BTC+ETH+SOL, equity+heat condivisi)** — CAGR <>% DD <>%` + comando `./atps portfolio-backtest --config configs/atps_portfolio.yaml` + nota "bot live resta single-symbol (3 istanze non condividono heat — degradazione vs backtest, roadmap multi-simbolo)".
- BOCCIATO: riga `Portfolio BTC+ETH+SOL: testato e scartato (vedi git history V4_VALIDATION.md)`.

- [ ] **Step 3: Verifica finale**

```bash
gofmt -l . && go vet ./... && go test ./... && go build -o atps ./cmd/atps
./atps backtest --config configs/atps_v2.yaml --variant A --symbol BTCUSDT --csv data/raw/BTCUSDT_4h.csv --out /tmp/opencode/fin_v2.html 2>&1 | grep "BTCUSDT A:"
./atps portfolio-backtest --config configs/atps_portfolio.yaml --out /tmp/opencode/fin_v4.html 2>&1 | grep "PORTFOLIO"
```

Expected: tutto verde; v2 = 34.31%/-17.01%; portfolio = numeri V4_VALIDATION.

- [ ] **Step 4: Commit**

```bash
git add README.md
git commit -m "docs: README v4 portfolio (esito validazione), fix link a validation report rimossi (git history)"
```

---

## Gate di successo (dallo spec v4)

1. Invariante single-symbol verificato (BTC e ETH) — Task 1
2. Portfolio risk 2%: teCAGR ≥ 12.4 AND teCal ≥ 0.73, DD full ≥ -18% — Task 4
3. Stage-2 risk promosso SOLO con stessi gate — Task 4
4. Numeri riproducibili: `./atps portfolio-backtest --config configs/atps_portfolio.yaml` — Task 5
5. Se tutto bocciato → resta atps_v2, decisione documentata — fallback onesto
