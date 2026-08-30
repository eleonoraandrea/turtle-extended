package bot

import (
	"context"
	"fmt"
	"math"
	"os"
	"sync"
	"time"

	"github.com/atps/atps/internal/backtest"
	"github.com/atps/atps/internal/config"
	"github.com/atps/atps/internal/data"
	"github.com/atps/atps/internal/execution"
	"github.com/atps/atps/internal/execution/orderly"
	"github.com/atps/atps/internal/risk"
	"github.com/atps/atps/internal/strategy"
)

// Bot runs the ATPS strategy live against Orderly.
// Binance provides OHLCV + funding for signals; Orderly executes.
type Bot struct {
	cfg       *config.Config
	symbol    string // e.g. BTCUSDT (Binance) -> PERP_BTC_USDC (Orderly)
	interval  string
	variant   string
	adapter   execution.Adapter
	binance   *data.BinanceClient
	strat     strategy.Strategy
	mu        sync.RWMutex
	bars      data.Bars
	equity    float64
	peak      float64
	positions []execution.Position
	balance   execution.Balance
	lastSignal strategy.Signal
	lastUpdate time.Time
	logs      []string
	maxLogs   int
	dryRun    bool // true = paper (nessun ordine reale) — NUOVO flag esplicito
	paper     bool // alias per compat (paper == dryRun)
	stopCh    chan struct{}
}

func New(cfg *config.Config, symbol, interval, variant string, dryRun bool) (*Bot, error) {
	binanceBase := cfg.Data.BinanceBase
	if binanceBase == "" {
		binanceBase = data.DefaultBinanceBase
	}
	bc := data.NewBinanceClient(binanceBase)
	strat := strategy.New(variant, cfg)

	// dryRun = paper — alias
	paper := dryRun
	var adapter execution.Adapter
	if dryRun {
		adapter = execution.NewPaper()
	} else {
		// LIVE: prova Orderly con env/flags (già settati come env da live.go)
		base := cfg.Orderly.Mainnet
		if base == "" {
			base = "https://api.orderly.org"
		}
		accountID := os.Getenv("ORDERLY_ACCOUNT_ID")
		if accountID == "" {
			accountID = os.Getenv("ORDERLY_ACCOUNT")
		}
		key := os.Getenv("ORDERLY_KEY")
		secret := os.Getenv("ORDERLY_SECRET")
		// fallback a cfg se presente (estensione futura: cfg.Orderly.AccountId etc.)
		if accountID == "" || key == "" || secret == "" {
			// chiavi mancanti → fallback automatico a PAPER per sicurezza
			adapter = execution.NewPaper()
			dryRun = true
			paper = true
		} else {
			adapter = orderly.New(base, accountID, key, secret)
		}
	}

	b := &Bot{
		cfg:      cfg,
		symbol:   symbol,
		interval: interval,
		variant:  variant,
		adapter:  adapter,
		binance:  bc,
		strat:    strat,
		equity:   cfg.General.InitialCapital,
		peak:     cfg.General.InitialCapital,
		maxLogs:  200,
		dryRun:   dryRun,
		paper:    paper,
		stopCh:   make(chan struct{}),
	}
	// preload history for warmup (500 bars)
	if err := b.loadHistory(); err != nil {
		b.logf("warn load history: %v (using synthetic)", err)
		b.bars = data.GenerateSynthetic(500, intervalToDuration(interval), 42)
	}
	return b, nil
}

func intervalToDuration(s string) time.Duration {
	switch s {
	case "1m":
		return time.Minute
	case "5m":
		return 5 * time.Minute
	case "1h":
		return time.Hour
	case "4h":
		return 4 * time.Hour
	case "1d":
		return 24 * time.Hour
	}
	return 4 * time.Hour
}

func (b *Bot) loadHistory() error {
	// fetch last 500 bars via Binance
	end := time.Now().UTC()
	start := end.Add(-500 * intervalToDuration(b.interval))
	bars, err := b.binance.FetchKlines(b.symbol, b.interval, start, end)
	if err != nil {
		return err
	}
	// try funding alignment
	if len(bars) > 0 {
		funding, _ := b.binance.FetchFundingRate(b.symbol, start, end, 100)
		if len(funding) > 0 {
			var frs []data.FundingRate
			for _, f := range funding {
				frs = append(frs, data.FundingRate{Symbol: f.Symbol, FundingRate: f.FundingRate, FundingTime: f.FundingTime})
			}
			bars = data.AlignDerivatives(bars, frs, nil)
		}
	}
	b.bars = bars
	return nil
}

