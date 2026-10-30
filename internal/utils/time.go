package utils

import "time"

func CurrentMillis() int64 {
	return time.Now().UnixMilli()
}

func MillisToTime(millis int64) time.Time {
	return time.UnixMilli(millis)
}

func TimeToMillis(t time.Time) int64 {
	return t.UnixMilli()
}
