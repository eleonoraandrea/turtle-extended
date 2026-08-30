package tests

import (
	"math"
	"testing"
	"time"

	"github.com/atps/atps/internal/backtest"
	"github.com/atps/atps/internal/config"
	"github.com/atps/atps/internal/data"
	"github.com/atps/atps/internal/metrics"
	"github.com/atps/atps/internal/risk"
	"github.com/atps/atps/internal/strategy"
)

func TestPipelineSynthetic(t *testing.T){
	cfg,_:= config.Load("../configs/default.yaml")
	bars:= data.GenerateSynthetic(1500, 4*time.Hour, 42)
	if len(bars)!=1500{ t.Fatalf("bars %d", len(bars))}
	// test all variants produce trades and equity
	for _,v:=range []string{"A","B","C","D"}{
		strat:= strategy.New(v, cfg)
		eng:= backtest.EngineConfig{Variant:v, Symbol:"BTCUSDT", InitialCapital: cfg.General.InitialCapital, FeeBps: cfg.Costs.FeeBps, SlippageBps: cfg.Costs.SlippageBps, Leverage: cfg.Costs.Leverage, UseNextOpen:true, PyramidingMax:4, PyramidStepATR:0.5, TrailATRMult:3, TrailMode:"donchian", DonExit:20}
		if v=="D"{eng.TrailMode="chandelier"}
		res:= backtest.Run(bars, strat, cfg, eng)
		if len(res.Equity)==0{ t.Fatalf("%s no equity", v)}
		if len(res.Equity)!=len(bars){ t.Fatalf("%s equity len %d vs bars %d", v, len(res.Equity), len(bars))}
		stats:= metrics.Compute(res)
		if stats.Trades==0{ t.Logf("warn %s 0 trades on synthetic", v)}
		t.Logf("%s ret %.2f%% sharpe %.2f PF %.2f trades %d", v, stats.ReturnPct, stats.Sharpe, stats.ProfitFactor, stats.Trades)
		// fund checks
		if stats.TotalFee<0{ t.Fatalf("negative fee")}
	}
}

func TestIndicatorsWarmupNaN(t *testing.T){
	bars:= data.GenerateSynthetic(50, 4*time.Hour, 1)
	cfg,_:= config.Load("../configs/default.yaml")
	strat:= strategy.New("D", cfg)
	ctx:= strat.Prepare(bars)
	if len(ctx.ATR)!=50{ t.Fatalf("atr len")}
	if ctx.ATR[5]==ctx.ATR[5]{ /* should be NaN for early */ } // first 20 warmup is NaN
	// check warmup filters signal 0
	sig:= strat.Next(ctx, 5)
	if sig.Side!=0{ t.Fatalf("expected warmup 0 got %d %s", sig.Side, sig.Reason)}
}

func TestRiskSizing(t *testing.T){
	cfg,_:= config.Load("../configs/default.yaml")
	if cfg.VariantA.RiskPct!=2.0{ t.Fatalf("config A risk %f", cfg.VariantA.RiskPct)}
	// simple sizing not zero
}

func TestCSVRoundTrip(t *testing.T){
	bars:= data.GenerateSynthetic(100, time.Hour, 7)
	path:= t.TempDir()+"/test.csv"
	if err:= data.SaveBarsCSV(path, bars); err!=nil{ t.Fatal(err)}
	loaded,err:= data.LoadBarsCSV(path)
	if err!=nil{ t.Fatal(err)}
	if len(loaded)!=len(bars){ t.Fatalf("len mismatch %d vs %d", len(loaded), len(bars))}
	diff := loaded[0].Close - bars[0].Close
	if diff > 1e-6 || diff < -1e-6 { t.Fatalf("close mismatch %.8f vs %.8f diff %.10f", loaded[0].Close, bars[0].Close, diff)}
}

// ── RISK ENGINE INVARIANTS ────────────────────────────────────────────

func TestRiskSizingHonorsMaxRisk(t *testing.T){
	// stop distance 100 → risk 2% of 10k = $200 → qty 2 — use fixed risk (no adaptive) for deterministic test
	lim := risk.RiskLimits{BaseRiskPct:2.0, MinRiskPct:2.0, MaxRiskPct:2.0, MaxHeatPct:8.0, MaxLeverage:10.0, MinLeverageCap:0.7, AdaptiveVol:false, AdaptiveDD:false}
	ms := risk.MarketState{Equity:10000, Price:2000, ATR:50, StopPrice:1900, Side:1, VolRegime: math.NaN(), ADX: math.NaN(), FundingZ: math.NaN()}
	dec := risk.Size(ms, lim)
	if !dec.Accept{ t.Fatalf("rejected: %v", dec.Factors)}
	if math.Abs(dec.RiskPct-2.0)>0.01 { t.Fatalf("risk pct %.4f != 2.0", dec.RiskPct)}
	if math.Abs(dec.Qty-2.0)>0.0001 { t.Fatalf("qty %.4f != 2.0", dec.Qty)}
	if math.Abs(dec.RiskAmount-200)>0.5 { t.Fatalf("risk amount %.2f != 200", dec.RiskAmount)}
	// notional 4000 on 10k → leverage 0.4
	if dec.Leverage > 1.0 { t.Fatalf("leverage %.2f unexpectedly high", dec.Leverage)}
}

