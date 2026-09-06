# Shared In-Memory Index

A small Go project implementing a concurrent in-memory cache with PostgreSQL as the persistent source of truth.

The primary goal of this project was **learning** — specifically understanding how in-memory data structures, concurrency, caching, persistence, and performance interact in a real application.

This is intentionally **not a production-ready cache implementation**.

---

## Overview

The system provides a simple HTTP API for storing and retrieving arbitrary JSON data.

It follows a read-through caching model:

```text
Client
  |
  v
Handler
  |
  v
Service
  |
  +----> Cache HIT ----> Response
  |
  +----> Cache MISS
             |
             v
        PostgreSQL
             |
             v
          Cache.Put()
             |
             v
          Response
```

PostgreSQL acts as the durable source of truth, while the in-memory cache provides fast access to frequently requested data.

---

## Features

- Concurrent-safe in-memory cache
- `map` for O(1) key lookups
- Custom min-heap implementation
- LRU-style eviction
- O(1) access to heap nodes using stored heap indices
- Mutex-protected cache operations
- PostgreSQL persistence
- Read-through caching
- PostgreSQL upserts using `ON CONFLICT`
- Generic `[]byte` cache values
- HTTP API using Chi
- Concurrent benchmarks
- Race detector testing

---

## Architecture

```text
Client
  |
  v
Handler
  |
  v
Service
  |
  +-------------> Cache
  |                  |
  |                  +-- HIT ------> return
  |                  |
  |                  +-- MISS
  |                       |
  |                       v
  |                  Repository
  |                       |
  |                       v
  |                  PostgreSQL
  |                       |
  |                       v
  |                  Cache.Put()
  |                       |
  +-----------------------+
```

The responsibilities are intentionally simple.

### Handler

Responsible for:

- HTTP requests
- URL parameters
- JSON encoding/decoding
- HTTP status codes

The handler does not know whether data comes from memory or PostgreSQL.

### Service

Responsible for:

- Cache behavior
- Read-through logic
- Deciding when to access the repository
- Coordinating cache and database operations

### Repository

Responsible for:

- PostgreSQL access
- SQL queries
- Persisting cache entries

### Cache

Responsible for:

- In-memory storage
- Fast lookups
- Updating access times
- Eviction
- Concurrency safety

---

## Cache Data Structure

The cache combines a hash map and a min-heap.

```text
Cache
├── map[string]*CachedEntry
├── MinHeap
│   └── []*HeapNode
└── Mutex
```

The map provides fast key lookup.

The heap provides efficient access to the least recently used entry.

Each cached entry contains the actual data and a pointer to its corresponding heap node:

```go
type CachedEntry struct {
    Data     []byte
    LastUsed time.Time
    Node     *HeapNode
}
```

The heap node contains:

```go
type HeapNode struct {
    DataID    string
    LastUsed  time.Time
    HeapIndex int
}
```

### Why Store `HeapIndex`?

Without `HeapIndex`, deleting or updating an arbitrary cache entry would require searching through the heap to find its node.

That would be O(n).

Instead:

```text
cacheMap[key]
     |
     v
CachedEntry
     |
     v
HeapNode
     |
     v
HeapIndex
```

The cache can directly locate the node in O(1).

Heap operations themselves remain O(log n).

The important invariant is:

```text
heap[i].HeapIndex == i
```

Whenever nodes are swapped or moved, their indices must be updated.

This was one of the main implementation details of the project.

---

## LRU Eviction

The cache uses `LastUsed` to implement an LRU-style eviction policy.

For example, with a capacity of 3:

```text
PUT A
PUT B
PUT C

GET A

PUT D
```

The access order is effectively:

```text
B -> C -> A
```

Therefore `B` is the least recently used entry and is evicted when `D` is inserted.

The min-heap keeps the oldest entry at the root.

---

## Why a Min-Heap?

A simple map gives excellent lookup performance, but it does not tell us which entry should be evicted.

The heap provides the second piece:

```text
Map
 |
 +-- Find entry quickly

Min-Heap
 |
 +-- Find least-recently-used entry quickly
```

This gives the cache two useful properties:

- O(1) key lookup
- O(log n) eviction/update operations

The implementation was written from scratch rather than using an existing cache library because understanding the underlying data structure was one of the goals of the project.

---

## Concurrency

The cache is protected by a `sync.Mutex`.

At first glance, `Get()` might look like a read-only operation.

It isn't.

A cache hit updates:

```go
hit.LastUsed = time.Now()
hit.Node.LastUsed = hit.LastUsed
```

and then modifies the heap.

Therefore a `Get()` changes shared state.

The mutex protects:

```text
map
heap
HeapIndex
LastUsed
```

as one logical unit.

This is important because the data structures depend on each other.

