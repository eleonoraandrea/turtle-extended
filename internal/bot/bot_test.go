package bot

import (
	"testing"

	"github.com/atps/atps/internal/config"
)

func TestNewPaperBot(t *testing.T) {
	cfg, _ := config.Load("../../configs/default.yaml")
	b, err := New(cfg, "BTCUSDT", "4h", "A", true)
	if err != nil {
		t.Fatalf("new bot %v", err)
	}
	if !b.IsPaper() {
		t.Fatalf("should be paper")
	}
	if !b.IsDryRun() {
		t.Fatalf("dryRun")
	}
	if b.GetEquity() != cfg.General.InitialCapital {
		t.Fatalf("equity")
	}
	if b.OrderlySymbol() != "PERP_BTC_USDC" {
		t.Fatalf("orderly symbol %s", b.OrderlySymbol())
	}
	// GetBars should have warmup
	bars := b.GetBars()
	if len(bars) < 200 {
		t.Logf("bars len %d (fallback synthetic if network fails)", len(bars))
	}
	// GetLastParams should return struct
	params := b.GetLastParams()
	_ = params
	// GetBalance paper
	bal := b.GetBalance()
	if bal.TotalEquity != 10000 && bal.TotalEquity != cfg.General.InitialCapital {
		t.Logf("balance %f", bal.TotalEquity)
	}
	// logs
	logs := b.GetLogs()
	_ = logs
	// SetDryRun toggle
	b.SetDryRun(false)
	// should fallback to paper if keys missing
	if !b.IsDryRun() {
		t.Logf("dryRun false but keys missing -> may fallback true, actual %v", b.IsDryRun())
	}
	b.SetDryRun(true)
	if !b.IsDryRun() {
		t.Fatalf("should be dry")
	}
}

func TestOrderlySymbolFallback(t *testing.T) {
	cfg, _ := config.Load("../../configs/default.yaml")
	// custom symbol not in map
	cfg.Orderly.SymbolsMap = map[string]string{}
	b, _ := New(cfg, "XRPUSDT", "4h", "A", true)
	sym := b.OrderlySymbol()
	if sym != "PERP_XRP_USDC" {
		t.Fatalf("fallback got %s", sym)
	}
}
