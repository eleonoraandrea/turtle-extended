# ATPS Improve — CAGR up con DD ≤ 15% (Implementation Plan)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Più CAGR a DD ≤ 15-18% su BTC (variant A) con numeri interamente riproducibili: engine config unificata (CLI = bot live = optimizer), risk tuning DD-constrained, entry intrabar + re-entry validati anti-overfit.

**Architecture:** (1) parametri engine per-variante nel config YAML con un'unica factory `backtest.EngineConfigFrom` usata da CLI/bot/optimizer; (2) nuova modalità entry intrabar nel motore backtest (fill a livello canale, stop pessimistico stessa-barra); (3) re-entry dopo stop-out via interfaccia strategia opzionale; (4) optimizer a stadi con selezione "max CAGR s.t. DD ≤ 15%" su train, conferma test, walk-forward, perturbazione, ETH/SOL.

**Tech Stack:** Go 1.x, yaml.v3, test standard `go test`. Spec: `docs/superpowers/specs/2026-09-04-atps-improve-cagr-dd15-design.md`.

**Convenzioni:** lavorare da repo root `/mnt/lavoro/trading/turtle-extended`. Dopo ogni task: `gofmt -l .` (vuoto), `go vet ./...`, `go test ./...` verdi, commit.

---

### Task 1: Config — parametri engine per-variante (EngineCfg inline + ReEntryCfg)

**Files:**
- Modify: `internal/config/config.go:131-199` (struct varianti)
- Modify: `internal/config/config.go:243-256` (Load, normalizzazione ReEntry)
- Test: `internal/config/config_test.go` (append)

- [ ] **Step 1: Scrivi il test fallito**

Appendi a `internal/config/config_test.go`:

```go
func TestVariantEngineCfgInlineAndReEntry(t *testing.T) {
	yml := `general:
  initial_capital: 10000.0
backtest:
  trail_atr_mult: 2.5
  pyramid_step_atr: 0.5
trend:
  donchian_entry: 55
  donchian_exit: 20
pyramiding:
  enabled: true
  max_additions: 4
  risk_neutral: true
variant_a:
  name: "A test"
  atr_stop_mult: 1.6
  trail_mode: chandelier
  trail_atr_mult: 3.5
  don_exit: 10
  entry_mode: intrabar
  pyramiding_max_units: 3
  reentry:
    enabled: true
    lookback: 12
    within_bars: 25
variant_b:
  name: "B test"
variant_d:
  name: "D test"
  trail_mode: chandelier
`
	path := filepath.Join(t.TempDir(), "cfg.yaml")
	if err := os.WriteFile(path, []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	// inline engine su variant_a
	if c.VariantA.Engine.TrailMode != "chandelier" {
		t.Errorf("VariantA.Engine.TrailMode = %q, want chandelier", c.VariantA.Engine.TrailMode)
	}
	if c.VariantA.Engine.TrailATRMult != 3.5 {
		t.Errorf("VariantA.Engine.TrailATRMult = %v, want 3.5", c.VariantA.Engine.TrailATRMult)
	}
	if c.VariantA.Engine.DonExit != 10 {
		t.Errorf("VariantA.Engine.DonExit = %v, want 10", c.VariantA.Engine.DonExit)
	}
	if c.VariantA.Engine.EntryMode != "intrabar" {
		t.Errorf("VariantA.Engine.EntryMode = %q, want intrabar", c.VariantA.Engine.EntryMode)
	}
	if c.VariantA.Engine.PyramidingUnits != 3 {
		t.Errorf("VariantA.Engine.PyramidingUnits = %v, want 3", c.VariantA.Engine.PyramidingUnits)
	}
	// reentry
	if !c.VariantA.ReEntry.Enabled || c.VariantA.ReEntry.Lookback != 12 || c.VariantA.ReEntry.WithinBars != 25 {
		t.Errorf("VariantA.ReEntry = %+v, want enabled/12/25", c.VariantA.ReEntry)
	}
	// variant_d legacy: trail_mode ora vive in Engine
	if c.VariantD.Engine.TrailMode != "chandelier" {
		t.Errorf("VariantD.Engine.TrailMode = %q, want chandelier", c.VariantD.Engine.TrailMode)
	}
	// default reentry su B (disabled)
	if c.VariantB.ReEntry.Enabled {
		t.Errorf("VariantB.ReEntry.Enabled default deve essere false")
	}
}

func TestReEntryDefaultsNormalization(t *testing.T) {
	yml := `variant_a:
  name: "A"
  reentry:
    enabled: true
`
	path := filepath.Join(t.TempDir(), "cfg.yaml")
	if err := os.WriteFile(path, []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if c.VariantA.ReEntry.Lookback != 10 || c.VariantA.ReEntry.WithinBars != 20 {
		t.Errorf("ReEntry defaults = %+v, want lookback 10 within 20", c.VariantA.ReEntry)
	}
}
```

Se `config_test.go` non importa già `os` e `path/filepath`, aggiungili agli import.

- [ ] **Step 2: Verifica che fallisca**

Run: `go test ./internal/config/ -run TestVariantEngineCfgInlineAndReEntry -v`
Expected: FAIL — compile error `c.VariantA.Engine undefined` / `c.VariantA.ReEntry undefined`.

- [ ] **Step 3: Implementa**

In `internal/config/config.go`, dopo `type Backtest struct {...}` (riga ~130) aggiungi:

```go
// EngineCfg — override engine per-variante (yaml inline sotto variant_a/b/c/d).
// Campi zero → fallback alla sezione globale backtest:/trend: in EngineConfigFrom.
type EngineCfg struct {
	TrailMode       string  `yaml:"trail_mode"`         // donchian|chandelier (default donchian)
	TrailATRMult    float64 `yaml:"trail_atr_mult"`     // moltiplicatore chandelier
	DonExit         int     `yaml:"don_exit"`           // lunghezza Donchian exit (default trend.donchian_exit)
	EntryMode       string  `yaml:"entry_mode"`         // close|intrabar (default close)
	PyramidingUnits int     `yaml:"pyramiding_max_units"` // override unità pyramiding
	PyramidStepATR  float64 `yaml:"pyramid_step_atr"`
}

// ReEntryCfg — re-entry dopo stop-out se il trend regge e prezzo fa nuovo high/low N barre
type ReEntryCfg struct {
	Enabled    bool `yaml:"enabled"`
	Lookback   int  `yaml:"lookback"`    // nuovo high/low su N barre (default 10)
	WithinBars int  `yaml:"within_bars"` // finestra barre dallo stop-out (default 20)
}
```

Modifica le 4 struct varianti. `VariantA` (riga ~131) diventa:

```go
type VariantA struct {
	Name          string     `yaml:"name"`
	DonchianEntry int        `yaml:"donchian_entry"`
	DonchianExit  int        `yaml:"donchian_exit"`
	DonchianAlt   int        `yaml:"donchian_alt"`
	ATRPeriod     int        `yaml:"atr_period"`
	ATRStopMult   float64    `yaml:"atr_stop_mult"`
	RiskPct       float64    `yaml:"risk_pct"`
	SMAFilter     int        `yaml:"sma_filter"`
	UseEMAFilter  bool       `yaml:"use_ema_filter"`
	Engine        EngineCfg  `yaml:",inline"`
	ReEntry       ReEntryCfg `yaml:"reentry"`
}
```

`VariantB` e `VariantC`: aggiungi in coda alle rispettive struct:

```go
	Engine        EngineCfg  `yaml:",inline"`
	ReEntry       ReEntryCfg `yaml:"reentry"`
```

`VariantD` (riga ~175): **rimuovi** i campi `TrailATRMult float64 \`yaml:"trail_atr_mult"\`` e `TrailMode string \`yaml:"trail_mode"\`` (ora in Engine, stesse chiavi YAML → nessun config esistente si rompe) e aggiungi in coda:

```go
	Engine        EngineCfg  `yaml:",inline"`
	ReEntry       ReEntryCfg `yaml:"reentry"`
```

In `func Load(path string)`, dopo `if c.General.InitialCapital == 0 {...}` aggiungi:

```go
	// normalizza default ReEntry (solo se enabled)
	for _, r := range []*ReEntryCfg{&c.VariantA.ReEntry, &c.VariantB.ReEntry, &c.VariantC.ReEntry, &c.VariantD.ReEntry} {
		if r.Enabled {
			if r.Lookback <= 0 {
				r.Lookback = 10
			}
			if r.WithinBars <= 0 {
				r.WithinBars = 20
			}
		}
	}
```

Cerca riferimenti ai campi rimossi di VariantD e aggiungiali:

Run: `grep -rn "VariantD.TrailMode\|VariantD.TrailATRMult" --include="*.go" .`

Ogni occorrenza (es. `cmd/atps/main.go` in `engineFromCfg`) va riscritta come `cfg.VariantD.Engine.TrailMode` / `cfg.VariantD.Engine.TrailATRMult` — sarà rimossa nel Task 2, per ora basta che compili.

