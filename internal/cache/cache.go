package cache

import (
	"fmt"
	"sync"
	"time"
)

type Cache struct {
	cacheMap map[string]*CachedEntry
	heap     MinHeap
	capacity int

	mu sync.Mutex
}

func NewCache(capacity int) *Cache {
	return &Cache{
		cacheMap: make(map[string]*CachedEntry),
		capacity: capacity,
	}
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
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cacheMap[key] != nil {
		c.cacheMap[key].Data = data
		c.cacheMap[key].LastUsed = time.Now()

		c.cacheMap[key].Node.LastUsed = c.cacheMap[key].LastUsed
		c.heap.FixDown(c.cacheMap[key].Node.HeapIndex)

		return
	}

	if len(c.cacheMap) >= c.capacity {
		//extract key from minHeap
		node, ok := c.heap.Extract()
		if !ok {
			fmt.Println("the heap is empty??!??!!?!?")
			return
		}

		//delete the node from the cache map
		delete(c.cacheMap, node.DataID)
	}

	//construct the heap node first.
	node := &HeapNode{
		DataID:   key,
		LastUsed: time.Now(),
	}

	//after this the node has the HeapIndex filled in now...
	c.heap.Insert(node)

	//now the entry
	entry := &CachedEntry{
		Data:     data,
		LastUsed: node.LastUsed,
		Node:     node,
	}

	//push it into the map now
	c.cacheMap[key] = entry
}

func (c *Cache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	//first remove from the cacheMap
	if c.cacheMap[key] == nil {
		fmt.Println("Key does not exist.")
		return
	}

	//first remove from the minHeap
	heapIndex := c.cacheMap[key].Node.HeapIndex
	lastIndex := len(c.heap.slice) - 1

	c.heap.slice[heapIndex] = c.heap.slice[lastIndex]
	c.heap.slice = c.heap.slice[:lastIndex]

	if heapIndex < len(c.heap.slice) {
		c.heap.slice[heapIndex].HeapIndex = heapIndex
		if heapIndex > 0 && c.heap.slice[heapIndex].LastUsed.Before(c.heap.slice[parent(heapIndex)].LastUsed) {
			c.heap.minHeapifyUp(heapIndex)
		} else {
			c.heap.minHeapifyDown(heapIndex)
		}
	}

	delete(c.cacheMap, key)

}