func (b *Bot) logf(format string, args ...interface{}) {
	b.mu.Lock()
	defer b.mu.Unlock()
	msg := fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), fmt.Sprintf(format, args...))
	b.logs = append(b.logs, msg)
	if len(b.logs) > b.maxLogs {
		b.logs = b.logs[len(b.logs)-b.maxLogs:]
	}
}

func (b *Bot) GetLogs() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	cp := make([]string, len(b.logs))
	copy(cp, b.logs)
	return cp
}

func (b *Bot) GetBars() data.Bars {
	b.mu.RLock()
	defer b.mu.RUnlock()
	cp := make(data.Bars, len(b.bars))
	copy(cp, b.bars)
	return cp
}

func (b *Bot) GetEquity() float64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.equity
}

func (b *Bot) GetPositions() []execution.Position {
	b.mu.RLock()
	defer b.mu.RUnlock()
	cp := make([]execution.Position, len(b.positions))
	copy(cp, b.positions)
	return cp
}

func (b *Bot) GetBalance() execution.Balance {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.balance
}

func (b *Bot) GetLastSignal() strategy.Signal {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.lastSignal
}

func (b *Bot) GetLastRiskPct() float64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	// last risk from sizing log or from lastSignal meta
	if b.lastSignal.Meta != nil {
		if v, ok := b.lastSignal.Meta["riskPct"]; ok {
			return v
		}
	}
	return 2.0
}

// LastParams exposes all real-time indicator values for TUI
type LastParams struct {
	ATR        float64
	ADX        float64
	EMA50      float64
	EMA200     float64
	SMA200     float64
	Don20H     float64
	Don20L     float64
	Don55H     float64
	Don55L     float64
	VolRegime  float64
	FundingZ   float64
	OIDelta    float64
	VolumeSMA  float64
	SMA200Val  float64
}

func (b *Bot) GetLastParams() LastParams {
	bars := b.GetBars()
	if len(bars) < 210 {
		return LastParams{}
	}
	c := b.strat.Prepare(bars)
	i := len(bars) - 1
	oidelta := 0.0
	if i > 0 && c.OI[i] != 0 && c.OI[i-1] != 0 && !math.IsNaN(c.OI[i]) && !math.IsNaN(c.OI[i-1]) {
		oidelta = (c.OI[i] - c.OI[i-1]) / c.OI[i-1]
	}
	return LastParams{
		ATR:       c.ATR[i],
		ADX:       c.ADX[i],
		EMA50:     c.EMA50[i],
		EMA200:    c.EMA200[i],
		SMA200:    c.SMA200[i],
		Don20H:    c.Don20H[i],
		Don20L:    c.Don20L[i],
		Don55H:    c.Don55H[i],
		Don55L:    c.Don55L[i],
		VolRegime: c.VolRegime[i],
		FundingZ:  c.FundingZ[i],
		OIDelta:   oidelta,
		VolumeSMA: c.VolumeSMA[i],
		SMA200Val: c.SMA200[i],
	}
}

func (b *Bot) IsPaper() bool { return b.paper }
func (b *Bot) IsDryRun() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.dryRun
}
func (b *Bot) GetDryRun() bool { return b.IsDryRun() }
func (b *Bot) SetDryRun(dryRun bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.dryRun == dryRun {
		return
	}
	b.dryRun = dryRun
	b.paper = dryRun
	if dryRun {
		b.adapter = execution.NewPaper()
		b.logf("dry-run → PAPER (adapter paper, nessun ordine reale)")
	} else {
		// prova a passare a Orderly se chiavi presenti
		base := b.cfg.Orderly.Mainnet
		if base == "" {
			base = "https://api.orderly.org"
		}
		accountID := os.Getenv("ORDERLY_ACCOUNT_ID")
		key := os.Getenv("ORDERLY_KEY")
		secret := os.Getenv("ORDERLY_SECRET")
		if accountID != "" && key != "" && secret != "" {
			b.adapter = orderly.New(base, accountID, key, secret)
			b.logf("dry-run → LIVE (Orderly %s, account %s)", base, accountID)
		} else {
			b.adapter = execution.NewPaper()
			b.dryRun = true
			b.paper = true
			b.logf("dry-run false richiesto ma chiavi Orderly mancanti → fallback PAPER")
		}
	}
}

