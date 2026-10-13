package provider

import (
	"time"
)

func ptr(t time.Time) *time.Time { return &t }
