package finam

import (
	"context"
	"crypto/tls"
	"fmt"
	"strings"
	"time"

	assetsPb "github.com/FinamWeb/finam-trade-api/go/grpc/tradeapi/v1/assets"
	mdPb "github.com/FinamWeb/finam-trade-api/go/grpc/tradeapi/v1/marketdata"
	"google.golang.org/genproto/googleapis/type/decimal"
	"google.golang.org/genproto/googleapis/type/interval"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const FinamGRPCTarget = "trade-api.finam.ru:443"

type Candle struct {
	Date  time.Time
	Open  float64
	High  float64
	Low   float64
	Close float64
}

type InstrumentInfo struct {
	Ticker string
	Symbol string
	Name   string
	Mic    string
	Type   string // STOCK, BOND, OTHER
}

type Client struct {
	conn      *grpc.ClientConn
	token     string
	marketSvc mdPb.MarketDataServiceClient
	assetsSvc assetsPb.AssetsServiceClient
}

func NewClient(token string) (*Client, error) {
	if token == "" {
		return nil, fmt.Errorf("finam token cannot be empty")
	}

	creds := credentials.NewTLS(&tls.Config{})
	conn, err := grpc.Dial(FinamGRPCTarget, grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, fmt.Errorf("failed to dial Finam gRPC: %w", err)
	}

	return &Client{
		conn:      conn,
		token:     token,
		marketSvc: mdPb.NewMarketDataServiceClient(conn),
		assetsSvc: assetsPb.NewAssetsServiceClient(conn),
	}, nil
}

func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func (c *Client) withAuthContext(ctx context.Context) context.Context {
	return metadata.NewOutgoingContext(ctx, metadata.Pairs("x-api-key", c.token))
}

func decimalToFloat(d *decimal.Decimal) float64 {
	if d == nil {
		return 0
	}
	var val float64
	fmt.Sscanf(d.GetValue(), "%f", &val)
	return val
}

// GetDailyCandles запрашивает дневные свечи
// symbol может быть тикером (например, "SBER") или полным символом ("SBER@MISX")
func (c *Client) GetDailyCandles(ctx context.Context, symbol string, count int) ([]Candle, error) {
	authCtx, cancel := context.WithTimeout(c.withAuthContext(ctx), 10*time.Second)
	defer cancel()

	// Запрашиваем с запасом на выходные дни
	startTime := time.Now().AddDate(0, 0, -(count * 3))
	endTime := time.Now()

	req := &mdPb.BarsRequest{
		Symbol:    symbol,
		Timeframe: mdPb.TimeFrame_TIME_FRAME_D,
		Interval: &interval.Interval{
			StartTime: timestamppb.New(startTime),
			EndTime:   timestamppb.New(endTime),
		},
	}

	resp, err := c.marketSvc.Bars(authCtx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get bars for %s: %w", symbol, err)
	}

	var candles []Candle
	for _, bar := range resp.GetBars() {
		candles = append(candles, Candle{
			Date:  bar.GetTimestamp().AsTime(),
			Open:  decimalToFloat(bar.GetOpen()),
			High:  decimalToFloat(bar.GetHigh()),
			Low:   decimalToFloat(bar.GetLow()),
			Close: decimalToFloat(bar.GetClose()),
		})
	}

	if len(candles) > count {
		candles = candles[len(candles)-count:]
	}

	return candles, nil
}

// SearchInstruments выполняет поиск активов через AssetsService
func (c *Client) SearchInstruments(ctx context.Context, query string) ([]InstrumentInfo, error) {
	authCtx, cancel := context.WithTimeout(c.withAuthContext(ctx), 10*time.Second)
	defer cancel()

	req := &assetsPb.AssetsRequest{}

	resp, err := c.assetsSvc.Assets(authCtx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get assets: %w", err)
	}

	upperQuery := strings.ToUpper(strings.TrimSpace(query))
	var results []InstrumentInfo

	for _, asset := range resp.GetAssets() {
		ticker := asset.GetTicker()
		name := asset.GetName()

		// Фильтрация по тикеру или названию компании
		if upperQuery != "" && !strings.Contains(strings.ToUpper(ticker), upperQuery) && !strings.Contains(strings.ToUpper(name), upperQuery) {
			continue
		}

		results = append(results, InstrumentInfo{
			Ticker: ticker,
			Symbol: asset.GetSymbol(),
			Name:   name,
			Mic:    asset.GetMic(),
			Type:   classifyAsset(asset.GetType()),
		})

		if len(results) >= 20 {
			break
		}
	}

	return results, nil
}

func classifyAsset(rawType string) string {
	raw := strings.ToUpper(rawType)
	switch {
	case strings.Contains(raw, "STOCK") || strings.Contains(raw, "SHARE") || raw == "EQ":
		return "STOCK"
	case strings.Contains(raw, "BOND"):
		return "BOND"
	default:
		return "OTHER"
	}
}