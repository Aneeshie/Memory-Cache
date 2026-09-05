package cache

import (
	"fmt"
	"sync"
	"time"
)

type Cache struct {
	cacheMap map[string]*CachedEntry
	heap     MinHeap

	mu sync.Mutex
}

func (c *Cache) Get(key string) (*CachedEntry, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	hit := c.cacheMap[key]
	if hit == nil {
		return nil, fmt.Errorf("Key not found.")
	}

	hit.LastUsed = time.Now()

	hit.Node.LastUsed = hit.LastUsed

	c.heap.FixDown(hit.Node.HeapIndex)

	return hit, nil
}
