package bybit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"trade_info/internal/volatility"
)

const baseURL = "https://api.bybit.com"

type Client struct {
	httpClient *http.Client
}

func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

type klineResponse struct {
	Result struct {
		List [][]string `json:"list"`
	} `json:"result"`
	RetCode int    `json:"retCode"`
	RetMsg  string `json:"retMsg"`
}

// GetCandles получает N дневных свечей для линейного фьючерса (например, BTCUSDT).
func (c *Client) GetCandles(ctx context.Context, symbol string, count int) ([]volatility.Candle, error) {
	url := fmt.Sprintf("%s/v5/market/kline?category=linear&symbol=%s&interval=D&limit=%d",
		baseURL, strings.ToUpper(symbol), count)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("bybit API error: status %d: %s", resp.StatusCode, string(body))
	}

	var data klineResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	if data.RetCode != 0 {
		return nil, fmt.Errorf("bybit API error: code %d: %s", data.RetCode, data.RetMsg)
	}

	if len(data.Result.List) == 0 {
		return nil, fmt.Errorf("no klines for %s", symbol)
	}

	candles := make([]volatility.Candle, 0, len(data.Result.List))
	for _, kline := range data.Result.List {
		// Формат: startTime, open, high, low, close, volume, turnover
		if len(kline) < 5 {
			continue
		}
		high, _ := strconv.ParseFloat(kline[2], 64)
		low, _ := strconv.ParseFloat(kline[3], 64)
		closePrice, _ := strconv.ParseFloat(kline[4], 64)
		candles = append(candles, volatility.Candle{High: high, Low: low, Close: closePrice})
	}

	return candles, nil
}