For example:

```text
CachedEntry.Node
      |
      v
HeapNode.HeapIndex
      |
      v
heap.slice[index]
```

Allowing concurrent modifications without synchronization could break these invariants.

---

## Race Detection

The implementation was tested using Go's race detector:

```bash
go test -race ./...
```

The cache passed the concurrent tests without reporting data races.

---

## PostgreSQL

PostgreSQL stores the durable copy of each entry.

The schema is intentionally simple:

```sql
CREATE TABLE cache_entries (
    key TEXT PRIMARY KEY,
    data BYTEA NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

PostgreSQL is treated as the source of truth.

The in-memory cache is an optimization.

---

## Why `[]byte`?

The cache stores values as:

```go
[]byte
```

rather than forcing the cache to understand a specific data structure.

For example, the bytes could represent:

- JSON
- MessagePack
- Protocol Buffers
- Serialized structs
- Other application-specific formats

The cache simply stores and returns the bytes.

The caller decides how to interpret them.

This keeps the cache relatively generic.

---

# Read-Through Caching

A `GET` follows this flow:

```text
Cache.Get(key)
      |
      +-- HIT ------> return data
      |
      +-- MISS
           |
           v
      PostgreSQL
           |
           +-- FOUND
           |     |
           |     v
           |  Cache.Put()
           |     |
           |     v
           |  return data
           |
           +-- NOT FOUND
                  |
                  v
                error
```

The client does not need to know where the data came from.

### First request

```text
Client
  |
  v
Cache MISS
  |
  v
PostgreSQL
  |
  v
Cache
  |
  v
Response
```

### Subsequent requests

```text
Client
  |
  v
Cache HIT
  |
  v
Response
```

This is the main reason the cache exists.

---

# Write Behavior

A `PUT` writes to PostgreSQL first and then updates the cache.

```text
Client
  |
  v
Service
  |
  v
PostgreSQL
  |
  v
