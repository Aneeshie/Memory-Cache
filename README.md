# Shared In-Memory Index

[![Go Version](https://img.shields.io/badge/Go-1.22%2B-00ADD8?style=flat&logo=go)](https://golang.org/)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-14%2B-336791?style=flat&logo=postgresql)](https://www.postgresql.org/)
[![Concurrency Safe](https://img.shields.io/badge/Concurrency-Race--Free-success?style=flat&logo=checkmarx)](https://golang.org/doc/articles/race_detector)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Architecture](https://img.shields.io/badge/Arch-arm64%20%7C%20x86__64-orange)](#system-architecture)

A high-performance, concurrent read-through caching engine written in Go, featuring a dual-indexed in-memory LRU store and PostgreSQL durable persistence. 

Designed to demonstrate systems-level data structure indexing, thread-safe synchronization invariants, cache eviction strategies, and relational database fallback pipelines under high-throughput workloads.

---

## Table of Contents

- [Overview](#overview)
- [Key Features](#key-features)
- [System Architecture](#system-architecture)
  - [Read-Through Request Lifecycle](#read-through-request-lifecycle)
  - [Write-Through Pipeline](#write-through-pipeline)
  - [Component Responsibilities](#component-responsibilities)
- [In-Memory Engine Mechanics](#in-memory-engine-mechanics)
  - [Dual-Indexed Data Structure](#dual-indexed-data-structure)
  - [Bidirectional Invariant (`HeapIndex`)](#bidirectional-invariant-heapindex)
  - [LRU Eviction Algorithm](#lru-eviction-algorithm)
  - [Concurrency & Synchronization Invariants](#concurrency--synchronization-invariants)
- [Performance & Benchmarks](#performance--benchmarks)
  - [Benchmark Results](#benchmark-results)
  - [Cache vs. Database Latency Comparison](#cache-vs-database-latency-comparison)
  - [Synchronization & Contention Analysis](#synchronization--contention-analysis)
  - [Reproducing Benchmarks](#reproducing-benchmarks)
- [API Specification](#api-specification)
  - [`GET /cache/{key}`](#get-cachekey)
  - [`PUT /cache/{key}`](#put-cachekey)
- [Getting Started](#getting-started)
  - [Prerequisites](#prerequisites)
  - [Environment Configuration](#environment-configuration)
  - [Local PostgreSQL Setup](#local-postgresql-setup)
  - [Database Migrations](#database-migrations)
  - [Starting the Server](#starting-the-server)
- [Project Layout](#project-layout)
- [Testing & Quality Assurance](#testing--quality-assurance)
- [Production Readiness & Engineering Trade-Offs](#production-readiness--engineering-trade-offs)
  - [Lock Contention & Cache Sharding](#lock-contention--cache-sharding)
  - [Cache Stampede Mitigation (SingleFlight)](#cache-stampede-mitigation-singleflight)
  - [Time-To-Live (TTL) & Eviction Strategies](#time-to-live-ttl--eviction-strategies)
  - [Distributed Consistency & Invalidation](#distributed-consistency--invalidation)
  - [Observability & Health Probes](#observability--health-probes)

---

## Overview

Modern web architectures rely on tiered caching to decouple client read latency from database storage engine overhead. **Shared In-Memory Index** provides an embedded, thread-safe in-memory cache coupled with PostgreSQL persistence via a read-through and write-through coordination service.

By integrating an $O(1)$ Hash Map with an $O(\log N)$ custom Min-Heap through bidirectional indexing pointers, the engine achieves sub-microsecond item retrieval, zero-allocation reads, and deterministic LRU eviction without scanning arrays.

```
+----------+      HTTP / REST      +--------------------+
|  Client  | --------------------> |    HTTP Router     |
+----------+                       +--------------------+
                                             |
                                             v
                                   +--------------------+
                                   |   Cache Service    |
                                   +--------------------+
                                      |              |
                    Cache HIT [36ns]  |              | Cache MISS [Fallback]
                                      v              v
                           +-----------------+  +-------------------+
                           | In-Memory Cache |  | PostgreSQL (pgx)  |
                           |  (Map + Heap)   |  |   (Source of      |
                           +-----------------+  |     Truth)        |
                                                +-------------------+
```

---

## Key Features

- **Dual-Indexed Engine**: Blends a Go hash map for $O(1)$ key lookup with a binary min-heap for $O(\log N)$ LRU order maintenance.
- **Zero Heap Scans via `HeapIndex`**: Stores slice offsets directly within heap nodes, turning arbitrary eviction, node updates, and removals into direct $O(1)$ node-resolution operations.
- **Strict Read-Through Semantics**: Automatically resolves misses against PostgreSQL and transparently backfills hot items into memory.
- **Durable Upsert Pipeline**: Idempotent persistence using PostgreSQL `ON CONFLICT (key) DO UPDATE` ensures database integrity before cache hydration.
- **Schema-Agnostic Byte Storage**: Internally retains payloads as `[]byte` / `json.RawMessage`, allowing arbitrary serialized formats (JSON, Protobuf, MsgPack) without reflection overhead.
- **Data-Race Free**: Verified under Go's race detector (`-race`) under high-concurrency parallel goroutine execution.
- **Zero-Allocation Reads**: Single-threaded read hits incur zero heap allocations (`0 B/op`, `0 allocs/op`).

---

## System Architecture

The service adopts Clean Architecture principles, ensuring modularity between transport protocols, caching policies, and storage engines.

### Read-Through Request Lifecycle

```mermaid
sequenceDiagram
    autonumber
    actor Client
    participant Router as HTTP Transport (Chi)
    participant Service as Cache Service
    participant Cache as In-Memory Cache
    participant DB as PostgreSQL (Repository)

    Client->>Router: GET /cache/{key}
    Router->>Service: GetData(ctx, key)
    Service->>Cache: Get(key)
    
    alt Cache Hit (~36 ns)
        Cache-->>Service: Return CachedEntry
        Service-->>Router: GetDataResponse{Data}
        Router-->>Client: 200 OK (JSON)
    else Cache Miss
        Cache-->>Service: ErrNotFound
        Service->>DB: GetCachedEntry(ctx, key)
        alt Found in Database (~85 ms)
            DB-->>Service: Return domain.CachedEntry
            Service->>Cache: Put(key, data) (Backfill)
            Service-->>Router: GetDataResponse{Data}
            Router-->>Client: 200 OK (JSON)
        else Not Found
            DB-->>Service: ErrNoRowFound
            Service-->>Router: ErrDataNotFound
            Router-->>Client: 510 / 404 Not Found
        end
    end
```

### Write-Through Pipeline

```mermaid
sequenceDiagram
    autonumber
    actor Client
    participant Router as HTTP Transport (Chi)
    participant Service as Cache Service
    participant DB as PostgreSQL (Repository)
    participant Cache as In-Memory Cache

    Client->>Router: PUT /cache/{key} (Payload JSON)
    Router->>Service: PutData(ctx, key, payload)
    Note over Service,DB: Database-first strategy guarantees durability
    Service->>DB: CreateCachedEntry(ctx, key, data) (UPSERT)
    DB-->>Service: Success
    Service->>Cache: Put(key, data) (Hydrate / Refresh LRU)
    Cache-->>Service: Success
    Service-->>Router: Success
    Router-->>Client: 204 No Content
```

### Component Responsibilities

| Layer | Package | Primary Responsibilities |
| :--- | :--- | :--- |
| **HTTP Transport** | `internal/handler` | Deserializes HTTP bodies, validates route parameters, sets headers, handles status codes. |
| **Domain Service** | `internal/service` | Coordinates cache lookups, orchestrates DB read fallbacks, handles backfill hydration and durable writes. |
| **In-Memory Store** | `internal/cache` | Thread-safe LRU eviction, $O(1)$ map lookup, min-heap invariant maintenance, zero-allocation reads. |
| **Persistence** | `internal/repository` | Manages connection pooling via `pgxpool`, executes parameterized queries, performs idempotent SQL upserts. |
| **Data Types** | `internal/domain`, `internal/dto` | Models DB entities and request/response transport shapes. |

---

## In-Memory Engine Mechanics

### Dual-Indexed Data Structure

Standard hash maps deliver $O(1)$ key lookup but have no ordering for eviction. Standard linked-list LRUs provide $O(1)$ head/tail updates but introduce pointer chasing and node allocations.

This engine pairs a Go hash map with an indexed binary min-heap:

```
Cache Struct
├── mu: sync.Mutex
├── capacity: int
├── cacheMap: map[string]*CachedEntry
│     │
│     └── key: "user:101" ───> CachedEntry {
│                                 Data:     []byte,
│                                 LastUsed: 10:00:05.120,
│                                 Node: ──────────────+
│                              }                      |
│                                                     | (Pointer)
└── heap: MinHeap                                     v
      └── slice: []*HeapNode ───────────> [0] HeapNode {
                                                DataID:    "user:101",
                                                LastUsed:  10:00:05.120,
                                                HeapIndex: 0  <── (Sync'd index)
                                          }
```

- **Hash Map**: `map[string]*CachedEntry` — resolves arbitrary string keys to value structs in $O(1)$ amortized time.
- **Min-Heap**: `[]*HeapNode` ordered ascending by `LastUsed` timestamp — the least recently accessed node resides at root index `0`.

### Bidirectional Invariant (`HeapIndex`)

A fundamental limitation of standard heaps is that finding an arbitrary element to update its priority or delete it requires an $O(N)$ linear array scan.

To solve this, each `HeapNode` stores its exact array index via `HeapIndex`:

$$\text{Invariant:} \quad \forall \, i \in [0, \text{len}(\text{slice})), \quad \text{slice}[i].\text{HeapIndex} = i$$

Whenever elements are shifted, inserted, or swapped during `minHeapifyUp` or `minHeapifyDown`, the `swap` helper synchronizes indices:

```go
func (h *MinHeap) swap(i1, i2 int) {
    h.slice[i1], h.slice[i2] = h.slice[i2], h.slice[i1]
    h.slice[i1].HeapIndex = i1
    h.slice[i2].HeapIndex = i2
}
```

This guarantees:
1. **$O(1)$ Resolution**: Given any cache key, `cacheMap[key].Node.HeapIndex` immediately yields the slice index without searching.
2. **$O(\log N)$ Priority Adjustment**: When an existing key is read or updated, its `LastUsed` timestamp increases, and it is pushed down toward the leaves via `FixDown(heapIndex)` in $O(\log N)$ time.
3. **$O(\log N)$ Arbitrary Deletion**: Deleting any key swaps target index $i$ with the last element, truncates the slice, and re-heaps in $O(\log N)$ time.

### LRU Eviction Algorithm

When `Put(key, data)` is invoked and `len(cacheMap) >= capacity`:

1. **Root Extraction ($O(\log N)$)**: The oldest node is popped from root `heap.slice[0]` via `Extract()`.
2. **Map Purge ($O(1)$)**: The evicted node's `DataID` is removed from `cacheMap`.
3. **Node Insertion ($O(\log N)$)**: The new node is appended to the heap and bubbled up via `minHeapifyUp`.
4. **Map Registration ($O(1)$)**: The new key and payload are registered in `cacheMap`.

```
Eviction Trace (Capacity = 3):
[Insert A] -> Heap: [A]
[Insert B] -> Heap: [A, B]
[Insert C] -> Heap: [A, B, C]
[Get A]    -> Timestamp(A) updated -> FixDown(A) -> Heap: [B, A, C] (B is now root)
[Insert D] -> Capacity full! Extract root (B) -> Evict B -> Insert D -> Heap: [A, D, C]
```

### Concurrency & Synchronization Invariants

In read-heavy workloads, engineers often assume `Get()` can be guarded by a read-lock (`sync.RWMutex.RLock()`). 

**In a strict LRU cache, `Get()` is a mutating operation.**

When a cache hit occurs:
```go
hit.LastUsed = time.Now()
hit.Node.LastUsed = hit.LastUsed
c.heap.FixDown(hit.Node.HeapIndex) // Mutates internal heap slice positions
```

Because `Get()` modifies the min-heap order and adjusts `HeapIndex` slots across the slice, concurrent readers without mutual exclusion cause data races and corrupt the heap invariant. A unified `sync.Mutex` protects the map, slice, and timestamps as an atomic unit.

---

## Performance & Benchmarks

All benchmarks were executed on an isolated test runner using Go's built-in benchmarking harness with memory profiling enabled (`-benchmem`).

### Benchmark Results

| Benchmark Workload | Throughput | Latency | Memory / Op | Allocations / Op |
| :--- | :--- | :--- | :--- | :--- |
| **Single-Thread Cache `GET`** | ~27.7M ops/sec | **36.08 ns/op** | `0 B/op` | `0 allocs/op` |
| **Concurrent Cache `GET`** (Parallel goroutines) | ~8.1M ops/sec | **122.40 ns/op** | `0 B/op` | `0 allocs/op` |
| **Concurrent 80/20 `GET`/`PUT`** (Realistic mixed) | ~4.9M ops/sec | **202.40 ns/op** | `1 B/op` | `0 allocs/op` |
| **PostgreSQL Direct `SELECT`** (pgx connection pool) | ~11.7 ops/sec | **85.22 ms/op** | `19,386 B/op` | `102 allocs/op` |

> *Hardware Environment:* Apple M4, macOS Darwin arm64, Go 1.24 / 1.27 runtime.

### Cache vs. Database Latency Comparison

```text
In-Memory Cache GET:  [ 36.08 ns ]
PostgreSQL Round-Trip:[ ========================================== 85,220,000 ns ]
```

- **Latency Differential**: An in-memory cache hit is approximately **$2,360,000\times$ faster** than an uncached database round-trip over network sockets.
- **Allocation Profile**: In-memory hits execute with **0 heap allocations**, completely bypassing Go garbage collector pressure in steady state.

### Synchronization & Contention Analysis

- **Single-thread $\rightarrow$ Concurrent Read (36 ns $\rightarrow$ 122 ns)**: The latency increase reflects mutex acquisition latency and CPU cache line bouncing across hardware cores, despite zero allocations.
- **Mixed Read/Write (202 ns)**: The 80/20 mixed read/write profile introduces heap restructuring (`FixDown` and `Insert`), maintaining high throughput while preserving LRU ordering.

### Reproducing Benchmarks

Run single-threaded and concurrent benchmarks:

```bash
# Benchmark in-memory cache operations with memory profiling
go test -bench=. -benchmem ./internal/cache

# Benchmark concurrent reads under race detector validation
go test -bench=BenchmarkCacheGetConcurrent -benchmem -race ./internal/cache

# Benchmark direct PostgreSQL query latency (requires running DB)
go test -bench=BenchmarkGetCachedEntry -benchmem ./internal/repository
```

---

## API Specification

The HTTP transport is served on port `:8080` using Chi router.

### `GET /cache/{key}`

Retrieves an entry by key via read-through caching.

#### Parameters

| Name | Type | In | Required | Description |
| :--- | :--- | :--- | :--- | :--- |
| `key` | `string` | path | Yes | Unique cache key identifier. |

#### Responses

- `200 OK`: Key found (returned from memory or backfilled from PostgreSQL).
- `510 Not Extended` / `404 Not Found`: Key does not exist in cache or database.

#### Example Request & Response

```bash
curl -i -X GET http://localhost:8080/cache/user:101
```

```http
HTTP/1.1 200 OK
Content-Type: application/json
Date: Sun, 06 Sep 2026 10:00:00 GMT
Content-Length: 35

{
  "data": {
    "name": "Ada Lovelace",
    "role": "Engineer"
  }
}
```

---

### `PUT /cache/{key}`

Upserts an entry into PostgreSQL and hydrates/updates the in-memory LRU cache.

#### Parameters

| Name | Type | In | Required | Description |
| :--- | :--- | :--- | :--- | :--- |
| `key` | `string` | path | Yes | Unique cache key identifier. |

#### Request Body

- `Content-Type: application/json`
- Schema: `{"data": <arbitrary_json_payload>}`

#### Responses

- `204 No Content`: Entry successfully persisted to database and cache.
- `400 Bad Request`: Malformed JSON payload.
- `500 Internal Server Error`: Persistence failure.

#### Example Request & Response

```bash
curl -i -X PUT http://localhost:8080/cache/user:101 \
  -H "Content-Type: application/json" \
  -d '{
    "data": {
      "name": "Ada Lovelace",
      "role": "Engineer"
    }
  }'
```

```http
HTTP/1.1 204 No Content
Date: Sun, 06 Sep 2026 10:00:01 GMT
```

---

## Getting Started

### Prerequisites

- **Go**: Version `1.22+` (or latest stable)
- **PostgreSQL**: Version `14+` (or Docker installed)
- **Git**

### Environment Configuration

Copy the example environment template:

```bash
cp .env.example .env
```

| Variable | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `PORT` | `int` | `8080` | Port on which the HTTP server listens. |
| `DATABASE_URL` | `string` | — | PostgreSQL connection URI (e.g. `postgres://user:pass@host:5432/dbname?sslmode=disable`). |

### Local PostgreSQL Setup

You can provision a PostgreSQL instance instantly with the provided Docker Compose configuration:

```bash
# Start PostgreSQL in detached mode
docker compose up -d

# Verify container health
docker compose ps
```

### Database Migrations

Execute the migration runner to apply SQL schemas to the database:

```bash
go run cmd/migrate/main.go
```

Output:
```text
Migrations applied successfully
```

The database schema initializes the following table:

```sql
CREATE TABLE IF NOT EXISTS cache_entries (
    key TEXT PRIMARY KEY,
    data BYTEA NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);
```

### Starting the Server

Run the API service:

```bash
go run cmd/server/main.go
```

Output:
```text
server running on :8080
```

Verify service operation by putting and getting an entry:

```bash
# Store entry
curl -i -X PUT http://localhost:8080/cache/service:health \
  -H "Content-Type: application/json" \
  -d '{"data":{"status":"healthy","uptime_pct":99.99}}'

# Read entry back
curl -i -X GET http://localhost:8080/cache/service:health
```

---

## Project Layout

Follows standard Go project layout conventions:

```text
shared-in-memory-index/
├── cmd/
│   ├── migrate/
│   │   └── main.go          # Database migration entrypoint
│   └── server/
│       └── main.go          # HTTP server bootstrap & dependency wiring
├── internal/
│   ├── cache/               # In-memory dual-indexed storage engine
│   │   ├── cache.go         # Cache API: Get, Put, Delete, mutex protection
│   │   ├── cache_bench_test.go # High-concurrency performance benchmarks
│   │   ├── cache_test.go    # Unit tests & LRU invariant verification
│   │   ├── entry.go         # Memory entry & heap node struct definitions
│   │   └── heap.go          # Custom binary min-heap with O(1) swap tracking
│   ├── database/            # pgxpool connection management
│   │   └── database.go
│   ├── domain/              # Core domain entities
│   │   └── domain.go
│   ├── dto/                 # Request & response transport definitions
│   │   └── dto.go
│   ├── handler/             # HTTP routing & JSON marshalling handlers
│   │   └── handler.go
│   ├── repository/          # PostgreSQL data access layer & SQL queries
│   │   ├── repository.go
│   │   └── repository_bench_test.go
│   └── service/             # Read-through caching coordination service
│       └── cacheService.go
├── migrations/              # Up/down database migration SQL files
│   ├── 000001_create_cache_entries_table.up.sql
│   └── 000001_create_cache_entries_table.down.sql
├── docker-compose.yml       # Local development PostgreSQL container definition
├── .env.example             # Configuration environment variable template
├── go.mod                   # Go module definition
├── go.sum                   # Checksums for direct and transitive dependencies
└── README.md                # System documentation & technical specifications
```

---

## Testing & Quality Assurance

### Unit Testing & Race Detection

All cache data structure invariants (LRU eviction ordering, bidirectional `HeapIndex` validity, duplicate key replacements, and deletion from arbitrary heap indices) are rigorously validated.

Execute tests with Go's race detector:

```bash
go test -v -race ./internal/cache
```

Expected result:
```text
=== RUN   TestCachePutGet
--- PASS: TestCachePutGet (0.00s)
=== RUN   TestCacheGetMissing
--- PASS: TestCacheGetMissing (0.00s)
=== RUN   TestCachePutExisting
--- PASS: TestCachePutExisting (0.00s)
=== RUN   TestCacheDelete
--- PASS: TestCacheDelete (0.00s)
=== RUN   TestCacheDeleteMissing
--- PASS: TestCacheDeleteMissing (0.00s)
=== RUN   TestCacheLRUEviction
--- PASS: TestCacheLRUEviction (0.00s)
=== RUN   TestHeapIndex
--- PASS: TestHeapIndex (0.00s)
=== RUN   TestCacheDeleteMiddle
--- PASS: TestCacheDeleteMiddle (0.00s)
PASS
ok  	github.com/Aneeshie/shared-in-memory-index/internal/cache	1.05s
```

---

## Production Readiness & Engineering Trade-Offs

In high-scale production systems, design choices involve trade-offs between architectural simplicity, consistency, and concurrency scalability.

### Lock Contention & Cache Sharding

- **Current Architecture**: A single `sync.Mutex` guards the global map and min-heap. This guarantees strict, deterministic global LRU ordering and invariant safety.
- **High-Concurrency Bottleneck**: When CPU core counts increase (e.g. 32–64 vCPUs) and workloads exceed $10^7$ req/sec, lock contention on the global mutex becomes the primary throughput ceiling.
- **Production Solution — Striped / Sharded Cache**:
  Partition keys into $K$ distinct shards using FNV-1a or Murmur3 hash:
  $$\text{shardIndex} = \text{hash}(\text{key}) \pmod K$$
  Each shard manages an independent mutex, map, and min-heap. This divides lock contention by a factor of $K$ at the expense of localized (approximate) rather than globally uniform LRU eviction.

### Cache Stampede Mitigation (SingleFlight)

- **The Problem (Thundering Herd)**: Under high traffic, if a popular key evicts or expires, thousands of concurrent requests will register a cache miss simultaneously. Each goroutine will fall through to PostgreSQL, exhausting connection pool limits (`pgxpool.Pool`) and degrading database responsiveness.
- **Production Solution**: Integrate Go's [`golang.org/x/sync/singleflight`](https://pkg.go.dev/golang.org/x/sync/singleflight) inside `CacheService.GetData()`. `singleflight.Group` coalesces concurrent duplicate calls into a single in-flight DB query, sharing the result across all callers.

### Time-To-Live (TTL) & Eviction Strategies

- **Current Behavior**: Entries persist until evicted by capacity pressure.
- **Production Solution**:
  1. **Passive Eviction**: Check `expires_at < time.Now()` on `Get()`. If expired, treat as a miss and delete asynchronously.
  2. **Active Reaper Goroutine**: Periodically sample heap roots or maintain a second min-heap keyed by expiration timestamp to continuously prune stale memory.
  3. **Frequency-Aware Eviction (W-TinyLFU)**: Strict LRU is susceptible to cache pollution during sequential scans. Modern systems (e.g., Caffeine, Ristretto) incorporate TinyLFU admission policies to weigh frequency against recency.

### Distributed Consistency & Invalidation

- **Current Topology**: Local in-process cache. In a multi-replica deployment (Kubernetes Pods $A, B, C$), a write to Pod $A$ updates Pod $A$'s cache and PostgreSQL, but Pods $B$ and $C$ serve stale in-memory data until restarted or overridden.
- **Production Solutions**:
  - **PostgreSQL `LISTEN` / `NOTIFY`**: Service instances subscribe to a WAL notification channel on `cache_entries` updates to broadcast local eviction signals.
  - **Pub/Sub Bus (Redis / Kafka)**: Emit cache invalidation messages (`CacheInvalidateEvent{Key: "..."}`) to purge replicas.
  - **Read-Through Distributed Cache**: Adopt Redis or Memcached as a shared second-tier cluster behind the application.

### Observability & Health Probes

For operational readiness in containerized environments (Kubernetes, AWS ECS):
- **Metrics**: Expose Prometheus `/metrics` detailing:
  - Cache Hit / Miss Ratio: `rate(cache_hits_total[1m]) / rate(cache_requests_total[1m])`
  - Eviction Count: `counter_cache_evictions_total`
  - Lock Acquisition Latency: `histogram_cache_lock_wait_seconds`
  - Memory Footprint & Node Count: `gauge_cache_items`
- **Health Probes**: Implement `/healthz` (liveness: process responsive) and `/readyz` (readiness: PostgreSQL connection pool ping successful).
- **Graceful Shutdown**: Intercept `SIGINT` / `SIGTERM` via `os/signal` to flush in-flight HTTP requests and cleanly call `dbPool.Close()` with context deadlines.

---

## License

This project is licensed under the MIT License — see the [LICENSE](LICENSE) file for details.
