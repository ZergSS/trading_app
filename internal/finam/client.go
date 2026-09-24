package finam

import (
	"context"
	"fmt"

	"trade_info/internal/volatility"
)

// Client — заглушка для Finam API.
type Client struct {
	token string
}

func NewClient(token string) *Client {
	return &Client{token: token}
}

func (c *Client) Connect(ctx context.Context) error {
	// TODO: реализовать подключение к Finam gRPC API
	return nil
}

func (c *Client) Close() {
	// TODO: закрыть соединение
}

// GetCandles возвращает N дневных свечей.
func (c *Client) GetCandles(ctx context.Context, symbol string, count int) ([]volatility.Candle, error) {
	// TODO: реализовать получение свечей через gRPC
	return nil, fmt.Errorf("finam client not implemented")
}

// Search ищет инструменты по строке.
func (c *Client) Search(ctx context.Context, query string) ([]Symbol, error) {
	// TODO: реализовать поиск инструментов через gRPC
	return nil, fmt.Errorf("finam client not implemented")
}

// Symbol — инструмент Finam.
type Symbol struct {
	Ticker   string
	Name     string
	Category string
}
