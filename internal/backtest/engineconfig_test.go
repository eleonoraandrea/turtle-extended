package backtest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/atps/atps/internal/config"
)

func TestEngineConfigFromDefaults(t *testing.T) {
	cfg, err := config.Load("../../configs/default.yaml")
	if err != nil {
		t.Fatal(err)
	}
	ec := EngineConfigFrom(cfg, "A", "BTCUSDT")
	if ec.TrailMode != "donchian" {
		t.Errorf("A TrailMode = %q, want donchian (legacy default)", ec.TrailMode)
	}
	if ec.EntryMode != "close" {
		t.Errorf("A EntryMode = %q, want close", ec.EntryMode)
	}
	if ec.DonExit != cfg.Trend.DonchianExit {
		t.Errorf("A DonExit = %d, want %d (trend.donchian_exit)", ec.DonExit, cfg.Trend.DonchianExit)
	}
	// pyramiding: enabled + max_additions 4 → 5 unità
	want := cfg.Pyramiding.MaxAdditions + 1
	if !cfg.Pyramiding.Enabled || ec.PyramidingMax != want {
		t.Errorf("A PyramidingMax = %d, want %d", ec.PyramidingMax, want)
	}
	if ec.TrailATRMult != cfg.Backtest.TrailATRMult {
		t.Errorf("A TrailATRMult = %v, want %v (backtest.trail_atr_mult)", ec.TrailATRMult, cfg.Backtest.TrailATRMult)
	}
	if ec.FeeBps != cfg.Costs.FeeBps || ec.SlippageBps != cfg.Costs.SlippageBps {
		t.Errorf("fee/slippage mismatch con costs")
	}
	if ec.UseNextOpen != cfg.Backtest.UseNextOpenFill {
		t.Errorf("UseNextOpen = %v, want %v", ec.UseNextOpen, cfg.Backtest.UseNextOpenFill)
	}
	// D: trail chandelier dal suo yaml
	ecD := EngineConfigFrom(cfg, "D", "BTCUSDT")
	if ecD.TrailMode != "chandelier" {
		t.Errorf("D TrailMode = %q, want chandelier (default.yaml variant_d.trail_mode)", ecD.TrailMode)
	}
}

func TestEngineConfigFromPerVariantOverrides(t *testing.T) {
	yml := `general:
  initial_capital: 10000.0
costs:
  fee_bps: 4.0
  slippage_bps: 2.0
backtest:
  trail_atr_mult: 2.5
  pyramid_step_atr: 0.5
  use_next_open_fill: true
trend:
  donchian_exit: 20
pyramiding:
  enabled: false
variant_a:
  name: A
  trail_mode: chandelier
  trail_atr_mult: 3.5
  don_exit: 10
  entry_mode: intrabar
  pyramiding_max_units: 3
  pyramid_step_atr: 0.75
`
	path := filepath.Join(t.TempDir(), "cfg.yaml")
	if err := os.WriteFile(path, []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	ec := EngineConfigFrom(cfg, "A", "X")
	if ec.TrailMode != "chandelier" || ec.TrailATRMult != 3.5 || ec.DonExit != 10 || ec.EntryMode != "intrabar" {
		t.Errorf("override per-variante non applicato: %+v", ec)
	}
	// pyramiding globale disabled MA override per-variante vince
	if ec.PyramidingMax != 3 {
		t.Errorf("PyramidingMax = %d, want 3 (override per-variante vince su enabled=false)", ec.PyramidingMax)
	}
	if ec.PyramidStepATR != 0.75 {
		t.Errorf("PyramidStepATR = %v, want 0.75", ec.PyramidStepATR)
	}
	// senza override per-variante: pyramiding disabled → 0
	ecB := EngineConfigFrom(cfg, "B", "X")
	if ecB.PyramidingMax != 0 {
		t.Errorf("B PyramidingMax = %d, want 0 (pyramiding.enabled false)", ecB.PyramidingMax)
	}
}

func TestEngineConfigFromLowercaseVariant(t *testing.T) {
	cfg, err := config.Load("../../configs/default.yaml")
	if err != nil {
		t.Fatal(err)
	}
	// variant minuscola deve risolvere gli override della variante (D → chandelier)
	ec := EngineConfigFrom(cfg, "d", "BTCUSDT")
	if ec.Variant != "D" || ec.TrailMode != "chandelier" {
		t.Errorf("lowercase variant: got %+v variant/trail, want Variant D + chandelier", ec)
	}
	ecA := EngineConfigFrom(cfg, "a", "BTCUSDT")
	if ecA.Variant != "A" || ecA.TrailMode != "donchian" {
		t.Errorf("lowercase variant a: got Variant %q TrailMode %q, want A + donchian", ecA.Variant, ecA.TrailMode)
	}
}
