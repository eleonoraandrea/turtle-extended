package main

import (
	"fmt"
	"time"

	"github.com/atps/atps/internal/config"
	"github.com/atps/atps/internal/data"
)

func main() {
	cfg, _ := config.Load("configs/default.yaml")
	seeds := map[string]int64{"BTCUSDT": 42, "ETHUSDT": 1337, "SOLUSDT": 9999}
	for _, sym := range cfg.General.Symbols {
		seed := seeds[sym]
		if seed == 0 {
			seed = 42
		}
		bars := data.GenerateSynthetic(3000, 4*time.Hour, seed)
		path := fmt.Sprintf("data/raw/%s_%s.csv", sym, cfg.General.Interval)
		data.SaveBarsCSV(path, bars)
		fmt.Printf("demo %s %d bars seed %d -> %s\n", sym, len(bars), seed, path)
	}
}
