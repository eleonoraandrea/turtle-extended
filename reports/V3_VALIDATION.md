# ATPS v3 — Report validazione (2026-09-04)

Baseline: configs/atps_v2.yaml (BTC 34.31/-17.01/Sharpe 1.50; test 12.4/-16.9/Cal 0.73).
Budget DD full: ≥ -22%.

## Decisione A — risk-4%
| Gate | Criterio | Valore | Esito |
|---|---|---|---|
| Riproducibilità | CLI == E6 misurato (39.49/-19.49 ±0.5%) | CLI 39.49/-19.49 vs E6 39.49/-19.49 (Return 821.67%, Sharpe 1.28, PF 1.82, 416 trades) | PASS |
| Holdout | v3 teCAGR ≥ 12.4 AND teCal ≥ 0.73 | teCAGR 7.16 vs 12.4, teCal 0.30 vs 0.73 (test DD -23.49 vs v2 -16.9; train 58.40/-19.49/Cal 3.00) | FAIL |
| DD full | ≥ -22% | -19.49% | PASS |
| WF 8f | mediana Sharpe > 0 | 1.50 (fold test Sharpe: 1.96, 0.93, 2.74, 0.96, 4.91, 1.02, 1.03, 2.38; avgTest 1.99, avgTrain 1.23, OOS 255.46%) | PASS |
| Perturb | degrado < 30% | worst -21.2% (atr_stop +20%: CAGR 31.13 vs 39.49; tutti i 6 perturb PROFITTEVOLI) (CAGR derivati dai total-return su ~6.7 anni: CAGR=(1+ret)^(1/6.7)-1; degrado=(pert-base)/base) | PASS |
| MC 2000 | informativo | p5 219.6 p50 800.2 p95 1615.9 probProfit 99.6% | INFO |
| ETH | CAGR ≥ 16.53% | 27.66% (DD -22.00, Sharpe 1.05, PF 1.88, 410 trades) (soglia = 0.8× atps_v2 stesso simbolo) | PASS |
| SOL | CAGR ≥ 4.81% | 5.72% (DD -23.33, Sharpe 0.38, PF 1.19, 452 trades) (soglia = 0.8× atps_v2 stesso simbolo) | PASS |

DECISIONE A: BOCCIATA — resta atps_v2, motivo: gate holdout fallito (test-window 2024-26: teCAGR 7.16% < 12.4 v2 e teCal 0.30 < 0.73 v2, test DD -23.49% peggiore di v2 -16.9%). Il risk-4% migliora il full-period train-driven (39.49 vs 34.31) ma degrada l'out-of-sample: leva di rischio raddoppiata amplifica il chop del regime test. Config candidata ELIMINATA (configs/atps_v3.yaml rimossa, non committata). Evidenza like-for-like: reports/V3_HOLDOUT_BTC.txt vs reports/V2_OPTIMIZE_RUN.txt top row su atps_v2 (train 47.1/DD -13.0/test 12.4/DD -16.9/Cal 0.73). Segnali secondari concordi: SOL v3 marginale sul gate (5.72 vs 4.81) con Sharpe dimezzato (0.38 vs 0.60) e DD -23.33 (vs -17.13); ETH v3 DD -22.00 (vs -16.14).

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
Baseline Task 4: configs/atps_v2.yaml (Decisione A BOCCIATA).
Challenger: pyramiding enabled/mode=separate/max_additions=6/risk_neutral=true (ignorato, warning atteso) — heat/corr/kelly/max invariati da atps_v2, satellite enabled:true nel file ma forzato OFF dal motore con warning. Diff esatto verificato vs atps_v2 (solo sezione pyramiding).
| Gate | Criterio | Valore | Esito |
|---|---|---|---|
| Holdout | sep teCAGR ≥ 12.4 AND sep teCal ≥ 0.73 | teCAGR 2.11 vs 12.4, teCal 0.08 vs 0.73 (test DD -27.51 vs v2 -16.9; train 46.40/-22.39/Cal 2.07) | FAIL |
| DD full | ≥ -22% | -28.66% (full 30.76/-28.66/Sharpe 0.99/PF 1.51/617tr vs v2 34.31/-17.01/1.50/2.14/416tr) | FAIL (osservato, secondario) |
| Warning motore | risk_neutral ignorato + satellite OFF forzato | entrambi i warning presenti in stdout (reports/V3_SEP_BTC_A.txt righe 4-5) | PASS |
| WF 8f | mediana Sharpe > 0 | non eseguito (stop da protocollo su holdout FAIL) | SKIPPED |
| Perturb | degrado < 30% | non eseguito (stop da protocollo su holdout FAIL) | SKIPPED |
| ETH | CAGR ≥ 16.53% | non eseguito (stop da protocollo su holdout FAIL) | SKIPPED |
| SOL | CAGR ≥ 4.81% | non eseguito (stop da protocollo su holdout FAIL) | SKIPPED |

DECISIONE B: BOCCIATA — resta pyramiding OFF (atps_v2 invariato, atps_v3 NON creato), motivo: gate holdout fallito (test-window 2024-26: teCAGR 2.11% << 12.4 v2 e teCal 0.08 << 0.73 v2, test DD -27.51% peggiore di v2 -16.9%). Il separate migliora il train (46.40 vs 47.1 v2-like) ma collassa out-of-sample: gambe full-size che competono per lo stesso budget amplificano il chop del regime test. Conferma secondaria: full DD -28.66% viola il budget -22% (vs -17.01 v2) e Sharpe dimezzato (0.99 vs 1.50) con 617 trade (+48% costi). Config candidata ELIMINATA (configs/atps_v3_sep.yaml rimossa, non committata). Evidenza: reports/V3_SEP_HOLDOUT_BTC.txt vs reports/V2_OPTIMIZE_RUN.txt top row su atps_v2 (train 47.1/DD -13.0/test 12.4/DD -16.9/Cal 0.73); full: reports/V3_SEP_BTC_A.txt.

## Numeri finali B
| Symbol | Config | CAGR | MaxDD | Sharpe | PF | Trades |
|---|---|---|---|---|---|---|
| BTC | atps_v2 | 34.31 | -17.01 | 1.50 | 2.14 | 416 |
| BTC | sep-cand (full) | 30.76 | -28.66 | 0.99 | 1.51 | 617 |
| BTC | sep-cand (train) | 46.40 | -22.39 | 1.31 | — | 390 |
| BTC | sep-cand (test) | 2.11 | -27.51 | 0.10 | — | 203 |

## Appendice — configs/atps_v3.yaml eliminata (rigettata, non committata)

Diff ESATTO vs configs/atps_v2.yaml (ricostruibile applicandolo):
```diff
 risk:
-  base: 0.02
-  max: 0.02
-  max_risk_per_trade_pct: 2.0
-  kelly_cap_pct: 2.0
+  base: 0.04
+  max: 0.04
+  max_risk_per_trade_pct: 4.0
+  kelly_cap_pct: 4.0
 portfolio:
-  max_open_risk: 0.03
-  max_correlated_risk: 0.02
+  max_open_risk: 0.06
+  max_correlated_risk: 0.04
 variant_a:
-  risk_pct: 2.0
+  risk_pct: 4.0
```
(resto identico a atps_v2.yaml; name era "Variant A — ATPS v3 candidate (risk 4%, vedi reports/V3_VALIDATION.md)")
