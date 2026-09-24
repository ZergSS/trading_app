package volatility

import (
	"math"
	"testing"
)

func TestCalculate(t *testing.T) {
	candles := []Candle{
		{High: 110, Low: 90, Close: 100},
		{High: 120, Low: 80, Close: 100},
		{High: 105, Low: 95, Close: 100},
		{High: 115, Low: 85, Close: 100},
		{High: 108, Low: 92, Close: 100},
	}

	got := Calculate(candles)
	want := 20.0 // (20+40+10+30+16)/5 = 116/5=23.2, 23.2/100*100=23.2?
	// Пересчитаем: sumRange = 20+40+10+30+16 = 116, avg=23.2, /100*100=23.2
	want = 23.2

	if math.Abs(got-want) > 1e-6 {
		t.Errorf("Calculate() = %v, want %v", got, want)
	}
}