- [ ] **Step 4: Verifica che passi**

Run: `go test ./internal/config/ -v`
Expected: PASS tutti (nessuna regressione test esistenti).

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat(config): engine params per-variante (EngineCfg inline + ReEntryCfg) — trail/don_exit/entry_mode/pyramiding/reentry"
```

---

### Task 2: `backtest.EngineConfigFrom` — unica fonte di verità (CLI = bot = optimizer)

**Files:**
- Create: `internal/backtest/engineconfig.go`
- Modify: `cmd/atps/main.go:569-603` (`engineFromCfg`)
- Modify: `internal/bot/bot.go:551-566` (`SnapshotResult`)
- Test: `internal/backtest/engineconfig_test.go` (create)

- [ ] **Step 1: Scrivi il test fallito**

Crea `internal/backtest/engineconfig_test.go`:

```go
package backtest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/atps/atps/internal/config"
)

func TestEngineConfigFromDefaults(t *testing.T) {
	cfg, err := config.Load("../../configs/default.yaml")
	if err != nil {
		t.Fatal(err)
	}
	ec := EngineConfigFrom(cfg, "A", "BTCUSDT")
	if ec.TrailMode != "donchian" {
		t.Errorf("A TrailMode = %q, want donchian (legacy default)", ec.TrailMode)
	}
	if ec.EntryMode != "close" {
		t.Errorf("A EntryMode = %q, want close", ec.EntryMode)
	}
	if ec.DonExit != cfg.Trend.DonchianExit {
		t.Errorf("A DonExit = %d, want %d (trend.donchian_exit)", ec.DonExit, cfg.Trend.DonchianExit)
	}
	// pyramiding: enabled + max_additions 4 → 5 unità
	want := cfg.Pyramiding.MaxAdditions + 1
	if !cfg.Pyramiding.Enabled || ec.PyramidingMax != want {
		t.Errorf("A PyramidingMax = %d, want %d", ec.PyramidingMax, want)
	}
	if ec.TrailATRMult != cfg.Backtest.TrailATRMult {
		t.Errorf("A TrailATRMult = %v, want %v (backtest.trail_atr_mult)", ec.TrailATRMult, cfg.Backtest.TrailATRMult)
	}
	if ec.FeeBps != cfg.Costs.FeeBps || ec.SlippageBps != cfg.Costs.SlippageBps {
		t.Errorf("fee/slippage mismatch con costs")
	}
	if ec.UseNextOpen != cfg.Backtest.UseNextOpenFill {
		t.Errorf("UseNextOpen = %v, want %v", ec.UseNextOpen, cfg.Backtest.UseNextOpenFill)
	}
	// D: trail chandelier dal suo yaml
	ecD := EngineConfigFrom(cfg, "D", "BTCUSDT")
	if ecD.TrailMode != "chandelier" {
		t.Errorf("D TrailMode = %q, want chandelier (default.yaml variant_d.trail_mode)", ecD.TrailMode)
	}
}

