package lib

import (
	"fmt"
	"time"
)

// GoldPricesUpload contains the current gold prices
type GoldPricesUpload struct {
	Prices     []int   `json:"Prices"`
	TimeStamps []int64 `json:"Timestamps"`
}

func (g *GoldPricesUpload) StringArrays() [][]string {
	result := [][]string{}

	for i := range g.Prices {
		// Convert timestamp from game units (10,000 per second) to Unix seconds
		const gameTicksPerSecond = 10000
		timestampSec := g.TimeStamps[i] / gameTicksPerSecond
		result = append(result, []string{
			fmt.Sprintf("%d", g.Prices[i]),
			time.Unix(timestampSec, 0).Format(time.RFC3339),
		})
	}

	return result
}
