package cache

import (
	"testing"
)

// Test putting and getting an item.
func TestCachePutGet(t *testing.T) {
	cache := NewCache(3)

	data := []byte(`{"name":"Aneesh"}`)

	cache.Put("user:101", data)

	entry, err := cache.Get("user:101")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if string(entry.Data) != string(data) {
		t.Fatalf("expected %s, got %s", data, entry.Data)
	}
}

// Test getting a key that doesn't exist.
func TestCacheGetMissing(t *testing.T) {
	cache := NewCache(3)

	_, err := cache.Get("user:999")

	if err == nil {
		t.Fatal("expected error for missing key")
	}
}

// Test updating an existing key.
func TestCachePutExisting(t *testing.T) {
	cache := NewCache(3)

	cache.Put("user:101", []byte(`{"name":"Aneesh"}`))
	cache.Put("user:101", []byte(`{"name":"Aneesh Updated"}`))

	entry, err := cache.Get("user:101")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := `{"name":"Aneesh Updated"}`

	if string(entry.Data) != expected {
		t.Fatalf("expected %s, got %s", expected, entry.Data)
	}

	// Updating an existing key should NOT create another heap node.
	if len(cache.heap.slice) != 1 {
		t.Fatalf("expected 1 heap node, got %d", len(cache.heap.slice))
	}
}

// Test deleting an existing key.
func TestCacheDelete(t *testing.T) {
	cache := NewCache(3)

	cache.Put("user:101", []byte(`A`))

	cache.Delete("user:101")

	_, err := cache.Get("user:101")

	if err == nil {
		t.Fatal("expected error after deleting key")
	}

	if len(cache.heap.slice) != 0 {
		t.Fatalf("expected empty heap, got %d nodes", len(cache.heap.slice))
	}
}

// Test deleting a key that doesn't exist.
func TestCacheDeleteMissing(t *testing.T) {
	cache := NewCache(3)

	// Should not panic.
	cache.Delete("user:999")
}

// Test LRU eviction.
func TestCacheLRUEviction(t *testing.T) {
	cache := NewCache(3)

	cache.Put("A", []byte(`A`))
	cache.Put("B", []byte(`B`))
	cache.Put("C", []byte(`C`))

	// Make A recently used.
	_, err := cache.Get("A")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Cache is full, so the least recently used item should be evicted.
	cache.Put("D", []byte(`D`))

	// B should have been evicted.
	_, err = cache.Get("B")
	if err == nil {
		t.Fatal("expected B to be evicted")
	}

	// A, C and D should still exist.
	for _, key := range []string{"A", "C", "D"} {
		if _, err := cache.Get(key); err != nil {
			t.Fatalf("expected %s to exist: %v", key, err)
		}
	}
}

// Test that HeapIndex stays correct after operations.
func TestHeapIndex(t *testing.T) {
	cache := NewCache(3)

	cache.Put("A", []byte(`A`))
	cache.Put("B", []byte(`B`))
	cache.Put("C", []byte(`C`))

	for i, node := range cache.heap.slice {
		if node.HeapIndex != i {
			t.Fatalf(
				"node %s: expected HeapIndex %d, got %d",
				node.DataID,
				i,
				node.HeapIndex,
			)
		}
	}

	// Cause an eviction and then check again.
	cache.Get("A")
	cache.Put("D", []byte(`D`))

	for i, node := range cache.heap.slice {
		if node.HeapIndex != i {
			t.Fatalf(
				"node %s: expected HeapIndex %d, got %d",
				node.DataID,
				i,
				node.HeapIndex,
			)
		}
	}
}

// Test deleting an item from the middle of the heap.
func TestCacheDeleteMiddle(t *testing.T) {
	cache := NewCache(5)

	cache.Put("A", []byte(`A`))
	cache.Put("B", []byte(`B`))
	cache.Put("C", []byte(`C`))
	cache.Put("D", []byte(`D`))
	cache.Put("E", []byte(`E`))

	cache.Delete("C")

	// C should no longer exist.
	if _, err := cache.Get("C"); err == nil {
		t.Fatal("expected C to be deleted")
	}

	// Four nodes should remain.
	if len(cache.heap.slice) != 4 {
		t.Fatalf("expected 4 heap nodes, got %d", len(cache.heap.slice))
	}

	// Every node's HeapIndex should match its actual position.
	for i, node := range cache.heap.slice {
		if node.HeapIndex != i {
			t.Fatalf(
				"node %s: expected HeapIndex %d, got %d",
				node.DataID,
				i,
				node.HeapIndex,
			)
		}
	}
}
