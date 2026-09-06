package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/Aneeshie/shared-in-memory-index/internal/cache"
	"github.com/Aneeshie/shared-in-memory-index/internal/database"
	"github.com/Aneeshie/shared-in-memory-index/internal/handler"
	"github.com/Aneeshie/shared-in-memory-index/internal/repository"
	"github.com/Aneeshie/shared-in-memory-index/internal/service"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()

	dbURL := os.Getenv("DATABASE_URL")

	if dbURL == "" {
		log.Fatal("No db url provided")
	}

	pool, err := database.NewDB(ctx, dbURL)

	if err != nil {
		log.Fatal(err)
	}

	defer pool.Close()

	repo := repository.NewRepository(pool)

	cache := cache.NewCache(5)

	service := service.NewCacheService(repo, cache)

	h := handler.NewHandler(service)

	r := chi.NewRouter()
	r.Use(middleware.Logger)

	r.Get("/cache/{key}", h.GetData)
	r.Put("/cache/{key}", h.PutData)

	log.Println("server running on :8080")

	if err := http.ListenAndServe(":8080", r); err != nil {
		log.Fatal(err)
	}

}
