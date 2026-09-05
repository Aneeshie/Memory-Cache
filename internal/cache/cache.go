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

func (c *Cache) Put(key string, data []byte) {
	//construct the heap node first.
	Node := &HeapNode{
		DataID:   key,
		LastUsed: time.Now(),
	}

	//after this the node has the HeapIndex filled in now...
	c.heap.Insert(Node)

	//now the entry
	entry := &CachedEntry{
		Data:     data,
		LastUsed: Node.LastUsed,
		Node:     Node,
	}

	//push it into the map now
	c.cacheMap[key] = entry
}