func TestEngineConfigFromPerVariantOverrides(t *testing.T) {
	yml := `general:
  initial_capital: 10000.0
costs:
  fee_bps: 4.0
  slippage_bps: 2.0
backtest:
  trail_atr_mult: 2.5
  pyramid_step_atr: 0.5
  use_next_open_fill: true
trend:
  donchian_exit: 20
pyramiding:
  enabled: false
variant_a:
  name: A
  trail_mode: chandelier
  trail_atr_mult: 3.5
  don_exit: 10
  entry_mode: intrabar
  pyramiding_max_units: 3
  pyramid_step_atr: 0.75
`
	path := filepath.Join(t.TempDir(), "cfg.yaml")
	if err := os.WriteFile(path, []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	ec := EngineConfigFrom(cfg, "A", "X")
	if ec.TrailMode != "chandelier" || ec.TrailATRMult != 3.5 || ec.DonExit != 10 || ec.EntryMode != "intrabar" {
		t.Errorf("override per-variante non applicato: %+v", ec)
	}
	// pyramiding globale disabled MA override per-variante vince
	if ec.PyramidingMax != 3 {
		t.Errorf("PyramidingMax = %d, want 3 (override per-variante vince su enabled=false)", ec.PyramidingMax)
	}
	if ec.PyramidStepATR != 0.75 {
		t.Errorf("PyramidStepATR = %v, want 0.75", ec.PyramidStepATR)
	}
	// senza override per-variante: pyramiding disabled → 0
	ecB := EngineConfigFrom(cfg, "B", "X")
	if ecB.PyramidingMax != 0 {
		t.Errorf("B PyramidingMax = %d, want 0 (pyramiding.enabled false)", ecB.PyramidingMax)
	}
}
```

- [ ] **Step 2: Verifica che fallisca**

Run: `go test ./internal/backtest/ -run TestEngineConfigFrom -v`
Expected: FAIL — `undefined: EngineConfigFrom`.

- [ ] **Step 3: Implementa**

Crea `internal/backtest/engineconfig.go`:

```go
package backtest

import "github.com/atps/atps/internal/config"

// EngineConfigFrom — UNICA fonte di verità per EngineConfig.
// Usata da CLI backtest, bot live e optimizer: ciò che si ottimizza è ciò che gira.
// Risoluzione: override per-variante (variant_x.engine, yaml inline) > sezione
// globale backtest:/trend:/pyramiding: > default hardcoded legacy.
func EngineConfigFrom(cfg *config.Config, variant, symbol string) EngineConfig {
	var e config.EngineCfg
	switch variant {
	case "A":
		e = cfg.VariantA.Engine
	case "B":
		e = cfg.VariantB.Engine
	case "C":
		e = cfg.VariantC.Engine
	case "D":
		e = cfg.VariantD.Engine
	}
	trailMode := e.TrailMode
	if trailMode == "" {
		trailMode = "donchian"
	}
	trailMult := e.TrailATRMult
	if trailMult <= 0 {
		trailMult = cfg.Backtest.TrailATRMult
	}
	donExit := e.DonExit
	if donExit <= 0 {
		donExit = cfg.Trend.DonchianExit
	}
	if donExit <= 0 {
		donExit = 20
	}
	entryMode := e.EntryMode
	if entryMode == "" {
		entryMode = "close"
	}
	// pyramiding: legacy identical logic (backtest.pyramiding_max_units base,
	// pyramiding.enabled/max_additions vince, disabled → 0), poi override per-variante
	pyrMax := cfg.Backtest.PyramidingMaxUnits
	if cfg.Pyramiding.Enabled {
		if cfg.Pyramiding.MaxAdditions > 0 {
			pyrMax = cfg.Pyramiding.MaxAdditions + 1
		}
	} else {
		pyrMax = 0
	}
	if e.PyramidingUnits > 0 {
		pyrMax = e.PyramidingUnits
	}
	step := e.PyramidStepATR
	if step <= 0 {
		step = cfg.Backtest.PyramidStepATR
	}
	return EngineConfig{
		Variant:        variant,
		Symbol:         symbol,
		InitialCapital: cfg.General.InitialCapital,
		FeeBps:         cfg.Costs.FeeBps,
		SlippageBps:    cfg.Costs.SlippageBps,
		Leverage:       cfg.Costs.Leverage,
		UseNextOpen:    cfg.Backtest.UseNextOpenFill,
		PyramidingMax:  pyrMax,
		PyramidStepATR: step,
		TrailATRMult:   trailMult,
		TrailMode:      trailMode,
		DonExit:        donExit,
		EntryMode:      entryMode,
	}
}
```

Nota: `EntryMode` verrà aggiunto a `EngineConfig` nel Task 3. Per far compilare ORA, aggiungi subito il campo in `internal/backtest/engine.go` alla struct `EngineConfig` (riga ~115, dopo `DonExit int`):

```go
	EntryMode     string // close|intrabar (default close; intrabar = fill a livello canale)
```

In `cmd/atps/main.go`, sostituisci l'intero corpo di `engineFromCfg` (righe ~569-603) con:

```go
func engineFromCfg(cfg *config.Config, variant, symbol string) backtest.EngineConfig {
	return backtest.EngineConfigFrom(cfg, variant, symbol)
}
```

In `internal/bot/bot.go` (`SnapshotResult`, righe ~551-566), sostituisci la chiamata `backtest.Run(bars, b.strat, b.cfg, backtest.EngineConfig{...hardcoded...})` con:

```go
	// run quick backtest on current bars for display — STESSA engine config del CLI
	res := backtest.Run(bars, b.strat, b.cfg, backtest.EngineConfigFrom(b.cfg, b.variant, b.symbol))
```

- [ ] **Step 4: Verifica che passi**

Run: `go test ./internal/backtest/ ./internal/config/ ./internal/bot/ -v`
Expected: PASS.

Verifica no-regression comportamentale (i numeri baseline non cambiano):

Run: `go build -o atps ./cmd/atps && ./atps backtest --config configs/btc_opt.yaml --variant A --symbol BTCUSDT --csv data/raw/BTCUSDT_4h.csv --out /tmp/opencode/t2_check.html`
Expected: `CAGR 29.55% ... MaxDD -23.04%` (identico al baseline pre-refactor — A non ha override engine e default.yaml dà donchian/20 come il vecchio codice hardcoded).

- [ ] **Step 5: Commit**

```bash
git add internal/backtest/engineconfig.go internal/backtest/engineconfig_test.go internal/backtest/engine.go cmd/atps/main.go internal/bot/bot.go
git commit -m "refactor: EngineConfigFrom unica fonte engine config — CLI/bot-live/optimizer allineati (bot era chandelier hardcoded, CLI donchian)"
```

---

### Task 3: Engine — EntryMode intrabar (fill a livello canale, stop pessimistico stessa barra)

**Files:**
- Modify: `internal/backtest/engine.go` (blocco signal/entry, righe ~414-460 + struct Trade/Position per EntryReason)
- Modify: `internal/strategy/strategy.go` (interfacce IntrabarLevels/ReEntryChecker/StopOutInfo — servono qui per il compile del type-assert)
- Test: `internal/backtest/engine_intrabar_test.go` (create)

- [ ] **Step 1: Interfacce in strategy.go**

Appendi a `internal/strategy/strategy.go` (dopo `type Strategy interface`):

```go
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
```

- [ ] **Step 2: Scrivi i test falliti**

Crea `internal/backtest/engine_intrabar_test.go`:

```go
package backtest

import (
	"math"
	"testing"
	"time"

	"github.com/atps/atps/internal/config"
	"github.com/atps/atps/internal/data"
	"github.com/atps/atps/internal/strategy"
)

// intrabarStrat — Next sempre flat; IntrabarEntry livelli fissi. Deterministico.
type intrabarStrat struct {
	scriptStrategy
	levels strategy.IntrabarEntryLevels
}

func (s *intrabarStrat) IntrabarEntry(_ *strategy.Context, _ int) strategy.IntrabarEntryLevels {
	return s.levels
}

// bar a prezzo p con wiggle ±w
func mkBar(t time.Time, p, w float64) data.Bar {
	return data.Bar{Time: t, Open: p, High: p + w, Low: p - w, Close: p, Volume: 100}
}

func flatBars(n int, p, w float64) data.Bars {
	out := make(data.Bars, n)
	for i := range out {
		out[i] = mkBar(time.Unix(int64(i)*14400, 0), p, w)
	}
	return out
}

func intrabarEng(cfg *config.Config, bars data.Bars, strat strategy.Strategy) *Result {
	cfg.Profit.Satellite.Enabled = false // test a posizione singola: niente split core/sat
	eng := EngineConfig{
		Variant: "A", Symbol: "TEST", InitialCapital: 10000,
		FeeBps: 4, SlippageBps: 2, UseNextOpen: true,
		PyramidingMax: 0, TrailMode: "donchian", DonExit: 20, EntryMode: "intrabar",
	}
	return Run(bars, strat, cfg, eng)
}

func TestIntrabarFillAtLevel(t *testing.T) {
	cfg, _ := config.Load("../../configs/default.yaml")
	bars := flatBars(30, 100, 0.5)
	// breakout bar: High attraversa il livello 100.5, Low non tocca lo stop
	// (ATR_prev su barre wiggle 0.5 = 1.0 → stop = 100.52 − 2×1.0 = 98.52 < Low 99.9)
	bars = append(bars, data.Bar{Time: time.Unix(30*14400, 0), Open: 100, High: 105, Low: 99.9, Close: 104, Volume: 100})
	bars = append(bars, flatBars(10, 104, 0.5)...)
	strat := &intrabarStrat{scriptStrategy{cfg: cfg}, strategy.IntrabarEntryLevels{
		Enabled: true, LongLevel: 100.5, LongStopATR: 2, ShortLevel: math.NaN(), ShortStopATR: 2,
	}}
	res := intrabarEng(cfg, bars, strat)
	// posizione aperta → chiusa a EOD: 1 trade, fill verificato al livello + slippage
	if len(res.Trades) != 1 {
		t.Fatalf("atteso 1 trade (eod), avuti %d", len(res.Trades))
	}
	tr := res.Trades[0]
	if tr.ExitReason != "eod" {
		t.Errorf("ExitReason = %q, want eod", tr.ExitReason)
	}
	if tr.EntryReason != "intrabar breakout" {
		t.Errorf("EntryReason = %q, want 'intrabar breakout'", tr.EntryReason)
	}
	wantFill := 100.5 * (1 + 2.0/10000.0) // livello + 2bps slippage
	if math.Abs(tr.EntryPrice-wantFill) > 1e-6 {
		t.Errorf("EntryPrice = %v, want %v (fill al livello + slip)", tr.EntryPrice, wantFill)
	}
}

func TestIntrabarGapOpenFillsAtOpen(t *testing.T) {
	cfg, _ := config.Load("../../configs/default.yaml")
	bars := flatBars(30, 100, 0.5)
	// gap: open già sopra il livello → fill alla open (101), non al livello
	bars = append(bars, data.Bar{Time: time.Unix(30*14400, 0), Open: 101, High: 106, Low: 100.8, Close: 105, Volume: 100})
	bars = append(bars, flatBars(10, 105, 0.5)...)
	strat := &intrabarStrat{scriptStrategy{cfg: cfg}, strategy.IntrabarEntryLevels{
		Enabled: true, LongLevel: 100.5, LongStopATR: 2, ShortLevel: math.NaN(), ShortStopATR: 2,
	}}
	res := intrabarEng(cfg, bars, strat)
	if len(res.Trades) != 1 {
		t.Fatalf("atteso 1 trade (eod): avuti %d", len(res.Trades))
	}
	// gap: open 101 > livello → fill alla open + slippage, non al livello
	wantFill := 101.0 * (1 + 2.0/10000.0)
	if math.Abs(res.Trades[0].EntryPrice-wantFill) > 1e-6 {
		t.Errorf("EntryPrice = %v, want %v (fill alla open per gap + slip)", res.Trades[0].EntryPrice, wantFill)
	}
}

func TestIntrabarSameBarStopPessimistic(t *testing.T) {
	cfg, _ := config.Load("../../configs/default.yaml")
	bars := flatBars(30, 100, 0.5)
	// ATR_prev = 1.0 → stop = 100.52 − 2×1.0 = 98.52; Low 98 ≤ stop → pessimistico
	bars = append(bars, data.Bar{Time: time.Unix(30*14400, 0), Open: 100, High: 105, Low: 98, Close: 99, Volume: 100})
	bars = append(bars, flatBars(10, 99, 0.1)...)
	strat := &intrabarStrat{scriptStrategy{cfg: cfg}, strategy.IntrabarEntryLevels{
		Enabled: true, LongLevel: 100.5, LongStopATR: 2, ShortLevel: math.NaN(), ShortStopATR: 2,
	}}
	res := intrabarEng(cfg, bars, strat)
	if len(res.Trades) != 1 {
		t.Fatalf("atteso 1 trade (fill→stop stessa barra, pessimistico), avuti %d", len(res.Trades))
	}
	tr := res.Trades[0]
	if tr.ExitReason != "stop_same_bar" {
		t.Errorf("ExitReason = %q, want stop_same_bar", tr.ExitReason)
	}
	if tr.BarsHeld != 0 {
		t.Errorf("BarsHeld = %d, want 0", tr.BarsHeld)
	}
	if tr.PnLNet >= 0 {
		t.Errorf("same-bar stop deve essere una perdita netta, avuto %.2f", tr.PnLNet)
	}
	if tr.RMultiple > 0 {
		t.Errorf("R-multiple deve essere negativo (≤ −1R circa), avuto %.2f", tr.RMultiple)
	}
}

func TestIntrabarBothSidesHitNoEntry(t *testing.T) {
	cfg, _ := config.Load("../../configs/default.yaml")
	bars := flatBars(30, 100, 0.5)
	// huge range bar: tocca sia livello long che short → path inconoscibile → niente entry
	bars = append(bars, data.Bar{Time: time.Unix(30*14400, 0), Open: 100, High: 110, Low: 90, Close: 100, Volume: 100})
	bars = append(bars, flatBars(10, 100, 0.5)...)
	strat := &intrabarStrat{scriptStrategy{cfg: cfg}, strategy.IntrabarEntryLevels{
		Enabled: true, LongLevel: 100.5, LongStopATR: 2, ShortLevel: 99.5, ShortStopATR: 2,
	}}
	res := intrabarEng(cfg, bars, strat)
	if len(res.Trades) != 0 || res.MaxLeverageUsed > 0 {
		t.Errorf("both-sides-hit: attesa nessuna entry, trades %d lev %.3f", len(res.Trades), res.MaxLeverageUsed)
	}
}

func TestIntrabarDisabledByDefault(t *testing.T) {
	cfg, _ := config.Load("../../configs/default.yaml")
	bars := flatBars(30, 100, 0.5)
	bars = append(bars, data.Bar{Time: time.Unix(30*14400, 0), Open: 100, High: 105, Low: 99.9, Close: 104, Volume: 100})
	strat := &intrabarStrat{scriptStrategy{cfg: cfg}, strategy.IntrabarEntryLevels{
		Enabled: true, LongLevel: 100.5, LongStopATR: 2, ShortLevel: math.NaN(), ShortStopATR: 2,
	}}
	eng := EngineConfig{
		Variant: "A", Symbol: "TEST", InitialCapital: 10000,
		FeeBps: 4, SlippageBps: 2, UseNextOpen: true,
		PyramidingMax: 0, TrailMode: "donchian", DonExit: 20, // EntryMode "" → close
	}
	res := Run(bars, strat, cfg, eng)
	if len(res.Trades) != 0 || res.MaxLeverageUsed > 0 {
		t.Errorf("entry_mode close default: strategy Next è flat → nessuna entry, avute trades %d", len(res.Trades))
	}
}
```

Nota: `scriptStrategy` è definito in `engine_fix_test.go` (stesso package) —riusalo con embedding.

- [ ] **Step 3: Verifica che fallisca**

Run: `go test ./internal/backtest/ -run TestIntrabar -v`
Expected: FAIL — `atteso 1 trade (eod), avuti 0` (nessuna entry intrabar implementata) e `stop_same_bar` inesistente.

- [ ] **Step 4: Implementa in engine.go**

**4a.** Aggiungi `EntryReason` a `Trade` (dopo `ExitReason`) e a `Position`:

```go
	EntryReason  string    `json:"entry_reason"`
```

e in `Position`:

```go
	EntryReason string
```

Popola in ogni punto dove viene costruito un `Trade{...}`: aggiungi `EntryReason: pos.EntryReason,`. Sono 4 punti: exit loop (~riga 346), crash brake (~riga 384), same-bar stop (nuovo, qui sotto), EOD close (~riga 674).

**4b.** Closure di supporto per registrare un'uscita (inserire dopo `brakeUntil := -1`, riga ~167) — riusata SOLO dal nuovo blocco same-bar (i percorsi esistenti non vengono toccati: zero rischio regressione):

```go
	// recordExit — registra chiusura posizione (usato dal path intrabar same-bar stop)
	recordExit := func(pos *Position, exitPrice float64, reason string, barIdx int) {
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
			Symbol: eng.Symbol, Side: pos.Side,
			EntryTime: pos.EntryTime, ExitTime: bars[barIdx].Time,
			EntryPrice: pos.EntryPrice, ExitPrice: exitPrice,
			Qty: pos.Qty, EntryATR: pos.EntryATR, StopPrice: pos.StopPrice,
			EntryReason: pos.EntryReason, ExitReason: reason,
			PnL: pnl, PnLNet: pnlNet, Fee: fee, FundingCost: pos.FundingAccum,
			BarsHeld: barIdx - pos.EntryBarIdx, MAE: pos.MAE, MFE: pos.MFE,
			ReturnPct: pnlNet / (pos.EntryPrice * pos.Qty) * 100,
			RiskPct:   pos.RiskPct, Leverage: pos.Leverage, Notional: pos.Notional,
			StopDist: math.Abs(pos.EntryPrice - pos.StopPrice), RMultiple: rMult,
			SizingLog: pos.SizingLog, IsSatellite: pos.IsSatellite,
		})
	}
