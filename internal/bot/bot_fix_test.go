package bot

import (
	"os"
	"testing"

	"github.com/atps/atps/internal/config"
)

// Safety: a bot constructed in PAPER mode must never switch to a live adapter
// via SetDryRun(false) — that would bypass the CLI double opt-in gates
// (--live --i-understand-live). Only a bot legitimately constructed in live
// mode may toggle.
func TestSetDryRunCannotEscalateToLive(t *testing.T) {
	cfg, err := config.Load("../../configs/default.yaml")
	if err != nil {
		t.Fatalf("cfg: %v", err)
	}
	t.Setenv("ORDERLY_ACCOUNT_ID", "0xfake")
	t.Setenv("ORDERLY_KEY", "ed25519:fake")
	t.Setenv("ORDERLY_SECRET", "fake")

	b, err := New(cfg, "BTCUSDT", "4h", "A", true)
	if err != nil {
		t.Fatalf("new bot: %v", err)
	}
	if !b.IsDryRun() {
		t.Fatal("must start in dry-run")
	}

	b.SetDryRun(false) // e.g. TUI 'd' toggle — must NOT escalate to live
	if !b.IsDryRun() {
		t.Fatal("SetDryRun(false) on a paper-only bot must stay PAPER (live requires CLI gates)")
	}
	_ = os.Unsetenv("ORDERLY_ACCOUNT_ID")
}
