package finam

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const FinamRESTBaseURL = "https://trade-api.finam.ru"

type Candle struct {
	Date  time.Time
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
	httpClient *http.Client
	token      string
	baseURL    string
}

func NewClient(token string) (*Client, error) {
	if token == "" {
		return nil, fmt.Errorf("finam token cannot be empty")
	}

	return &Client{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		token:   token,
		baseURL: FinamRESTBaseURL,
	}, nil
}

// Connect оставлен для совместимости с app.go
func (c *Client) Connect() error {
	return nil
}

func (c *Client) Close() error {
	return nil
}

// --- Получение дневных свечей ---

type restBarsResponse struct {
	Bars []struct {
		Timestamp string `json:"timestamp"`
		Open      string `json:"open"`
		High      string `json:"high"`
		Low       string `json:"low"`
		Close     string `json:"close"`
	} `json:"bars"`
}

func (c *Client) GetDailyCandles(ctx context.Context, symbol string, count int) ([]Candle, error) {
	// Если передали чистый тикер без биржи, по умолчанию берем Московскую биржу
	if !strings.Contains(symbol, "@") {
		symbol = symbol + "@MISX"
	}

	from := time.Now().AddDate(0, 0, -(count * 4)).Format(time.RFC3339)
	to := time.Now().Format(time.RFC3339)

	endpoint := fmt.Sprintf("%s/api/v1/marketdata/bars", c.baseURL)
	reqURL, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}

	q := reqURL.Query()
	q.Set("symbol", symbol)
	q.Set("timeframe", "TIME_FRAME_D")
	q.Set("interval.startTime", from)
	q.Set("interval.endTime", to)
	reqURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("finam api error (code %d): %s", resp.StatusCode, string(body))
	}

	var data restBarsResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var candles []Candle
	for _, b := range data.Bars {
		t, _ := time.Parse(time.RFC3339, b.Timestamp)
		high, _ := strconv.ParseFloat(b.High, 64)
		low, _ := strconv.ParseFloat(b.Low, 64)
		cls, _ := strconv.ParseFloat(b.Close, 64)

		candles = append(candles, Candle{
			Date:  t,
			High:  high,
			Low:   low,
			Close: cls,
		})
	}

	if len(candles) > count {
		candles = candles[len(candles)-count:]
	}

	return candles, nil
}

// --- Поиск инструментов ---

type restAssetsResponse struct {
	Assets []struct {
		Symbol string `json:"symbol"`
		Ticker string `json:"ticker"`
		Name   string `json:"name"`
		Type   string `json:"type"`
		Mic    string `json:"mic"`
	} `json:"assets"`
}

func (c *Client) SearchInstruments(ctx context.Context, query string) ([]InstrumentInfo, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}

	endpoint := fmt.Sprintf("%s/api/v1/assets", c.baseURL)
	reqURL, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}

	// Передаем фильтр в query, чтобы сервер сам отфильтровал список
	q := reqURL.Query()
	q.Set("ticker", strings.ToUpper(query))
	reqURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("finam api error (code %d): %s", resp.StatusCode, string(body))
	}

	var data restAssetsResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode assets: %w", err)
	}

	var results []InstrumentInfo
	upperQuery := strings.ToUpper(query)

	for _, a := range data.Assets {
		if !strings.Contains(strings.ToUpper(a.Ticker), upperQuery) && !strings.Contains(strings.ToUpper(a.Name), upperQuery) {
			continue
		}

		results = append(results, InstrumentInfo{
			Ticker: a.Ticker,
			Symbol: a.Symbol,
			Name:   a.Name,
			Mic:    a.Mic,
			Type:   classifyAsset(a.Type),
		})

		if len(results) >= 15 {
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

// Алиасы для полной совместимости с app.go
func (c *Client) GetCandles(ctx context.Context, symbol string, count int) ([]Candle, error) {
	return c.GetDailyCandles(ctx, symbol, count)
}

func (c *Client) Search(ctx context.Context, query string) ([]InstrumentInfo, error) {
	return c.SearchInstruments(ctx, query)
}