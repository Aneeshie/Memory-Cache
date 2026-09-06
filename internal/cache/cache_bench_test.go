package cache_test

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/Aneeshie/shared-in-memory-index/internal/cache"
)

func BenchmarkCacheGet(b *testing.B) {
	c := cache.NewCache(100)

	data := []byte(`{"name":"Ada","age":20}`)
	c.Put("user:101", data)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := c.Get("user:101")
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCacheGetConcurrent(b *testing.B) {
	c := cache.NewCache(100)
	c.Put("user:101", []byte(`{"name":"Aneesh"}`))

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := c.Get("user:101")
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkCacheMixedConcurrent(b *testing.B) {
	c := cache.NewCache(100)

	keys := make([]string, 100)

	for i := 0; i < 100; i++ {
		keys[i] = fmt.Sprintf("key:%d", i)
		c.Put(keys[i], []byte("data"))
	}

	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			key := keys[rand.Intn(100)]

			if rand.Intn(10) < 8 {
				_, err := c.Get(key)
				if err != nil {
					b.Fatal(err)
				}
			} else {
				c.Put(key, []byte("updated"))
			}
		}
	})
}
