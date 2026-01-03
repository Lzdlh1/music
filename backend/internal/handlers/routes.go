package handlers

import (
	"github.com/Lzdlh1/music-backend/internal/config"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func RegisterRoutes(rg *gin.RouterGroup, gormDB *gorm.DB, cfg *config.Config) {
	h := &Handler{DB: gormDB, Cfg: cfg}

	rg.POST("/register", h.Register)
	rg.POST("/login", h.Login)

	authed := rg.Group("")
	authed.Use(AuthMiddleware(cfg.SecretKey))
	{
		authed.POST("/tasks", h.CreateTask)
		authed.POST("/tasks/:id/retry", h.RetryTask)
		authed.GET("/tasks", h.ListTasks)
		authed.GET("/tasks/:id", h.GetTask)
	}
}
