package main

// Thin wrapper legacy — prefer `atps download` CLI.
// Kept for compatibility with original Python atps/scripts/download_binance.py
import (
	"fmt"
	"os"

	"github.com/atps/atps/internal/config"
	"github.com/atps/atps/internal/data"
	"time"
)

func main() {
	cfg, _ := config.Load("configs/default.yaml")
	client := data.NewBinanceClient(cfg.Data.BinanceBase)
	for _, sym := range cfg.General.Symbols {
		bars, err := client.FetchKlines(sym, cfg.General.Interval, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), time.Now().UTC())
		if err != nil {
			fmt.Fprintf(os.Stderr, "err %s %v\n", sym, err)
			continue
		}
		path := fmt.Sprintf("data/raw/%s_%s.csv", sym, cfg.General.Interval)
		data.SaveBarsCSV(path, bars)
		fmt.Printf("saved %s %d\n", path, len(bars))
	}
}
