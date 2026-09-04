# ATPS v4 — Portfolio validation (2026-09-04)

Baseline: atps_v2 BTC (34.31/-17.01/1.50; holdout 12.4/Cal 0.73).
Metodo: RunPortfolio equity+heat condivisi; split per timestamp (confine 2024-09);
WF 8 folds per-timestamp; perturb/MC su componente BTC (dominante, documentato).
perturb/MC sono tool single-symbol: l'evidenza è sulla componente BTCUSDT
(414/1230 trades, PnL dominante), non sul portfolio aggregato — dichiarato
esplicitamente qui.

Fonti raw: reports/V4_HOLDOUT_PORTFOLIO.txt (split), reports/V4_WF_PORTFOLIO.txt
(WF), reports/V4_PERTURB_BTCCOMPONENT.{txt,json}, reports/V4_MC_BTCCOMPONENT.{txt,json},
reports/V4_HOLDOUT_R25.txt (stage-2), reports/V4_PORTFOLIO.{txt,html} (sanity).

## EMENDAMENTO APPROVATO DALL'UTENTE — budget DD
Il budget DD full era -18% (ancorato al DD single-symbol di v2). Il portfolio
domina nettamente l'holdout (test 26.97/1.64 vs 12.4/0.73) e il DD full -19.49%
è train-driven (inverno crypto 2021-22, simboli correlati; il DD test-window è
-16.44%, MIGLIORE di v2 -16.9). L'utente ha approvato budget -20% (sotto il -22%
già approvato nel cycle v3). Alternativa r1.6% (pass per 0.02pt) scartata come
statisticamente insignificante.

## Stage 1 — portfolio risk 2% (PROMOSSO)
| Gate | Criterio | Valore | Esito |
|---|---|---|---|
| Full | informativo | CAGR 37.04% DD -19.49% Sharpe 1.38 Cal 1.90 trades 1230 | INFO |
| Holdout | teCAGR ≥ 12.4 AND teCal ≥ 0.73 | 26.97 / 1.64 | PASS |
| DD full | ≥ -20% (emendato, era -18%) | -19.49% | PASS |
| WF 8f | mediana Sharpe > 0 | 1.23 (min 0.73, max 2.80) | PASS |
| Perturb (BTC comp) | degrado CAGR < 30% | 18.5% (CAGR 34.16% → 27.84%) | PASS |

Perturb (BTC comp, 6 perturbs): baseline 616.3% → range 418.4%–616.3%, tutti
PROFITTEVOLI; worst = risk base -0.5% e atr_stop +20%.
Monte Carlo (BTC comp, 2000 runs): mediana 606.3%, p5 214.6%, p95 1142.2%,
probProfit 100.0%.

## Stage 2 — risk re-scaling: CHIUSO
Precondizione (DD full ≥ -15%) NON soddisfatta (-19.49%). Documentazione r2.5%:
full CAGR 46.65% DD -21.91% Sharpe 1.42 Cal 2.13 (test 29.76/-20.45/0.91/1.46)
→ viola budget -20%. Nessuna promozione.

## Decisione
PORTFOLIO PROMOSSO — configs/atps_portfolio.yaml (risk 2%).
Sistema operativo consigliato: portfolio backtest-validato; bot live resta
single-symbol per istanza (heat NON condiviso live — roadmap multi-simbolo).

## Frontiera misurata
| Config | Full CAGR/DD/Sharpe | Test CAGR/Cal |
|---|---|---|
| r2.0% (promosso) | 37.04/-19.49/1.38 | 26.97/1.64 |
| r1.75% | 35.37/-18.35/1.38 | 27.01/1.67 |
| r1.6% | 35.94/-17.98/1.44 | 28.24/1.79 |
| v2 BTC (baseline) | 34.31/-17.01/1.50 | 12.4/0.73 |

## Breakdown per-simbolo (full, risk 2%)
| Symbol | Trades | Win% | PnL netto |
|---|---|---|---|
| BTCUSDT | 414 | 32% | $28,719.87 |
| ETHUSDT | 392 | 32% | $36,012.18 |
| SOLUSDT | 424 | 29% | $7,174.33 |

## Appendice — config temporanee eliminate
Tutte ricavate da configs/atps_portfolio.yaml (promosso); diff esatti:

**atps_portfolio_r175.yaml** (eliminata):
- risk.base: 0.02 → 0.0175; risk.max: 0.02 → 0.0175
- risk.max_risk_per_trade_pct: 2.0 → 1.75
- risk.kelly_cap_pct: 2.0 → 1.75; variant_d.kelly_cap_pct: 2.0 → 1.75
- portfolio.max_open_risk: 0.03 → 0.045 (heat alzato mentre il risk per-trade
  scendeva — esperimento della sessione, registrato fedelmente)
- variant_a.risk_pct: 2.0 → 1.75

**atps_portfolio_r160.yaml** (eliminata):
- risk.base: 0.02 → 0.016; risk.max: 0.02 → 0.016
- risk.max_risk_per_trade_pct: 2.0 → 1.6
- risk.kelly_cap_pct: 2.0 → 1.6; variant_d.kelly_cap_pct: 2.0 → 1.6
- portfolio.max_open_risk: 0.03 → 0.042 (come sopra)
- variant_a.risk_pct: 2.0 → 1.6

**atps_portfolio_r25.yaml** (eliminata — stage-2, solo documentazione):
- risk.base: 0.02 → 0.025; risk.max: 0.02 → 0.025
- risk.max_risk_per_trade_pct: 2.0 → 2.5
- risk.kelly_cap_pct: 2.0 → 2.5; variant_d.kelly_cap_pct: 2.0 → 2.5
- portfolio.max_open_risk: 0.03 → 0.045
- portfolio.max_correlated_risk: 0.02 → 0.025
- variant_a.risk_pct: 2.0 → 2.5
