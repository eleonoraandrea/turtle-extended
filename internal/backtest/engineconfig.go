package backtest

import (
	"strings"

	"github.com/atps/atps/internal/config"
)

// EngineConfigFrom — UNICA fonte di verità per EngineConfig.
// Usata da CLI backtest, bot live e optimizer: ciò che si ottimizza è ciò che gira.
// Risoluzione: override per-variante (variant_x.engine, yaml inline) > sezione
// globale backtest:/trend:/pyramiding: > default hardcoded legacy.
func EngineConfigFrom(cfg *config.Config, variant, symbol string) EngineConfig {
	variant = strings.ToUpper(variant)
	var e config.EngineCfg
	switch variant {
	case "A":
		e = cfg.VariantA.Engine
	case "B":
		e = cfg.VariantB.Engine
	case "C":
		e = cfg.VariantC.Engine
	case "D":
		e = cfg.VariantD.Engine
	}
	trailMode := e.TrailMode
	if trailMode == "" {
		trailMode = "donchian"
	}
	trailMult := e.TrailATRMult
	if trailMult <= 0 {
		trailMult = cfg.Backtest.TrailATRMult
	}
	donExit := e.DonExit
	if donExit <= 0 {
		donExit = cfg.Trend.DonchianExit
	}
	if donExit <= 0 {
		donExit = 20
	}
	entryMode := e.EntryMode
	if entryMode == "" {
		entryMode = "close"
	}
	// pyramiding: legacy identical logic (backtest.pyramiding_max_units base,
	// pyramiding.enabled/max_additions vince, disabled → 0), poi override per-variante
	pyrMax := cfg.Backtest.PyramidingMaxUnits
	if cfg.Pyramiding.Enabled {
		if cfg.Pyramiding.MaxAdditions > 0 {
			pyrMax = cfg.Pyramiding.MaxAdditions + 1
		}
	} else {
		pyrMax = 0
	}
	if e.PyramidingUnits > 0 {
		pyrMax = e.PyramidingUnits
	}
	step := e.PyramidStepATR
	if step <= 0 {
		step = cfg.Backtest.PyramidStepATR
	}
	return EngineConfig{
		Variant:        variant,
		Symbol:         symbol,
		InitialCapital: cfg.General.InitialCapital,
		FeeBps:         cfg.Costs.FeeBps,
		SlippageBps:    cfg.Costs.SlippageBps,
		Leverage:       cfg.Costs.Leverage,
		UseNextOpen:    cfg.Backtest.UseNextOpenFill,
		PyramidingMax:  pyrMax,
		PyramidStepATR: step,
		TrailATRMult:   trailMult,
		TrailMode:      trailMode,
		DonExit:        donExit,
		EntryMode:      entryMode,
	}
}
