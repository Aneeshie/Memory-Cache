package cache

import "time"

type CachedEntry struct {
	Data     []byte
	LastUsed time.Time
}

type HeapNode struct {
	DataID    string
	LastUsed  time.Time
	HeapIndex int
}
