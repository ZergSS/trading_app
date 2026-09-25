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
	Ticker   string
	Symbol   string
	Name     string
	Mic      string
	Type     string
	Category string // stocks, bonds, other
}

type decimalValue struct {
	Value string
}

func (d decimalValue) Float() float64 {
	if d.Value == "" {
		return 0
	}
	var val float64
	fmt.Sscanf(d.Value, "%f", &val)
	return val
}

func categoryFromType(in string) string {
	s := strings.ToLower(strings.TrimSpace(in))
	switch {
	case s == "stock" || s == "share":
		return "stocks"
	case s == "bond":
		return "bonds"
	default:
		return "other"
	}
}

func normalizeFinamQuery(in string) []string {
	q := strings.TrimSpace(in)
	if q == "" {
		return nil
	}

	upper := strings.ToUpper(q)

	if strings.Contains(upper, "@") {
		parts := strings.Split(upper, "@")
		ticker := parts[0]
		board := parts[1]
		if board == "MOEIX" {
			return []string{ticker + "@MISX"}
		}
		return []string{upper}
	}

	return []string{upper + "@MISX", upper}
}

type Client struct {
	httpClient *http.Client
	token      string
	baseURL    string
}

func NewClient(token string) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		token:   token,
		baseURL: FinamRESTBaseURL,
	}
}

func (c *Client) Connect(ctx ...context.Context) error {
	if c.token == "" {
		return fmt.Errorf("empty finam token")
	}
	return nil
}

func (c *Client) Close() error {
	return nil
}

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
		return nil, fmt.Errorf("bars request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("finam bars error (%d): %s", resp.StatusCode, string(body))
	}

	var data restBarsResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode bars: %w", err)
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

func (c *Client) Search(ctx context.Context, query string) ([]InstrumentInfo, error) {
	q := strings.ToUpper(strings.TrimSpace(query))
	if q == "" {
		return nil, nil
	}

	candidates := normalizeFinamQuery(q)
	if strings.HasSuffix(q, "@TQBR") {
		ticker := strings.TrimSuffix(q, "@TQBR")
		candidates = append(candidates, ticker+"@MISX")
	} else if !strings.Contains(q, "@") {
		candidates = append([]string{q + "@TQBR"}, candidates...)
	}

	for _, sym := range candidates {
		candles, err := c.GetDailyCandles(ctx, sym, 2)
		if err == nil && len(candles) > 0 {
			ticker := sym
			mic := "MISX"
			if idx := strings.Index(sym, "@"); idx != -1 {
				ticker = sym[:idx]
				mic = sym[idx+1:]
			}

			cat := "stocks"
			rawType := "STOCK"
			if strings.HasPrefix(ticker, "SU") || strings.Contains(ticker, "OFZ") {
				cat = "bonds"
				rawType = "BOND"
			}

			return []InstrumentInfo{
				{
					Ticker:   ticker,
					Symbol:   sym,
					Name:     ticker,
					Mic:      mic,
					Type:     rawType,
					Category: cat,
				},
			}, nil
		}
	}

	ticker := q
	mic := "TQBR"
	if idx := strings.Index(q, "@"); idx != -1 {
		ticker = q[:idx]
		mic = q[idx+1:]
	}

	cat := "stocks"
	rawType := "STOCK"
	if strings.HasPrefix(ticker, "SU") || strings.Contains(ticker, "OFZ") {
		cat = "bonds"
		rawType = "BOND"
	}

	return []InstrumentInfo{
		{
			Ticker:   ticker,
			Symbol:   q,
			Name:     ticker,
			Mic:      mic,
			Type:     rawType,
			Category: cat,
		},
	}, nil
}

func (c *Client) GetCandles(ctx context.Context, symbol string, count int) ([]Candle, error) {
	return c.GetDailyCandles(ctx, symbol, count)
}