Cache.Put()
```

The database uses an upsert:

```sql
INSERT INTO cache_entries (key, data, created_at, updated_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (key)
DO UPDATE SET
    data = EXCLUDED.data,
    updated_at = NOW();
```

This means the same endpoint can be used to create or update an entry.

---

# API

## PUT

```http
PUT /cache/{key}
```

Example:

```bash
curl -X PUT localhost:8080/cache/user:101   -H "Content-Type: application/json"   -d '{"data":{"name":"Aneesh","age":20}}'
```

Response:

```text
204 No Content
```

---

## GET

```http
GET /cache/{key}
```

Example:

```bash
curl localhost:8080/cache/user:101
```

Response:

```json
{
    "data": {
        "name": "Aneesh",
        "age": 20
    }
}
```

---

# Project Structure

The project intentionally uses a relatively simple structure:

```text
shared-in-memory-index/
├── cmd/
│   └── server/
│       └── main.go
│
├── internal/
│   ├── cache/
│   │   ├── cache.go
│   │   ├── entry.go
│   │   ├── heap.go
│   │   └── cache_test.go
│   │
│   ├── database/
│   ├── dto/
│   ├── handler/
│   ├── repository/
│   └── service/
│
├── migrations/
│
├── go.mod
└── README.md
```

This is not intended to be a definitive Go project structure.

It is simply enough structure to keep the responsibilities understandable without introducing unnecessary abstraction.

---

# Benchmarks

The project was benchmarked to compare the in-memory cache against PostgreSQL and to observe concurrent behavior.

## Single-threaded cache GET

```text
BenchmarkCacheGet-10
29,623,808 operations
36.31 ns/op
0 B/op
0 allocs/op
```

This demonstrates the extremely low overhead of accessing data already in memory.

---

## Concurrent cache GET

The cache was also tested with multiple goroutines:

```text
BenchmarkCacheGetConcurrent-10
1,897,567 operations
634.4 ns/op
0 B/op
0 allocs/op
```

The benchmark was run with the race detector:

```bash
go test -bench=BenchmarkCacheGetConcurrent -benchmem -race ./internal/cache
```

The race detector completed successfully.

---

## Concurrent GET + PUT

An approximately 80/20 GET/PUT workload was also tested:

```text
BenchmarkCacheMixedConcurrent-10
855,655 operations
1364 ns/op
0 B/op
```

This benchmark was useful because it represents a more realistic situation than only reading the same key repeatedly.

It also demonstrates the synchronization cost introduced by the shared mutex.

---

## PostgreSQL lookup

A direct repository benchmark produced:

```text
BenchmarkGetCachedEntry-10
12 operations
85,222,229 ns/op
19,386 B/op
102 allocs/op
```

This is an actual Go -> pgx -> PostgreSQL round trip, so the number is environment-dependent and should not be treated as a universal database latency figure.

The important observation is the scale difference between an in-memory lookup and a database-backed lookup.

---


## Performance Summary

The project was benchmarked at several levels to understand the cost of in-memory access, concurrency, synchronization, and PostgreSQL access.

| Operation | Time | Memory | Allocations |
|---|---:|---:|---:|
| Cache GET | **36.31 ns/op** | 0 B/op | 0 allocs/op |
| Concurrent Cache GET | **634.4 ns/op** | 0 B/op | 0 allocs/op |
| Concurrent 80/20 GET+PUT | **1364 ns/op** | 3 B/op | 0 allocs/op |
| PostgreSQL GET | **85.22 ms/op** | 19,386 B/op | 102 allocs/op |

### Cache vs PostgreSQL

The measured single-threaded cache lookup was approximately:

```text
Cache:       36.31 ns/op
PostgreSQL:  85.22 ms/op
```

That is roughly **2.35 million times lower latency** in this particular benchmark:

```text
85.22 ms / 36.31 ns ≈ 2.35 million
```

This number should **not** be interpreted as a universal cache-vs-database speed ratio. The PostgreSQL benchmark includes the Go → pgx → PostgreSQL round trip and depends heavily on the local environment, connection state, database configuration, and workload.

The useful takeaway is the enormous difference between an in-process memory access and a database round trip.

### Concurrent performance

The cache was also tested under concurrent access.

```text
BenchmarkCacheGetConcurrent-10
1,897,567 operations
634.4 ns/op
0 B/op
0 allocs/op
```

And with a mixed workload of approximately 80% GET and 20% PUT:

```text
BenchmarkCacheMixedConcurrent-10
855,655 operations
1364 ns/op
3 B/op
0 allocs/op
```

The increase from **36.31 ns/op** to **634.4 ns/op** under concurrent execution highlights that synchronization and concurrent scheduling introduce overhead even when the underlying data structure performs no heap allocations.

The mixed workload is slower still because `PUT` operations modify the cache and heap in addition to acquiring the mutex.

### Race Detector

The concurrent cache benchmarks were also run with Go's race detector:

```bash
go test -bench=BenchmarkCacheGetConcurrent -benchmem -race ./internal/cache
```

and the test suite was run with:

```bash
go test -race ./...
```

No data races were reported.

### What These Numbers Show

The benchmarks helped demonstrate several important points:

- In-memory access is dramatically cheaper than a database round trip.
- The cache performs zero allocations per operation in the measured GET workloads.
- Concurrency introduces synchronization overhead.
- `Get()` is not free of synchronization because it updates LRU metadata.
- Mixed read/write workloads increase contention.
- Performance should be measured rather than assumed.

These measurements were used primarily as a **learning and profiling exercise**, not as a claim that this implementation is faster or better than production cache systems.

# What I Learned

This project was primarily built as a learning exercise.

Some of the main concepts explored were:

### Data Structures

- Hash maps
- Min-heaps
- LRU eviction
- Heap indexing
- Maintaining data structure invariants

### Go Concurrency

- Goroutines
- Mutexes
- Concurrent access
- Race detection
- Lock contention
- The difference between logically reading data and physically mutating shared state

### Systems Design

- Cache vs source of truth
- Read-through caching
- Service/repository separation
- Persistence
- HTTP request flow
- Failure boundaries

### Performance

- Benchmarking with `go test -bench`
- Allocation measurements with `-benchmem`
- Race detection with `-race`
- Comparing in-memory access against database access
- Understanding synchronization overhead

---

# Caveats

This project intentionally does **not** attempt to be a production-grade cache.

The main objective was to understand the underlying concepts by implementing them myself.

### 1. The architecture is intentionally simplified

I did not strictly follow conventional production Go project structures.

For example, rather than introducing additional layers or packages purely for architectural convention, the application uses simple:

```text
Handler
   |
Service
   |
Repository
```

structs.

This was intentional.

The goal was to understand the responsibilities of each layer rather than maximize architectural complexity.

### 2. The cache uses a single global mutex

The entire cache is protected by one `sync.Mutex`.

This makes the implementation straightforward and keeps the map/heap invariants safe, but it also introduces contention.

Even `Get()` requires the mutex because updating `LastUsed` modifies the heap.

A production implementation could explore:

- Sharded caches
- Per-shard mutexes
- Approximate LRU policies
- Different eviction algorithms
- More specialized synchronization strategies

Sharding could improve concurrency, but maintaining a globally ordered LRU would become more complicated.

### 3. The LRU implementation is custom

This implementation was primarily intended to learn how the data structure works.

Production systems would likely use an existing, well-tested cache implementation rather than maintaining a custom heap and eviction mechanism.

The custom implementation exists because understanding the mechanism was the point of the project.

### 4. No TTL

Entries currently don't expire based on time.

A production cache might support:

```text
expires_at
TTL
background cleanup
lazy expiration
```

This was intentionally left out.

### 5. No cache stampede protection

If many goroutines request the same missing key simultaneously:

```text
GET A -> Cache MISS -> DB
GET A -> Cache MISS -> DB
GET A -> Cache MISS -> DB
GET A -> Cache MISS -> DB
```

multiple database requests can occur before the first request populates the cache.

A production implementation could use request coalescing or a single-flight style mechanism.

### 6. No distributed cache invalidation

The cache is local to a single process.

If multiple application instances are running:

```text
Instance A -> Cache A
Instance B -> Cache B
Instance C -> Cache C
```

each instance has its own state.

Updating data through instance A does not automatically invalidate or update the caches of B and C.

A distributed system could use Redis, pub/sub, messaging, or explicit invalidation protocols.

### 7. Limited failure handling

The project does not attempt to fully solve complicated failure scenarios such as:

```text
DB succeeds
Cache update fails

Cache update succeeds
DB fails

Process crashes after DB write
Process crashes before cache update
```

The service currently follows a simple DB-first approach for writes.

Production systems would need more deliberate consistency and recovery semantics.

### 8. No full production observability/shutdown setup

The server is intentionally minimal.

It does not currently provide a complete production setup for:

- Graceful shutdown
- Structured logging
- Metrics
- Tracing
- Health checks
- Readiness/liveness probes
- Configuration management
- Connection pool tuning
- Load testing

These are separate engineering concerns that were outside the main learning objective.

---

# Optimizations / Improvements

There are several ways this implementation could be improved.

The interesting part is that many of these optimizations introduce new tradeoffs rather than simply making everything faster.

### Cache sharding

Instead of one global lock:

```text
             Cache
               |
       +-------+-------+
       v       v       v
    Shard 1  Shard 2  Shard 3
      |        |        |
     lock     lock     lock
```

Each shard could have its own map, heap, and mutex.

This would reduce lock contention under high concurrency.

The tradeoff is increased complexity and potentially less precise global LRU behavior.

### Alternative eviction policies

Instead of a strict LRU-style policy, alternatives could be explored:

- CLOCK
- TinyLFU
- Segmented LRU
- Approximate LRU

These can sometimes provide better performance or lower synchronization overhead depending on workload.

### Read/write synchronization

`sync.RWMutex` was not directly useful for the current design because `Get()` updates recency information.

A different cache design could separate data access from recency tracking and potentially allow more concurrent reads.

### Cache stampede protection

A single-flight mechanism could ensure that concurrent misses for the same key share one database request.

```text
GET A --+
GET A --+--> one DB request --> Cache A
GET A --+
GET A --+
```

### TTL

Entries could expire after a configurable duration.

This would be useful for data that becomes stale over time.

### Distributed caching

For multiple application instances, a shared external cache such as Redis could be introduced.

That changes the problem substantially because network latency, distributed consistency, failures, and invalidation become important.

---

# Why Build It Yourself?

A production application probably shouldn't reinvent a cache.

The value of this project was not:

> "I built something better than Redis."

It was:

> "I now understand what is happening underneath a cache."

I had to reason about:

```text
Map
 ↓
Heap
 ↓
LRU
 ↓
Mutex
 ↓
Concurrent access
 ↓
Database fallback
 ↓
HTTP
 ↓
Performance
```

That was the actual goal of the project.

---

# Future Experiments

If I wanted to take this project further, interesting experiments would include:

- Sharded caches
- TTL expiration
- Cache stampede prevention
- Approximate LRU
- `sync.RWMutex` experiments
- Concurrent benchmark comparisons
- Load testing the HTTP API
- Cache hit/miss metrics
- Graceful shutdown
- Better error handling
- Cache invalidation across multiple instances
- Comparing the implementation against an existing cache library

These are intentionally left as future experiments rather than part of the core implementation.

---

# Stack

- **Go**
- **Chi**
- **PostgreSQL**
- **pgx**
- **godotenv**

---

# Status

**Complete — Learning Project**

The implementation is intentionally simple and has known limitations.

The project is considered complete because the primary learning goals were achieved:

- Building a concurrent in-memory cache from scratch
- Implementing a min-heap and LRU-style eviction
- Maintaining data structure invariants
- Integrating the cache with PostgreSQL
- Implementing read-through caching
- Exposing the system through HTTP
- Testing concurrent behavior
- Using the race detector
- Benchmarking cache vs database performance
- Identifying synchronization bottlenecks
- Understanding possible production optimizations and their tradeoffs

The project is therefore considered **finished for its intended learning scope**, rather than production-ready.
