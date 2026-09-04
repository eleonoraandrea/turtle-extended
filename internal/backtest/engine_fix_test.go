package backtest

import (
	"math"
	"testing"
	"time"

	"github.com/atps/atps/internal/config"
	"github.com/atps/atps/internal/data"
	"github.com/atps/atps/internal/strategy"
)

// scriptStrategy emits scripted signals per bar index for deterministic tests.
type scriptStrategy struct {
	cfg     *config.Config
	signals map[int]strategy.Signal
}

func (s *scriptStrategy) Name() string                    { return "script" }
func (s *scriptStrategy) Variant() string                 { return "A" }
func (s *scriptStrategy) Warmup() int                     { return 0 }
func (s *scriptStrategy) Prepare(bars data.Bars) *strategy.Context {
	return strategy.PrepareCommon(bars, s.cfg, "A")
}
func (s *scriptStrategy) Next(_ *strategy.Context, i int) strategy.Signal { return s.signals[i] }

func fixCfg(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.Load("../../configs/default.yaml")
	if err != nil {
		t.Fatalf("load cfg: %v", err)
	}
	cfg.Profit.Satellite.Enabled = false
	cfg.Costs.FeeBps = 0
	cfg.Costs.SlippageBps = 0
	return cfg
}

func fixBars(closes []float64, spread float64) data.Bars {
	t0 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	bars := make(data.Bars, len(closes))
	for i, c := range closes {
		bars[i] = data.Bar{
			Time: t0.Add(time.Duration(i) * 4 * time.Hour),
			Open: c, High: c + spread/2, Low: c - spread/2, Close: c, Volume: 1e6,
		}
	}
	return bars
}

func fixEngine(cfg *config.Config) EngineConfig {
	return EngineConfig{
		Variant: "A", Symbol: "TEST", InitialCapital: 10000,
		FeeBps: cfg.Costs.FeeBps, SlippageBps: cfg.Costs.SlippageBps,
		UseNextOpen: false, PyramidingMax: 4, PyramidStepATR: 0.5,
		TrailMode: "donchian", DonExit: 20,
	}
}

// Test 1 — risk-neutral pyramid must update EntryPrice to the weighted average.
func TestPyramidRiskNeutralWeightedEntryPrice(t *testing.T) {
	cfg := fixCfg(t)
	closes := make([]float64, 30)
	for i := range closes {
		closes[i] = 100
	}
	closes[28] = 104 // +4% < crash brake 8%
	closes[29] = 108 // +3.85% < crash brake 8%
	bars := fixBars(closes, 1.0)
	strat := &scriptStrategy{cfg: cfg, signals: map[int]strategy.Signal{
		26: {Side: 1, StopPrice: 90},
		28: {Side: 1, StopPrice: 90},
	}}
	cfg.Risk.VolTargetPct = 1e9 // deterministic: vol-target never binds in this test
	res := Run(bars, strat, cfg, fixEngine(cfg))
	if len(res.Trades) != 1 {
		t.Fatalf("expect 1 trade, got %d", len(res.Trades))
	}
	tr := res.Trades[0]
	// entry 100 qty 10 (risk $100 / stopDist 10). At the add bar marked equity is
	// 10000 + (104-100)*10 = 10040; risk_neutral halves the 1% risk → $50.2 over
	// stopDist |104-90|=14 → q1 = 3.585714…
	q1 := (0.01 / 2) * 10040 / 14
	wantEP := (100*10 + 104*q1) / (10 + q1)
	if math.Abs(tr.EntryPrice-wantEP) > 1e-6 {
		t.Fatalf("pyramid EntryPrice: want weighted avg %.6f, got %.6f (stale entry not averaged)", wantEP, tr.EntryPrice)
	}
	wantPnL := (108 - wantEP) * tr.Qty
	if math.Abs(tr.PnL-wantPnL) > 1e-6 {
		t.Fatalf("pyramid PnL: want %.6f, got %.6f", wantPnL, tr.PnL)
	}
}

// Test 2 — stop gap-through must fill at the open, not at the stop price.
func TestGapThroughStopFillsAtOpen(t *testing.T) {
	cfg := fixCfg(t)
	bars := fixBars([]float64{100, 0}, 1.0)
	// bar 1 gaps down: open 90, low 89 — stop 98 must fill at 90
	bars[1] = data.Bar{Time: bars[1].Time, Open: 90, High: 91, Low: 89, Close: 90.5, Volume: 1e6}
	strat := &scriptStrategy{cfg: cfg, signals: map[int]strategy.Signal{
		0: {Side: 1, StopPrice: 98},
	}}
	res := Run(bars, strat, cfg, fixEngine(cfg))
	if len(res.Trades) != 1 {
		t.Fatalf("expect 1 trade, got %d", len(res.Trades))
	}
	tr := res.Trades[0]
	if tr.ExitPrice != 90 {
		t.Fatalf("gap stop fill: want 90 (min(stop,open)), got %.4f", tr.ExitPrice)
	}
	if tr.PnL >= 0 {
		t.Fatalf("gap stop must be a loss, got PnL %.4f", tr.PnL)
	}
}

