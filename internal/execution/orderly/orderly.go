//go:build live
package orderly

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/atps/atps/internal/execution"
)

// Orderly implements execution.Adapter with ed25519 signing.
// Symbol: PERP_BTC_USDC etc

type Client struct {
	BaseURL string
	AccountID string
	OrderlyKey string // api key (ed25519 public)
	Secret    string // base64 private key or raw
	HTTP *http.Client
}

func New(base, accountID, orderlyKey, secret string) *Client {
	return &Client{BaseURL: strings.TrimRight(base,"/"), AccountID: accountID, OrderlyKey: orderlyKey, Secret: secret, HTTP: &http.Client{Timeout:15*time.Second}}
}
func (c *Client) Name() string { return "orderly" }

// sign: message = timestamp + method + path + body (if any). Signature = base64(ed25519.Sign(priv, message))
func (c *Client) sign(timestamp, method, path, body string) (string, error) {
	// Secret may be base64-encoded ed25519 private key (64 bytes) or seed (32 bytes) plus pub
	var priv ed25519.PrivateKey
	if b,err:=base64.StdEncoding.DecodeString(c.Secret); err==nil && len(b)==64 {
		priv = ed25519.PrivateKey(b)
	} else if b,err:=base64.StdEncoding.DecodeString(c.Secret); err==nil && len(b)==32 {
		priv = ed25519.NewKeyFromSeed(b)
	} else {
		// try raw string as base58? fallback: treat as base64 url
		return "", fmt.Errorf("invalid secret format for orderly, expected base64 32 or 64 bytes")
	}
	msg:= timestamp + method + path + body
	sig:= ed25519.Sign(priv, []byte(msg))
	return base64.StdEncoding.EncodeToString(sig), nil
}

func (c *Client) do(ctx context.Context, method, path string, body interface{}, out interface{}) error {
	var bodyStr string
	var reader io.Reader
	if body!=nil{
		b,_:=json.Marshal(body)
		bodyStr=string(b)
		reader=bytes.NewReader(b)
	}
	timestamp:= strconv.FormatInt(time.Now().UnixMilli(),10)
	sig,err:=c.sign(timestamp, method, path, bodyStr)
	if err!=nil{return err}
	req,err:=http.NewRequestWithContext(ctx, method, c.BaseURL+path, reader)
	if err!=nil{return err}
	req.Header.Set("Content-Type","application/json")
	req.Header.Set("orderly-account-id", c.AccountID)
	req.Header.Set("orderly-key", c.OrderlyKey)
	req.Header.Set("orderly-timestamp", timestamp)
	req.Header.Set("orderly-signature", sig)
	resp,err:=c.HTTP.Do(req)
	if err!=nil{return err}
	defer resp.Body.Close()
	b,_:=io.ReadAll(resp.Body)
	if resp.StatusCode!=200{
		return fmt.Errorf("orderly %s %s %d %s", method, path, resp.StatusCode, string(b))
	}
	if out!=nil{
		// orderly wraps in {success, data}
		var wrapper struct{Success bool `json:"success"`; Data json.RawMessage `json:"data"`; Message string `json:"message"`}
		if err:=json.Unmarshal(b,&wrapper);err!=nil{
			return fmt.Errorf("orderly decode %w body %s", err, string(b))
		}
		if !wrapper.Success{return fmt.Errorf("orderly !success %s", wrapper.Message)}
		if len(wrapper.Data)>0{
			if err:=json.Unmarshal(wrapper.Data,out);err!=nil{return err}
		}
	}
	return nil
}

func (c *Client) PlaceOrder(ctx context.Context, req execution.OrderRequest) (execution.OrderResponse, error) {
	// POST /v1/order
	payload:=map[string]interface{}{
		"symbol": req.Symbol,
		"order_type": req.Type,
		"side": strings.ToUpper(req.Side),
		"order_quantity": req.Qty,
		"order_price": req.Price,
	}
	if req.Type=="MARKET"{payload["order_price"]=nil}
	if req.ReduceOnly{payload["reduce_only"]=true}
	var resp struct{OrderID int64 `json:"order_id"`; ClientOrderID string `json:"client_order_id"`}
	if err:=c.do(ctx,"POST","/v1/order",payload,&resp);err!=nil{return execution.OrderResponse{},err}
	return execution.OrderResponse{OrderID: strconv.FormatInt(resp.OrderID,10), ClientOID: resp.ClientOrderID, Status:"SUBMITTED", Price:req.Price, Qty:req.Qty},nil
}
func (c *Client) CancelOrder(ctx context.Context, symbol, orderID string) error {
	id,_:=strconv.ParseInt(orderID,10,64)
	return c.do(ctx,"DELETE",fmt.Sprintf("/v1/order?symbol=%s&order_id=%d", symbol, id),nil,nil)
}
func (c *Client) GetPositions(ctx context.Context) ([]execution.Position, error){
	var data []struct{
		Symbol string `json:"symbol"`
		PositionQty float64 `json:"position_qty"`
		CostPosition float64 `json:"cost_position"`
		MarkPrice float64 `json:"mark_price"`
		UnrealizedPnL float64 `json:"unrealized_pnl"`
	}
	if err:=c.do(ctx,"GET","/v1/positions",nil,&data);err!=nil{return nil,err}
	var out []execution.Position
	for _,p:=range data{
		side:="FLAT"
		if p.PositionQty>0{side="LONG"} else if p.PositionQty<0{side="SHORT"}
		out=append(out, execution.Position{Symbol:p.Symbol, Side:side, Qty:p.PositionQty, EntryPrice:p.CostPosition, MarkPrice:p.MarkPrice, UnrealizedPnL:p.UnrealizedPnL, Timestamp: time.Now()})
	}
	return out,nil
}
func (c *Client) GetBalance(ctx context.Context) (execution.Balance, error){
	var data struct{ Holding []struct{Token string `json:"token"`; Holding float64 `json:"holding"`} `json:"holding"`}
	if err:=c.do(ctx,"GET","/v1/client/holding",nil,&data);err!=nil{return execution.Balance{},err}
	total:=0.0
	for _,h:=range data.Holding{total+=h.Holding}
	return execution.Balance{TotalEquity: total, Available: total},nil
}
func (c *Client) GetSymbols(ctx context.Context) ([]execution.SymbolInfo, error){
	var data struct{ Rows []struct{Symbol string `json:"symbol"`; QuoteTick float64 `json:"quote_tick"`; BaseTick float64 `json:"base_tick"`; MinNotional int `json:"min_notional"`} `json:"rows"`}
	if err:=c.do(ctx,"GET","/v1/public/info",nil,&data);err!=nil{return nil,err}
	var out []execution.SymbolInfo
	for _,r:=range data.Rows{ out=append(out, execution.SymbolInfo{Symbol:r.Symbol, QuoteTick: r.QuoteTick, BaseTick: r.BaseTick, MinNotional: float64(r.MinNotional)})}
	return out,nil
}
var _ execution.Adapter = (*Client)(nil)
