package config

import (
	"os"
	"path/filepath"
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
	if c.IsRegimeFilter() {
		t.Fatalf("regime: btc_filter default OFF dal 2026-09-05 (validazione: ON degrada test — vedi reports/V5_VALIDATION.md)")
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

func TestVariantEngineCfgInlineAndReEntry(t *testing.T) {
	yml := `general:
  initial_capital: 10000.0
backtest:
  trail_atr_mult: 2.5
  pyramid_step_atr: 0.5
trend:
  donchian_entry: 55
  donchian_exit: 20
pyramiding:
  enabled: true
  max_additions: 4
  risk_neutral: true
variant_a:
  name: "A test"
  atr_stop_mult: 1.6
  trail_mode: chandelier
  trail_atr_mult: 3.5
  don_exit: 10
  entry_mode: intrabar
  pyramiding_max_units: 3
  reentry:
    enabled: true
    lookback: 12
    within_bars: 25
variant_b:
  name: "B test"
variant_d:
  name: "D test"
  trail_mode: chandelier
`
	path := filepath.Join(t.TempDir(), "cfg.yaml")
	if err := os.WriteFile(path, []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	// inline engine su variant_a
	if c.VariantA.Engine.TrailMode != "chandelier" {
		t.Errorf("VariantA.Engine.TrailMode = %q, want chandelier", c.VariantA.Engine.TrailMode)
	}
	if c.VariantA.Engine.TrailATRMult != 3.5 {
		t.Errorf("VariantA.Engine.TrailATRMult = %v, want 3.5", c.VariantA.Engine.TrailATRMult)
	}
	if c.VariantA.Engine.DonExit != 10 {
		t.Errorf("VariantA.Engine.DonExit = %v, want 10", c.VariantA.Engine.DonExit)
	}
	if c.VariantA.Engine.EntryMode != "intrabar" {
		t.Errorf("VariantA.Engine.EntryMode = %q, want intrabar", c.VariantA.Engine.EntryMode)
	}
	if c.VariantA.Engine.PyramidingUnits != 3 {
		t.Errorf("VariantA.Engine.PyramidingUnits = %v, want 3", c.VariantA.Engine.PyramidingUnits)
	}
	// reentry
	if !c.VariantA.ReEntry.Enabled || c.VariantA.ReEntry.Lookback != 12 || c.VariantA.ReEntry.WithinBars != 25 {
		t.Errorf("VariantA.ReEntry = %+v, want enabled/12/25", c.VariantA.ReEntry)
	}
	// variant_d legacy: trail_mode ora vive in Engine
	if c.VariantD.Engine.TrailMode != "chandelier" {
		t.Errorf("VariantD.Engine.TrailMode = %q, want chandelier", c.VariantD.Engine.TrailMode)
	}
	// default reentry su B (disabled)
	if c.VariantB.ReEntry.Enabled {
		t.Errorf("VariantB.ReEntry.Enabled default deve essere false")
	}
}

func TestReEntryDefaultsNormalization(t *testing.T) {
	yml := `variant_a:
  name: "A"
  reentry:
    enabled: true
`
	path := filepath.Join(t.TempDir(), "cfg.yaml")
	if err := os.WriteFile(path, []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if c.VariantA.ReEntry.Lookback != 10 || c.VariantA.ReEntry.WithinBars != 20 {
		t.Errorf("ReEntry defaults = %+v, want lookback 10 within 20", c.VariantA.ReEntry)
	}
}

func TestPyramidingModeDefault(t *testing.T) {
	yml := "pyramiding:\n  enabled: true\n  max_additions: 4\n  risk_neutral: true\n"
	path := filepath.Join(t.TempDir(), "cfg.yaml")
	if err := os.WriteFile(path, []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Pyramiding.Mode != "" {
		t.Errorf("Mode default deve essere \"\" (merged), avuto %q", c.Pyramiding.Mode)
	}
	yml2 := "pyramiding:\n  mode: separate\n  enabled: true\n  max_additions: 6\n"
	path2 := filepath.Join(t.TempDir(), "cfg2.yaml")
	if err := os.WriteFile(path2, []byte(yml2), 0o644); err != nil {
		t.Fatal(err)
	}
	c2, err := Load(path2)
	if err != nil {
		t.Fatal(err)
	}
	if c2.Pyramiding.Mode != "separate" {
		t.Errorf("Mode = %q, want separate", c2.Pyramiding.Mode)
	}
}
