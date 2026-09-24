package volatility

type Candle struct {
	High  float64
	Low   float64
	Close float64
}

// Calculate возвращает процент волатильности на основе последних свечей.
func Calculate(candles []Candle) float64 {
	if len(candles) == 0 {
		return 0
	}

	var sumRange float64
	for _, c := range candles {
		sumRange += c.High - c.Low
	}

	avgRange := sumRange / float64(len(candles))
	lastClose := candles[len(candles)-1].Close
	if lastClose == 0 {
		return 0
	}

	return avgRange / lastClose * 100.0
}
