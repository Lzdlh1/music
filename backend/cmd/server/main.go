package main

import (
	"log"
	"os"

	"github.com/Lzdlh1/music-backend/internal/config"
	"github.com/Lzdlh1/music-backend/internal/db"
	"github.com/Lzdlh1/music-backend/internal/handlers"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	dbConn, err := db.Init(cfg.DatabasePath)
	if err != nil {
		log.Fatalf("failed to init db: %v", err)
	}

	if err := db.AutoMigrate(dbConn); err != nil {
		log.Fatalf("failed to migrate db: %v", err)
	}

	r := gin.Default()
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{cfg.FrontendOrigin},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
	}))

	api := r.Group("/api")
	handlers.RegisterRoutes(api, dbConn, cfg)

	port := os.Getenv("PORT")
	if port == "" {
		port = cfg.Port
	}

	log.Printf("starting server on :%s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
