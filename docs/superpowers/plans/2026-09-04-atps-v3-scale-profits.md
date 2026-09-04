# ATPS v3 — Scala i profitti (Implementation Plan)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Promuovere risk-4% a config v3 validata, aggiungere guardrail anti-clipping silenzioso, ridisegnare il pyramid a gamba separata con uscita wide — tutto sotto protocollo di validazione con baseline atps_v2 e budget DD full ≤ 22%.

**Architecture:** (1) Task A solo-config: candidata v3 = atps_v2 + risk 4% coordinato, validata con protocollo (holdout/WF/perturb/MC/ETH-SOL); (2) Task C codice piccolo: `risk.ScalingCeiling` + `Result.Warnings/Ceiling/Binding/NotionalCapHits` + stampa CLI + righe report; (3) Task B codice: `pyramiding.mode=separate` crea posizioni indipendenti con stop proprio ed exit Don55 ( riusa ramo `DonExitLen==55` esistente); (4) Task B-validation: challenger separate contro baseline promossa, promote-or-off; (5) README + verifica finale.

**Tech Stack:** Go, yaml.v3, `go test`. Spec: `docs/superpowers/specs/2026-09-04-atps-v3-scale-profits-design.md`. Baseline efficaci misurati: v2 BTC 34.31%/-17.01 (test-window 12.4/-16.9/Cal 0.73); E6 risk-4% full 39.49%/-19.49/Sharpe 1.28/PF 1.82/tr 416.

**Convenzioni:** repo root `/mnt/lavoro/trading/turtle-extended`, branch main autorizzato. Dopo ogni task: `gofmt -l .` vuoto, `go vet ./...`, `go test ./...` verdi, commit. Baseline di confronto = **atps_v2** salvo Task 1 promuova v3 (Task 4 legge la "Decisione A" in V3_VALIDATION.md).

---

### Task 1 — A: candidata v3 risk-4% + validazione completa (nessun codice)

**Files:**
- Create: `configs/atps_v3.yaml` (copia atps_v2 + 7 modifiche)
- Create: `reports/V3_VALIDATION.md`, `reports/V3_*.txt` (+ html/json untracked ok)

- [ ] **Step 1: Crea configs/atps_v3.yaml**

```bash
cp configs/atps_v2.yaml configs/atps_v3.yaml
```

