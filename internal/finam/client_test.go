package finam

import "testing"

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

func TestNormalizeFinamQuery(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"короткий тикер", "sber", []string{"SBER@TQBR", "SBER@MOEX", "SBER"}},
		{"с суффиксом moex", "SBER@MOEIX", []string{"SBER@MOEX"}},
		{"точный запрос с суффиксом", "SBER@TQBR", []string{"SBER@TQBR"}},
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
