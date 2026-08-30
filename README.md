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

## Risk Engine — 2% max, sizing per |entry-stop|, leva dinamica, satellite 30% skew

```
qty = (equity × risk%) / |entry − stop|   ← SCALA con equity (compounding)
risk% = base 1.0% → min 0.25% (high vol/DD) → max 2.0% (low vol) → heat 3%/2% → Kelly
lev = notional / equity                    ← DERIVATA, mai fissa (hard cap 5×)
```

`internal/risk/risk.go` + `configs/default.yaml` (user spec):
```yaml
risk: {base: 0.01, min: 0.0025, max: 0.02}          # 1% base, 0.25% min, 2% max
portfolio: {max_open_risk: 0.03, max_correlated_risk: 0.02} # 3% totale, 2% correlati
leverage: {max: 5}  # hard cap
pyramiding: {enabled: true, max_additions: 4, risk_neutral: true} # non aumenta heat
profit: {trailing: true, satellite: {enabled: true, allocation: 0.30}} # 70% core Don20, 30% sat Don55 → skew
```
Cap dinamico leva: vol regime>80 ×0.70 / >60 ×0.85 / <20 ×1.30, ADX<20 ×0.80, funding|z|>2.5 ×0.85. Heat totale/correlati, drawdown adaptive (dd 10%→25% → risk → min, non 0), vol target 50% ann.

Audit per trade in HTML: `Lev× Risk% R-multiple` + sizing log. Invariante test: stop perdente **R ≥ -1.8** (mai più del budget).

Obiettivo max: **Expectancy × Positive Skew × Compounding** — 55-65 perdite -1R, 20-30 small +0.5/+2R, 5-10 large +5/+90R (BTC A: mean R 2.49, median -1.0, skew 3.96).

Motore: bar-by-bar, fill **next open** anti look-ahead, stop intrabar (low ≤ stop), trailing chandelier/donchian, pyramiding 0.5 ATR max 4, crash brake flat 24h se drop 4h ≥8%.

## Report HTML

`reports/*.html` — self-contained (embed JSON, inline SVG equity/drawdown, histogram trade, tabella 32 metriche, monthly/yearly heatmap, regime breakdown LONG/SHORT/Year, trade list MT5 con MAE/MFE/fee/funding/R, Lightweight-Charts 4.1 per candele+equity overlay). Offline apribile via `file://`.

- `Sharpe/Sortino/Calmar`, `CAGR`, `MaxDD`, `PF`, `SQN`, `Kelly`, `Ulcera`, `Exposure`, `fundingDrag`...
- Comparison rank per Sharpe.

## Live Bot — Orderly con TUI bella (paper/live)

Bot live: Binance klines (segnali) → `risk.Size` (qty = equity×2% / |entry-stop|, leva dinamica 5×) → Orderly `PERP_*_USDC` (ed25519). Backtester **mai** importa `execution` (isolato, `go list -deps ./internal/backtest` = 0).

```bash
# Paper (sicuro, default) — simula ordini, nessun capitale reale
./atps live --symbol BTCUSDT --variant D --interval 4h --poll 30

# Live reale — richiede env + flag esplicito
ORDERLY_ACCOUNT_ID=... ORDERLY_KEY=... ORDERLY_SECRET=... \
./atps live --symbol BTCUSDT --variant D --live --i-understand-live

# Testnet
./atps live --testnet --symbol BTCUSDT --variant D --live --i-understand-live

# Headless (no TTY, es. server): log su stdout, Ctrl-C per stop
timeout 60 ./atps live --poll 30 2>&1 | tail

# Build separato con Orderly reale (tag live)
go build -tags live -o atps-live ./cmd/atps
./atps-live live --live --i-understand-live
```

TUI Live (`internal/tui/live.go`, Bubble Tea):
- Header: equity + PnL, balance, strat, Orderly symbol, mode PAPER/LIVE-TESTNET/LIVE
- `▣ MERCATO` asciigraph 60×8 delle close 4h + funding/OI
- `◆ POSIZIONI` Orderly (`GET /v1/positions`) con uPnL, `Adapter: paper/orderly`
- `◇ SEGNALE` LONG/SHORT/HOLD + stop + risk% + qty + lev
- `▤ LOGS` viewport scrollabile con sizing log (`qty = (equity×risk%)/|entry-stop|`)
- Help: `q` esci, `r` tick manuale, `p` auto ON/OFF, `?` guida

Safety (`docs/LIVE_EXECUTION_SPEC.md`):
- Paper default, live richiede `--i-understand-live` + env
- Kill-switch `touch /tmp/atps.halt` → blocca ordini
- Heat 3% totale / 2% correlati, leva hard 5×, crash brake 8% → flat 24h
- `internal/bot/bot.go` — `risk.LimitsFromConfig` + `Satellite 30%` (core 70% Don20, sat 30% Don55 per skew positivo)

## Config — user spec per max performance

`configs/default.yaml` include `risk/base/min/max`, `portfolio/max_open/max_correlated`, `leverage/max:5`, `trend 55/20`, `atr 20/2.0`, `pyramiding 4 risk_neutral`, `profit satellite 30%`, `regime btc_filter/adx_min 20`, `volatility/drawdown adaptive`, `funding/OI filter` — tutti usati da `risk.LimitsFromConfig`.

## Struttura

```
cmd/atps/{main.go,tui.go,live.go}
internal/bot/bot.go               # live bot: Binance klines → strategy → risk.Size → Orderly
internal/tui/{model.go,styles.go,live.go}  # backtest TUI + live TUI (BubbleTea, asciigraph)
internal/data/ (binance.go, bar.go, demo.go)
internal/indicators/  internal/strategy/ (A/B/C/D)  internal/risk/ (base/min/max adaptive)
internal/backtest/ (engine satellite 70/30, Don20/55)  internal/analysis/ (walkforward, montecarlo, perturb, portfolio)
internal/metrics/ (skew, ExpectancyR, PosSkewScore)  internal/report/ (MT5 + Lightweight-Charts 4.1)
internal/execution/ (adapter, orderly live/paper)
configs/default.yaml  data/raw/ reports/  docs/LIVE_EXECUTION_SPEC.md  .planning/graphs/
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