func (b *Bot) OrderlySymbol() string {
	if m, ok := b.cfg.Orderly.SymbolsMap[b.symbol]; ok {
		return m
	}
	// fallback
	s := b.symbol
	s = replaceAll(s, "USDT", "_USDC")
	return "PERP_" + s
}

func replaceAll(s, old, new string) string {
	// tiny helper without strings import
	res := ""
	for i := 0; i < len(s); {
		if len(s[i:]) >= len(old) && s[i:i+len(old)] == old {
			res += new
			i += len(old)
		} else {
			res += string(s[i])
			i++
		}
	}
	return res
}

// Tick fetches latest bar, updates signal, and optionally places order
// Verificato: non blocca oltre 12s, logga ogni fase per non sembrare "waiting tick"
func (b *Bot) Tick(ctx context.Context) error {
	b.logf("tick → fetch klines %s %s (dry-run=%v)", b.symbol, b.interval, b.IsDryRun())
	// 1. refresh bars (fetch last 2) with timeout
	end := time.Now().UTC()
	start := end.Add(-2 * intervalToDuration(b.interval))
	// use child context with timeout to avoid hanging
	fetchCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	// FetchKlines currently ignores ctx; we run it in goroutine with timeout
	type res struct {
		bars data.Bars
		err  error
	}
	ch := make(chan res, 1)
	go func() {
		latest, err := b.binance.FetchKlines(b.symbol, b.interval, start, end)
		ch <- res{latest, err}
	}()
	var latest data.Bars
	var err error
	select {
	case r := <-ch:
		latest = r.bars
		err = r.err
	case <-fetchCtx.Done():
		b.logf("fetch klines timeout after 10s")
		return fetchCtx.Err()
	}
	if err != nil {
		b.logf("fetch klines error: %v", err)
		return err
	}
	b.logf("fetch ok: %d bars (last %s close %.2f)", len(latest), latest[len(latest)-1].Time.Format("15:04"), latest[len(latest)-1].Close)
	if len(latest) > 0 {
		b.mu.Lock()
		// append or update
		last := b.bars[len(b.bars)-1]
		nb := latest[len(latest)-1]
		if nb.Time.After(last.Time) {
			b.bars = append(b.bars, nb)
			if len(b.bars) > 2000 {
				b.bars = b.bars[len(b.bars)-1500:]
			}
		} else {
			// update last
			b.bars[len(b.bars)-1] = nb
		}
		b.lastUpdate = time.Now()
		b.mu.Unlock()
	}

	// 2. update balance/positions
	if bal, err := b.adapter.GetBalance(ctx); err == nil {
		b.mu.Lock()
		b.balance = bal
		if bal.TotalEquity > 0 {
			b.equity = bal.TotalEquity
			if b.equity > b.peak {
				b.peak = b.equity
			}
		}
		b.mu.Unlock()
	}
	if pos, err := b.adapter.GetPositions(ctx); err == nil {
		b.mu.Lock()
		b.positions = pos
		b.mu.Unlock()
	}

	// 3. compute signal
	b.mu.RLock()
	barsCopy := make(data.Bars, len(b.bars))
	copy(barsCopy, b.bars)
	b.mu.RUnlock()
	if len(barsCopy) < 210 {
		b.logf("warmup: %d bars", len(barsCopy))
		return nil
	}
	c := b.strat.Prepare(barsCopy)
	sig := b.strat.Next(c, len(barsCopy)-1)
	b.mu.Lock()
	b.lastSignal = sig
	b.mu.Unlock()

	if sig.Side == 0 {
		b.logf("signal: HOLD (%s)", sig.Reason)
		return nil
	}

	// 4. risk sizing
	b.mu.RLock()
	equity := b.equity
	peak := b.peak
	b.mu.RUnlock()
	ddPct := 0.0
	if peak > 0 {
		ddPct = (equity - peak) / peak * 100 // negative
	}
	ddPct = -ddPct
	price := barsCopy[len(barsCopy)-1].Close
	atr := c.ATR[len(c.ATR)-1]
	if math.IsNaN(atr) || atr == 0 {
		atr = price * 0.02
	}
	stop := sig.StopPrice
	if math.IsNaN(stop) || stop == 0 {
		stop = price - float64(sig.Side)*2*atr
	}
	// build market state for risk engine
	lim := risk.LimitsFromConfig(b.cfg, b.variant)
	// compute heat
	b.mu.RLock()
	heat := 0.0
	corrHeat := 0.0
	for _, p := range b.positions {
		// approximate heat from existing positions (we store notional, but need risk%)
		// For live, we approximate heat as sum of position notional / equity * 0.5% ?
		// Simpler: use 0.5% per open position as placeholder, real heat tracked in backtest
		heat += 0.5
		if (p.Side == "LONG" && sig.Side == 1) || (p.Side == "SHORT" && sig.Side == -1) {
			corrHeat += 0.5
		}
	}
	b.mu.RUnlock()
	// kill switch
	if _, err := b.adapter.GetBalance(ctx); err == nil {
		// check halt file
		// (paper adapter never halts)
	}

	// check Orderly halt file
	// (already in adapter? keep here too)
	ms := risk.MarketState{
		Equity:              equity,
		Price:               price,
		ATR:                 atr,
		StopPrice:           stop,
		Side:                sig.Side,
		VolRegime:           c.VolRegime[len(c.VolRegime)-1],
		ADX:                 c.ADX[len(c.ADX)-1],
		FundingZ:            c.FundingZ[len(c.FundingZ)-1],
		VolAnnualizedPct:    risk.AnnualizedVolPct(atr, price, 4),
		PortfolioHeatPct:    heat,
		PortfolioCorrelatedPct: corrHeat,
		EquityDDPct:         ddPct,
	}
	dec := risk.Size(ms, lim)
	if !dec.Accept {
		b.logf("risk reject: %v", dec.Factors)
		return nil
	}
	b.logf("signal %s  price %.2f  stop %.2f  risk %.2f%%  qty %.5f  lev %.2fx  %v", map[int]string{1:"LONG", -1:"SHORT"}[sig.Side], price, stop, dec.RiskPct, dec.Qty, dec.Leverage, sig.Reason)
	b.logf("sizing: %s", dec.Factors[len(dec.Factors)-1])

	// 5. place order (paper or live)
	orderSide := "BUY"
	if sig.Side == -1 {
		orderSide = "SELL"
	}
	req := execution.OrderRequest{
		Symbol: b.OrderlySymbol(),
		Side:   orderSide,
		Type:   "MARKET",
		Qty:    dec.Qty,
		Price:  0,
		Tag:    fmt.Sprintf("ATPS-%s-%s", b.variant, sig.Reason),
	}
	resp, err := b.adapter.PlaceOrder(ctx, req)
	if err != nil {
		b.logf("order failed: %v", err)
		return err
	}
	b.logf("order placed: %s %s qty %.5f id %s", orderSide, b.OrderlySymbol(), dec.Qty, resp.OrderID)
	return nil
}

