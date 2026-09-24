package finam

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"trade_info/internal/volatility"
)

const (
	baseURL          = "https://api.finam.ru"
	sourceAppID      = "traiding_app"
	maxSearchResults = 10
	maxSearchPages   = 12
	extraDaysBuffer  = 7
	maxErrorBody     = 500
	searchTimeout    = 7 * time.Second
)

// Client — клиент Finam Trade API (REST).
type Client struct {
	secret     string
	token      string
	httpClient *http.Client
}

func NewClient(secret string) *Client {
	return &Client{
		secret:     secret,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// decimalValue — цена/объём в формате {"value": "123.45"}.
type decimalValue struct {
	Value string `json:"value"`
}

func (d decimalValue) Float() float64 {
	v, err := strconv.ParseFloat(d.Value, 64)
	if err != nil {
		return 0
	}
	return v
}

// Connect получает JWT-токен по секрету (POST /v1/sessions).
func (c *Client) Connect(ctx context.Context) error {
	body, err := json.Marshal(map[string]string{
		"secret":        c.secret,
		"source_app_id": sourceAppID,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/sessions", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("авторизация Finam: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("авторизация Finam: статус %d: %s", resp.StatusCode, truncate(b))
	}

	var out struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return fmt.Errorf("авторизация Finam: %w", err)
	}

	c.token = out.Token
	if c.token == "" {
		return fmt.Errorf("авторизация Finam: пустой токен")
	}
	return nil
}

// GetCandles возвращает N последних дневных свечей по символу вида тикер@мик.
func (c *Client) GetCandles(ctx context.Context, symbol string, count int) ([]volatility.Candle, error) {
	if c.token == "" {
		return nil, fmt.Errorf("finam client not authenticated")
	}

	end := time.Now()
	start := end.AddDate(0, 0, -(count + extraDaysBuffer))
	u := fmt.Sprintf("%s/v1/instruments/%s/bars?timeframe=%s&interval.start_time=%s&interval.end_time=%s",
		baseURL, url.PathEscape(symbol), "TIME_FRAME_D",
		start.UTC().Format(time.RFC3339), end.UTC().Format(time.RFC3339))

	var out struct {
		Bars []struct {
			High  decimalValue `json:"high"`
			Low   decimalValue `json:"low"`
			Close decimalValue `json:"close"`
		} `json:"bars"`
	}
	if err := c.doJSON(ctx, http.MethodGet, u, nil, &out); err != nil {
		return nil, fmt.Errorf("finam bars %s: %w", symbol, err)
	}

	bars := out.Bars
	if len(bars) == 0 {
		return nil, fmt.Errorf("нет свечей для %s", symbol)
	}
	if len(bars) > count {
		bars = bars[len(bars)-count:]
	}

	candles := make([]volatility.Candle, 0, len(bars))
	for _, b := range bars {
		candles = append(candles, volatility.Candle{
			High:  b.High.Float(),
			Low:   b.Low.Float(),
			Close: b.Close.Float(),
		})
	}

	return candles, nil
}

// Search ищет инструменты по тикеру или названию (GET /v1/assets/all).
func normalizeFinamQuery(raw string) []string {
	query := strings.TrimSpace(raw)
	if query == "" {
		return nil
	}

	query = strings.ToUpper(query)
	query = strings.ReplaceAll(query, "@MOEIX", "@MOEX")
	query = strings.ReplaceAll(query, "@MOEX.", "@MOEX")

	if strings.Contains(query, "@") || strings.Contains(query, ":") || strings.Contains(query, "/") {
		return []string{query}
	}

	return []string{query + "@TQBR", query + "@MOEX", query}
}

func (c *Client) Search(ctx context.Context, query string) ([]Symbol, error) {
	if c.token == "" {
		return nil, fmt.Errorf("finam client not authenticated")
	}

	candidates := normalizeFinamQuery(query)
	if len(candidates) == 0 {
		return nil, nil
	}

	searchCtx, cancel := context.WithTimeout(ctx, searchTimeout)
	defer cancel()

	var results []Symbol
	seen := make(map[string]bool)
	cursor := ""

	for page := 0; page < maxSearchPages && len(results) < maxSearchResults; page++ {
		u := baseURL + "/v1/assets/all?only_active=true"
		if cursor != "" {
			u += "&cursor=" + url.QueryEscape(cursor)
		}

		var out struct {
			Assets     []asset `json:"assets"`
			NextCursor string  `json:"next_cursor"`
		}
		if err := c.doJSON(searchCtx, http.MethodGet, u, nil, &out); err != nil {
			if searchCtx.Err() != nil {
				return results, fmt.Errorf("finam search timeout: %w", searchCtx.Err())
			}
			return results, fmt.Errorf("finam search: %w", err)
		}

		for _, a := range out.Assets {
			if len(results) >= maxSearchResults {
				break
			}
			if a.IsArchived || a.Symbol == "" {
				continue
			}

			assetSymbol := strings.ToUpper(a.Symbol)
			assetTicker := strings.ToUpper(a.Ticker)
			matched := false
			for _, candidate := range candidates {
				candidate = strings.ToUpper(candidate)
				if candidate == "" {
					continue
				}
				if assetSymbol == candidate || assetTicker == candidate || strings.Contains(assetSymbol, candidate) || strings.Contains(assetTicker, candidate) {
					matched = true
					break
				}
			}
			if !matched {
				assetName := strings.ToUpper(a.Name)
				for _, candidate := range candidates {
					candidate = strings.ToUpper(candidate)
					if strings.Contains(assetName, candidate) || strings.Contains(assetName, strings.TrimSuffix(candidate, "@MOEX")) || strings.Contains(assetName, strings.TrimSuffix(candidate, "@TQBR")) {
						matched = true
						break
					}
				}
			}
			if !matched {
				continue
			}

			if seen[a.Symbol] {
				continue
			}
			seen[a.Symbol] = true
			results = append(results, Symbol{
				Ticker:   a.Symbol,
				Name:     a.Name,
				Category: categoryFromType(a.Type),
			})
			if len(results) >= maxSearchResults {
				break
			}
		}

		if out.NextCursor == "" || out.NextCursor == cursor {
			break
		}
		cursor = out.NextCursor
	}

	return results, nil
}

type asset struct {
	Symbol     string `json:"symbol"`
	Ticker     string `json:"ticker"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	IsArchived bool   `json:"is_archived"`
}

// doJSON выполняет запрос с JWT в заголовке Authorization и декодирует ответ.
func (c *Client) doJSON(ctx context.Context, method, u string, body []byte, out interface{}) error {
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("статус %d: %s", resp.StatusCode, truncate(b))
	}

	return json.NewDecoder(resp.Body).Decode(out)
}

func truncate(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > maxErrorBody {
		s = s[:maxErrorBody] + "..."
	}
	return s
}

// categoryFromType сопоставляет тип инструмента Finam с категорией таблицы.
func categoryFromType(t string) string {
	switch t = strings.ToUpper(t); {
	case strings.Contains(t, "BOND"):
		return "bonds"
	case strings.Contains(t, "STOCK"), strings.Contains(t, "SHARE"):
		return "stocks"
	default:
		return "other"
	}
}

// Symbol — инструмент Finam.
type Symbol struct {
	Ticker   string
	Name     string
	Category string
}