Poi applica ESATTAMENTE queste 7 modifiche (edit ai valori, nient'altro):

```yaml
risk:
  base: 0.04                  # 4.0% (era 0.02)
  max: 0.04                   # 4.0% (era 0.02)
  max_risk_per_trade_pct: 4.0 # (era 2.0)
  kelly_cap_pct: 4.0          # (era 2.0)
portfolio:
  max_open_risk: 0.06         # (era 0.03)
  max_correlated_risk: 0.04   # (era 0.02)
variant_a:
  risk_pct: 4.0               # (era 2.0) — LimitsFromConfig lo usa come MAX per variante
  name: "Variant A — ATPS v3 candidate (risk 4%, vedi reports/V3_VALIDATION.md)"
```

Verifica diff: `diff configs/atps_v2.yaml configs/atps_v3.yaml` deve mostrare SOLO queste righe (+ commenti toccati).

- [ ] **Step 2: Gate riproducibilità + holdout like-for-like**

```bash
go build -o atps ./cmd/atps
./atps backtest --config configs/atps_v3.yaml --variant A --symbol BTCUSDT --csv data/raw/BTCUSDT_4h.csv --out reports/V3_BTC_A.html | tee reports/V3_BTC_A.txt
go run ./scripts/baseline_split -config configs/atps_v3.yaml -csv data/raw/BTCUSDT_4h.csv -variant A | tee reports/V3_HOLDOUT_BTC.txt
```

Expected: full ≈ CAGR 39.49% DD -19.49% (±0.5%). Annota test-window v3 (teCAGR/teCal) da V3_HOLDOUT_BTC.txt.
Gate holdout: `v3 teCAGR ≥ 12.4 AND v3 teCal ≥ 0.73` (numeri v2 da reports/V2_OPTIMIZE_RUN.txt + V2_BASELINE_HOLDOUT_BTC.txt: baseline v2 test = 12.4/-16.9/Cal 0.73; baseline btc_opt test = 8.31/Cal 0.36).

- [ ] **Step 3: Gate WF + perturb + MC**

```bash
./atps walk-forward --config configs/atps_v3.yaml --symbol BTCUSDT --variant A --csv data/raw/BTCUSDT_4h.csv --folds 8 --out reports/V3_BTC_A_WF.json | tee reports/V3_BTC_A_WF.txt
./atps perturb --config configs/atps_v3.yaml --symbol BTCUSDT --variant A --csv data/raw/BTCUSDT_4h.csv --out reports/V3_BTC_A_PERTURB.json | tee reports/V3_BTC_A_PERTURB.txt
./atps montecarlo --config configs/atps_v3.yaml --symbol BTCUSDT --variant A --csv data/raw/BTCUSDT_4h.csv --runs 2000 --out reports/V3_BTC_A_MC.json | tee reports/V3_BTC_A_MC.txt
```

Gate: WF mediana Sharpe > 0; perturb degrado CAGR < 30% (ricava i CAGR dai return come fatto in V2: CAGR = (1+ret%)^(1/anni)-1, anni ≈ 6.7 su full; per perturb usa i total-return riportati); MC informativo.

- [ ] **Step 4: Gate ETH/SOL (nessuna ri-ottimizzazione)**

```bash
./atps backtest --config configs/atps_v3.yaml --variant A --symbol ETHUSDT --csv data/raw/ETHUSDT_4h.csv --out reports/V3_ETH_A.html | tee reports/V3_ETH_A.txt
./atps backtest --config configs/atps_v3.yaml --variant A --symbol SOLUSDT --csv data/raw/SOLUSDT_4h.csv --out reports/V3_SOL_A.html | tee reports/V3_SOL_A.txt
```

Gate: ETH CAGR ≥ 0.8 × 20.66 = 16.53% AND SOL CAGR ≥ 0.8 × 6.01 = 4.81% (baseline = atps_v2, numeri da reports/V2_VALIDATION.md).

- [ ] **Step 5: Scrivi reports/V3_VALIDATION.md**

Struttura (riempi <> con numeri REALI):

```markdown
# ATPS v3 — Report validazione (2026-09-04)

Baseline: configs/atps_v2.yaml (BTC 34.31/-17.01/Sharpe 1.50; test 12.4/-16.9/Cal 0.73).
Budget DD full: ≥ -22%.

## Decisione A — risk-4%
| Gate | Criterio | Valore | Esito |
|---|---|---|---|
| Riproducibilità | CLI == E6 misurato (39.49/-19.49 ±0.5%) | <> | <> |
| Holdout | v3 teCAGR ≥ 12.4 AND teCal ≥ 0.73 | <> vs <>, <> vs <> | <> |
| DD full | ≥ -22% | <>% | <> |
| WF 8f | mediana Sharpe > 0 | <> | <> |
| Perturb | degrado < 30% | <>% | <> |
| MC 2000 | informativo | p5 <> p50 <> p95 <> | INFO |
| ETH | CAGR ≥ 16.53% | <>% | <> |
| SOL | CAGR ≥ 4.81% | <>% | <> |

DECISIONE A: [PROMOSSA — atps_v3.yaml è la baseline per Task 4 / BOCCIATA — resta atps_v2, motivo: <>]

## Numeri finali A
| Symbol | Config | CAGR | MaxDD | Sharpe | PF | Trades |
|---|---|---|---|---|---|---|
| BTC | atps_v2 | 34.31 | -17.01 | 1.50 | 2.14 | 416 |
| BTC | v3-cand | <> | <> | <> | <> | <> |
| ETH | atps_v2 | 20.66 | -16.14 | 1.17 | 2.18 | 410 |
| ETH | v3-cand | <> | <> | <> | <> | <> |
| SOL | atps_v2 | 6.01 | -17.13 | 0.60 | 1.38 | 452 |
| SOL | v3-cand | <> | <> | <> | <> | <> |

## Decisione B — pyramid separate
(da compilare nel Task 4)
```

Se un gate fallisce: NON promuovere (lascia atps_v3.yaml come candidate non promossa? NO — se bocciata, ELIMINA configs/atps_v3.yaml e scrivi DECISIONE A: BOCCIATA con motivo; Task 4 userà atps_v2). Se promossa: aggiorna `variant_a.name` in "Variant A — ATPS v3 (risk 4% validata, vedi reports/V3_VALIDATION.md)".

- [ ] **Step 6: Commit**

```bash
git add configs/atps_v3.yaml reports/V3_* 2>/dev/null; git commit -m "feat(v3): candidata risk-4% + validazione protocollo (holdout/WF/perturb/MC/ETH-SOL) — decisione A documentata"
```

(Se bocciata: `git add reports/V3_*` senza config — il commit message termina con "BOCCIATA, resta v2".)

---

### Task 2 — C: guardrail scaling (ScalingCeiling + Warnings + NotionalCapHits)

**Files:**
- Modify: `internal/risk/risk.go` (ScalingCeiling + CappedByNotional)
- Modify: `internal/config/config.go` (PyramidingCfg.Mode)
- Modify: `internal/backtest/engine.go` (Result fields, Run wiring, EngineConfig.PyramidingMode)
- Modify: `internal/backtest/engineconfig.go` (legge Mode)
- Modify: `cmd/atps/main.go` (stampa CLI)
- Modify: `internal/report/report.go` (dataMap + righe template)
- Test: `internal/risk/risk_test.go` (append), `internal/backtest/engine_scaling_test.go` (create), `internal/config/config_test.go` (append Mode)

- [ ] **Step 1: Test falliti — risk.ScalingCeiling**

Appendi a `internal/risk/risk_test.go`:

```go
func TestScalingCeiling(t *testing.T) {
	lim := DefaultLimits() // MaxRisk 2.0, Kelly 2.0?, Corr 2.0, Heat 3.0 — leggi i default reali
	_ = lim
	// caso 1: kelly lega
	l1 := DefaultLimits()
	l1.MaxRiskPct = 4.0
	l1.KellyCapPct = 2.0
	l1.MaxCorrelatedPct = 4.0
	l1.MaxHeatPct = 6.0
	c, b := ScalingCeiling(l1)
	if c != 2.0 || b != "kelly_cap" {
		t.Errorf("caso kelly: ceiling %.2f (%s), want 2.0 (kelly_cap)", c, b)
	}
	// caso 2: correlated lega
	l2 := DefaultLimits()
	l2.MaxRiskPct = 4.0
	l2.KellyCapPct = 4.0
	l2.MaxCorrelatedPct = 3.0
	l2.MaxHeatPct = 6.0
	c, b = ScalingCeiling(l2)
	if c != 3.0 || b != "correlated" {
		t.Errorf("caso correlated: ceiling %.2f (%s), want 3.0 (correlated)", c, b)
	}
	// caso 3: niente lega (heat 6 sopra max 4)
	l3 := DefaultLimits()
	l3.MaxRiskPct = 4.0
	l3.KellyCapPct = 4.0
	l3.MaxCorrelatedPct = 4.0
	l3.MaxHeatPct = 6.0
	c, b = ScalingCeiling(l3)
	if c != 4.0 || b != "maxRisk" {
		t.Errorf("caso libero: ceiling %.2f (%s), want 4.0 (maxRisk)", c, b)
	}
	// caso 4: cap disabilitati (0) ignorati
	l4 := DefaultLimits()
	l4.MaxRiskPct = 4.0
	l4.KellyCapPct = 0
	l4.MaxCorrelatedPct = 0
	l4.MaxHeatPct = 0
	c, b = ScalingCeiling(l4)
	if c != 4.0 || b != "maxRisk" {
		t.Errorf("caso no-cap: ceiling %.2f (%s), want 4.0 (maxRisk)", c, b)
	}
}
```

PRIMA di scrivere il codice, verifica i default reali di DefaultLimits() (risk.go:63+): adatta il test se KellyCap default è 0 (nel test uso valori espliciti comunque — il test è autocontenuto, ok).

Run: `go test ./internal/risk/ -run TestScalingCeiling -v` → FAIL `undefined: ScalingCeiling`.

- [ ] **Step 2: Implementa ScalingCeiling + CappedByNotional in risk.go**

Dopo `DefaultLimits()` (o vicino a ValidateLimitInvariants), aggiungi:

```go
// ScalingCeiling — tetto statico al rischio per-trade per una fresh entry
// (heat/correlati a zero): minimo tra maxRisk e i cap attivi. Ritorna il tetto
// e il nome del vincolo legante ("maxRisk", "kelly_cap", "correlated", "heat").
// Cap a 0 = disabilitato e ignorato.
func ScalingCeiling(lim RiskLimits) (float64, string) {
	ceil := lim.MaxRiskPct
	binding := "maxRisk"
	if lim.KellyCapPct > 0 && lim.KellyCapPct < ceil {
		ceil = lim.KellyCapPct
		binding = "kelly_cap"
	}
	if lim.MaxCorrelatedPct > 0 && lim.MaxCorrelatedPct < ceil {
		ceil = lim.MaxCorrelatedPct
		binding = "correlated"
	}
	if lim.MaxHeatPct > 0 && lim.MaxHeatPct < ceil {
		ceil = lim.MaxHeatPct
		binding = "heat"
	}
	return ceil, binding
}
```

In `type SizingDecision`, aggiungi campo:

```go
	CappedByNotional bool     // true se qty è stata ridotta dal cap nozionali (lev dinamica o assoluto)
```

In `Size()`, nel blocco cap nozionali (dov'è `dec.Factors = append(dec.Factors, fmt.Sprintf("notional capped by dyn lev %.2fx → $%.0f", levCap, notional))`), aggiungi prima dell'append:

```go
		dec.CappedByNotional = true
```

(verifica che ci sia UN SOLO punto dove notional viene ridotto: `if notional > capNotional {`. Se ce ne sono due, metti il flag in entrambi.)

- [ ] **Step 3: Test falliti — config Mode + engine wiring**

Appendi a `internal/config/config_test.go`:

```go
func TestPyramidingModeDefault(t *testing.T) {
	yml := `pyramiding:
  enabled: true
  max_additions: 4
  risk_neutral: true
`
	path := filepath.Join(t.TempDir(), "cfg.yaml")
	if err := os.WriteFile(path, []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Pyramiding.Mode != "" {
		t.Errorf("Mode default deve essere \"\" (merged), avuto %q", c.Pyramiding.Mode)
	}
	yml2 := `pyramiding:
  mode: separate
  enabled: true
  max_additions: 6
`
	path2 := filepath.Join(t.TempDir(), "cfg2.yaml")
	if err := os.WriteFile(path2, []byte(yml2), 0o644); err != nil {
		t.Fatal(err)
	}
	c2, err := Load(path2)
	if err != nil {
		t.Fatal(err)
	}
	if c2.Pyramiding.Mode != "separate" {
		t.Errorf("Mode = %q, want separate", c2.Pyramiding.Mode)
	}
}
```

Crea `internal/backtest/engine_scaling_test.go`:

```go
package backtest

import (
	"strings"
	"testing"

	"github.com/atps/atps/internal/config"
)

func TestScalingWarningOnClippedRisk(t *testing.T) {
	cfg, err := config.Load("../../configs/default.yaml")
	if err != nil {
		t.Fatal(err)
	}
	// risk richiesto 4% ma kelly 2% → warning + ceiling 2.0 (kelly_cap)
	cfg.Risk.Base = 0.04
	cfg.Risk.Max = 0.04
	cfg.Risk.MaxRiskPerTradePct = 4.0
	cfg.Risk.KellyCapPct = 2.0
	cfg.Profit.Satellite.Enabled = false
	bars := flatBars(30, 100, 0.4)
	bars = append(bars, flatBars(10, 100, 0.4)...)
	// flatStrat (Next sempre flat): 0 trade, ma ceiling/warning calcolati all'avvio
	res := Run(bars, &flatStrat{scriptStrategy{cfg: cfg}}, cfg, EngineConfig{
		Variant: "A", Symbol: "TEST", InitialCapital: 10000,
		FeeBps: 4, SlippageBps: 2, UseNextOpen: true,
		PyramidingMax: 0, TrailMode: "donchian", DonExit: 20, EntryMode: "close",
	})
	if res.ScalingCeilingPct != 2.0 || res.ScalingBinding != "kelly_cap" {
		t.Errorf("ceiling %.2f (%s), want 2.0 (kelly_cap)", res.ScalingCeilingPct, res.ScalingBinding)
	}
	found := false
	for _, w := range res.Warnings {
		if strings.Contains(w, "kelly_cap") {
			found = true
		}
	}
	if !found {
		t.Errorf("Warnings %+v: atteso warning kelly_cap", res.Warnings)
	}
}

func TestNotionalCapHitsCounter(t *testing.T) {
	cfg, err := config.Load("../../configs/default.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Profit.Satellite.Enabled = false
	cfg.Risk.MaxNotional = 1000 // forza il cap su ogni entry (qty×price >> 1000)
	bars := flatBars(30, 100, 0.4)
	bars = append(bars, flatBars(10, 100, 0.4)...)
	res := Run(bars, &flatStrat{cfg: cfg}, cfg, EngineConfig{
		Variant: "A", Symbol: "TEST", InitialCapital: 10000,
		FeeBps: 4, SlippageBps: 2, UseNextOpen: true,
		PyramidingMax: 0, TrailMode: "donchian", DonExit: 20, EntryMode: "close",
	})
	if res.NotionalCapHits < 1 {
		t.Errorf("NotionalCapHits = %d, want ≥ 1 con MaxNotional 1000", res.NotionalCapHits)
	}
}
```

con helper (stesso file):

```go
// flatStrat — Next sempre flat (signals nil → zero Signal): nessuna entry,
// serve per testare il wiring di avvio (ceiling/warning) senza trade.
type flatStrat struct {
	scriptStrategy
}
```

- [ ] **Step 4: Verifica che fallisca**

Run: `go test ./internal/risk/ -run TestScalingCeiling -v; go test ./internal/backtest/ -run "TestScalingWarning|TestNotionalCap" -v; go test ./internal/config/ -run TestPyramidingModeDefault -v`
Expected: FAIL (undefined ScalingCeiling; undefined Mode; undefined ScalingCeilingPct/Warnings/NotionalCapHits).

- [ ] **Step 5: Implementa**

**5a. config.go** — in `type PyramidingCfg`, aggiungi in testa:

```go
	Mode         string `yaml:"mode"` // merged|separate ("" = merged, comportamento attuale)
```

**5b. engine.go — Result struct**, aggiungi campi (dopo MaxHeatSeen):

```go
	Warnings           []string `json:"warnings,omitempty"`
	ScalingCeilingPct  float64  `json:"scaling_ceiling_pct"`
	ScalingBinding     string   `json:"scaling_binding"`
	NotionalCapHits    int      `json:"notional_cap_hits"`
```

**5c. engine.go — EngineConfig struct**, aggiungi dopo PyramidStepATR (o vicino a PyramidingMax):

```go
	PyramidingMode string // merged|separate (default merged)
```

**5d. engine.go — Run()**, dopo `res.RiskLimitsUsed = lim` (riga ~134), aggiungi:

```go
	// ── scaling guardrails: tetto effettivo + warning (nessun cambio sizing) ──
	res.ScalingCeilingPct, res.ScalingBinding = risk.ScalingCeiling(lim)
	if res.ScalingCeilingPct < lim.MaxRiskPct {
		res.Warnings = append(res.Warnings, fmt.Sprintf("scaling: risk richiesto %.2f%% → tetto effettivo %.2f%% (%s lega)",
			lim.MaxRiskPct, res.ScalingCeilingPct, res.ScalingBinding))
	}
	if eng.PyramidingMode == "separate" && lim.PyramidingRiskNeutral {
		res.Warnings = append(res.Warnings, "pyramiding.mode=separate ignora risk_neutral (vale solo per merged)")
	}
```

(engine.go importa già `fmt`? verifica — se manca, aggiungi `"fmt"` agli import.)

**5e. engine.go — conteggio NotionalCapHits**: nei DUE punti dove viene chiamato `risk.Size` (fresh entry `dec := risk.Size(ms, lim)` e pyramid `dec := risk.Size(ms, lim)`), subito dopo aggiungi:

```go
						if dec.CappedByNotional {
							res.NotionalCapHits++
						}
```

(indentazione al livello del blocco; `res` è accessibile — è creato a inizio Run.)

**5f. engineconfig.go** — in EngineConfigFrom, prima del `return EngineConfig{...}`, aggiungi:

```go
	pyrMode := cfg.Pyramiding.Mode
	if pyrMode == "" {
		pyrMode = "merged"
	}
```

e nel literal: `PyramidingMode: pyrMode,`.

**5g. cmd/atps/main.go** — nel comando backtest, subito dopo la riga `fmt.Printf("%s %s: Return %.2f%% ...` (riga ~216), aggiungi:

```go
			fmt.Printf("scaling ceiling: %.2f%% (%s lega)\n", res.ScalingCeilingPct, res.ScalingBinding)
			for _, w := range res.Warnings {
				fmt.Printf("warn: %s\n", w)
			}
			if res.NotionalCapHits > 0 {
				fmt.Printf("notional cap: %d entry ridotte dal cap nozionali\n", res.NotionalCapHits)
			}
```

**5h. report.go** — in dataMap (dopo `"DDFlat": lim.DDFlatPct,`) aggiungi:

```go
		"ScaleCeil":     in.Result.ScalingCeilingPct,
		"ScaleBinding":  in.Result.ScalingBinding,
		"NotionalHits":  in.Result.NotionalCapHits,
		"Warnings":      in.Result.Warnings,
```

e nel template, dopo la riga con `Heat max (portafoglio)` (riga ~529, il div con `{{fmtPct .MaxHeat}}`), aggiungi una card analoga:

```html
      <div style="background:#0f172a;padding:10px;border-radius:8px;border:1px solid #1f2937"><div class="small">Tetto scaling (risk effettivo)</div><div style="font-size:18px;font-weight:800">{{fmtPct .ScaleCeil}} <span class="small">({{ .ScaleBinding }} lega)</span></div></div>
```

e nella nota sizing (riga ~531, quella con `Heat budget {{fmtPct .HeatLimit}}`), aggiungi in coda prima di `</div>`:

```html
 Notional cap hits: {{ .NotionalHits }}.{{range .Warnings}} <b style="color:#fbbf24">Avviso: {{ . }}</b>{{end}}
```

(leggi le righe 525-532 del file per l'indentazione esatta e adatta.)

- [ ] **Step 6: Verifica**

Run: `go test ./internal/risk/ ./internal/backtest/ ./internal/config/ -v` → PASS.
Run: `gofmt -l .` vuoto; `go vet ./...`.
Regressione: backtest v2 e v3 invariati (i campi sono additivi; Mode "" = merged):
```bash
go build -o atps ./cmd/atps
./atps backtest --config configs/atps_v2.yaml --variant A --symbol BTCUSDT --csv data/raw/BTCUSDT_4h.csv --out /tmp/opencode/c2_v2.html | tee /tmp/opencode/c2_v2.txt
./atps backtest --config configs/atps_v3.yaml --variant A --symbol BTCUSDT --csv data/raw/BTCUSDT_4h.csv --out /tmp/opencode/c2_v3.html | tee /tmp/opencode/c2_v3.txt
```
Expected: v2 → 34.31%/-17.01% + riga `scaling ceiling: 2.00% (kelly_cap lega)`? ATTENZIONE: v2 ha risk max 2.0 E kelly 2.0 E corr 2.0 → ceiling = 2.0 = max → NESSUN warning (ceiling < max è falso: 2.0 < 2.0 falso). Binding sarà "maxRisk" o "kelly_cap"/"correlated" a pari merito (il primo minore stretto: maxRisk 2.0 resta, kelly 2.0 non < 2.0 → resta maxRisk). Quindi v2: `scaling ceiling: 2.00% (maxRisk lega)`, zero warn. v3 (max 4, kelly 4, corr 4, heat 6): ceiling 4.0 (maxRisk), zero warn. Verifica che i numeri CAGR/DD siano IDENTICI al pre-change e che le righe ceiling compaiano. Se un warning appare su v2/v3 → bug (i cap sono allineati al max).

- [ ] **Step 7: Commit**

```bash
git add internal/risk/ internal/config/ internal/backtest/ cmd/atps/main.go internal/report/
git commit -m "feat(scaling): ScalingCeiling + Warnings + NotionalCapHits — niente più clipping silenzioso; pyramiding.mode merged|separate (default merged)"
```

---

### Task 3 — B codice: pyramid `mode: separate` (gamba indipendente, exit Don55)

**Files:**
- Modify: `internal/backtest/engine.go` (ramo pyramid)
- Test: `internal/backtest/engine_pyramid_separate_test.go` (create)

Contesto ramo pyramid esistente (engine.go ~r.585-670): se `hasSameSide` (earliest != nil) e `risk.CanPyramid(earliest.EntryPrice, bar.Close, atr, sig.Side, sameSideUnits, eng.PyramidingMax, eng.PyramidStepATR)` (sameSideUnits = earliest.Units), allora `dec := risk.Size(ms, lim)`; se `lim.PyramidingRiskNeutral` → merge dimezzato in earliest; else merge pieno in earliest (qty/media entry/risk/stop update). Fee/slip: `fee := fillPrice*dec.Qty*eng.FeeBps/10000`, `slipCost := slipAmt*dec.Qty`, `equity -= fee`, totali aggiornati.

- [ ] **Step 1: Scrivi i test falliti**

Crea `internal/backtest/engine_pyramid_separate_test.go`:

```go
package backtest

import (
	"testing"
	"time"

	"github.com/atps/atps/internal/config"
	"github.com/atps/atps/internal/data"
	"github.com/atps/atps/internal/strategy"
)

// sepStrat — long scriptato a bar 2 (stop 90) e bar 4 (stop 98, per add);
// ReEntry mai. DonExit 10 per il core.
type sepStrat struct {
	scriptStrategy
}

func sepCfg(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.Load("../../configs/default.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Profit.Satellite.Enabled = false
	return cfg
}

func sepEng() EngineConfig {
	return EngineConfig{
		Variant: "A", Symbol: "TEST", InitialCapital: 10000,
		FeeBps: 4, SlippageBps: 2, UseNextOpen: true,
		PyramidingMax: 4, PyramidStepATR: 0.5, PyramidingMode: "separate",
		TrailMode: "donchian", DonExit: 10, EntryMode: "close",
	}
}

func TestSeparatePyramidCreatesIndependentLeg(t *testing.T) {
	cfg := sepCfg(t)
	// barre piatte 100; entry bar 2 (fill open bar 3 = 100); prezzo sale a 103
	// (CanPyramid: entry 100 + 0.5×ATR(~1)×1 = 100.5 ≤ 103) → add a bar 5
	// (segnale), fill open bar 6.
	bars := flatBars(40, 100, 0.5)
	for i := 4; i < 40; i++ {
		bars[i] = data.Bar{Time: time.Unix(int64(i)*14400, 0), Open: 102, High: 103.5, Low: 101.5, Close: 103, Volume: 100}
	}
	signals := map[int]strategy.Signal{
		2: {Side: 1, Strength: 1, StopPrice: 98, Reason: "script core"},
		5: {Side: 1, Strength: 1, StopPrice: 101, Reason: "script add"},
	}
	strat := &sepStrat{scriptStrategy{cfg: cfg, signals: signals}}
	res := Run(bars, strat, cfg, sepEng())
	if len(res.Trades) != 2 {
		t.Fatalf("attesi 2 trades a EOD (core + gamba), avuti %d", len(res.Trades))
	}
	var core, leg *Trade
	for i := range res.Trades {
		if res.Trades[i].EntryReason == "script core" {
			core = &res.Trades[i]
		} else {
			leg = &res.Trades[i]
		}
	}
	if core == nil || leg == nil {
		t.Fatalf("trade core/leg non trovati: %+v", res.Trades)
	}
	// core NON fuso: qty core = sizing iniziale (add non somma)
	if core.Qty == leg.Qty && core.EntryPrice == leg.EntryPrice {
		t.Errorf("core e leg sembrano la stessa posizione fusa")
	}
	if leg.EntryReason != "script add | pyramid separate" {
		t.Errorf("leg EntryReason = %q", leg.EntryReason)
	}
	// stop proprio della gamba = stop del segnale (101), non trailing del core
	if leg.StopPrice != 101 {
		t.Errorf("leg StopPrice = %v, want 101 (stop proprio)", leg.StopPrice)
	}
}

func TestSeparatePyramidWideExitSurvivesCoreExit(t *testing.T) {
	cfg := sepCfg(t)
	// core entra bar 2 (fill 100, stop 98); add bar 5 (fill ~102, stop proprio 97);
	// poi close crolla a 99 (sotto Don10 ~101.5 → core esce donchian_exit) ma sopra
	// Don55 (~93) e sopra lo stop 97 della leg → leg sopravvive a EOD.
	bars := flatBars(40, 100, 0.5)
	for i := 4; i < 10; i++ {
		bars[i] = data.Bar{Time: time.Unix(int64(i)*14400, 0), Open: 102, High: 103.5, Low: 101.5, Close: 103, Volume: 100}
	}
	for i := 10; i < 40; i++ {
		bars[i] = data.Bar{Time: time.Unix(int64(i)*14400, 0), Open: 99, High: 99.5, Low: 98.5, Close: 99, Volume: 100}
	}
	signals := map[int]strategy.Signal{
		2: {Side: 1, Strength: 1, StopPrice: 98, Reason: "script core"},
		5: {Side: 1, Strength: 1, StopPrice: 97, Reason: "script add"},
	}
	strat := &sepStrat{scriptStrategy{cfg: cfg, signals: signals}}
	res := Run(bars, strat, cfg, sepEng())
	if len(res.Trades) != 2 {
		t.Fatalf("attesi 2 trades (core exit + leg eod), avuti %d", len(res.Trades))
	}
	if res.Trades[0].EntryReason != "script core" || res.Trades[0].ExitReason != "donchian_exit" {
		t.Errorf("trade[0] = %s/%s, want script core/donchian_exit", res.Trades[0].EntryReason, res.Trades[0].ExitReason)
	}
	if res.Trades[1].EntryReason != "script add | pyramid separate" {
		t.Errorf("trade[1] EntryReason = %q", res.Trades[1].EntryReason)
	}
	if res.Trades[1].ExitReason != "eod" {
		t.Errorf("trade[1] ExitReason = %q, want eod (leg sopravvive con exit Don55)", res.Trades[1].ExitReason)
	}
	if res.Trades[1].StopPrice != 97 {
		t.Errorf("leg StopPrice = %v, want 97 (stop proprio del segnale)", res.Trades[1].StopPrice)
	}
}

func TestSeparatePyramidRespectsMaxUnits(t *testing.T) {
	cfg := sepCfg(t)
	// heat alzato: 4 posizioni × 1% devono passare tutte (default heat 3% ne bloccherebbe la 4ª)
	cfg.Portfolio.MaxOpenRisk = 0.10
	cfg.Portfolio.MaxCorrelatedRisk = 0.10
	bars := flatBars(40, 100, 0.5)
	for i := 4; i < 40; i++ {
		bars[i] = data.Bar{Time: time.Unix(int64(i)*14400, 0), Open: 102, High: 103.5, Low: 101.5, Close: 103, Volume: 100}
	}
	// segnali ogni barra 2..10 → con PyramidingMax 4: 1 core + 3 add max
	signals := map[int]strategy.Signal{}
	for b := 2; b <= 10; b++ {
		signals[b] = strategy.Signal{Side: 1, Strength: 1, StopPrice: 98, Reason: "script"}
	}
	strat := &sepStrat{scriptStrategy{cfg: cfg, signals: signals}}
	res := Run(bars, strat, cfg, sepEng())
	if len(res.Trades) != 4 {
		t.Fatalf("attesi 4 trades (1 core + 3 add, max 4 unità), avuti %d", len(res.Trades))
	}
}
```

- Test 2: a bar 10 close 99 < donExitL(10)[9] (≈101.5) → core esce donchian_exit; leg con stop 97: Low 98.5 > 97 e close 99 > Don55 (~93) → sopravvive a EOD.
- Test 3: segnali barre 2..10 con prezzo flat 103 da bar 4: core entra (fill bar 3 = 100); add quando CanPyramid: bar 4 close 103 ≥ 100.5 ✓ add#1 (fill bar 5); bar 5: count=2 → trigger 100+0.5×2=101 ≤ 103 ✓ add#2; bar 6: count=3 → trigger 101.5 ≤ 103 ✓ add#3; bar 7: count=4 ≥ max 4 → stop. Totale 4 posizioni → EOD 4 trades ✓. MA: i segnali alle barre 7..10 con posizioni esistenti vanno nel ramo pyramid (CanPyramid falso → niente). ✓. E heat: risk 1% × 4 = 4% > heat cap 3%! risk.Size taglia il 4° sizing (heat cap total) ma qty > 0 comunque → posizione creata ✓ (4 trades). E satellite OFF ✓. Don10 exit: close 103 > donL ✓ nessun exit. EOD 4 ✓.

- [ ] **Step 2: Verifica che fallisca**

Run: `go test ./internal/backtest/ -run "TestSeparatePyramid" -v`
Expected: FAIL (1 trade solo — gli add si fondono in earliest).

- [ ] **Step 3: Implementa nel ramo pyramid di engine.go**

Nel ramo `if risk.CanPyramid(...)` (dopo il calcolo `dec := risk.Size(ms, lim)` e il conteggio NotionalCapHits del Task 2), inserisci come PRIMA cosa:

```go
						if eng.PyramidingMode == "separate" {
							// gamba indipendente: stop proprio + exit wide Don55
							fee := fillPrice * dec.Qty * eng.FeeBps / 10000.0
							slipCost := slipAmt * dec.Qty
							equity -= fee
							totalFee += fee
							totalSlippage += slipCost
							leg := &Position{
								Symbol: eng.Symbol, Side: sig.Side, Qty: dec.Qty,
								EntryPrice: fillPrice, EntryTime: fillTime, EntryATR: atr,
								StopPrice: stopPx, Units: 1, EntryBarIdx: i,
								RiskPct: dec.RiskPct, Leverage: dec.Leverage,
								Notional: dec.Notional, RiskAmount: dec.RiskAmount,
								SizingLog:   logFactors(dec) + " | pyramid separate (wide Don55)",
								EntryFee:    fee, EntryReason: sig.Reason + " | pyramid separate",
								IsSatellite: false, DonExitLen: 55,
							}
							positions = append(positions, leg)
						} else if lim.PyramidingRiskNeutral {
```

e il resto del ramo esistente (`if lim.PyramidingRiskNeutral {...} else {...}`) diventa `} else if ... } else {...}` — cioè trasforma l'`if` esistente in `else if`. Verifica che fee/slip/equity accounting del nuovo ramo replichi quello merged.

E la condizione di gate per separate: il `CanPyramid` esistente usa `sameSideUnits = earliest.Units` (merged counting). Per separate, il conteggio logico = posizioni same-side NON satellite. Sostituisci il calcolo:

```go
				sameSideUnits := 0
				if earliest != nil {
					sameSideUnits = earliest.Units
					// If satellite split active, the total Units is ~2× logical, adjust maxUnits logic by using earliest only
				}
```

con:

```go
				sameSideUnits := 0
				if earliest != nil {
					sameSideUnits = earliest.Units
					// If satellite split active, the total Units is ~2× logical, adjust maxUnits logic by using earliest only
				}
				if eng.PyramidingMode == "separate" {
					// unità logiche = core + gambe (satellite escluso: non partecipa al pyramid)
					sameSideUnits = 0
					for _, p := range positions {
						if p.Side == sig.Side && !p.IsSatellite {
							sameSideUnits++
						}
					}
				}
```

(il CanPyramid successivo usa sameSideUnits per trigger step×units e cap maxUnits — per separate: count include core → primo add a step×1 ✓, stop a count ≥ max ✓.)

- [ ] **Step 4: Verifica**

Run: `go test ./internal/backtest/ -v` → PASS tutti (merged invariato dai test esistenti).
Regressione close/no-pyr invariata + v2/v3 numeri identici:
```bash
go build -o atps ./cmd/atps
./atps backtest --config configs/atps_v2.yaml --variant A --symbol BTCUSDT --csv data/raw/BTCUSDT_4h.csv --out /tmp/opencode/b3_v2.html 2>&1 | grep "BTCUSDT A:"
./atps backtest --config configs/atps_v3.yaml --variant A --symbol BTCUSDT --csv data/raw/BTCUSDT_4h.csv --out /tmp/opencode/b3_v3.html 2>&1 | grep "BTCUSDT A:"
```
Expected: v2 34.31%/-17.01%, v3 = numero Task 1 (mode default merged → zero change).

- [ ] **Step 5: Commit**

```bash
git add internal/backtest/
git commit -m "feat(pyramid): mode separate — gambe indipendenti con stop proprio ed exit wide Don55 (default merged invariato)"
```

---

### Task 4 — B validation: challenger separate vs baseline + decisione

**Files:**
- Create: `configs/atps_v3_sep.yaml` (challenger, temporanea se bocciata)
- Modify: `reports/V3_VALIDATION.md` (sezione Decisione B), `configs/atps_v3.yaml` (solo se promossa), `README.md` (solo se promossa)

BASELINE = configs/atps_v3.yaml se Decisione A = PROMOSSA, altrimenti configs/atps_v2.yaml (leggi reports/V3_VALIDATION.md).

- [ ] **Step 1: Challenger config**

Copia la BASELINE in `configs/atps_v3_sep.yaml` e applica:
```yaml
pyramiding:
  enabled: true
  mode: separate
  max_additions: 6
  risk_neutral: true   # ignorato con separate (warning atteso in output!)
```
(heat/corr/kelly restano quelli della baseline → le gambe competono per lo stesso budget.)

- [ ] **Step 2: Run challenger (BTC full + holdout)**

```bash
go build -o atps ./cmd/atps
./atps backtest --config configs/atps_v3_sep.yaml --variant A --symbol BTCUSDT --csv data/raw/BTCUSDT_4h.csv --out reports/V3_SEP_BTC_A.html | tee reports/V3_SEP_BTC_A.txt
go run ./scripts/baseline_split -config configs/atps_v3_sep.yaml -csv data/raw/BTCUSDT_4h.csv -variant A | tee reports/V3_SEP_HOLDOUT_BTC.txt
```

Annota: full CAGR/DD, test teCAGR/teCal. Gate holdout: `sep teCAGR ≥ baseline teCAGR AND sep teCal ≥ baseline teCal`. Verifica che l'output mostri il warning `pyramiding.mode=separate ignora risk_neutral`.

- [ ] **Step 3: Se holdout PASS → WF + perturb + ETH/SOL; se FAIL → stop qui**

```bash
./atps walk-forward --config configs/atps_v3_sep.yaml --symbol BTCUSDT --variant A --csv data/raw/BTCUSDT_4h.csv --folds 8 --out reports/V3_SEP_BTC_A_WF.json | tee reports/V3_SEP_BTC_A_WF.txt
./atps perturb --config configs/atps_v3_sep.yaml --symbol BTCUSDT --variant A --csv data/raw/BTCUSDT_4h.csv --out reports/V3_SEP_BTC_A_PERTURB.json | tee reports/V3_SEP_BTC_A_PERTURB.txt
./atps backtest --config configs/atps_v3_sep.yaml --variant A --symbol ETHUSDT --csv data/raw/ETHUSDT_4h.csv --out reports/V3_SEP_ETH_A.html | tee reports/V3_SEP_ETH_A.txt
./atps backtest --config configs/atps_v3_sep.yaml --variant A --symbol SOLUSDT --csv data/raw/SOLUSDT_4h.csv --out reports/V3_SEP_SOL_A.html | tee reports/V3_SEP_SOL_A.txt
```

Gate: full DD ≥ -22%; WF mediana Sharpe > 0; perturb < 30%; ETH/SOL degrado < 20% vs baseline stesso simbolo.

- [ ] **Step 4: Decisione + V3_VALIDATION.md + eventuale promozione**

Compila la sezione "Decisione B" in V3_VALIDATION.md (tabella gate come Decisione A + DECISIONE B: PROMOSSA/BOCCIATA).
- Se PROMOSSA: applica la sezione pyramiding del challenger in configs/atps_v3.yaml (enabled/mode/max_additions), aggiorna variant_a.name se contiene "candidate", aggiorna README tabella con riga sep, commit tutto.
- Se BOCCIATA: ELIMINA configs/atps_v3_sep.yaml (`git rm` se committato? NO — non committarlo mai se bocciato: `rm`), documenta motivo, commit solo reports/V3_VALIDATION.md + reports/V3_SEP_*.

```bash
git add reports/V3_* configs/atps_v3.yaml README.md 2>/dev/null; git commit -m "feat(v3): decisione B pyramid separate — [PROMOSSA in atps_v3 | BOCCIATA resta OFF documentato]"
```

---

### Task 5 — README + verifica finale

**Files:**
- Modify: `README.md`
- Verify: tutto

- [ ] **Step 1: README**

Nella sezione `## Performance verificata`, aggiungi riga v3 (numeri da V3_VALIDATION.md Decisione A; se B promossa, riga sep / nota pyramid):
```markdown
| BTCUSDT | **atps_v3** | **<>%** | **<>%** | **<>** | **<>** | **<>** |
```
+ nota scaling: `Scaling: risk 4% con tetti coordinati (kelly/corr/heat) — il backtest avvisa se un cap lega (scaling ceiling nel report).`
+ se B promossa: riga su pyramid separate; se bocciata: una riga `pyramid separate testato e scartato (vedi V3_VALIDATION.md Decisione B)`.

- [ ] **Step 2: Verifica finale**

```bash
gofmt -l . && go vet ./... && go test ./... && go build -o atps ./cmd/atps
./atps backtest --config configs/atps_v3.yaml --variant A --symbol BTCUSDT --csv data/raw/BTCUSDT_4h.csv --out /tmp/opencode/fin_v3.html 2>&1 | grep "BTCUSDT A:"
./atps backtest --config configs/atps_v2.yaml --variant A --symbol BTCUSDT --csv data/raw/BTCUSDT_4h.csv --out /tmp/opencode/fin_v2.html 2>&1 | grep "BTCUSDT A:"
```

Expected: tutto verde; v3 = numero V3_VALIDATION; v2 = 34.31%/-17.01% (mai toccato).

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs: README con numeri v3 verificati + nota scaling ceiling + esito pyramid separate"
```

---

## Gate di successo del pacchetto (dallo spec)

1. v3 promossa: CAGR full > 34.31%, DD full ≥ -22%, gate verdi — Task 1
2. Niente clipping silenzioso: warning + ceiling in stdout/report, testati — Task 2
3. Pyramid separate promosso (gate) o OFF documentato — Task 3+4
4. Numeri pubblicati riproducibili — Task 5
