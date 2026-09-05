package main

import (
	"fmt"

	"github.com/Aneeshie/shared-in-memory-index/internal/cache"
)

func main() {
	capacity := 3

	cache := cache.NewCache(capacity)

	cache.Put("user:101", []byte(`{"name":"Aneesh","age":20}`))
	cache.Put("user:102", []byte(`{"name":"Rahul","age":21}`))
	cache.Put("user:103", []byte(`{"name":"Priya","age":19}`))

	entry, _ := cache.Get("user:101")
	fmt.Println(string(entry.Data))

	cache.Put("user:104", []byte(`{"name":"Arjun","age":22}`))
	cache.Delete("user:103")

}