```

**4c.** Stato stop-out per re-entry (dopo `brakeUntil := -1`):

```go
	// ultimo stop-out per logica re-entry (interfaccia ReEntryChecker, Task 4)
	type stopOutState struct {
		valid      bool
		side       int
		exitBarIdx int
	}
	var lastStop stopOutState
```

Nell'exit loop, dentro `if exit {`, subito dopo `exitReason = "stop"` viene gestito il caso — aggiungi alla fine del ramo `if exit {` (prima di `} else { remaining... }`):

```go
			if exitReason == "stop" {
				lastStop = stopOutState{valid: true, side: pos.Side, exitBarIdx: i}
			}
```

**4d.** Blocco segnale: sostituisci la riga `sig := strat.Next(ctx, i)` (~415) con:

```go
		// ── signal: intrabar (livelli da barre < i) → Next (close-mode) → re-entry ──
		var sig strategy.Signal
		intrabarFill, intrabarSlip := 0.0, 0.0
		isIntrabar := false
		if eng.EntryMode == "intrabar" && len(positions) == 0 && i >= 1 && i+1 < n {
			if lv, ok := strat.(strategy.IntrabarLevels); ok {
				levels := lv.IntrabarEntry(ctx, i)
				atrPrev := ctx.ATR[i-1]
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
							fill = bar.Open // gap oltre il livello: fill alla open
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
			sig = strat.Next(ctx, i)
		}
		if sig.Side == 0 && lastStop.valid {
			if rc, ok := strat.(strategy.ReEntryChecker); ok {
				sig = rc.ReEntry(ctx, i, strategy.StopOutInfo{Side: lastStop.side, ExitBarIdx: lastStop.exitBarIdx})
			}
		}
```

**4e.** Guardia fill e fill price: sostituisci

```go
		if sig.Side != 0 && !(eng.UseNextOpen && i+1 >= n) {
```

con

```go
		if sig.Side != 0 && !(eng.UseNextOpen && !isIntrabar && i+1 >= n) {
```

e nel corpo, sostituisci il blocco `fillPrice := bar.Close; fillTime := bar.Time; slipAmt := 0.0; if eng.UseNextOpen && i+1 < n {...} else if ...` con:

```go
			// fill price: intrabar (già calcolato, slippage incluso) oppure next-open/close
			fillPrice := bar.Close
			fillTime := bar.Time
			slipAmt := 0.0
			if isIntrabar {
				fillPrice = intrabarFill
				slipAmt = intrabarSlip
			} else if eng.UseNextOpen && i+1 < n {
				fillPrice = bars[i+1].Open
				fillTime = bars[i+1].Time
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
```

**4f.** EntryReason nelle posizioni: nei due punti di creazione fresh-entry (satellite split e posizione singola) aggiungi `EntryReason: sig.Reason,` e nel ramo pyramiding lascia invariato (eredita).

**4g.** Stop pessimistico stessa barra (intrabar): dopo la chiusura del blocco fresh-entry (`}` che chiude `else if len(positions) == 0 {...}`), ancora dentro `if sig.Side != 0 {...}`, aggiungi:

```go
				// intrabar same-bar stop — PESSIMISTICO: se dopo il fill anche lo stop
				// è toccabile nella stessa barra, assumiamo fill→stop (path inconoscibile)
				if isIntrabar {
					var survived []*Position
					for _, p := range positions {
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
						// MAE della barra di entry
						if p.Side == 1 {
							if mae := (bar.Low - p.EntryPrice) / p.EntryPrice * 100; mae < p.MAE {
								p.MAE = mae
							}
						} else {
							if mae := (p.EntryPrice - bar.High) / p.EntryPrice * 100; mae < p.MAE {
								p.MAE = mae
							}
						}
						recordExit(p, exitPrice, "stop_same_bar", i)
					}
					positions = survived
				}
```

Attenzione all'indentazione: il blocco va al livello giusto (dentro `if sig.Side != 0`, dopo il fresh-entry `else if len(positions) == 0 { ... }`).

- [ ] **Step 5: Verifica che passi + no regression**

Run: `go test ./internal/backtest/ -v`
Expected: PASS tutti (inclusi test esistenti — close-mode invariato).

Run: `go build -o atps ./cmd/atps && ./atps backtest --config configs/btc_opt.yaml --variant A --symbol BTCUSDT --csv data/raw/BTCUSDT_4h.csv --out /tmp/opencode/t3_check.html`
Expected: `CAGR 29.55% ... MaxDD -23.04%` (identico: default entry_mode=close).

- [ ] **Step 6: Commit**

```bash
git add internal/backtest/ internal/strategy/strategy.go
git commit -m "feat(engine): entry_mode intrabar — fill a livello canale (gap→open), slippage, stop stesso-barra pessimistico; traccia EntryReason + stato stop-out"
```

---

### Task 4: VariantA — IntrabarEntry + ReEntry (con test unitari)

**Files:**
- Modify: `internal/strategy/variant_a.go`
- Test: `internal/strategy/variant_a_ext_test.go` (create)
- Test: `internal/backtest/engine_reentry_test.go` (create)

- [ ] **Step 1: Scrivi i test falliti**

Crea `internal/strategy/variant_a_ext_test.go`:

```go
package strategy

import (
	"math"
	"testing"

	"github.com/atps/atps/internal/config"
)

func ctxFor(n int) *Context {
	ctx := &Context{
		Close:  make([]float64, n),
		High:   make([]float64, n),
		Low:    make([]float64, n),
		ATR:    make([]float64, n),
		SMA200: make([]float64, n),
	}
	for i := 0; i < n; i++ {
		ctx.Close[i] = 100
		ctx.High[i] = 101
		ctx.Low[i] = 99
		ctx.ATR[i] = 2
		ctx.SMA200[i] = 90
	}
	return ctx
}

func testCfgA(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.Load("../../configs/default.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestVariantAIntrabarEntry(t *testing.T) {
	cfg := testCfgA(t)
	s := NewA(cfg)
	n := 250
	ctx := ctxFor(n)
	ctx.Don20H = make([]float64, n)
	ctx.Don20L = make([]float64, n)
	for i := 0; i < n; i++ {
		ctx.Don20H[i] = 102
		ctx.Don20L[i] = 98
	}
	// trend up: close 100 > sma 90 → long abilitato, short no
	lv := s.IntrabarEntry(ctx, n-1)
	if !lv.Enabled {
		t.Fatalf("Enabled = false, want true")
	}
	if lv.LongLevel != 102 {
		t.Errorf("LongLevel = %v, want 102 (Don20H[i-1])", lv.LongLevel)
	}
	if !math.IsNaN(lv.ShortLevel) {
		t.Errorf("ShortLevel deve essere NaN (filtro SMA: close>sma)")
	}
	if lv.LongStopATR != cfg.VariantA.ATRStopMult {
		t.Errorf("LongStopATR = %v, want %v", lv.LongStopATR, cfg.VariantA.ATRStopMult)
	}
	// trend down: close < sma → short abilitato
	for i := 0; i < n; i++ {
		ctx.SMA200[i] = 110
	}
	lv = s.IntrabarEntry(ctx, n-1)
	if !math.IsNaN(lv.LongLevel) || lv.ShortLevel != 98 {
		t.Errorf("trend down: Long NaN e Short 98 attesi, avuti %v/%v", lv.LongLevel, lv.ShortLevel)
	}
	// warmup
	if s.IntrabarEntry(ctx, 100).Enabled {
		t.Errorf("warmup (< 200): Enabled deve essere false")
	}
	// SMA NaN (non calcolabile): entrambi i lati abilitati (comportamento Next attuale)
	for i := 0; i < n; i++ {
		ctx.SMA200[i] = math.NaN()
	}
	lv = s.IntrabarEntry(ctx, n-1)
	if math.IsNaN(lv.LongLevel) || math.IsNaN(lv.ShortLevel) {
		t.Errorf("SMA NaN: entrambi i lati devono restare abilitati")
	}
}

func reentryCtx(n int) *Context {
	ctx := ctxFor(n)
	ctx.Don20H = make([]float64, n)
	ctx.Don20L = make([]float64, n)
	for i := 0; i < n; i++ {
		ctx.Don20H[i] = 102
		ctx.Don20L[i] = 98
	}
	return ctx
}

func TestVariantAReEntry(t *testing.T) {
	cfg := testCfgA(t)
	cfg.VariantA.ReEntry = config.ReEntryCfg{Enabled: true, Lookback: 10, WithinBars: 20}
	s := NewA(cfg)
	n := 250
	ctx := reentryCtx(n)

	// scenario: stop-out long a bar 230; a bar 235 close 100>sma 90 e High[235]=103
	// nuovo high 10-barre (massimo High[225..234] = 101) → re-entry long
	for i := 225; i <= 234; i++ {
		ctx.High[i] = 101
	}
	ctx.High[235] = 103
	ctx.Close[235] = 102.5
	sig := s.ReEntry(ctx, 235, StopOutInfo{Side: 1, ExitBarIdx: 230})
	if sig.Side != 1 {
		t.Fatalf("atteso re-entry long, avuto side %d (%s)", sig.Side, sig.Reason)
	}
	wantStop := 102.5 - cfg.VariantA.ATRStopMult*2.0
	if math.Abs(sig.StopPrice-wantStop) > 1e-9 {
		t.Errorf("StopPrice = %v, want %v", sig.StopPrice, wantStop)
	}

	// finestra scaduta: stop-out a 210, i=235 → 25 > within 20 → niente
	if s2 := s.ReEntry(ctx, 235, StopOutInfo{Side: 1, ExitBarIdx: 210}); s2.Side != 0 {
		t.Errorf("finestra scaduta: side %d, want 0", s2.Side)
	}

	// trend contrario: sma sopra il close → niente re-entry long
	for i := 0; i < n; i++ {
		ctx.SMA200[i] = 110
	}
	if s3 := s.ReEntry(ctx, 235, StopOutInfo{Side: 1, ExitBarIdx: 230}); s3.Side != 0 {
		t.Errorf("trend contrario: side %d, want 0", s3.Side)
	}

	// disabled
	cfg.VariantA.ReEntry.Enabled = false
	if s4 := s.ReEntry(ctx, 235, StopOutInfo{Side: 1, ExitBarIdx: 230}); s4.Side != 0 {
		t.Errorf("disabled: side %d, want 0", s4.Side)
	}
}
```

E crea `internal/backtest/engine_reentry_test.go`:

```go
package backtest

import (
	"testing"
	"time"

	"github.com/atps/atps/internal/config"
	"github.com/atps/atps/internal/data"
	"github.com/atps/atps/internal/strategy"
)

// reentryStrat — entry long scriptata alla bar 2 (stop 95); ReEntry long quando
// last.ExitBarIdx == reentryAfterBar. Deterministico per test engine.
type reentryStrat struct {
	scriptStrategy
	reentryAfterBar int
}

func (s *reentryStrat) ReEntry(_ *strategy.Context, _ int, last strategy.StopOutInfo) strategy.Signal {
	if last.ExitBarIdx == s.reentryAfterBar {
		return strategy.Signal{Side: 1, Strength: 1, StopPrice: 95, Reason: "script reentry long"}
	}
	return strategy.Signal{Side: 0}
}

func TestEngineReEntryAfterStopOut(t *testing.T) {
	cfg, _ := config.Load("../../configs/default.yaml")
	cfg.Profit.Satellite.Enabled = false // test a posizione singola
	// 40 barre: segnale long alla bar 2 (stop 95, fill open bar 3 = 100);
	// discesa alle barre 5-7 (low 93 → stop colpito alla bar 5, exitBarIdx=5);
	// risalita dalle barre 8+ (low 95.5 > stop) → re-entry sopravvive fino a EOD.
	bars := flatBars(40, 100, 0.5)
	for i := 5; i <= 7; i++ {
		bars[i] = data.Bar{Time: time.Unix(int64(i)*14400, 0), Open: 97, High: 97.5, Low: 93, Close: 94, Volume: 100}
	}
	for i := 8; i < 40; i++ {
		bars[i] = data.Bar{Time: time.Unix(int64(i)*14400, 0), Open: 96, High: 101, Low: 95.5, Close: 100, Volume: 100}
	}
	signals := map[int]strategy.Signal{2: {Side: 1, Strength: 1, StopPrice: 95, Reason: "script long"}}
	strat := &reentryStrat{scriptStrategy{cfg: cfg, signals: signals}, 5} // re-entry dopo stop alla bar 5
	eng := EngineConfig{
		Variant: "A", Symbol: "TEST", InitialCapital: 10000,
		FeeBps: 4, SlippageBps: 2, UseNextOpen: true,
		PyramidingMax: 0, TrailMode: "donchian", DonExit: 20, EntryMode: "close",
	}
	res := Run(bars, strat, cfg, eng)
	if len(res.Trades) != 2 {
		t.Fatalf("attesi 2 trades (entry→stop, re-entry→eod), avuti %d", len(res.Trades))
	}
	if res.Trades[0].ExitReason != "stop" {
		t.Errorf("trade[0].ExitReason = %q, want stop", res.Trades[0].ExitReason)
	}
	if res.Trades[1].EntryReason != "script reentry long" {
		t.Errorf("trade[1].EntryReason = %q, want 'script reentry long'", res.Trades[1].EntryReason)
	}
	if res.Trades[1].ExitReason != "eod" {
		t.Errorf("trade[1].ExitReason = %q, want eod", res.Trades[1].ExitReason)
	}
}
```

- [ ] **Step 2: Verifica che fallisca**

Run: `go test ./internal/strategy/ -run TestVariantAIntrabar -v; go test ./internal/backtest/ -run TestEngineReEntry -v`
Expected: FAIL — `s.IntrabarEntry undefined`, `s.ReEntry undefined`; engine test: 1 trade solo.

- [ ] **Step 3: Implementa in variant_a.go**

Appendi a `internal/strategy/variant_a.go`:

```go
// IntrabarEntry — livelli entry stop-order (modalità intrabar engine).
// Livelli da barre < i SOLO (no lookahead); filtro SMA deciso sul close di i-1.
func (s *VariantA) IntrabarEntry(ctx *Context, i int) IntrabarEntryLevels {
	c := s.cfg.VariantA
	l := IntrabarEntryLevels{
		Enabled:      true,
		LongLevel:    math.NaN(),
		ShortLevel:   math.NaN(),
		LongStopATR:  c.ATRStopMult,
		ShortStopATR: c.ATRStopMult,
	}
	if i < s.Warmup() || i < 1 {
		l.Enabled = false
		return l
	}
	hh := ctx.Don20H[i-1]
	ll := ctx.Don20L[i-1]
	sma := ctx.SMA200[i-1]
	if math.IsNaN(hh) || math.IsNaN(ll) || math.IsNaN(ctx.ATR[i-1]) || ctx.ATR[i-1] <= 0 {
		l.Enabled = false
		return l
	}
	// close di i-1 PUO' essere > SMA200 → long; il canale 20 è sempre ≥ close (include la propria barra)
	if math.IsNaN(sma) || ctx.Close[i-1] > sma {
		l.LongLevel = hh
	}
	if math.IsNaN(sma) || ctx.Close[i-1] < sma {
		l.ShortLevel = ll
	}
	return l
}

// ReEntry — dopo stop-out: se il trend filter regge e la barra corrente fa un
// nuovo high/low sulle Lookback barre precedenti, entro WithinBars dallo stop.
func (s *VariantA) ReEntry(ctx *Context, i int, last StopOutInfo) Signal {
	r := s.cfg.VariantA.ReEntry
	zero := Signal{Side: 0, Reason: "no reentry"}
	if !r.Enabled || i < s.Warmup() || last.ExitBarIdx <= 0 {
		return zero
	}
	if i-last.ExitBarIdx > r.WithinBars {
		return zero
	}
	lo := i - r.Lookback
	if lo < 1 {
		lo = 1
	}
	if i-lo < 2 {
		return zero
	}
	atr := ctx.ATR[i]
	if math.IsNaN(atr) || atr <= 0 {
		return zero
	}
	sma := ctx.SMA200[i]
	closePx := ctx.Close[i]
	nh, nl := ctx.High[lo], ctx.Low[lo]
	for j := lo + 1; j < i; j++ {
		if ctx.High[j] > nh {
			nh = ctx.High[j]
		}
		if ctx.Low[j] < nl {
			nl = ctx.Low[j]
		}
	}
	mult := s.cfg.VariantA.ATRStopMult
	if last.Side == 1 && (math.IsNaN(sma) || closePx > sma) && ctx.High[i] > nh {
		return Signal{Side: 1, Strength: 1, StopPrice: closePx - mult*atr, Reason: "A reentry long (nuovo high)"}
	}
	if last.Side == -1 && (math.IsNaN(sma) || closePx < sma) && ctx.Low[i] < nl {
		return Signal{Side: -1, Strength: 1, StopPrice: closePx + mult*atr, Reason: "A reentry short (nuovo low)"}
	}
	return zero
}
```

Se nel test `TestVariantAReEntry` la `StopPrice` non torna: ATR[235]=2 e mult è `cfg.VariantA.ATRStopMult` (default.yaml = 1.8) → wantStop = 102.5 − 1.8×2 = 98.9. Il test usa `cfg.VariantA.ATRStopMult*2.0` → consistente.

- [ ] **Step 4: Verifica che passi**

Run: `go test ./internal/strategy/ ./internal/backtest/ -v`
Expected: PASS tutti.

- [ ] **Step 5: Commit**

```bash
git add internal/strategy/ internal/backtest/engine_reentry_test.go
git commit -m "feat(strategy): VariantA IntrabarEntry (livelli canale i-1 + filtro SMA) e ReEntry post stop-out (trend + nuovo high N-barre)"
```

---

### Task 5: Optimizer v2 — stadi, vincolo DD, leve nuove, base unificata

**Files:**
- Modify: `scripts/optimize/main.go` (rewrite completo)

- [ ] **Step 1: Riscrivi scripts/optimize/main.go**

Sostituisci l'intero file con:

```go
// optimize v2 — grid search a STADI con selezione DD-constrained, sul percorso
// config unificato (backtest.EngineConfigFrom — identico a CLI e bot live):
//
//	stage 1: grid base su TRAIN 70% → rank per Sharpe → top 30
//	stage 2: varianti curva DD {(7,17),(8,20),(10,25)} sui top 30 su TRAIN
//	         → selezione max CAGR con MaxDD ≥ −maxdd → top 10
//	stage 3: TEST 30% (una sola lettura) + full → vincitore
//
// Uso: go run ./scripts/optimize -symbol BTCUSDT -csv data/raw/BTCUSDT_4h.csv -variant A -maxdd 15
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

type combo struct {
	atrMult   float64
	trailMode string
	trailMult float64
	donExit   int
	pyrOn     bool
	pyrAdds   int
	satAlloc  float64
	riskPct   float64 // base = max
	entryMode string
	reentry   bool
	ddStart   float64
	ddFlat    float64
}

type result struct {
	c             combo
	trainSharpe   float64
	trainCAGR     float64
	trainDD       float64
	testSharpe    float64
	testCAGR      float64
	testDD        float64
	testCalmar    float64
	testTrades    int
	fullCAGR      float64
	fullSharpe    float64
	fullDD        float64
	fullReturnPct float64
}

func engineCfgPtr(cfg *config.Config, variant string) *config.EngineCfg {
	switch variant {
	case "A":
		return &cfg.VariantA.Engine
	case "B":
		return &cfg.VariantB.Engine
	case "C":
		return &cfg.VariantC.Engine
	default:
		return &cfg.VariantD.Engine
	}
}

func reentryCfgPtr(cfg *config.Config, variant string) *config.ReEntryCfg {
	switch variant {
	case "A":
		return &cfg.VariantA.ReEntry
	case "B":
		return &cfg.VariantB.ReEntry
	case "C":
		return &cfg.VariantC.ReEntry
	default:
		return &cfg.VariantD.ReEntry
	}
}

func setVariantStopMult(cfg *config.Config, variant string, m float64) {
	switch variant {
	case "A":
		cfg.VariantA.ATRStopMult = m
	case "B":
		cfg.VariantB.ATRStopMult = m
	case "C":
		cfg.VariantC.ATRStopMult = m
	default:
		cfg.VariantD.ATRStopMult = m
	}
}

func buildCfg(c combo, variant string) *config.Config {
	cfg, err := config.Load("configs/default.yaml")
	if err != nil {
		panic(err)
	}
	setVariantStopMult(cfg, variant, c.atrMult)
	eng := engineCfgPtr(cfg, variant)
	eng.TrailMode = c.trailMode
	eng.TrailATRMult = c.trailMult
	eng.DonExit = c.donExit
	eng.EntryMode = c.entryMode
	eng.PyramidingUnits = 0 // usa la sezione pyramiding: globale qui sotto
	cfg.Pyramiding.Enabled = c.pyrOn
	cfg.Pyramiding.MaxAdditions = c.pyrAdds
	cfg.Pyramiding.RiskNeutral = true
	cfg.Profit.Satellite.Enabled = c.satAlloc > 0
	cfg.Profit.Satellite.Allocation = c.satAlloc
	cfg.Risk.Base = c.riskPct
	cfg.Risk.Max = c.riskPct
	cfg.Risk.MaxRiskPerTradePct = c.riskPct * 100
	cfg.Risk.DDDeleverageStart = c.ddStart
	cfg.Risk.DDFlatPct = c.ddFlat
	re := reentryCfgPtr(cfg, variant)
	re.Enabled = c.reentry
	if c.reentry {
		re.Lookback = 10
		re.WithinBars = 20
	}
	return cfg
}

func runOnce(bars data.Bars, c combo, variant, symbol string) metrics.Stats {
	cfg := buildCfg(c, variant)
	strat := strategy.New(variant, cfg)
	eng := backtest.EngineConfigFrom(cfg, variant, symbol) // STESSO percorso di CLI/bot
	eng.InitialCapital = 10000
	res := backtest.Run(bars, strat, cfg, eng)
	return metrics.Compute(res)
}

func comboName(c combo) string {
	return fmt.Sprintf("atr%.1f %s%.1f don%d pyr(%v,a%d) sat%.1f r%.3f entry:%s reentry:%v dd(%.0f/%.0f)",
		c.atrMult, c.trailMode[:3], c.trailMult, c.donExit, c.pyrOn, c.pyrAdds, c.satAlloc, c.riskPct, c.entryMode, c.reentry, c.ddStart, c.ddFlat)
}

func main() {
	symbol := flag.String("symbol", "BTCUSDT", "")
	csvPath := flag.String("csv", "data/raw/BTCUSDT_4h.csv", "")
	variant := flag.String("variant", "A", "A/B/C/D")
	maxDD := flag.Float64("maxdd", 15.0, "vincolo DD train (%) per selezione CAGR")
	flag.Parse()

	bars, err := data.LoadBarsCSV(*csvPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load csv: %v\n", err)
		os.Exit(1)
	}
	split := int(float64(len(bars)) * 0.7)
	train, test := bars[:split], bars[split:]
	fmt.Printf("optimize v2 %s %s — %d bars, train %d (%s→%s), test %d (%s→%s), vincolo DD %.0f%%\n",
		*symbol, *variant, len(bars), len(train),
		train[0].Time.Format("2006-01"), train[len(train)-1].Time.Format("2006-01"),
		len(test), test[0].Time.Format("2006-01"), test[len(test)-1].Time.Format("2006-01"), *maxDD)

	// ── stage 1: grid base su train ──
	// Nota pruning vs spec §2: pyr adds {3,6} (il 4 è interpolativo tra 3 e 6);
	// satellite {0, 0.3, 0.4} completo. 2592 combos ≈ 2.9h a ~4s/run.
	// Per run più rapide: ridurre risk a {0.02, 0.025} → 1296 (~1.5h), documentandolo.
	var base []combo
	trailOpts := []struct {
		mode string
		mult float64
	}{{"donchian", 0}, {"chandelier", 2.5}, {"chandelier", 3.0}}
	for _, atr := range []float64{1.4, 1.6, 1.8} {
		for _, tr := range trailOpts {
			for _, de := range []int{10, 20} {
				for _, pyr := range []struct {
					on   bool
					adds int
				}{{false, 0}, {true, 3}, {true, 6}} {
					for _, risk := range []float64{0.015, 0.02, 0.025, 0.03} {
						for _, sat := range []float64{0, 0.3, 0.4} {
							for _, entry := range []string{"close", "intrabar"} {
								for _, re := range []bool{false, true} {
									base = append(base, combo{
										atrMult: atr, trailMode: tr.mode, trailMult: tr.mult,
										donExit: de, pyrOn: pyr.on, pyrAdds: pyr.adds,
										satAlloc: sat, riskPct: risk, entryMode: entry, reentry: re,
										ddStart: 10, ddFlat: 25, // curva attuale nella stage 1
									})
								}
							}
						}
					}
				}
			}
		}
	}
	fmt.Printf("stage 1: %d combos su train (≈ %.0f min)\n", len(base), float64(len(base))*4.0/60.0)

	type s1 struct {
		c  combo
		sh float64
	}
	var stage1 []s1
	t0 := time.Now()
	for i, c := range base {
		s := runOnce(train, c, *variant, *symbol)
		stage1 = append(stage1, s1{c, s.Sharpe})
		if (i+1)%200 == 0 {
			fmt.Printf("  s1 %d/%d (%.0fs)\n", i+1, len(base), time.Since(t0).Seconds())
		}
	}
	sort.Slice(stage1, func(a, b int) bool { return stage1[a].sh > stage1[b].sh })
	if len(stage1) > 30 {
		stage1 = stage1[:30]
	}

	// ── stage 2: curve DD sui top-30 ──
	ddOpts := [][2]float64{{7, 17}, {8, 20}, {10, 25}}
	type s2 struct {
		c    combo
		cagr float64
		dd   float64
		cal  float64
	}
	var stage2 []s2
	for _, x := range stage1 {
		for _, dd := range ddOpts {
			c := x.c
			c.ddStart, c.ddFlat = dd[0], dd[1]
			s := runOnce(train, c, *variant, *symbol)
			stage2 = append(stage2, s2{c, s.ReturnAnnual, s.MaxDD, s.Calmar})
		}
	}
	// selezione: max CAGR tra chi rispetta DD; se nessuno, max Calmar
	var feasible []s2
	for _, x := range stage2 {
		if x.dd >= -*maxDD {
			feasible = append(feasible, x)
		}
	}
	if len(feasible) > 0 {
		sort.Slice(feasible, func(a, b int) bool { return feasible[a].cagr > feasible[b].cagr })
	} else {
		fmt.Printf("NESSUNA combo rispetta DD %.0f%% sul train — fallback a max Calmar\n", *maxDD)
		feasible = stage2
		sort.Slice(feasible, func(a, b int) bool { return feasible[a].cal > feasible[b].cal })
	}
	if len(feasible) > 10 {
		feasible = feasible[:10]
	}

	// ── stage 3: test (una sola lettura) + full ──
	var results []result
	for _, x := range feasible {
		st := runOnce(test, x.c, *variant, *symbol)
		sf := runOnce(bars, x.c, *variant, *symbol)
		results = append(results, result{
			c: x.c, trainCAGR: x.cagr, trainDD: x.dd,
			testSharpe: st.Sharpe, testCAGR: st.ReturnAnnual, testDD: st.MaxDD,
			testCalmar: st.Calmar, testTrades: st.Trades,
			fullCAGR: sf.ReturnAnnual, fullSharpe: sf.Sharpe, fullDD: sf.MaxDD,
			fullReturnPct: sf.ReturnPct,
		})
	}
	sort.Slice(results, func(a, b int) bool { return results[a].testCalmar > results[b].testCalmar })

	fmt.Println("\n=== FINAL — top per TEST Calmar (train CAGR/DD, test, full) ===")
	fmt.Printf("%-72s %8s %7s | %8s %7s %7s %6s | %8s %7s %7s\n",
		"combo", "trCAGR%", "trDD%", "teCAGR%", "teDD%", "teCal", "teTrd", "fullCAGR%", "fullDD%", "fullRet%")
	for i, r := range results {
		if i >= 10 {
			break
		}
		fmt.Printf("%-72s %8.1f %7.1f | %8.1f %7.1f %7.2f %6d | %8.1f %7.1f %7.0f\n",
			comboName(r.c), r.trainCAGR, r.trainDD, r.testCAGR, r.testDD, r.testCalmar, r.testTrades, r.fullCAGR, r.fullDD, r.fullReturnPct)
	}

	fmt.Printf(`
PROTOCOLLO (spec §4) — prossimi passi manuali:
  1. gate test:     degrado CAGR train→test < 1/3 e Calmar test > 0
  2. walk-forward:  ./atps walk-forward --config configs/atps_v2.yaml --symbol %s --variant %s --csv %s --folds 8
  3. perturbazione: ./atps perturb --config configs/atps_v2.yaml --symbol %s --variant %s --csv %s
  4. ETH/SOL:       backtest con atps_v2.yaml, confronto vs btc_opt.yaml — degrado < 20%%
`, *symbol, *variant, *csvPath, *symbol, *variant, *csvPath)
}
```

- [ ] **Step 2: Verifica compilazione e smoke test rapido**

Run: `go vet ./scripts/optimize/ && go build ./...`
Expected: nessun errore.

- [ ] **Step 3: Commit**

```bash
git add scripts/optimize/main.go
git commit -m "feat(optimizer): v2 a stadi — selezione max CAGR con vincolo DD, entry intrabar/reentry in grid, base EngineConfigFrom unificata"
```

---

### Task 6: Run ottimizzazione + validazione + `configs/atps_v2.yaml` + report

**Files:**
- Create: `configs/atps_v2.yaml` (dopo la run)
- Create: `reports/V2_VALIDATION.md`
- Output: report HTML/JSON in `reports/`

> ATTENZIONE: la stage 1 (2592 run su train) richiede ~2.5-3h. Lancia con output su file. Se il tempo è un problema, ridurre `risk` a `{0.02, 0.025}` nella grid (1296 run, ~1.5h) — decisione da documentare in V2_VALIDATION.md.

- [ ] **Step 1: Baseline onesti freschi (per il confronto)**

```bash
./atps backtest --config configs/btc_opt.yaml --variant A --symbol BTCUSDT --csv data/raw/BTCUSDT_4h.csv --out reports/V2_BASELINE_BTC_A.html | tee reports/V2_BASELINE_BTC_A.txt
./atps backtest --config configs/btc_opt.yaml --variant A --symbol ETHUSDT --csv data/raw/ETHUSDT_4h.csv --out reports/V2_BASELINE_ETH_A.html | tee reports/V2_BASELINE_ETH_A.txt
./atps backtest --config configs/btc_opt.yaml --variant A --symbol SOLUSDT --csv data/raw/SOLUSDT_4h.csv --out reports/V2_BASELINE_SOL_A.html | tee reports/V2_BASELINE_SOL_A.txt
```

Annota i tre numeri CAGR/DD da stdout (servono dopo).

- [ ] **Step 2: Lancia optimizer v2 (BTC, variant A)**

```bash
go run ./scripts/optimize -symbol BTCUSDT -csv data/raw/BTCUSDT_4h.csv -variant A -maxdd 15 2>&1 | tee reports/V2_OPTIMIZE_RUN.txt
```

Expected: tabella FINAL con 10 righe; annota la riga migliore per test Calmar che passa il gate (degrado CAGR train→test < 1/3, Calmar test > 0).

- [ ] **Step 3: Crea `configs/atps_v2.yaml`**

Copia `configs/default.yaml` come base e applica i valori della combo vincitrice. Struttura delle modifiche (i valori `<>` si riempiono dalla riga BEST dell'output):

```yaml
# ATPS v2 — vincitore optimizer v2 BTC A, validato (reports/V2_VALIDATION.md)
# baseline onesto: btc_opt.yaml → CAGR <baseline> DD <dd>
risk:
  base: <riskPct>          # es 0.02
  max: <riskPct>
  max_risk_per_trade_pct: <riskPct*100>
  dd_deleverage_start_pct: <ddStart>   # es 7
  dd_flat_pct: <ddFlat>               # es 17
pyramiding:
  enabled: <pyrOn>
  max_additions: <pyrAdds>
  risk_neutral: true
profit:
  satellite:
    enabled: <satAlloc > 0>
    allocation: <satAlloc>
variant_a:
  atr_stop_mult: <atrMult>
  trail_mode: "<trailMode>"
  trail_atr_mult: <trailMult>
  don_exit: <donExit>
  entry_mode: "<entryMode>"
  reentry:
    enabled: <reentry>
    lookback: 10
    within_bars: 20
```

Tutte le altre sezioni restano identiche a default.yaml.

- [ ] **Step 4: Gate 1 — riproducibilità full (deve combaciare con colonna fullCAGR/fullDD dell'optimizer)**

```bash
./atps backtest --config configs/atps_v2.yaml --variant A --symbol BTCUSDT --csv data/raw/BTCUSDT_4h.csv --out reports/V2_BTC_A.html | tee reports/V2_BTC_A.txt
```

Expected: CAGR/DD entro ±0.5% dei valori `fullCAGR/fullDD` della riga BEST (se divergono → config scritta male, CORREGGERE prima di proseguire).

Gate: `fullDD ≥ −18%` e `fullCAGR > baseline BTC`.

- [ ] **Step 5: Gate 2 — walk-forward + perturbazione + Monte Carlo**

```bash
./atps walk-forward --config configs/atps_v2.yaml --symbol BTCUSDT --variant A --csv data/raw/BTCUSDT_4h.csv --folds 8 --out reports/V2_BTC_A_WF.json | tee reports/V2_BTC_A_WF.txt
./atps perturb --config configs/atps_v2.yaml --symbol BTCUSDT --variant A --csv data/raw/BTCUSDT_4h.csv --out reports/V2_BTC_A_PERTURB.json | tee reports/V2_BTC_A_PERTURB.txt
./atps montecarlo --config configs/atps_v2.yaml --symbol BTCUSDT --variant A --csv data/raw/BTCUSDT_4h.csv --runs 2000 --out reports/V2_BTC_A_MC.json | tee reports/V2_BTC_A_MC.txt
```

Gate: mediana Sharpe WF > 0; degrado perturbazione < 30%.

- [ ] **Step 6: Gate 3 — conferma ETH/SOL (nessuna ri-ottimizzazione)**

```bash
./atps backtest --config configs/atps_v2.yaml --variant A --symbol ETHUSDT --csv data/raw/ETHUSDT_4h.csv --out reports/V2_ETH_A.html | tee reports/V2_ETH_A.txt
./atps backtest --config configs/atps_v2.yaml --variant A --symbol SOLUSDT --csv data/raw/SOLUSDT_4h.csv --out reports/V2_SOL_A.html | tee reports/V2_SOL_A.txt
```

Gate: CAGR ETH ≥ 0.8× baseline ETH e CAGR SOL ≥ 0.8× baseline SOL.

- [ ] **Step 7: Scrivi `reports/V2_VALIDATION.md`**

Template (compilare con i numeri REALI degli output sopra):

```markdown
# ATPS v2 — Report validazione (2026-09-04)

Spec: docs/superpowers/specs/2026-09-04-atps-improve-cagr-dd15-design.md

## Combo vincitrice (optimizer v2, BTC A)
<comboName della riga BEST>

## Gate protocollo
| Gate | Criterio | Valore | Esito |
|---|---|---|---|
| Riproducibilità | CLI full == optimizer full (±0.5%) | CAGR <> vs <>, DD <> vs <> | PASS/FAIL |
| DD full | ≥ −18% | <>% | PASS/FAIL |
| CAGR full | > baseline 29.55% (btc_opt) | <>% | PASS/FAIL |
| Test holdout | degrado CAGR train→test < 1/3, Calmar test > 0 | <> → <> | PASS/FAIL |
| Walk-forward 8f | mediana Sharpe > 0 | <> | PASS/FAIL |
| Perturbazione ±20% | degrado < 30% | <>% | PASS/FAIL |
| ETH conferma | CAGR ≥ 0.8× baseline | <> vs <> | PASS/FAIL |
| SOL conferma | CAGR ≥ 0.8× baseline | <> vs <> | PASS/FAIL |

## Decisione
- [ ] TUTTI PASS → atps_v2.yaml promossa, feature come da combo
- [ ] qualche FAIL → disabilitare la feature in causa (entry_mode: close e/o
      reentry: enabled false), rieseguire Step 4-6, documentare qui

## Numeri finali (dopo decisione)
| Symbol | Config | CAGR | MaxDD | Sharpe | Trades |
|---|---|---|---|---|---|
| BTC | btc_opt (baseline) | | | | |
| BTC | atps_v2 | | | | |
| ETH | btc_opt (baseline) | | | | |
| ETH | atps_v2 | | | | |
| SOL | btc_opt (baseline) | | | | |
| SOL | atps_v2 | | | | |
```

- [ ] **Step 8: Commit**

```bash
git add configs/atps_v2.yaml reports/V2_*
git commit -m "feat(atps_v2): config vincitrice validata + report validazione protocollo (train/test/WF/perturb/ETH/SOL)"
```

---

### Task 7: Pulizia onestà — README, config stale, nota live

**Files:**
- Modify: `configs/btc_opt.yaml:99` (nome variante senza numeri aspirazionali)
- Modify: `README.md` (sezione performance + nota live entry_mode)

- [ ] **Step 1: Correggi nome variant_a in btc_opt.yaml**

Riga 99 di `configs/btc_opt.yaml`:

```yaml
  name: "Variant A — Classic Turtle OPT (post-fix engine, vedi reports/V2_VALIDATION.md)"
```

(stesso trattamento per `configs/best*.yaml` se contengono CAGR nei nomi: `grep -l "CAGR" configs/*.yaml` e rimuovi i numeri dai `name:`).

- [ ] **Step 2: README — sezione performance onesta**

In `README.md`, dopo la sezione "## Risk Engine", aggiorna/inserisci:

```markdown
## Performance verificata (2026-09-04, engine post-fix)

Numeri riproducibili con un comando — baseline `btc_opt.yaml` vs `atps_v2.yaml`
(vedi `reports/V2_VALIDATION.md` per protocollo completo train/test/walk-forward/perturbazione):

```bash
./atps backtest --config configs/atps_v2.yaml --variant A --symbol BTCUSDT --csv data/raw/BTCUSDT_4h.csv
```

| Symbol | Config | CAGR | MaxDD | Note |
|---|---|---|---|---|
| BTCUSDT | btc_opt (baseline) | 29.55% | −23.04% | motore corrente post-fix |
| BTCUSDT | atps_v2 | <compilare da V2_VALIDATION.md> | <...> | validato WF+perturb+ETH/SOL |

> Nota: i numeri CAGR >90% in commit precedenti (es. 94.26%) NON sono riproducibili
> sul motore corrente — si riferivano al motore pre-fix audit. Fidati solo di numeri
> che puoi rigenerare col comando sopra.

## entry_mode intrabar — limite live

`entry_mode: intrabar` è implementato nel **backtest** (fill a livello canale).
Il **bot live** genera ancora segnali close-mode su barra chiusa (poll 30s):
l'esecuzione intrabar live richiede stop-entry orders su Orderly (roadmap).
Il bot usa comunque la STESSA `EngineConfigFrom` del backtest per sizing/snapshot.
```

- [ ] **Step 3: Verifica finale completa**

```bash
gofmt -l . && go vet ./... && go test ./... && go build -o atps ./cmd/atps
```

Expected: gofmt vuoto, vet ok, tutti i test PASS, build ok.

- [ ] **Step 4: Commit**

```bash
git add configs/ README.md
git commit -m "docs: numeri onesti verificati in README, rimossi CAGR aspirazionali dai config, nota limite intrabar live"
```

---

## Riepilogo gate di successo (dallo spec)

1. DD full-history ≤ 18% (target 15%) — Task 6 Step 4
2. CAGR full-history > 29.55% baseline (target 35-50%) — Task 6 Step 4
3. CLI e bot live producono lo stesso EngineConfig — Task 2
4. WF mediana Sharpe > 0; perturb < 30% degrado; ETH/SOL ≥ 0.8× baseline — Task 6 Step 5-6
5. Tutti i numeri pubblicati riproducibili con un comando — Task 7

Se un gate strutturale fallisce (intrabar/reentry), la feature resta OFF in atps_v2.yaml e la decisione è documentata in V2_VALIDATION.md — il improve DD-constrained del Task 6 resta comunque valido.
