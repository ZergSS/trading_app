package finam

import (
	"context"
	"crypto/tls"
	"fmt"

	finam "github.com/FinamWeb/finam-trade-api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"finam-dashboard/internal/volatility"
)

type Client struct {
	conn *grpc.ClientConn
	api  finam.TradeApiClient
}

func NewClient(token string) *Client {
	return &Client{}
}

func (c *Client) Connect(ctx context.Context) error {
	creds := credentials.NewTLS(&tls.Config{})
	conn, err := grpc.DialContext(ctx, "grpc.finam.ru:443",
		grpc.WithTransportCredentials(creds),
		grpc.WithPerRPCCredentials(tokenCredentials{token: token}),
	)
	if err != nil {
		return err
	}
	c.conn = conn
	c.api = finam.NewTradeApiClient(conn)
	return nil
}

func (c *Client) Close() {
	if c.conn != nil {
		c.conn.Close()
	}
}

// GetCandles получает N дневных свечей.
func (c *Client) GetCandles(ctx context.Context, symbol string, count int) ([]volatility.Candle, error) {
	if c.api == nil {
		return nil, fmt.Errorf("finam client not connected")
	}

	resp, err := c.api.GetCandles(ctx, &finam.GetCandlesRequest{
		SecurityCode: symbol,
		TimeFrame:    finam.CandleInterval_CANDLE_INTERVAL_D,
		Count:        int32(count),
	})
	if err != nil {
		return nil, err
	}

	candles := make([]volatility.Candle, 0, len(resp.Candles))
	for _, c := range resp.Candles {
		candles = append(candles, volatility.Candle{
			High:  float64(c.High),
			Low:   float64(c.Low),
			Close: float64(c.Close),
		})
	}
	return candles, nil
}

// Search ищет инструменты по строке.
func (c *Client) Search(ctx context.Context, query string) ([]Symbol, error) {
	if c.api == nil {
		return nil, fmt.Errorf("finam client not connected")
	}

	resp, err := c.api.GetSymbols(ctx, &finam.GetSymbolsRequest{
		SearchString: query,
	})
	if err != nil {
		return nil, err
	}

	var symbols []Symbol
	for _, s := range resp.Symbols {
		symbols = append(symbols, Symbol{
			Ticker:   s.Symbol,
			Name:     s.Description,
			Category: mapFinamType(s.Type),
		})
	}
	return symbols, nil
}

type Symbol struct {
	Ticker   string
	Name     string
	Category string
}

func mapFinamType(t finam.SecurityType) string {
	switch t {
	case finam.SecurityType_STOCK:
		return "stocks"
	case finam.SecurityType_BOND:
		return "bonds"
	default:
		return "other"
	}
}

type tokenCredentials struct {
	token string
}

func (c tokenCredentials) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return map[string]string{"authorization": "Bearer " + c.token}, nil
}

func (c tokenCredentials) RequireTransportSecurity() bool { return true }
