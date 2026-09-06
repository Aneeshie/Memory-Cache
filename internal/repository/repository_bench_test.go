package repository

import (
	"context"
	"os"
	"testing"

	"github.com/Aneeshie/shared-in-memory-index/internal/database"

	"github.com/joho/godotenv"
)

func BenchmarkGetCachedEntry(b *testing.B) {
	if err := godotenv.Load("../../.env"); err != nil {
		b.Fatal(err)
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		b.Fatal("DATABASE_URL not set")
	}

	ctx := context.Background()

	pool, err := database.NewDB(ctx, dbURL)
	if err != nil {
		b.Fatal(err)
	}
	defer pool.Close()

	repo := NewRepository(pool)

	key := "user:101"

	//do not include the setup time
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		entry, err := repo.GetCachedEntry(ctx, key)
		if err != nil {
			b.Fatal(err)
		}

		if entry == nil {
			b.Fatal("entry is nil")
		}
	}
}