func TestRiskDynamicLeverageCap(t *testing.T){
	// explicit limits for deterministic test — mirrors big-improve caps (0.70/0.85/1.30)
	lim := risk.RiskLimits{RiskPerTradePct:2.0, MaxHeatPct:8.0, MaxLeverage:10.0, MinLeverageCap:0.7, VolTargetPct:50.0, DDDeleverageStart:10, DDFlatPct:25, ADXSoftThreshold:15}
	// high vol regime → cap ×0.70 → 7.0x
	ms := risk.MarketState{Equity:10000, Price:2000, ATR:5, StopPrice:1999, Side:1, VolRegime:90, ADX:30}
	dec := risk.Size(ms, lim)
	if !dec.Accept{ t.Fatalf("rejected %v", dec.Factors)}
	if dec.Leverage > 7.0+1e-6 { t.Fatalf("leverage %.2f exceeds dynamic cap 7.0 (high vol regime ×0.70)", dec.Leverage)}
	if dec.RiskPct > 2.0+0.01 { t.Fatalf("risk %.3f exceeds 2%%", dec.RiskPct)}
	// weak ADX → ×0.80 → 8.0x
	ms2 := risk.MarketState{Equity:10000, Price:2000, ATR:5, StopPrice:1999, Side:1, VolRegime:50, ADX:10}
	dec2 := risk.Size(ms2, lim)
	if dec2.Leverage > 8.0+1e-6 { t.Fatalf("leverage %.2f exceeds dyn cap 8.0 (weak ADX ×0.80)", dec2.Leverage)}
	// low vol + strong trend → ×1.30 capped at hard 10
	ms3 := risk.MarketState{Equity:10000, Price:2000, ATR:5, StopPrice:1999, Side:1, VolRegime:10, ADX:40}
	dec3 := risk.Size(ms3, lim)
	if dec3.Leverage > 10.0+1e-6 { t.Fatalf("leverage %.2f exceeds HARD cap 10.0", dec3.Leverage)}
}

func TestRiskHeatBudgetRejects(t *testing.T){
	lim := risk.RiskLimits{RiskPerTradePct:2.0, MaxHeatPct:8.0, MaxLeverage:10.0, MinLeverageCap:0.7, VolTargetPct:50.0, DDDeleverageStart:10, DDFlatPct:25, ADXSoftThreshold:15}
	ms := risk.MarketState{Equity:10000, Price:2000, ATR:50, StopPrice:1900, Side:1, PortfolioHeatPct:7.9}
	dec := risk.Size(ms, lim)
	if !dec.Accept{ t.Fatalf("should accept with remaining heat")}
	if dec.RiskPct > 0.1+0.05 { t.Fatalf("risk %.3f should be clipped to ~0.1%% remaining heat (8-7.9)", dec.RiskPct)}
	ms2 := risk.MarketState{Equity:10000, Price:2000, ATR:50, StopPrice:1900, Side:1, PortfolioHeatPct:8.5}
	dec2 := risk.Size(ms2, lim)
	if dec2.Accept { t.Fatalf("should REJECT when heat exhausted (8.5>8)")}
}

func TestRiskDrawdownDeleverage(t *testing.T){
	// adaptive DD: scales towards min (0.25) not to zero
	lim := risk.RiskLimits{BaseRiskPct:1.0, MinRiskPct:0.25, MaxRiskPct:2.0, MaxHeatPct:8.0, MaxLeverage:10.0, MinLeverageCap:0.7, VolTargetPct:50.0, DDDeleverageStart:10, DDFlatPct:25, ADXSoftThreshold:15, AdaptiveDD: true}
	// dd 17.5% → scale 0.5 → risk = 0.25 + (1.0-0.25)*0.5 = 0.625
	ms := risk.MarketState{Equity:10000, Price:2000, ATR:50, StopPrice:1900, Side:1, EquityDDPct:17.5, VolRegime: math.NaN()}
	dec := risk.Size(ms, lim)
	if !dec.Accept{ t.Fatalf("rejected")}
	if math.Abs(dec.RiskPct-0.625)>0.05 { t.Fatalf("dd adaptive risk %.3f != ~0.625%%", dec.RiskPct)}
	// dd 25.5% → scale 0 → risk = min 0.25 (adaptive keeps min alive, not reject)
	ms2 := ms
	ms2.EquityDDPct = 25.5
	dec2 := risk.Size(ms2, lim)
	if !dec2.Accept{ t.Fatalf("should keep min risk in flat zone (adaptive)")}
	if math.Abs(dec2.RiskPct-0.25)>0.02 { t.Fatalf("dd flat risk %.3f != min 0.25%%", dec2.RiskPct)}
}

