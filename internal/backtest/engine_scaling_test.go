package backtest

import (
	"strings"
	"testing"

	"github.com/atps/atps/internal/config"
	"github.com/atps/atps/internal/strategy"
)

// flatStrat — Next sempre flat (signals nil → zero Signal): nessuna entry,
// serve per testare il wiring di avvio (ceiling/warning) senza trade.
type flatStrat struct {
	scriptStrategy
}

func TestScalingWarningOnClippedRisk(t *testing.T) {
	cfg, err := config.Load("../../configs/default.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Risk.Base = 0.04
	cfg.Risk.Max = 0.04
	cfg.Risk.MaxRiskPerTradePct = 4.0
	cfg.Risk.KellyCapPct = 2.0
	cfg.VariantA.RiskPct = 4.0 // variant risk_pct vince su risk.max nel cascade di LimitsFromConfig
	cfg.Profit.Satellite.Enabled = false
	bars := flatBars(30, 100, 0.4)
	bars = append(bars, flatBars(10, 100, 0.4)...)
	res := Run(bars, &flatStrat{scriptStrategy{cfg: cfg}}, cfg, EngineConfig{
		Variant: "A", Symbol: "TEST", InitialCapital: 10000,
		FeeBps: 4, SlippageBps: 2, UseNextOpen: true,
		PyramidingMax: 0, TrailMode: "donchian", DonExit: 20, EntryMode: "close",
	})
	if res.ScalingCeilingPct != 2.0 || res.ScalingBinding != "kelly_cap" {
		t.Errorf("ceiling %.2f (%s), want 2.0 (kelly_cap)", res.ScalingCeilingPct, res.ScalingBinding)
	}
	found := false
	for _, w := range res.Warnings {
		if strings.Contains(w, "kelly_cap") {
			found = true
		}
	}
	if !found {
		t.Errorf("Warnings %+v: atteso warning kelly_cap", res.Warnings)
	}
}

func TestNotionalCapHitsCounter(t *testing.T) {
	cfg, err := config.Load("../../configs/default.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Profit.Satellite.Enabled = false
	cfg.Risk.MaxNotional = 1000 // forza il cap su ogni entry
	bars := flatBars(30, 100, 0.4)
	bars = append(bars, flatBars(10, 100, 0.4)...)
	res := Run(bars, &flatStrat{scriptStrategy{cfg: cfg, signals: map[int]strategy.Signal{2: {Side: 1, Strength: 1, StopPrice: 98, Reason: "script"}}}}, cfg, EngineConfig{
		Variant: "A", Symbol: "TEST", InitialCapital: 10000,
		FeeBps: 4, SlippageBps: 2, UseNextOpen: true,
		PyramidingMax: 0, TrailMode: "donchian", DonExit: 20, EntryMode: "close",
	})
	if res.NotionalCapHits < 1 {
		t.Errorf("NotionalCapHits = %d, want ≥ 1 con MaxNotional 1000", res.NotionalCapHits)
	}
}
