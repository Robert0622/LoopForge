package httpapi

import (
	"time"

	"github.com/Tencent/LoopForge/internal/cronexpr"
)

var assistantCronLocation = cronexpr.Location

func parseAssistantCron(expr string) (cronexpr.Schedule, error) {
	return cronexpr.Parse(expr)
}

func nextAssistantCron(expr string, after time.Time) (time.Time, error) {
	return cronexpr.Next(expr, after)
}
