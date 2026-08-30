# LIVE_EXECUTION_SPEC — ATPS Orderly Isolation

## Principio

`internal/backtest` **MAI** importa `internal/execution`. Verifica con `go list -deps ./internal/backtest` deve mostrare zero `execution`. Il package `orderly` è compilato solo con `-tags live`. Così un bug del backtester non può mai chiamare `PlaceOrder`.

```
backtest → strategy → indicators → data   (safe)
execution → orderly                      (isolato, solo cmd/atps con tag)
```

## Build

- Backtest/report: `go build -o atps ./cmd/atps`  (stub execution, `ErrNotLiveCompiled` se provi PlaceOrder)
- Live: `go build -tags live -o atps-live ./cmd/atps`  (vera Orderly client)

## Auth Orderly (ed25519)

- Base: `https://api.orderly.org` mainnet, `https://testnet-api.orderly.org` testnet. WS `wss://ws.orderly.org/ws`.
- Symbol: `PERP_BTC_USDC`, `PERP_ETH_USDC`, `PERP_SOL_USDC` (`config.orderly.symbols_map`).
- Header: `orderly-account-id`, `orderly-key`, `orderly-timestamp` (ms), `orderly-signature` = `base64( sign(ed25519(priv), timestamp+METHOD+path+body) )`.
- Secret: base64 32-byte seed o 64-byte privkey (da Orderly API key). Mai committed, via env `ORDERLY_SECRET`.

## Safety gates

1. **Paper default**: `atps-live` senza `ORDERLY_SECRET` usa `PaperAdapter` (dry-run, logga ma non invia).
2. **Flag --i-understand-live**: per inviare ordini reali serve esplicito flag.
3. **Max notional**: `costs.max_notional_per_trade` 50k, leva max 3x, heat portfolio 6%.
4. **Kill-switch**: se `/tmp/atps.halt` esiste, ogni `PlaceOrder` torna `halted`.
5. **Crash brake live**: se equity drawdown 8% in 4h → chiude all, block 24h.
6. **Rate limit**: 1200/min Binance, 10/sec Orderly public. Backoff 429.
7. **Testnet first**: sempre testare su `testnet-api.orderly.org` con piccola size.

## Flusso live

```
Binance klines (poll) → data.AlignDerivatives → strategy.Next → risk.PositionSize → execution.PlaceOrder → WS order update → reporting
```

Funding live non impatta ordini (drag contabile), ma loggato per PnL net.

## Monitor

- Health: `GET /v1/client/holding`, `GET /v1/positions` polling 30s, WS per fill immediato.
- Log trades + funding in `data/live/*.csv`, report giornaliero HTML.

## Checklist prima di LIVE

- [ ] Compilato con `-tags live` e testato su paper 1 settimana.
- [ ] Verificato `orderly.GetSymbols` e lot/tick.
- [ ] Funded account, leva 3, small notional 100 USDC test.
- [ ] Kill-switch funzionale.
- [ ] Backup config e報告 reproduction.
- NOT financial advice.

