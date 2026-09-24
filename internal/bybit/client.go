package bybit

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"finam-dashboard/internal/volatility"
)

type Client struct {
	api *bybit.Client
}

func NewClient() *Client {
	return &Client{api: bybit.NewClient()}
}

// GetCandles получает N дневных свечей для линейного фьючерса (например, BTCUSDT).
func (c *Client) GetCandles(ctx context.Context, symbol string, count int) ([]volatility.Candle, error) {
	resp, err := c.api.V5().Market().GetKline(&bybit.V5GetKlineParam{
		Category: "linear",
		Symbol:   strings.ToUpper(symbol),
		Interval: "D",
		Limit:    count,
	})
	if err != nil {
		return nil, err
	}

	if resp.Result == nil || len(resp.Result.List) == 0 {
		return nil, fmt.Errorf("no klines for %s", symbol)
	}

	candles := make([]volatility.Candle, 0, len(resp.Result.List))
	for _, raw := range resp.Result.List {
		arr, ok := raw.([]interface{})
		if !ok || len(arr) < 5 {
			continue
		}
		high, _ := strconv.ParseFloat(fmt.Sprint(arr[2]), 64)
		low, _ := strconv.ParseFloat(fmt.Sprint(arr[3]), 64)
		close, _ := strconv.ParseFloat(fmt.Sprint(arr[4]), 64)
		candles = append(candles, volatility.Candle{High: high, Low: low, Close: close})
	}

	return candles, nil
}
