package finam

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

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
	Category string
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

func normalizeFinamQuery(in string) []string {
	q := strings.TrimSpace(in)
	if q == "" {
		return nil
	}

	upper := strings.ToUpper(q)
	if strings.Contains(upper, "@") {
		parts := strings.Split(upper, "@")
		return []string{parts[0]}
	}

	return []string{upper}
}

type Client struct {
	httpClient *http.Client
	token      string
}

func NewClient(token string) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 6 * time.Second,
		},
		token: token,
	}
}

func (c *Client) Connect(ctx ...context.Context) error {
	return nil
}

func (c *Client) Close() error {
	return nil
}

type moexCandlesResponse struct {
	History struct {
		Columns []string        `json:"columns"`
		Data    [][]interface{} `json:"data"`
	} `json:"history"`
}

type moexSecDescriptionResponse struct {
	Description struct {
		Columns []string        `json:"columns"`
		Data    [][]interface{} `json:"data"`
	} `json:"description"`
}

// getSecurityName получает полное наименование бумаги с MOEX
func (c *Client) getSecurityName(ctx context.Context, secID string) string {
	url := fmt.Sprintf("https://iss.moex.com/iss/securities/%s.json", strings.ToUpper(secID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return secID
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return secID
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return secID
	}

	var parsed moexSecDescriptionResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return secID
	}

	nameIdx, valIdx := -1, -1
	for i, col := range parsed.Description.Columns {
		if strings.ToUpper(col) == "NAME" {
			nameIdx = i
		}
		if strings.ToUpper(col) == "VALUE" {
			valIdx = i
		}
	}

	if nameIdx == -1 || valIdx == -1 {
		return secID
	}

	var shortName, secName string
	for _, row := range parsed.Description.Data {
		if len(row) <= valIdx {
			continue
		}
		paramName := fmt.Sprint(row[nameIdx])
		if paramName == "SECNAME" {
			secName = fmt.Sprint(row[valIdx])
		}
		if paramName == "SHORTNAME" {
			shortName = fmt.Sprint(row[valIdx])
		}
	}

	if secName != "" {
		return secName
	}
	if shortName != "" {
		return shortName
	}
	return secID
}

func (c *Client) fetchMoexCandles(ctx context.Context, secID string, isBond bool, count int) ([]Candle, error) {
	from := time.Now().UTC().AddDate(0, 0, -(count*4 + 14)).Format("2006-01-02")

	engine := "stock"
	market := "shares"
	if isBond {
		market = "bonds"
	}

	url := fmt.Sprintf("https://iss.moex.com/iss/history/engines/%s/markets/%s/securities/%s.json?from=%s",
		engine, market, strings.ToUpper(secID), from)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ошибка запроса MOEX: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("MOEX HTTP %d", resp.StatusCode)
	}

	var parsed moexCandlesResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}

	colIdx := make(map[string]int)
	for i, col := range parsed.History.Columns {
		colIdx[strings.ToUpper(col)] = i
	}

	dateIdx, hasDate := colIdx["TRADEDATE"]
	highIdx, hasHigh := colIdx["HIGH"]
	lowIdx, hasLow := colIdx["LOW"]
	closeIdx, hasClose := colIdx["CLOSE"]

	if !hasDate || !hasHigh || !hasLow || !hasClose {
		return nil, fmt.Errorf("неверный формат колонок от MOEX")
	}

	var candles []Candle
	for _, row := range parsed.History.Data {
		if len(row) <= closeIdx || row[highIdx] == nil || row[lowIdx] == nil || row[closeIdx] == nil {
			continue
		}

		t, _ := time.Parse("2006-01-02", fmt.Sprint(row[dateIdx]))
		high := parseFloatAny(row[highIdx])
		low := parseFloatAny(row[lowIdx])
		cls := parseFloatAny(row[closeIdx])

		if high == 0 && low == 0 && cls == 0 {
			continue
		}

		candles = append(candles, Candle{
			Date:  t,
			High:  high,
			Low:   low,
			Close: cls,
		})
	}

	if len(candles) == 0 {
		return nil, fmt.Errorf("нет данных по инструменту %s на MOEX", secID)
	}

	if len(candles) > count {
		candles = candles[len(candles)-count:]
	}

	return candles, nil
}

func parseFloatAny(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case string:
		f, _ := strconv.ParseFloat(val, 64)
		return f
	default:
		return 0
	}
}

func (c *Client) GetDailyCandles(ctx context.Context, symbol string, count int) ([]Candle, error) {
	ticker := strings.ToUpper(strings.TrimSpace(symbol))
	if idx := strings.Index(ticker, "@"); idx != -1 {
		ticker = ticker[:idx]
	}

	isBond := strings.HasPrefix(ticker, "SU") || strings.HasPrefix(ticker, "RU") || strings.Contains(ticker, "OFZ")

	candles, err := c.fetchMoexCandles(ctx, ticker, isBond, count)
	if err == nil && len(candles) > 0 {
		return candles, nil
	}

	candles, errAlt := c.fetchMoexCandles(ctx, ticker, !isBond, count)
	if errAlt == nil && len(candles) > 0 {
		return candles, nil
	}

	return nil, fmt.Errorf("свечи не найдены: %v", err)
}

func (c *Client) Search(ctx context.Context, query string) ([]InstrumentInfo, error) {
	q := strings.ToUpper(strings.TrimSpace(query))
	if q == "" {
		return nil, nil
	}

	ticker := q
	if idx := strings.Index(ticker, "@"); idx != -1 {
		ticker = ticker[:idx]
	}

	candles, err := c.GetDailyCandles(ctx, ticker, 2)
	if err == nil && len(candles) > 0 {
		cat := "stocks"
		rawType := "STOCK"
		if strings.HasPrefix(ticker, "SU") || strings.HasPrefix(ticker, "RU") || strings.Contains(ticker, "OFZ") {
			cat = "bonds"
			rawType = "BOND"
		}

		fullName := c.getSecurityName(ctx, ticker)

		return []InstrumentInfo{
			{
				Ticker:   ticker,
				Symbol:   ticker,
				Name:     fullName,
				Mic:      "MISX",
				Type:     rawType,
				Category: cat,
			},
		}, nil
	}

	return nil, fmt.Errorf("инструмент '%s' не найден на бирже", ticker)
}

func (c *Client) GetCandles(ctx context.Context, symbol string, count int) ([]Candle, error) {
	return c.GetDailyCandles(ctx, symbol, count)
}