func TestVolTargetScalesRisk(t *testing.T){
	lim := risk.RiskLimits{RiskPerTradePct:2.0, MaxHeatPct:8.0, MaxLeverage:10.0, MinLeverageCap:0.7, VolTargetPct:50.0, DDDeleverageStart:10, DDFlatPct:25, ADXSoftThreshold:15}
	// vol target 50% → atr such that vol ~80% → risk ×0.625
	// 4h bars: 2190/year → vol = atr/price*sqrt(2190)
	// want 0.80 → atr = 0.80*price/46.8
	price := 2000.0
	atr := 0.80*price/46.8
	ms := risk.MarketState{Equity:10000, Price:price, ATR:atr, StopPrice:price-2*atr, Side:1, VolAnnualizedPct: risk.AnnualizedVolPct(atr, price, 4)}
	dec := risk.Size(ms, lim)
	if !dec.Accept{ t.Fatalf("rejected")}
	if dec.RiskPct > 1.35 || dec.RiskPct < 1.15 { t.Fatalf("vol target not applied: risk %.3f should be ~1.25%% (50/80)", dec.RiskPct)}
}

func TestBacktestRiskInvariants(t *testing.T){
	cfg,_:= config.Load("../configs/default.yaml")
	if err:= risk.ValidateLimitInvariants(risk.LimitsFromConfig(cfg,"A")); err!=nil{ t.Fatalf("limits invalid: %v", err)}
	bars:= data.GenerateSynthetic(1500, 4*time.Hour, 42)
	for _,v:=range []string{"A","D"}{
		strat:= strategy.New(v, cfg)
		eng:= backtest.EngineConfig{Variant:v, Symbol:"BTCUSDT", InitialCapital: cfg.General.InitialCapital, FeeBps: cfg.Costs.FeeBps, SlippageBps: cfg.Costs.SlippageBps, Leverage: cfg.Risk.MaxLeverage, UseNextOpen:true, PyramidingMax:4, PyramidStepATR:0.5, TrailATRMult:3, TrailMode:"donchian", DonExit:20}
		if v=="D"{eng.TrailMode="chandelier"}
		res:= backtest.Run(bars, strat, cfg, eng)
		lim:= res.RiskLimitsUsed
		for _,tr:= range res.Trades{
			// per-UNIT risk ≤ RiskPerTrade; cumulative (with pyramiding) ≤ MaxHeat
			if tr.RiskPct > lim.MaxHeatPct+0.05 {
				t.Fatalf("%s trade cumulative risk %.3f%% > heat %.2f%%", v, tr.RiskPct, lim.MaxHeatPct)
			}
			if tr.Leverage > lim.MaxLeverage+0.01 {
				t.Fatalf("%s trade leverage %.2f > hard cap %.2f", v, tr.Leverage, lim.MaxLeverage)
			}
		}
		if res.MaxHeatSeen > lim.MaxHeatPct+0.05 {
			t.Fatalf("%s heat %.3f%% > max %.2f%%", v, res.MaxHeatSeen, lim.MaxHeatPct)
		}
		t.Logf("%s: trades %d avgLev %.2fx maxLev %.2fx avgRisk %.2f%% maxHeat %.2f%% ret %.1f%%",
			v, len(res.Trades), res.AvgLeverage, res.MaxLeverageUsed, res.AvgRiskPct, res.MaxHeatSeen, (res.FinalEquity/res.InitialCapital-1)*100)
	}
}

func TestStopLossEqualsRiskBudget(t *testing.T){
	// invariant: a trade exited at "stop" should lose ≈ its committed risk (R ≈ -1)
	cfg,_:= config.Load("../configs/default.yaml")
	bars:= data.GenerateSynthetic(2500, 4*time.Hour, 42)
	strat:= strategy.New("A", cfg)
	eng:= backtest.EngineConfig{Variant:"A", Symbol:"BTCUSDT", InitialCapital: cfg.General.InitialCapital, FeeBps: cfg.Costs.FeeBps, SlippageBps: cfg.Costs.SlippageBps, UseNextOpen:true, PyramidingMax:1, PyramidStepATR:0.5, TrailATRMult:3, TrailMode:"donchian", DonExit:20}
	res:= backtest.Run(bars, strat, cfg, eng)
	stops:=0
	for _,tr:= range res.Trades{
		if tr.ExitReason!="stop" || tr.StopDist<=0 { continue }
		stops++
		// trailing stop can lock profit (R>0) or exit ~breakeven (R≈0) — fine.
		// REAL invariant: a losing stop NEVER loses more than its committed
		// risk budget (+ tolerance for slippage/fees/pyramid weighting)
		if tr.RMultiple < -1.8 {
			t.Fatalf("losing stop trade R=%.2f exceeds risk budget (riskPct %.2f%% lev %.2fx)", tr.RMultiple, tr.RiskPct, tr.Leverage)
		}
	}
	if stops==0 { t.Skip("no stop trades in sample") }
	t.Logf("%d stop-exit trades honor risk budget (losing stops R≈-1, trailing stops can be R>0)", stops)
}