// Test 3 — fee ledger reconciliation: FinalEquity-InitialCapital == NetPnL, TotalFee == sum(Trade.Fee).
func TestFeeLedgerReconciliation(t *testing.T) {
	cfg := fixCfg(t)
	cfg.Costs.FeeBps = 4
	bars := fixBars([]float64{100, 105}, 1.0)
	strat := &scriptStrategy{cfg: cfg, signals: map[int]strategy.Signal{
		0: {Side: 1, StopPrice: 95},
	}}
	eng := fixEngine(cfg)
	res := Run(bars, strat, cfg, eng)
	if len(res.Trades) != 1 {
		t.Fatalf("expect 1 trade, got %d", len(res.Trades))
	}
	if math.Abs(res.FinalEquity-res.InitialCapital-res.NetPnL) > 1e-9 {
		t.Fatalf("equity ledger: Final-Init %.6f != NetPnL %.6f", res.FinalEquity-res.InitialCapital, res.NetPnL)
	}
	sumFee := 0.0
	for _, tr := range res.Trades {
		sumFee += tr.Fee
	}
	if math.Abs(res.TotalFee-sumFee) > 1e-9 {
		t.Fatalf("TotalFee %.6f != sum(Trade.Fee) %.6f (entry fee double-charged)", res.TotalFee, sumFee)
	}
}

// Test 4 — invalid stop side (long with stop above entry) must not open a position.
func TestInvalidStopSideRejected(t *testing.T) {
	cfg := fixCfg(t)
	bars := fixBars([]float64{100, 100, 100}, 1.0)
	strat := &scriptStrategy{cfg: cfg, signals: map[int]strategy.Signal{
		0: {Side: 1, StopPrice: 105}, // stop ABOVE long entry — invalid
	}}
	res := Run(bars, strat, cfg, fixEngine(cfg))
	if len(res.Trades) != 0 {
		t.Fatalf("invalid stop side must be rejected, got %d trades", len(res.Trades))
	}
}

// Test 5 — zero/negative equity must not trade (no fictitious $1 sizing).
func TestBankruptEquityNoTrading(t *testing.T) {
	cfg := fixCfg(t)
	bars := fixBars([]float64{100, 101, 102, 103}, 1.0)
	strat := &scriptStrategy{cfg: cfg, signals: map[int]strategy.Signal{
		0: {Side: 1, StopPrice: 95}, 1: {Side: 1, StopPrice: 95},
		2: {Side: 1, StopPrice: 95}, 3: {Side: 1, StopPrice: 95},
	}}
	eng := fixEngine(cfg)
	eng.InitialCapital = 0
	res := Run(bars, strat, cfg, eng)
	if len(res.Trades) != 0 {
		t.Fatalf("bankrupt account must not trade, got %d trades", len(res.Trades))
	}
	if res.FinalEquity != 0 {
		t.Fatalf("bankrupt account equity must stay 0, got %.4f", res.FinalEquity)
	}
}

// Test 6 — signal on the last bar with UseNextOpen must not create a phantom trade.
func TestUseNextOpenLastBarNoEntry(t *testing.T) {
	cfg := fixCfg(t)
	cfg.Costs.FeeBps = 4
	bars := fixBars([]float64{100, 101, 102}, 1.0)
	strat := &scriptStrategy{cfg: cfg, signals: map[int]strategy.Signal{
		2: {Side: 1, StopPrice: 95},
	}}
	eng := fixEngine(cfg)
	eng.UseNextOpen = true
	res := Run(bars, strat, cfg, eng)
	if len(res.Trades) != 0 {
		t.Fatalf("last-bar signal with UseNextOpen must not trade, got %d trades", len(res.Trades))
	}
}

// Test 7 — donchian close exit must be reachable (compare vs prior bar's channel).
func TestDonchianCloseExitReachable(t *testing.T) {
	cfg := fixCfg(t)
	closes := make([]float64, 27)
	for i := range closes {
		closes[i] = 100
	}
	bars := fixBars(closes, 1.1)
	// bar 26: closes below the prior 20-bar low (99.45) but above the chandelier stop
	bars[26] = data.Bar{Time: bars[26].Time, Open: 99.2, High: 99.3, Low: 98.7, Close: 98.8, Volume: 1e6}
	strat := &scriptStrategy{cfg: cfg, signals: map[int]strategy.Signal{
		25: {Side: 1, StopPrice: 90},
	}}
	eng := fixEngine(cfg)
	eng.TrailMode = "chandelier"
	eng.TrailATRMult = 2.5
	res := Run(bars, strat, cfg, eng)
	if len(res.Trades) != 1 {
		t.Fatalf("expect 1 trade, got %d", len(res.Trades))
	}
	if res.Trades[0].ExitReason != "donchian_exit" {
		t.Fatalf("expect donchian_exit (close below prior channel), got %q", res.Trades[0].ExitReason)
	}
}

// Test 8 — with UseNextOpen the signal bar's equity must not include the not-yet-filled position.
func TestUseNextOpenNoMarkAtSignalBar(t *testing.T) {
	cfg := fixCfg(t)
	bars := fixBars([]float64{100, 102, 103}, 1.0)
	strat := &scriptStrategy{cfg: cfg, signals: map[int]strategy.Signal{
		0: {Side: 1, StopPrice: 95},
	}}
	eng := fixEngine(cfg)
	eng.UseNextOpen = true
	res := Run(bars, strat, cfg, eng)
	if len(res.Equity) == 0 {
		t.Fatal("no equity curve")
	}
	if math.Abs(res.Equity[0].Equity-10000) > 1e-9 {
		t.Fatalf("signal-bar equity must equal pre-entry equity 10000, got %.4f (position marked before fill)", res.Equity[0].Equity)
	}
}
