# ATPS Improve — Più CAGR a DD ~15% (Design)

Data: 2026-09-04
Stato: approvato dall'utente
Scope: BTC primario, conferma ETH/SOL — Variant A (Turtle classico) come base

## Contesto e finding critico

Verifica del 2026-09-04 (motore corrente, dati `data/raw/BTCUSDT_4h.csv`, 14627 barre 4h 2020→2026):

| Config | CAGR dichiarato | CAGR reale verificato |
|---|---|---|
| `configs/btc_opt.yaml` (commit 268f9fa) | 94.26%, DD -14.61% | **29.55%, DD -23.04%** |

- Il 94.26% NON è riproducibile con alcuna combinazione della grid dell'optimizer
  sul motore corrente (post-fix audit b485251). Era un numero del motore pre-fix
  (fill next-open, funding, stop-gap fix hanno eliminato profitti fantasma).
- Sweep completa grid optimizer (trail mode × atr × pyramiding × risk × satellite):
  **massimo reale = ~34.4% CAGR, DD -23.1%** (donchian trail, atr 1.6, pyr 3 add,
  risk 2.0%, sat 0.3).
- **Tre engine config divergenti coesistono**:
  - CLI backtest: `TrailMode="donchian"` hardcoded per A/B/C (cmd/atps/main.go:570)
  - Bot live: `TrailMode="chandelier"`, `DonExit=20` hardcoded (internal/bot/bot.go:564)
  - Optimizer: configurabile per combo (scripts/optimize/main.go)
  → backtest ≠ live ≠ numeri ottimizzati.

Baseline onesta di partenza (btc_opt.yaml, CLI): **29.55% CAGR, DD -23.04%, Sharpe 1.15**.

## Obiettivo

Massimizzare CAGR con vincolo **DD ≤ 15-18%**, numeri interamente riproducibili
dal CLI e dal bot live. Realistico: **35-50% CAGR**. Nessun numero non verificato.

## 1. Fondamenta: unificazione engine config

- Ogni variante (A/B/C/D) porta i **propri** parametri engine nel config YAML:
  `trail_mode` (`donchian|chandelier`), `trail_atr_mult`, `don_exit`,
  `pyramiding_max_units`, `pyramid_step_atr`. Fallback alla sezione globale
  `backtest:` se omessi (retrocompatibilità con config esistenti).
- Nuova funzione unica `EngineConfigFrom(cfg, variant)` usata da:
  CLI backtest, bot live, optimizer. Eliminare gli hardcoded.
- Test di consistenza: stessa config+variante → EngineConfig identica nei 3 punti.

## 2. Risk tuning: comprimere DD a ~15% e recuperare CAGR

Il risk engine ha già la curva DD-adaptive lineare (risk.go §3): da
`dd_deleverage_start_pct` a `dd_flat_pct`, scala verso minRisk (mai a zero).
Oggi: start 10%, flat 25% → troppo permissivo, DD arriva a -23%.

- Stringere la curva: sweep `dd_deleverage_start {7,8,10}` × `dd_flat {17,20,25}`
- Sweep congiunto completo:
  - risk base {1.5, 2.0, 2.5, 3.0}% (max = base, min 0.25% fisso)
  - atr_stop_mult {1.4, 1.6, 1.8}
  - pyr adds {3, 4, 6} (step 0.5, risk_neutral)
  - satellite {0, 0.30, 0.40}
  - trail {donchian, chandelier 2.5, chandelier 3.0}
  - don_exit {10, 20}
- **Selezione sul train 70%: max CAGR tra le combo con DD ≤ 15%**
  (vincolo di ottimizzazione; l'accettazione finale ammette DD ≤ 18% sul
  full history se il test lo conferma).
- La leva dinamica e i cap (heat 3%, correlati 2%, lev 5×) restano invariati.

## 3. Alpha strutturale (nuovo codice, OFF di default)

### 3a. Entry intrabar a livello canale

Oggi: conferma a close sopra HH20[-1] → fill alla open successiva. Perde tratto
di breakout e paga lo slippage sulla open.

Nuovo (`entry_mode: intrabar` per variante):
- Trigger: `bar.High ≥ HH20[-1]` (long) / `bar.Low ≤ LL20[-1]` (short)
- Fill al livello canale; se la open gap-a-favore oltre il livello → fill alla open
- Slippage bps sul fill, fee invariata
- Anti-lookahead: livello calcolato SOLO su barre precedenti (i-1)
- Caso pessimistico stessa-barra: se dopo il fill anche lo stop è toccabile
  nella stessa barra (low ≤ stop per long) → assumere sequenza fill→stop
  (perdita -1R registrata), non fill→profitto
- Il segnale strategia non cambia: cambia dove l'engine esegue il fill

### 3b. Re-entry dopo stop-out

Dopo un'uscita "stop", se:
- il trend filter che ha autorizzato l'entry regge ancora (close > SMA200 per long), e
- il prezzo fa un nuovo high N-barre (`reentry.lookback`, default 10) entro
  `reentry.within_bars` (default 20) dall'uscita,

→ nuovo segnale entry stessa direzione, stop a `atr_stop_mult × ATR` dal fill.
Config: `reentry: {enabled: false, lookback: 10, within_bars: 20}` per variante.

## 4. Protocollo anti-overfitting (rigido, in ordine)

1. Grid search **solo su train 70%** → top 10 per CAGR con DD ≤ 15%
2. **Test 30%, una sola lettura**: promuove chi mantiene CAGR e Calmar
   (degrado CAGR train→test < 1/3, Calmar test > 0)
3. **Walk-forward 8 folds** del vincitore: mediana Sharpe > 0 richiesta
4. **Perturbazione** ±20% parametri: degrado CAGR < 30%
5. **Conferma ETH/SOL** senza ri-ottimizzazione: nessun simbolo peggiora > 20%
6. Report finale: numeri reali train/test/WF/full; i nomi config non contengono
   numeri di performance non verificati

Se una feature strutturale (3a/3b) non passa il protocollo → resta OFF di default.

## 5. Deliverable

- `configs/atps_v2.yaml` — configurazione vincitrice validata (BTC) + verifiche ETH/SOL
- Optimizer esteso: vincolo DD in selezione, nuove leve (dd curve, entry_mode, reentry)
- Bot live che usa `EngineConfigFrom` (identico al backtest)
- Report HTML: baseline vs v2, walk-forward, Monte Carlo
- README aggiornato con numeri onesti verificati

## 6. Testing

- Unit: `EngineConfigFrom` consistenza CLI/bot/optimizer + fallback legacy
- Unit engine intrabar: fill a livello, fill a open su gap, stessa-barra stop pessimistico
- Unit re-entry: condizione trend, finestra within_bars, nuovo high lookback
- Unit config: parsing nuove chiavi per variante + default retrocompatibili
- Integrazione: `go test ./...` verde (nessuna regressione esistenti)
- Validazione: protocollo §4 eseguito e documentato nei report

## Criteri di successo

- DD full-history ≤ 18% (target 15%)
- CAGR full-history ≥ baseline 29.55% (target 35-50%)
- Riproducibilità: CLI backtest e bot live producono lo stesso EngineConfig
- WF mediana Sharpe > 0; perturb < 30% degrado; ETH/SOL non peggiorano > 20%
- Tutti i numeri pubblicati riproducibili con un comando

## Fuori scope

- Portfolio engine multi-simbolo vero (approccio C — futuro)
- Modifiche alle varianti B/C/D oltre alla config engine per-variante
- Esecuzione live reale su Orderly (solo allineamento config)
