package finam

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestCategoryFromType(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"акция STOCK", "STOCK", "stocks"},
		{"акция share", "share", "stocks"},
		{"облигация BOND", "BOND", "bonds"},
		{"облигация bond строчными", "bond", "bonds"},
		{"неизвестный тип", "ETF", "other"},
		{"пустой тип", "", "other"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := categoryFromType(tc.in); got != tc.want {
				t.Fatalf("categoryFromType(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestDecimalValueFloat(t *testing.T) {
	cases := []struct {
		name  string
		value decimalValue
		want  float64
	}{
		{"обычное значение", decimalValue{Value: "123.45"}, 123.45},
		{"пустое", decimalValue{Value: ""}, 0},
		{"не число", decimalValue{Value: "abc"}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.value.Float(); got != tc.want {
				t.Fatalf("Float() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestFinamSearch_RealRequests(t *testing.T) {
	token := os.Getenv("FINAM_TOKEN")
	if token == "" {
		t.Skip("FINAM_TOKEN is not set")
	}

	client := NewClient(token)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Connect() failed: %v", err)
	}

	for _, q := range []string{"SBER@TQBR", "GMKN@TQBR"} {
		res, err := client.Search(context.Background(), q)
		if err != nil {
			t.Fatalf("Search(%q) failed: %v", q, err)
		}
		if len(res) == 0 {
			t.Fatalf("Search(%q) returned no results", q)
		}
		t.Logf("Search(%q) -> %d result(s): %s | %s", q, len(res), res[0].Ticker, res[0].Name)
	}
}

func TestNormalizeFinamQuery(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"короткий тикер", "sber", []string{"SBER@MISX", "SBER"}},
		{"с суффиксом moex", "SBER@MOEIX", []string{"SBER@MISX"}},
		{"точный запрос с суффиксом", "SBER@MISX", []string{"SBER@MISX"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeFinamQuery(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("normalizeFinamQuery(%q) len = %d, want %d (%v)", tc.in, len(got), len(tc.want), got)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("normalizeFinamQuery(%q)[%d] = %q, want %q", tc.in, i, got[i], tc.want[i])
				}
			}
		})
	}
}