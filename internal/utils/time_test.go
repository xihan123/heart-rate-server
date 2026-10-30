package utils

import (
	"math"
	"testing"
	"time"
)

func TestMillisRoundTripDoesNotOverflow(t *testing.T) {
	for _, millis := range []int64{0, -1, time.Now().UnixMilli() + (int64(1) << 58), math.MaxInt64, math.MinInt64} {
		if got := TimeToMillis(MillisToTime(millis)); got != millis {
			t.Fatalf("round trip for %d = %d", millis, got)
		}
	}
}
