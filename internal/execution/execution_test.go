package execution

import (
	"context"
	"testing"
)

func TestPaperAdapter(t *testing.T) {
	p := NewPaper()
	if p.Name() != "paper" {
		t.Fatalf("name")
	}
	bal, err := p.GetBalance(context.Background())
	if err != nil {
		t.Fatalf("balance %v", err)
	}
	if bal.TotalEquity != 10000 {
		t.Fatalf("equity %f", bal.TotalEquity)
	}
	pos, err := p.GetPositions(context.Background())
	if err != nil {
		t.Fatalf("pos %v", err)
	}
	if len(pos) != 0 {
		t.Fatalf("pos should be empty")
	}
	resp, err := p.PlaceOrder(context.Background(), OrderRequest{Symbol: "PERP_BTC_USDC", Side: "BUY", Type: "MARKET", Qty: 0.1})
	if err != nil {
		t.Fatalf("place %v", err)
	}
	if resp.OrderID == "" {
		t.Fatalf("orderID empty")
	}
	if err := p.CancelOrder(context.Background(), "PERP_BTC_USDC", resp.OrderID); err != nil {
		t.Fatalf("cancel %v", err)
	}
	sym, err := p.GetSymbols(context.Background())
	if err != nil {
		t.Fatalf("symbols %v", err)
	}
	_ = sym
	if ErrNotLiveCompiled.Error() == "" {
		t.Fatalf("err string")
	}
}
