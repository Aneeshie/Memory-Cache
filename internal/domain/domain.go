package domain

import "time"

type CachedEntry struct {
	Key       string
	Data      []byte
	CreatedAt time.Time
	UpdatedAt time.Time
}
