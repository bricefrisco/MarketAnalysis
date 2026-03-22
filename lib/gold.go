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
		// Timestamps are .NET DateTime ticks (100ns intervals since Jan 1, 0001).
		// Subtract the offset to Unix epoch, then divide by ticks-per-second to get Unix seconds.
		const dotNetTicksPerSecond = 10_000_000
		const dotNetEpochOffset = 621_355_968_000_000_000
		timestampSec := (g.TimeStamps[i] - dotNetEpochOffset) / dotNetTicksPerSecond
		result = append(result, []string{
			fmt.Sprintf("%d", g.Prices[i]),
			time.Unix(timestampSec, 0).Format(time.RFC3339),
		})
	}

	return result
}
