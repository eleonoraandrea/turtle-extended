# ATPS — Adaptive Turtle Perpetual System (Go)

Framework quantitativo perpetuo: **Binance dati, Orderly esecuzione**.

> Variant A classic Turtle → B + regime → C + funding/OI/volume → D full adaptive (breakout 20/55/100, ATR, ADX, EMA 50/200, vol regime, crash brake, pyramiding, trailing chandelier, vol-targeting). Report HTML MT5-style dettagliato, self-contained, Lightweight-Charts 4.1.

## Quickstart — TUI

```bash
go mod tidy
./atps tui                          # TUI interattiva: seleziona simbolo/variante, vedi KPI + sparkline + trades
# oppure CLI diretta:
make demo        # sintetico A/B/C/D → reports/demo_*.html + reports/comparison.html
make backtest    # BTCUSDT D → reports/BTCUSDT_D_*.html
make compare     # A/B/C/D × BTC/ETH/SOL
./atps backtest --variant D --symbol BTCUSDT --out reports/report.html
```

TUI comandi: `Tab` focus, `↑/↓` seleziona, `Enter` esegui, `r` Run, `c` Compare, `o` apri HTML, `?` Help, `q` Esci.

Report dal TUI: `reports/TUI_*.html` self-contained, apribile `file://`.

Serve report:
```bash
make report-serve   # http://localhost:8000
xdg-open reports/TUI_BTCUSDT_D_*.html
```

## Dati reali Binance

```bash
./atps download --symbol BTCUSDT --interval 4h --start 2020-01-01 --funding=true --oi=true
./atps download --symbol ETHUSDT --interval 4h --funding=true --oi=true
./atps download --symbol SOLUSDT --interval 4h --funding=true --oi=true
# output: data/raw/BTCUSDT_4h.csv (OHLCV + funding+OI allineati) + data/cache
```

Endpoints Binance pubblici usati:
- `GET /fapi/v1/klines?symbol=BTCUSDT&interval=4h&limit=1500` paginato
- `GET /fapi/v1/fundingRate?symbol=BTCUSDT&limit=1000` storico 8h
- `GET /futures/data/openInterestHist?symbol=BTCUSDT&period=4h&limit=500` (sumOpenInterest notional)

Visione bulk alternativa per storia lunga: `https://data.binance.vision`

## Backtest

```bash
./atps backtest --variant D --symbol BTCUSDT --csv data/raw/BTCUSDT_4h.csv --out reports/BTC_D.html
./atps compare --symbols BTCUSDT,ETHUSDT,SOLUSDT --variants A,B,C,D --out reports/comparison.html
./atps walk-forward --symbol BTCUSDT --variant D --folds 8
./atps montecarlo --symbol BTCUSDT --variant D --runs 2000
```

Costi: `fee 4 bps + slippage 2 bps` (cfg `costs.fee_bps/slippage_bps`), funding 8h scalato su interval, heat max 6%.

## Risk Engine — rischio max configurabile, leva DINAMICA

```
qty = (equity × max_risk_per_trade_pct%) / |entry − stop|     ← sizing per rischio
lev = notional / equity                                        ← DERIVATA, mai fissa
```

Cap dinamico della leva in base alla rischiosità (`internal/risk/risk.go`):
- **vol regime** (ATR percentile): >80 → ×0.50, >60 → ×0.75, <20 → ×1.20
- **ADX** < 18 (trend debole) → ×0.60
- **funding z** |z|>2 → ×0.70
- **hard cap** `risk.max_leverage: 5` (mai superabile), floor 0.5×
- **portfolio heat**: somma risk% aperto ≤ 6% → entry rifiutata/clippata se esausto
- **drawdown de-leverage** (anti-martingale): dd>5% scala il rischio linearmente → 0 a dd 15%
- **vol target** 20% ann.: rischio scalato se vol reale > target (variante D)
- **pyramiding**: ogni add-on ri-dimensionato sul heat residuo

Audit completo per trade nel report HTML: `Lev`, `Risk%`, `R-multiple`, sizing log.
Invariante verificata dai test: stop-out perdente ≈ **-1R** (perde esattamente il rischio impegnato, mai di più).

```yaml
risk:
  max_risk_per_trade_pct: 2.0   # ← configurabile: max perdita per trade
  max_portfolio_heat_pct: 6.0
  max_leverage: 5.0             # hard cap
  dd_deleverage_start_pct: 5.0
  dd_flat_pct: 15.0
```

Motore: bar-by-bar, fill **next open** anti look-ahead, stop intrabar (low ≤ stop), trailing chandelier/donchian, pyramiding 0.5 ATR max 4, crash brake flat 24h se drop 4h ≥8%.

## Report HTML

`reports/*.html` — self-contained (embed JSON, inline SVG equity/drawdown, histogram trade, tabella 32 metriche, monthly/yearly heatmap, regime breakdown LONG/SHORT/Year, trade list MT5 con MAE/MFE/fee/funding/R, Lightweight-Charts 4.1 per candele+equity overlay). Offline apribile via `file://`.

- `Sharpe/Sortino/Calmar`, `CAGR`, `MaxDD`, `PF`, `SQN`, `Kelly`, `Ulcera`, `Exposure`, `fundingDrag`...
- Comparison rank per Sharpe.

## Orderly live (isolato)

Backtester **non importa** `internal/execution`. Live si compila separato:

```bash
go build -tags live -o atps-live ./cmd/atps
# paper:
./atps-live paper --dry-run  # PaperAdapter
# real: richiede -tags live + env ORDERLY_ACCOUNT_ID, ORDERLY_KEY, ORDERLY_SECRET (base64 ed25519), ORDERLY_BASE=https://api.orderly.org
```

Spec isolamento in `docs/LIVE_EXECUTION_SPEC.md`. Simboli: `BTCUSDT → PERP_BTC_USDC` (`orderly.symbols_map`).

## Config

`configs/default.yaml` — varianti, costi, walk-forward 8 folds 70% train, MonteCarlo 2000 block 20, report theme dark, data align, orderly mainnet/testnet.

## Struttura

```
cmd/atps/main.go (+tui.go)
internal/tui/ (model.go, styles.go)  — BubbleTea + Lipgloss + asciigraph
internal/data/ (binance.go, bar.go, demo.go)
internal/indicators/
internal/strategy/ (A/B/C/D + factory)
internal/risk/
internal/backtest/ (engine) + internal/analysis/ (walkforward, montecarlo)
internal/metrics/
internal/report/ (HTML MT5 + comparison, Lightweight-Charts 4.1)
internal/execution/ (adapter, orderly/* con build tag live)
configs/default.yaml
data/raw/ reports/
docs/LIVE_EXECUTION_SPEC.md
```

## Test

```bash
go test ./...
```

Dataset sintetico verifica pipeline: non sono risultati reali.

## Roadmap quant

```
REAL MARKET DATA (Binance klines + funding + OI)
→ A/B/C/D backtest → COST ADJUST → WALK-FORWARD → PERTURB → MONTECARLO → REGIME → PORTFOLIO → PAPER → LIVE
```

## Sicurezza live

- Nessun import `execution` nel backtester (compile fail se tentato senza tag).
- Paper default, `atps-live` richiede `--i-understand-live` + leva max 3x + kill-switch ` /tmp/atps.halt`.
- Funding sottratto da equity, fee per-entry+exit, notional cap.

## Licenza

MIT — Lightweight-Charts vendored (Apache-2.0) via CDN `unpkg.com/lightweight-charts@4.1.3`.