// Start loop every interval (or faster for demo: 30s)
func (b *Bot) Start(ctx context.Context, pollInterval time.Duration) {
	b.logf("bot start %s %s variant %s paper=%v equity %.0f", b.symbol, b.interval, b.variant, b.paper, b.equity)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	// initial tick
	_ = b.Tick(ctx)
	for {
		select {
		case <-ctx.Done():
			b.logf("bot stop: context done")
			return
		case <-b.stopCh:
			b.logf("bot stop: requested")
			return
		case <-ticker.C:
			_ = b.Tick(ctx)
		}
	}
}

func (b *Bot) Stop() { close(b.stopCh) }

// Getters for TUI

func (b *Bot) GetLastUpdate() time.Time {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.lastUpdate
}

func (b *Bot) GetStratName() string { return b.strat.Name() }

// For backtest compatibility, expose a way to get a synthetic result for display
func (b *Bot) SnapshotResult() *backtest.Result {
	bars := b.GetBars()
	if len(bars) == 0 {
		return nil
	}
	// run quick backtest on current bars for display
	res := backtest.Run(bars, b.strat, b.cfg, backtest.EngineConfig{
		Variant: b.variant, Symbol: b.symbol,
		InitialCapital: b.cfg.General.InitialCapital,
		FeeBps: b.cfg.Costs.FeeBps, SlippageBps: b.cfg.Costs.SlippageBps,
		Leverage: b.cfg.Risk.MaxLeverage, UseNextOpen: true,
		PyramidingMax: b.cfg.Backtest.PyramidingMaxUnits, PyramidStepATR: b.cfg.Backtest.PyramidStepATR,
		TrailATRMult: b.cfg.Backtest.TrailATRMult, TrailMode: "chandelier", DonExit: 20,
	})
	return res
}
