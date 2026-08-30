# Guida Avvio — ATPS (Adaptive Turtle Perpetual System)

> Framework Go: **Binance per dati, Orderly per esecuzione**. Risk `base 1% / min 0.25% / max 2%` → `qty = (equity × risk%) / |entry-stop|`, leva dinamica hard cap 5×, satellite 30% per skew positivo. Testato su 14602 barre reali 4h (2020-2026).

---

## 0) Prerequisiti

```bash
Go 1.22+ (testato 1.24)  # go version
Git + GitHub CLI (gh)    # opzionale, per repo privato
# Binance: nessun API key per dati (fapi pubblico)
# Orderly: solo per LIVE — crea API key su https://orderly.network (ed25519)
```

Chiavi Orderly (solo LIVE): `ORDERLY_ACCOUNT_ID`, `ORDERLY_KEY`, `ORDERLY_SECRET` (base64 32/64 byte seed/priv). Mai committare — usa env.

---

## 1) Clona & Build (repo privato)

```bash
# HTTPS (token gh già loggato come eleonoraandrea)
git clone https://github.com/eleonoraandrea/turtle-extended.git
cd turtle-extended

# oppure SSH
# git clone git@github.com:eleonoraandrea/turtle-extended.git

go mod tidy
make build          # → ./atps (paper + backtest)
make build-live     # → ./atps-live (con Orderly reale, tag live) — opzionale

./atps --help
./atps tui --help
./atps live --help
```

> Binari `atps`/`atps-live` sono git-ignored (`/atps` in .gitignore) — builda localmente.

---

## 2) Configurazione

File: `configs/default.yaml` — già pronto per max performance (user spec):

```yaml
risk: {base: 0.01, min: 0.0025, max: 0.02}
portfolio: {max_open_risk: 0.03, max_correlated_risk: 0.02}
leverage: {max: 5}
trend: {donchian_entry: 55, donchian_exit: 20}
atr: {period: 20, initial_stop: 2.0}
pyramiding: {enabled: true, max_additions: 4, risk_neutral: true}
profit: {trailing: true, satellite: {enabled: true, allocation: 0.30}}
regime: {btc_filter: true, adx_min: 20}
volatility: {adaptive_risk: true}
drawdown: {adaptive_risk: true}
```

Non serve toccare nulla per iniziare. Override via CLI: `--symbol`, `--variant`, `--interval`.

---

## 3) Dati reali Binance (una volta — 14602 barre)

```bash
./atps download --symbol BTCUSDT --interval 4h --start 2020-01-01 --funding --oi
./atps download --symbol ETHUSDT --interval 4h --start 2020-01-01 --funding --oi
./atps download --symbol SOLUSDT --interval 4h --start 2020-01-01 --funding --oi
# Output: data/raw/BTCUSDT_4h.csv (OHLCV + funding/OI allineati)
# Verifica:
wc -l data/raw/*.csv          # BTC 14602, ETH 14602, SOL 13059
head -n 2 data/raw/BTCUSDT_4h.csv
```

Endpoints usati (pubblici, no key):
- `GET /fapi/v1/klines?symbol=BTCUSDT&interval=4h&limit=1500` paginato (fix gap pre-listing SOL)
- `GET /fapi/v1/fundingRate`
- `GET /futures/data/openInterestHist` (opzionale, gracefully degraded se 400)

Se sei offline, il backtest usa sintetico deterministico (`seed 42/1337/9999`).

---

## 4) Backtest + Report HTML (MT5-style, self-contained)

### TUI interattiva (consigliata)
```bash
./atps tui
# Tab: Simbolo → Variante → Timeframe → Azioni
# ↑↓ seleziona, Enter esegue, r: Run, c: Compare, ?: Help, q: esci
# Output: reports/TUI_BTCUSDT_D_*.html (file:// apribile, offline)
```

### CLI diretta
```bash
# Singolo
./atps backtest --symbol BTCUSDT --variant D --interval 4h --out reports/BTC_D.html
xdg-open reports/BTC_D.html   # o open su macOS

# Confronto A/B/C/D × BTC/ETH/SOL (come da pipeline quant)
./atps compare --symbols BTCUSDT,ETHUSDT,SOLUSDT --variants A,B,C,D
# → reports/comparison.html (rank Sharpe) + 12 HTML individuali A/B/C/D per simbolo

# Report contiene: equity vs BH + Lightweight-Charts 4.1 candele, drawdown, histogram PnL/R, 32 metriche
# (Sharpe, Sortino, Calmar, SkewR, ExpectancyR, PosSkewScore, PF, Kelly), monthly/yearly heatmap,
# regime LONG/SHORT/Year, costi fee/funding, trades MT5 (Entry/Exit, Qty, Lev×, Risk%, Bars, MAE/MFE, Fee, Funding, PnL, R, Satellite badge)
```

