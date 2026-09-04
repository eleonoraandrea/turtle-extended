# ATPS v3 — Report validazione (2026-09-04)

Baseline: configs/atps_v2.yaml (BTC 34.31/-17.01/Sharpe 1.50; test 12.4/-16.9/Cal 0.73).
Budget DD full: ≥ -22%.

## Decisione A — risk-4%
| Gate | Criterio | Valore | Esito |
|---|---|---|---|
| Riproducibilità | CLI == E6 misurato (39.49/-19.49 ±0.5%) | CLI 39.49/-19.49 vs E6 39.49/-19.49 (Return 821.67%, Sharpe 1.28, PF 1.82, 416 trades) | PASS |
| Holdout | v3 teCAGR ≥ 12.4 AND teCal ≥ 0.73 | teCAGR 7.16 vs 12.4, teCal 0.30 vs 0.73 (test DD -23.49 vs v2 -16.92; train 58.40/-19.49/Cal 3.00) | FAIL |
| DD full | ≥ -22% | -19.49% | PASS |
| WF 8f | mediana Sharpe > 0 | 1.50 (fold test Sharpe: 1.96, 0.93, 2.74, 0.96, 4.91, 1.02, 1.03, 2.38; avgTest 1.99, avgTrain 1.23, OOS 255.46%) | PASS |
| Perturb | degrado < 30% | worst -21.2% (atr_stop +20%: CAGR 31.13 vs 39.49; tutti i 6 perturb PROFITTEVOLI) | PASS |
| MC 2000 | informativo | p5 219.6 p50 800.2 p95 1615.9 probProfit 99.6% | INFO |
| ETH | CAGR ≥ 16.53% | 27.66% (DD -22.00, Sharpe 1.05, PF 1.88, 410 trades) | PASS |
| SOL | CAGR ≥ 4.81% | 5.72% (DD -23.33, Sharpe 0.38, PF 1.19, 452 trades) | PASS |

DECISIONE A: BOCCIATA — resta atps_v2, motivo: gate holdout fallito (test-window 2024-26: teCAGR 7.16% < 12.4 v2 e teCal 0.30 < 0.73 v2, test DD -23.49% peggiore di v2 -16.92%). Il risk-4% migliora il full-period train-driven (39.49 vs 34.31) ma degrada l'out-of-sample: leva di rischio raddoppiata amplifica il chop del regime test. Config candidata ELIMINATA (configs/atps_v3.yaml rimossa, non committata). Evidenza like-for-like: reports/V3_HOLDOUT_BTC.txt vs baseline_split su atps_v2 (train 47.08/test 12.41/Cal 0.73). Segnali secondari concordi: SOL v3 marginale sul gate (5.72 vs 4.81) con Sharpe dimezzato (0.38 vs 0.60) e DD -23.33 (vs -17.13); ETH v3 DD -22.00 (vs -16.14).

## Numeri finali A
| Symbol | Config | CAGR | MaxDD | Sharpe | PF | Trades |
|---|---|---|---|---|---|---|
| BTC | atps_v2 | 34.31 | -17.01 | 1.50 | 2.14 | 416 |
| BTC | v3-cand | 39.49 | -19.49 | 1.28 | 1.82 | 416 |
| ETH | atps_v2 | 20.66 | -16.14 | 1.17 | 2.18 | 410 |
| ETH | v3-cand | 27.66 | -22.00 | 1.05 | 1.88 | 410 |
| SOL | atps_v2 | 6.01 | -17.13 | 0.60 | 1.38 | 452 |
| SOL | v3-cand | 5.72 | -23.33 | 0.38 | 1.19 | 452 |

## Decisione B — pyramid separate
(da compilare nel Task 4)
