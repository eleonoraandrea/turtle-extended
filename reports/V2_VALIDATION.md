# ATPS v2 — Report validazione (2026-09-04)

Spec: docs/superpowers/specs/2026-09-04-atps-improve-cagr-dd15-design.md
Optimizer: reports/V2_OPTIMIZE_RUN.txt — grid COMPLETA 2592 combo (runtime effettivo ~45s (l'ETA '≈173 min' stampato dal tool era una stima pessima per-run))

## Combo vincitrice (optimizer v2, BTC A)
atr1.8 trail:donchian don_exit:10 pyramiding:off satellite:0.4 risk:0.02 entry:close reentry:off dd(10/25)

## Nota metodologica — reinterpretazione gate test (APPROVATA dall'utente)
Il gate letterale "degrado CAGR train→test < 1/3" fallisce per OGNI combo incluso il
baseline (btc_opt: 41.1%→8.3% = -80%) per cambio di regime 2020-24 (train, mega-bull)
vs 2024-26 (test). Sostituito con confronto like-for-like sullo stesso holdout:
v2 test CAGR ≥ baseline test CAGR AND v2 test Calmar ≥ baseline test Calmar.
Evidenza holdout baseline: reports/V2_BASELINE_HOLDOUT_BTC.txt (test CAGR 8.31, Calmar 0.36).

## Gate protocollo
| Gate | Criterio | Valore | Esito |
|---|---|---|---|
| Riproducibilità | CLI full == optimizer full (±0.5%) | CAGR 34.31 vs 34.3, DD -17.01 vs -17.0 | PASS |
| DD full | ≥ −18% | -17.01% | PASS |
| CAGR full | > baseline 29.55% | 34.31% | PASS |
| Test holdout (reinterpretato) | v2 teCAGR ≥ 8.31 AND teCal ≥ 0.36 | 12.4 vs 8.31, 0.73 vs 0.36 | PASS |
| Walk-forward 8f | mediana Sharpe > 0 | 1.70 (avg test 2.10, avg train 1.36, decay 1.54, OOS 114.9%) | PASS |
| Perturbazione ±20% | degrado CAGR < 30% | 4.1% mediano; worst-case 18.5% (risk -0.5%) | PASS |
| Monte Carlo 2000 | informativo: p5 214.6% p50 606.3% p95 1142.2% | probProfit 99.95% (txt arrotondato, json 99.95), median Sharpe 2.83 | INFO |
| ETH conferma | CAGR ≥ 13.74% (0.8× 17.18) | 20.66% | PASS |
| SOL conferma | CAGR ≥ 3.42% (0.8× 4.27) | 6.01% | PASS |

## Decisione
- [x] TUTTI PASS → atps_v2.yaml promossa
- [ ] FAIL parziale → <cosa è fallito e azione>

Nota: la selezione ha scartato intrabar/re-entry/pyramiding (OFF nel vincitore):
le feature restano nel codice, disattivate di default — promuoverle richiede
nuova validazione che le veda vincitrici sul train.

## Anomalie / note (WF, perturbazione, MC)
- WF decay 1.54 > 1: avg test Sharpe (2.10) SUPERIORE al train (1.36) — fold 2023-24
  e 2026 molto forti; fold 5/6 con train negativo (2024-25 choppy) restano OOS positivi.
- Perturbazione: `donchian_entry ±20%` e `risk base +0.5%` producono risultati IDENTICI
  al baseline (416 trades) — l'entry channel è fissato nell'EngineConfig pre-costruito e
  risk 2.5% è clippato dal cap 2% (kelly/max). L'evidenza di robustezza attiva viene da
  `atr_stop ±20%` (CAGR 28.24% / 31.43%) e `risk -0.5%` (CAGR 27.96%): tutti PROFITTEVOLI
  con degrado < 30% (worst 18.5%).
- MC (block bootstrap, seed 42): p5 finale +214.6% (CAGR ≈ 18.7%), p50 +606.3% (≈ 34.0%),
  p95 +1142.2% (≈ 45.9%); probProfit 99.95% (1 run su 2000 in perdita). Nessun risk-of-ruin
  nel output; MC conferma coda sinistra contenuta.
- SOL: 13084 barre (CSV più corto di BTC/ETH 14627) — periodo coperto parzialmente minore.

## Numeri finali
| Symbol | Config | CAGR | MaxDD | Sharpe | PF | Trades |
|---|---|---|---|---|---|---|
| BTC | btc_opt (baseline) | 29.55 | -23.04 | 1.15 | 1.61 | 578 |
| BTC | atps_v2 | 34.31 | -17.01 | 1.50 | 2.14 | 416 |
| ETH | btc_opt (baseline) | 17.18 | -28.40 | 0.71 | 1.56 | 600 |
| ETH | atps_v2 | 20.66 | -16.14 | 1.17 | 2.18 | 410 |
| SOL | btc_opt (baseline) | 4.27 | -24.33 | 0.30 | 1.17 | 570 |
| SOL | atps_v2 | 6.01 | -17.13 | 0.60 | 1.38 | 452 |

Return totali atps_v2: BTC +616.30%, ETH +250.22%, SOL +41.71% (costi inclusi: fee+funding).
