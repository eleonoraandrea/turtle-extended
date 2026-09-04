# ATPS v3 — Scala i profitti: risk-4% validato + guardrail scaling + pyramid a gamba separata

Data: 2026-09-04
Stato: approvato dall'utente (pacchetto completo)
Spec precedente: docs/superpowers/specs/2026-09-04-atps-improve-cagr-dd15-design.md
Baseline di confronto per tutto il pacchetto: **configs/atps_v2.yaml**
(BTC 34.31% CAGR / DD -17.01% / Sharpe 1.50; test-window 2024-26: CAGR 12.4 / DD -16.9 / Calmar 0.73)

## Contesto misurato (esperimenti 2026-09-04, BTC full)

- Il sistema non scalava per costruzione: `kelly_cap 2%` + `max_correlated_risk 2%`
  cappavano ogni singolo trade al 2% (risk 2.5% ≡ 2.0% identici).
- Pyramid attuale (merged, step 0.5 ATR) DISTRUGGE rendimento:
  v2+pyr6 neutral 30.00%/-20.10, non-neutral 23.48%/-25.61 vs v2 34.31%/-17.01.
  Causa: add tardivi su close fusi in posizione con uscita rapida Don10.
- Intrabar (26.59%) e re-entry (30.55%) non aggiungono CAGR. Restano OFF.
- Frontiera solo-rischio (no pyr): 2% → 34.31/-17.01; **4% → 39.49/-19.49/Sharpe 1.28**;
  6% → 37.68/-22.79; 8% → 32.45/-25.54. Picco ~4%, poi rendimenti decrescenti
  (costi + deleverage DD + vol-target clipping).
- Budget DD per il pacchetto (approvato): **full DD ≤ 22%**.

## 1. Workstream A — validare e promuovere risk-4% (nessun codice)

Config candidata = atps_v2 con: risk base=max=4%, max_risk_per_trade_pct=4.0,
kelly_cap_pct=4.0, portfolio max_open_risk=0.06, max_correlated_risk=0.04,
variant_a risk_pct=4.0. Resto identico a atps_v2 (donchian don10, no pyr, sat 0.4).

Gate (tutti obbligatori):
1. Riproducibilità CLI (E6 misurato: 39.49%/-19.49).
2. Holdout like-for-like vs atps_v2: test CAGR ≥ 12.4 AND test Calmar ≥ 0.73.
3. Full DD ≥ -22%.
4. Walk-forward 8 folds: mediana Sharpe > 0.
5. Perturbazione ±20%: degrado CAGR < 30%.
6. ETH/SOL senza ri-ottimizzazione: degrado CAGR < 20% vs atps_v2 su stesso simbolo.
Se passa tutto → `configs/atps_v3.yaml` + `reports/V3_VALIDATION.md`.
Se un gate fallisce → resta atps_v2, documentato in V3_VALIDATION.md.

## 2. Workstream C — guardrail di scaling (piccolo codice)

- Nuovo helper `risk.ScalingCeiling(lim RiskLimits) (ceilingPct float64, binding string)`:
  minimo statico tra maxRisk, kellyCap, corrCap, heatCap (il tetto che il sizing
  non potrà mai superare per una fresh entry). `backtest.Run` lo calcola una volta
  all'avvio: se ceiling < lim.MaxRiskPct lo stampa a stdout
  (`scaling: risk richiesto 4.0%, tetto effettivo 2.0% (kelly_cap lega)`) e lo
  include nel report. I Factors per-trade restano invariati.
- Contatore `Result.NotionalCapHits` (+ report riga): quante entry hanno toccato
  `max_notional_per_trade` (prossimo tetto alla crescita con equity grande).
- Nessun cambio di comportamento di sizing. Test: unit su clipping+warning
  (config con risk 4% + kelly 2% → warning e risk effettivo 2%).

## 3. Workstream B — pyramid a gamba separata (codice + validazione)

Nuovo `pyramiding.mode: separate` (default `merged` = comportamento attuale invariato).
Con `separate`: ogni add che passa `CanPyramid` crea una posizione INDIPENDENTE
(non fusa in earliest) con:
- stop proprio = stop del segnale corrente (come fresh entry),
- uscita WIDE = Donchian 55 (stesso canale del satellite), non DonExit del core,
- heat e sizing normali (risk.Size con stato di mercato corrente).
Il core resta con uscita DonExit configurata. `risk_neutral` resta valido solo per
`mode: merged`: con `separate` viene ignorato e `backtest.Run` stampa una riga
di avviso all'avvio (`pyramiding.mode=separate ignora risk_neutral`).

Gate promozione (stesso protocollo §1, baseline atps_v2 o atps_v3 se promossa):
test CAGR ≥ baseline AND test Calmar ≥ baseline, full DD ≥ -22%, WF/perturb/ETH-SOL
come §1. Se perde → resta OFF di default.

## 4. Ordine, deliverable, testing

Ordine: A (solo config+run) → C (codice piccolo) → B (codice+validazione).
Deliverable: `configs/atps_v3.yaml` (risk-4 promossa + eventuale pyramid vincente),
`reports/V3_VALIDATION.md`, README tabella aggiornata.
Testing: `go test ./...` verde sempre; unit per C (clipping/warning, notional counter)
e per B (add separato con stop/exit propri, no merge, heat conteggiato, mode merged
invariato = regressione A/B esistenti).

## Criteri di successo del pacchetto

- atps_v3 promossa con CAGR full > 34.31% e DD full ≥ -22%, tutti i gate verdi.
- Nessun clipping silenzioso: ogni cap legante è visibile in log/report.
- Pyramid separate o promosso (gate verdi) o OFF documentato.
- Tutti i numeri pubblicati riproducibili con un comando.

## Fuori scope

- Esecuzione live reale; timeframe < 4h; nuovi simboli; strategia non-Turtle;
  leva max > 5× (hard cap resta); rimozione del path merged.
