package config

import (
	"os"
	"testing"
)

func TestLoadDefault(t *testing.T) {
	c, err := Load("../../configs/default.yaml")
	if err != nil {
		t.Fatalf("load %v", err)
	}
	if c.General.InitialCapital != 10000 {
		t.Fatalf("capital %f", c.General.InitialCapital)
	}
	if c.General.Interval != "4h" {
		t.Fatalf("interval")
	}
	if len(c.General.Symbols) != 3 {
		t.Fatalf("symbols")
	}
	if c.Risk.Base != 0.01 {
		t.Fatalf("risk base")
	}
	if c.Risk.Max != 0.02 {
		t.Fatalf("risk max")
	}
	if c.LeverageCfg.Max != 5 {
		t.Fatalf("leverage")
	}
	if !c.Volatility.AdaptiveRisk {
		t.Fatalf("vol adaptive")
	}
	if !c.Drawdown.AdaptiveRisk {
		t.Fatalf("dd adaptive")
	}
	if !c.Profit.Satellite.Enabled {
		t.Fatalf("satellite")
	}
}

func TestLoadMissingDefaults(t *testing.T) {
	path := t.TempDir() + "/empty.yaml"
	os.WriteFile(path, []byte("general:\n  interval: \"1h\"\n"), 0644)
	c, err := Load(path)
	if err != nil {
		t.Fatalf("load %v", err)
	}
	if c.General.InitialCapital != 10000 {
		t.Fatalf("default capital %f", c.General.InitialCapital)
	}
}

func TestLoadInvalidPath(t *testing.T) {
	_, err := Load("/nonexistent.yaml")
	if err == nil {
		t.Fatalf("should fail")
	}
}

func TestHelpers(t *testing.T) {
	c, _ := Load("../../configs/default.yaml")
	if c.RiskBasePct() != 1.0 {
		t.Fatalf("RiskBasePct %f", c.RiskBasePct())
	}
	if c.RiskMinPct() != 0.25 {
		t.Fatalf("RiskMinPct")
	}
	if c.RiskMaxPct() != 2.0 {
		t.Fatalf("RiskMaxPct")
	}
	if c.PortfolioMaxOpenRiskPct() != 3.0 {
		t.Fatalf("max open %f", c.PortfolioMaxOpenRiskPct())
	}
	if c.PortfolioMaxCorrelatedRiskPct() != 2.0 {
		t.Fatalf("corr")
	}
	if c.LeverageMax() != 5 {
		t.Fatalf("lev max")
	}
	if !c.IsVolatilityAdaptive() {
		t.Fatalf("vol adaptive helper")
	}
	if !c.IsDrawdownAdaptive() {
		t.Fatalf("dd adaptive")
	}
	if c.IsFundingFilter() != false {
		t.Fatalf("funding filter disabled per config")
	}
	if !c.IsOpenInterestFilter() {
		t.Fatalf("OI filter true")
	}
	if !c.IsRegimeFilter() {
		t.Fatalf("regime")
	}
	// DefaultPath
	p := DefaultPath()
	if p == "" {
		t.Fatalf("default path empty")
	}
}

func TestHelpersFallback(t *testing.T) {
	c := &Config{}
	c.Risk.MaxRiskPerTradePct = 2.0
	if c.RiskBasePct() != 1.0 { // legacy half of max
		t.Fatalf("fallback base %f", c.RiskBasePct())
	}
	c.Risk.Base = 0
	c.Risk.Max = 0
	if c.RiskMaxPct() != 2.0 {
		t.Fatalf("legacy max")
	}
}
