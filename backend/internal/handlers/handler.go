package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Lzdlh1/music-backend/internal/config"
	"github.com/Lzdlh1/music-backend/internal/models"
	"github.com/Lzdlh1/music-backend/internal/services"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"log"
)

type Handler struct {
	DB  *gorm.DB
	Cfg *config.Config
}

// Register request
type registerReq struct {
	Username string `json:"username" binding:"required,min=3"`
	Password string `json:"password" binding:"required,min=6"`
}

func (h *Handler) Register(c *gin.Context) {
	var req registerReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	pw, _ := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	user := &models.User{Username: req.Username, PasswordHash: string(pw)}
	if err := h.DB.Create(user).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username taken or invalid"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": user.ID, "username": user.Username})
}

// Login request
type loginReq struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (h *Handler) Login(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var user models.User
	if err := h.DB.First(&user, "username = ?", req.Username).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}
	// create token
	token, err := GenerateJWT(user.ID, h.Cfg.SecretKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token creation failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": token})
}

// Create task: accepts optional `cookie` field but DOES NOT persist it to DB
type createTaskReq struct {
	Title  string `json:"title" binding:"required"`
	URL    string `json:"url" binding:"required,url"`
	Cookie string `json:"cookie"` // optional, will only be used for this request
}

func (h *Handler) CreateTask(c *gin.Context) {
	log.Printf("CreateTask called")
	var req createTaskReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	userID := c.GetUint("user_id")
	task := &models.Task{Title: req.Title, URL: req.URL, Status: "queued", OwnerID: userID}
	if err := h.DB.Create(task).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create task"})
		return
	}


	// helper to run the task
	run := func(t *models.Task, cookie string) {
		// update status
		h.DB.Model(t).Updates(map[string]interface{}{"status": "running", "updated_at": time.Now(), "error_message": ""})

		// create temp file
		log.Printf("task %d: starting download %s", t.ID, t.URL)
		fpath, err := services.DownloadToTemp(t.URL, cookie)
		if err != nil {
			log.Printf("task %d: download failed: %v", t.ID, err)
			h.DB.Model(t).Updates(map[string]interface{}{"status": "failed", "error_message": err.Error(), "updated_at": time.Now()})
			return
		}
		defer os.Remove(fpath)

		// upload with rclone
		log.Printf("task %d: uploading %s to %s", t.ID, fpath, h.Cfg.RcloneRemote)
		if err := services.RcloneCopy(fpath, h.Cfg.RcloneRemote); err != nil {
			log.Printf("task %d: upload failed: %v", t.ID, err)
			h.DB.Model(t).Updates(map[string]interface{}{"status": "failed", "error_message": err.Error(), "updated_at": time.Now()})
			return
		}

		log.Printf("task %d: done", t.ID)
		h.DB.Model(t).Updates(map[string]interface{}{"status": "done", "updated_at": time.Now()})
	}

	// launch background worker
	log.Printf("task %d: launching background worker", task.ID)
	go run(task, req.Cookie)

	c.JSON(http.StatusAccepted, gin.H{"id": task.ID, "status": task.Status})
}

// RetryTask accepts optional { cookie } body; only owner may retry a failed task
func (h *Handler) RetryTask(c *gin.Context) {
	id := c.Param("id")
	var task models.Task
	if err := h.DB.First(&task, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	userID := c.GetUint("user_id")
	if task.OwnerID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	if task.Status != "failed" && task.Status != "done" {
		// allow retry if failed; if already done, also allow re-run (user choice)
		c.JSON(http.StatusBadRequest, gin.H{"error": "task not retryable"})
		return
	}
	var body struct{ Cookie string `json:"cookie"` }
	if err := c.ShouldBindJSON(&body); err != nil {
		// ignore missing body (no cookie provided)
		var syntaxErr *json.UnmarshalTypeError
		if err != nil && !errors.As(err, &syntaxErr) {
			// if it's not a missing body error, return
			if !strings.Contains(err.Error(), "EOF") {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
		}
	}
	// update status and clear error
	h.DB.Model(&task).Updates(map[string]interface{}{"status": "queued", "error_message": "", "updated_at": time.Now()})
	// re-run in background with provided cookie (if any)
	go func(t *models.Task, cookie string) {
		h.DB.Model(t).Updates(map[string]interface{}{"status": "running", "updated_at": time.Now(), "error_message": ""})
		fpath, err := services.DownloadToTemp(t.URL, cookie)
		if err != nil {
			h.DB.Model(t).Updates(map[string]interface{}{"status": "failed", "error_message": err.Error(), "updated_at": time.Now()})
			return
		}
		defer os.Remove(fpath)
		if err := services.RcloneCopy(fpath, h.Cfg.RcloneRemote); err != nil {
			h.DB.Model(t).Updates(map[string]interface{}{"status": "failed", "error_message": err.Error(), "updated_at": time.Now()})
			return
		}
		h.DB.Model(t).Updates(map[string]interface{}{"status": "done", "updated_at": time.Now()})
	}(&task, body.Cookie)
	c.JSON(http.StatusAccepted, gin.H{"id": task.ID, "status": "queued"})
}

func (h *Handler) ListTasks(c *gin.Context) {
	userID := c.GetUint("user_id")
	var tasks []models.Task
	h.DB.Where("owner_id = ?", userID).Order("created_at desc").Find(&tasks)
	c.JSON(http.StatusOK, tasks)
}

func (h *Handler) GetTask(c *gin.Context) {
	userID := c.GetUint("user_id")
	var task models.Task
	if err := h.DB.First(&task, c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if task.OwnerID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	c.JSON(http.StatusOK, task)
}