Serve report via HTTP:
```bash
make report-serve   # http://localhost:8000
```

---

## 5) Validazione severa (obbligatoria prima di live)

```bash
# Walk-forward 6 folds (70% train / 30% test)
./atps walk-forward --symbol BTCUSDT --variant D --folds 6
# → reports/BTCUSDT_D_WF.json  (decay, OOS, per-fold Sharpe)

# Parameter perturbation ±20% (robustezza, no overfit)
./atps perturb --symbol BTCUSDT --variant D
# → reports/BTCUSDT_D_PERTURB.json  (baseline 221% → perturb 150-273% tutti PROFITTEVOLI)

# Monte Carlo 1000 run (block bootstrap trade)
./atps montecarlo --symbol BTCUSDT --variant D --runs 1000
# → reports/BTCUSDT_D_MC.json  (median 222% p5 93% p95 396% prob 100%)

# Portfolio BTC+ETH+SOL con heat condiviso 3%
./atps portfolio --variant D
# → reports/PORTFOLIO_D.html (COMBINED 110% CAGR 11.8% Sharpe 0.77 PF 2.33)

# Test invarianti risk
go test ./tests -count=1 -v
# 11 PASS: heat ≤3%, lev ≤5×, stop R ≥ -1.8, sizing honoring 2% etc.
```

Solo se **tutti** sono verdi (walk-forward decay ≈1, perturb tutti profittevoli, Monte Carlo median ≈ backtest, portfolio DD controllato) → passa a paper.

---

## 6) Bot Live — Orderly

> **Isolamento:** `internal/backtest` MAI importa `internal/execution` (verifica `go list -deps ./internal/backtest` = 0). Live compila solo con `-tags live`.

### 6a) Paper trading (sicuro, default) — dry-run con capitale configurabile

Simula ordini, nessun capitale reale, stessa logica risk. **Capitale dry-run configurabile** via `--capital` o `configs/default.yaml: general.initial_capital`:

```bash
# Dry-run 10.000 USD (default da configs/default.yaml)
./atps live --symbol BTCUSDT --variant D --interval 4h --poll 30 --capital 10000 --dry-run=true

# Dry-run 25.000 USD per testare compounding più alto
./atps live --symbol BTCUSDT --variant D --poll 30 --capital 25000

# Dry-run usa sempre risk 2% max per |entry-stop| → qty = (equity×risk%)/|entry-stop|
# → più equity = più qty (verificato: equity 10000 → qty 0.021, equity 25000 → qty 0.052)

# TUI Live (dopo fix: prezzo + tutti i parametri real-time, NO grafico)
# Header: equity + PnL, balance, strat, Orderly symbol, DRY-RUN/LIVE, last tick
# ▣ MERCATO: Price $78826.40 +0.25% | Open/High/Low/Volume/Funding/OI | Funding veto spiegazione
# ◆ POSIZIONI: Orderly GET /v1/positions con uPnL
# ◇ PARAMETRI: ATR, ADX, EMA50/200, SMA200, Don55/20 H/L, VolRegime, FundingZ, OI Δ, Vol mult — tutti real-time
# ▤ LOGS: viewport scrollabile con sizing log
# Comandi TUI live: q esc, r tick manuale, p auto ON/OFF, d toggle dry-run (PAPER↔LIVE), ? help
# Headless (server, no TTY): log su stdout
timeout 60 ./atps live --poll 30 --capital 10000 2>&1 | tail
```

**Cosa vuol dire `funding veto`?** `fundingZ = (fundingRate - SMA30)/std30`. Se `|z|>2.8` il funding è estremo (mercato iper-affollato long, pagheresti funding alto) → filtro salta l’entry (`HOLD (D funding veto)`). È voluto per evitare di pagare funding da -0.1% ogni 8h su trend affollati. Disattivabile via `funding.filter: false` o alzando `funding_z_threshold` a 3.5.

