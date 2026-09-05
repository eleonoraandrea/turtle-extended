package strategy

import "github.com/atps/atps/internal/config"

func New(variant string, cfg *config.Config) Strategy {
	switch variant {
	case "A", "a":
		return NewA(cfg)
	case "B", "b":
		return NewB(cfg)
	case "C", "c":
		return NewC(cfg)
	case "D", "d":
		return NewD(cfg)
	case "M", "m":
		return NewM(cfg)
	default:
		return NewD(cfg)
	}
}
