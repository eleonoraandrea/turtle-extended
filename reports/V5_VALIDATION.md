# ATPS v5 — Validazione cycle 2026-09-05 (focus H1/H4)

Baseline di partenza: **v4** (portfolio BTC+ETH+SOL risk 2%, parametri v2: sma200 atr1.8)
— full 37.04%/-19.49%/1.38, test-window 26.97%/Cal 1.64 (`reports/V4_VALIDATION.md`).

## Cambi infrastruttura (prerequisito)

1. **Periodi indicatori configurabili per variante** (`internal/strategy/strategy.go`):
   canali donchian (slot fast/slow via `donchian_alt`/`donchian_entry`), `sma_filter`,
   `atr_period` — prima hardcoded (20/55/100/200). Backward-compat: config assente →
   default storici → risultati 4h esistenti **immutati** (verificato: BTC v2 full
   616.3%/34.31%/1.50 identico pre/post).
2. **`satellite_exit_len`** configurabile (engine singolo + portfolio, default 55).
3. **Variante M** (mean reversion H1): `internal/strategy/variant_m.go`, RSI in
   indicators, `exit_mode: reversion` nei due motori (mean-touch/bounce, stop fisso).
4. Dati: **H1 2020→2026** per BTC/ETH/SOL (58.5k/58.5k/52.4k barre, 0 gap) +
   H4 per BNB/XRP/DOGE/LINK.

## Stage 1 — v3 H4 single-symbol (PROMOSSA)

Optimizer v3 (`scripts/optimize2 -interval 4h`, griglia 1152, train 70%):
vincitore per train CAGR sotto vincolo DD ≤17%:
**alt20 sma300 atr20×1.6 dx10 sx55 sat0.4 dd(10/25) r2.0%** → `configs/atps_v3.yaml`.

Nota scoperta: lo slot donchian_entry è inerte in variante A (il canale fast scatta
sempre prima quando flat) — il grado di libertà reale è `donchian_alt`.

| Gate | Criterio | v3 | v2 baseline | Esito |
|---|---|---|---|---|
| Train | info | 53.66% / -12.37 / Sharpe 1.94 | — | INFO |
| Holdout BTC | teCal > 0.73 (v2) | **38.06% / -12.54 / Cal 3.04** | 12.4 / Cal 0.73 | PASS |
| WF 8f BTC | mediana test Sharpe > 0 | avg train 1.38 → avg test 2.17, 7/8 fold ≥0.73 | — | PASS |
| Perturb BTC (6) | degrado CAGR < 30% | tutti profittevoli, worst ~22% (690% vs 1009%) | — | PASS |
| MC BTC 2000 | probProfit alta | mediana 978%, p5 463%, probProfit **100%** | — | PASS |
| ETH (no re-opt) | full ≥ v2 | 21.42%/-20.02, test **31.56%/Cal 2.89** vs 20.66% | 20.66% | PASS |
| SOL (no re-opt) | full ≥ v2 | 7.95%/-18.05, test 9.09%/Cal 0.50 vs 6.01% | 6.01% | PASS |

Raffinamento (`scripts/optimize4`): piatto robusto su don_exit 6/8/10 (test Cal
3.37/3.06/3.04) → conferma NON-overfit, tenuto dx10 (selezione train). Chandelier,
pyramiding, re-entry: tutti peggiorano OOS → confermati OFF.

## Stage 2 — v5 portfolio (PROMOSSA — config finale `configs/atps_v5.yaml`)

Portfolio 3 simboli, equity+heat condivisi, parametri v3, **risk 1.8%**.

| Config | Full CAGR | MaxDD | Sharpe | Test CAGR/Cal | Trades |
|---|---|---|---|---|---|
| v4 (baseline) | 37.04% | -19.49% | 1.38 | 26.97 / 1.64 | 1230 |
| v5 r2.0% | 49.64% | -20.36% | 1.58 | 41.39 / 2.03 | 1162 |
| **v5 r1.8% (finale)** | **47.09%** | **-19.84%** | **1.57** | **42.35 / 2.29** | 1176 |

- Budget DD: r2.0% sfora di 0.36pt (-20.36%) → **r1.8% rientra** (-19.84%, test-window
  DD -18.46%) mantenendo +10pt CAGR su v4.
- WF portfolio 8 folds: **8/8 fold test positivi**, mediana test Sharpe **1.40**
  (v4: 1.23). Fold migliore +148.10%, peggiore +20.16%.
- Contributi full: BTC $62.4k > ETH $49.1k > SOL $9.8k su $10k iniziali ($131.3k finali).
- Perturb/MC: eseguiti sul componente BTC dominante con v3 r2.0 (`reports/V5_PERTURB_BTCCOMPONENT.txt`,
  `V5_MC_BTCCOMPONENT.txt`) — r1.8% è solo riscaling lineare del rischio.

## Tentativi BOCCIATI (evidenza negativa, non sprecare cicli)

### H1 breakout trend-following (grid 2304/simbolo, calendar-scaled ×4)

| Symbol | Miglior test window | Esito |
|---|---|---|
| BTCUSDT 1h | 7.0% / Cal 0.36 (train 40.8%) | BOCCIATO — collasso OOS |
| ETHUSDT 1h | 17.2% / Cal 1.01 (train 12.6%) | margine reale ma ≪ H4 v3 (31.6/2.89) |
| SOLUSDT 1h | 6.9% / Cal 0.33 | BOCCIATO |
| Portfolio H1 3-sym | train 38.1% → test 18.9%, **DD test -26.2%** | BOCCIATO — viola budget |

Conclusione: su 2020-2026 (test 2024-26 chop/efficienza) il breakout H1 non copre
fee+slippage+funding dopo il degrado OOS. L'H4 domina la stessa classe di edge.

### H1 mean reversion (variante M, grid 2304/simbolo)

Dip-buy in uptrend / rip-short in downtrend, exit mean-touch/bounce, stop ATR:
**tutti i top candidate test-window NEGATIVI** su BTC/ETH/SOL (teCal -0.08…-0.50).
L'infrastruttura resta (variant M utilizzabile in config), l'edge no — in questo
regime comprare i dip H1 è pagare la volatilità, non raccoglierla.

### Portfolio 7 simboli H4 (BNB/XRP/DOGE/LINK aggiunti, v3 params)

Full 55.72%/-27.42%, test Cal 1.42 < 2.29 del 3-sym: la correlazione invernale
2021-22 amplifica il DD oltre ogni budget; il rischio heat cap non basta quando
tutti i simboli breakano insieme. BOCCIATO — i 4 simboli extra restano nel dataset
(`data/raw/*_4h.csv`) per esperimenti futuri (es. selezione dinamica regime-aware).

## Sistema operativo raccomandato

**`configs/atps_v5.yaml`** — portfolio BTC+ETH+SOL H4, variant A v3, risk 1.8%,
heat 3%/2% correlati, leva dinamica cap 5×. Backtest portfolio = riferimento di
performance onesto (bot live resta single-symbol per istanza, roadmap multi-simbolo).

Riproducibilità:
```bash
./atps portfolio-backtest --config configs/atps_v5.yaml --out reports/V5_PORTFOLIO.html
go run ./scripts/portfolio_split -config configs/atps_v5.yaml -csvs "data/raw/{SYMBOL}_4h.csv" -wf
./atps backtest --config configs/atps_v3.yaml --variant A --symbol BTCUSDT --csv data/raw/BTCUSDT_4h.csv
```
