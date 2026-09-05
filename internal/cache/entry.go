package cache

import "time"

type CachedEntry struct {
	Data     []byte
	LastUsed time.Time
	Node     *HeapNode
}

type HeapNode struct {
	DataID    string
	LastUsed  time.Time
	HeapIndex int
}