Poll log atteso (paper):
```
[17:45:52] bot start BTCUSDT 4h variant D paper=true equity 10000
[17:45:52] signal: HOLD (D funding veto)
[17:47:19] signal: LONG  price 78200  stop 77000  risk 0.85%  qty 0.023  lev 0.42x  D 55 long adaptive
[17:47:19] order placed: BUY PERP_BTC_USDC qty 0.02300 id paper-... (PAPER)
```

### 6b) Live reale — Orderly

1. Crea API key su Orderly (ed25519): AccountId + Key (pub) + Secret (base64 seed/priv 32/64 byte)
2. Esporta env (mai committare):
```bash
export ORDERLY_ACCOUNT_ID="0x..."
export ORDERLY_KEY="ed25519:..."
export ORDERLY_SECRET="base64-seed-..."   # 32 o 64 byte base64
# Opzionale: testnet
export ORDERLY_BASE="https://testnet-api.orderly.org" # via --testnet
```
3. Build live + avvio con flag esplicito (double-opt-in):
```bash
go build -tags live -o atps-live ./cmd/atps
ORDERLY_ACCOUNT_ID=... ORDERLY_KEY=... ORDERLY_SECRET=... \
./atps-live live --symbol BTCUSDT --variant D --live --i-understand-live --poll 30

# Testnet (consigliato prima di mainnet)
./atps-live live --testnet --symbol BTCUSDT --variant D --live --i-understand-live
```

**Safety gates** (`docs/LIVE_EXECUTION_SPEC.md`):
- Paper default — senza `--live --i-understand-live` + env → resta paper
- Kill-switch: `touch /tmp/atps.halt` → blocca immediatamente PlaceOrder
- Heat 3% totale / 2% correlati, leva hard 5×, crash brake 8% → flat 24h (brakeUntil)
- Max notional 75k, fee 4bps + slippage 2bps, funding 8h scalato
- Rate limit Binance 1200/min, Orderly 10/sec — backoff 429
- **Checklist**: testnet 1 settimana, `GET /v1/public/info` lot/tick, small notional 100 USDC, backup config

---

## 7) Comandi utili

```bash
make build        # atps
make build-live   # atps-live
make demo         # sintetico + compare
make backtest     # BTCUSDT D real
make compare      # A/B/C/D ×3
make download     # BTC/ETH/SOL 4h + funding/OI
make walk         # walk-forward D BTC
make montecarlo   # MC D BTC
./atps perturb --symbol BTCUSDT --variant D
./atps portfolio --variant D
make test         # 11 tests
make report-serve # http://localhost:8000
make clean        # rm atps reports
```

---

## 8) Troubleshooting

| Problema | Causa → Fix |
|---|---|
| `csv not found → synthetic demo` | `data/raw/BTCUSDT_4h.csv` mancante → `make download` o `generate-demo` |
| `oiHist 400 param invalid` | OI Binance solo ~30gg → warning ignorabile, strategy usa `hasOI` check |
| `TUI error: could not open a new TTY` | No TTY (CI) → auto headless, log su stdout (`isatty` check) |
| `leverage exceeds` / `heat exhausted` | Normale — risk engine rifiuta entry, vedi `sizing log` in trade |
| `Orderly 401` | Secret formato errato → base64 32/64 byte, header `orderly-*` |
| `graphify disabled` | `node gsd-tools.cjs config-set graphify.enabled true` + `graphify . --update --code-only` |
| Report 14M lento | Normale (Lightweight-Charts + 500 trades) — apri via `file://` o `make report-serve` |

---

## 9) Roadmap subito dopo

```
REAL DATA (14602 barre) → A/B/C/D → COST ADJUST → WALK-FORWARD → PERTURB (±20% tutti PROFITTEVOLI) 
→ MONTE CARLO (median ≈ backtest) → REGIME → PORTFOLIO (110% combined) → PAPER (1 settimana) → LIVE con satellite 30%
```

Obiettivo: **Expectancy × Skew × Compounding** — 506 trades BTC A: mean R 2.49, median -1.0, max +90R, skew 3.96.

---

## 10) Repo

Privato: `https://github.com/eleonoraandrea/turtle-extended` — `git clone` + `go mod tidy` per iniziare.
Non committare `data/raw/*.csv` / `reports/*.html` (gitignored) né `.env` con secret.

