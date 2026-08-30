//go:build !live
package orderly

import (
	"context"

	"github.com/atps/atps/internal/execution"
)

type Client struct{}

func New(_,_,_,_ string) *Client { return &Client{} }
func (c *Client) Name() string { return "orderly (stub — build with -tags live to enable)" }
func (c *Client) PlaceOrder(ctx context.Context, req execution.OrderRequest) (execution.OrderResponse, error) {
	return execution.OrderResponse{}, execution.ErrNotLiveCompiled
}
func (c *Client) CancelOrder(ctx context.Context, symbol, orderID string) error {
	return execution.ErrNotLiveCompiled
}
func (c *Client) GetPositions(ctx context.Context) ([]execution.Position, error) {
	return nil, execution.ErrNotLiveCompiled
}
func (c *Client) GetBalance(ctx context.Context) (execution.Balance, error) {
	return execution.Balance{}, execution.ErrNotLiveCompiled
}
func (c *Client) GetSymbols(ctx context.Context) ([]execution.SymbolInfo, error) {
	return nil, execution.ErrNotLiveCompiled
}
var _ execution.Adapter = (*Client)(nil)
