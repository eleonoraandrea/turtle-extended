package execution

import (
	"context"
	"time"
)

// Adapter is isolated from backtest. No backtest code imports this.
type Adapter interface {
	Name() string
	PlaceOrder(ctx context.Context, req OrderRequest) (OrderResponse, error)
	CancelOrder(ctx context.Context, symbol, orderID string) error
	GetPositions(ctx context.Context) ([]Position, error)
	GetBalance(ctx context.Context) (Balance, error)
	GetSymbols(ctx context.Context) ([]SymbolInfo, error)
}

type OrderRequest struct {
	Symbol    string  // e.g. PERP_BTC_USDC
	Side      string  // BUY/SELL
	Type      string  // MARKET/LIMIT
	Qty       float64
	Price     float64 // 0 for market
	ReduceOnly bool
	Tag       string
}

type OrderResponse struct {
	OrderID string
	ClientOID string
	Status  string
	Price   float64
	Qty     float64
}

type Position struct {
	Symbol string
	Side   string
	Qty    float64
	EntryPrice float64
	MarkPrice  float64
	UnrealizedPnL float64
	Leverage int
	Timestamp time.Time
}

type Balance struct {
	TotalEquity float64
	Available  float64
	UsedMargin float64
}

type SymbolInfo struct {
	Symbol string
	BaseTick float64
	QuoteTick float64
	MinNotional float64
}

var ErrNotLiveCompiled = errNotLive{}
type errNotLive struct{}
func (e errNotLive) Error() string { return "live execution not compiled: build with -tags live" }

// PaperAdapter for dry-run / paper trading
type PaperAdapter struct {
	Equity float64
	Positions map[string]Position
}

func NewPaper() *PaperAdapter { return &PaperAdapter{Equity: 10000, Positions: map[string]Position{}} }
func (p *PaperAdapter) Name() string { return "paper" }
func (p *PaperAdapter) PlaceOrder(ctx context.Context, req OrderRequest) (OrderResponse, error) {
	// simulate immediate fill
	return OrderResponse{OrderID: "paper-"+req.Symbol+"-"+time.Now().Format("20060102150405"), Status:"FILLED", Price:req.Price, Qty:req.Qty}, nil
}
func (p *PaperAdapter) CancelOrder(ctx context.Context, symbol, orderID string) error { return nil}
func (p *PaperAdapter) GetPositions(ctx context.Context) ([]Position, error){
	var out []Position
	for _,v:=range p.Positions{out=append(out, v)}
	return out,nil
}
func (p *PaperAdapter) GetBalance(ctx context.Context) (Balance, error){ return Balance{TotalEquity:p.Equity, Available:p.Equity},nil}
func (p *PaperAdapter) GetSymbols(ctx context.Context) ([]SymbolInfo, error){ return nil,nil}
