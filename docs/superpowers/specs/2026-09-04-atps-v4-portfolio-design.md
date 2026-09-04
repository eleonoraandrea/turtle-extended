# ATPS v4 — Portfolio engine vero BTC+ETH+SOL (Design)

Data: 2026-09-04
Stato: approvato dall'utente
Spec precedenti: 2026-09-04-atps-improve-cagr-dd15-design.md (v2), 2026-09-04-atps-v3-scale-profits-design.md (v3)
Baseline di confronto: **configs/atps_v2.yaml** su BTC (34.31% CAGR / DD -17.01% / Sharpe 1.50; holdout test 12.4 / Cal 0.73)

## Razionale dai dati raccolti

- Risk-scaling su BTC-sole: BOCCIATO (holdout 7.16/0.30 — il regime chop 2024-26 punisce il rischio concentrato).
- Pyramiding (merged e separate): BOCCIATO entrambi.
- Intrabar: peggiora. Re-entry: neutro.
- v2 SENZA ri-ottimizzazione è valido su tutti e 3 i simboli:
  BTC 34.31/-17.01, ETH 20.66/-16.14 (baseline ETH era 17.18/-28.40), SOL 6.01/-17.13.
  DD simili ma NON sincronizzati → la diversificazione è l'unica leva dati-supportata
  per più CAGR a pari DD. L'attuale `analysis.RunPortfolio` è un'approssimazione
  (somma di equity per-simbolo, niente heat condiviso): serve l'engine vero.

## 1. Engine multi-simbolo

Nuovo file `internal/backtest/engine_portfolio.go`:

```go
func RunPortfolio(barsMap map[string]data.Bars, strats map[string]strategy.Strategy,
    cfg *config.Config, eng EngineConfig) *Result
```

- Timeline = UNION dei timestamp di tutti i simboli (ordinata). Ogni simbolo ha il
  proprio cursore; processa la propria barra quando il timestamp combacia. SOL
  (parte 2020-09) semplicemente non tradea finché non ha warmup.
- UNA equity condivisa: PnL/fee/funding/slippage di tutti i simboli confluiscono
  nella stessa equity compounding. Peak/DD calcolati sulla curva combinata
  (mark-to-market per-simbolo alle close disponibili).
- Heat condiviso: `MarketState.PortfolioHeatPct` = Σ RiskPct di TUTTE le posizioni
  aperte (cap `portfolio.max_open_risk` 3%); `PortfolioCorrelatedPct` = Σ RiskPct
  same-side su TUTTI i simboli (cap 2% — tutti crypto correlati). La logica di cap
  esiste già in `risk.Size`; oggi l'engine singolo la alimenta per-simbolo.
- Per-simbolo: `Prepare` indipendente (indicatori propri), lista posizioni propria,
  uscite stop/donchian/trailing/pyramiding IDENTICHE al motore singolo (stessa
  semantica `EngineConfig`: TrailMode/DonExit/EntryMode/PyramidingMode per tutti).
- Riusa `risk.Size`/`risk.CanPyramid`/`TrailStop` e il pattern `recordExit`.
- Funding: per-simbolo come nel motore singolo, sulla equity condivisa.
- Il loop è adattato da engine.Run con heat/equity condivisi (duplicazione
  controllata e dichiarata, come recordExit: niente refactor invasivo del motore
  singolo validato).

**Test invariante forte**: `RunPortfolio` con UN solo simbolo (BTC, atps_v2) deve
riprodurre ESATTAMENTE il risultato di `engine.Run` (34.31%/-17.01%, stessi trades).
Dimostra zero divergenza logica tra i due motori.

## 2. CLI + script

- `atps portfolio-backtest --config configs/atps_portfolio.yaml --csvs "data/raw/{SYMBOL}_4h.csv"`
  → stdout summary + report HTML portfolio (equity combinata SVG + breakdown
  per-simbolo: CAGR contribution, trades, DD) in `reports/`.
- `scripts/portfolio_split/main.go` (pattern baseline_split): train 70%/test 30%/full
  del PORTFOLIO (holdout gate evidence). Split per timestamp (stesso confine
  2024-09 dei cycle precedenti: 70% della timeline comune).

## 3. Config

`configs/atps_portfolio.yaml` = atps_v2 con `general.symbols:
[BTCUSDT, ETHUSDT, SOLUSDT]` e `orderly_symbols` coerenti. Risk/heat/corr invariati
(2% per trade, heat 3%, correlati 2% — ora davvero condivisi). variant_a name
aggiornata.

## 4. Protocollo di validazione (baseline = atps_v2 BTC)

Stage 1 — portfolio risk 2%:
- Riproducibilità (run full deterministica)
- Holdout like-for-like: portfolio teCAGR ≥ 12.4 AND teCal ≥ 0.73 (v2 BTC)
- DD full ≥ -18%
- Monte Carlo 2000 sul trade-list combinato (informativo)
Stage 2 — SOLO se DD full ≥ -15% (headroom ≥ 2pt vs budget):
risk 2.5% e 3% SUL PORTFOLIO (le dosi bocciate su BTC-sole), stessi gate holdout +
DD ≥ -18%.
WF: 8 folds nel portfolio_split (mediana Sharpe > 0). Perturb: ±20% sui parametri
BTC nel portfolio (documentato: domina il contribution BTC).
Se nulla passa → resta atps_v2, decisione documentata in `reports/V4_VALIDATION.md`.

## 5. Testing

- Invariante single-symbol (§1) su BTC E su ETH (4 cifze significative su
  CAGR/DD/trades/equity finale).
- Test sintetici: heat condiviso blocca la 3ª entry quando la somma supera 3%;
  correlated cap blocca same-side sul 2° simbolo oltre 2%; equity unica accresciuta
  da fee/PnL di simboli diversi; SOL post-warmup tradea sulla stessa equity.
- `go test ./...` sempre verde; regressione: engine.Run singolo intatto.

## 6. Deliverable

`internal/backtest/engine_portfolio.go` + test, cmd `portfolio-backtest`,
`scripts/portfolio_split`, `configs/atps_portfolio.yaml`, `reports/V4_VALIDATION.md`,
README (tabella portfolio + nota live multi-simbolo come roadmap).

## Criteri di successo

- Portfolio risk 2% promosso: teCAGR ≥ 12.4 AND teCal ≥ 0.73, DD full ≥ -18%.
- Eventuale stage-2 risk promosso con gli stessi gate.
- Invariante single-symbol verificato (zero divergenza engine).
- Numeri riproducibili con un comando.

## Fuori scope

Bot live multi-simbolo (il bot resta single-symbol per istanza; 3 istanze NON
condividono heat → degradazione vs backtest documentata in README come roadmap).
Ri-ottimizzazione per-simbolo; timeframe nuovi; varianti B/C/D.
