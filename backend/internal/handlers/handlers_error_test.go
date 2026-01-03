package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/Lzdlh1/music-backend/internal/config"
	"github.com/Lzdlh1/music-backend/internal/models"
	"github.com/Lzdlh1/music-backend/internal/services"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Ensure that when upload/download fails, the error message is persisted
func TestTaskErrorMessagePersisted(t *testing.T) {
	// test download server
	fileServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))
	defer fileServer.Close()

	// failing rclone script
	f, err := os.CreateTemp("", "fake-rclone-fail-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString("#!/bin/sh\necho 'ERROR upload failed' >&2\nexit 1\n")
	f.Chmod(0755)
	f.Close()

	old := services.RcloneCmd
	services.RcloneCmd = f.Name()
	defer func() { services.RcloneCmd = old }()

	cfg := &config.Config{SecretKey: "test-secret", RcloneRemote: "myremote:base", Port: "12233", FrontendOrigin: "http://localhost:12233"}
	dbConn, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	if err := dbConn.AutoMigrate(&models.User{}, &models.Task{}); err != nil {
		t.Fatalf("automigrate failed: %v", err)
	}

	r := gin.Default()
	h := &Handler{DB: dbConn, Cfg: cfg}
	r.POST("/api/register", h.Register)
	r.POST("/api/login", h.Login)
	r.POST("/api/tasks", AuthMiddleware(cfg.SecretKey), h.CreateTask)
	r.GET("/api/tasks", AuthMiddleware(cfg.SecretKey), h.ListTasks)

	// register & login
	regBody := map[string]string{"username": "erruser", "password": "password"}
	b, _ := json.Marshal(regBody)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/register", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("register failed: %d %s", w.Code, w.Body.String())
	}

	// login
	w = httptest.NewRecorder()
	b, _ = json.Marshal(regBody)
	req = httptest.NewRequest("POST", "/api/login", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", w.Code, w.Body.String())
	}
	var lr map[string]string
	json.Unmarshal(w.Body.Bytes(), &lr)
	token := lr["token"]
	if token == "" {
		t.Fatal("token missing")
	}

	// create task
	w = httptest.NewRecorder()
	payload := map[string]string{"title": "errtask", "url": fileServer.URL, "cookie": "CM=abc123"}
	b, _ = json.Marshal(payload)
	req = httptest.NewRequest("POST", "/api/tasks", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("create task failed: %d %s", w.Code, w.Body.String())
	}

	// wait for background worker to complete
	tout := time.After(5 * time.Second)
	for {
		select {
		case <-tout:
			t.Fatalf("task didn't finish in time")
		default:
			w = httptest.NewRecorder()
			req = httptest.NewRequest("GET", "/api/tasks", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			r.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("list tasks failed: %d %s", w.Code, w.Body.String())
			}
			var tasks []map[string]interface{}
			json.Unmarshal(w.Body.Bytes(), &tasks)
			if len(tasks) == 0 {
				t.Fatalf("no tasks returned")
			}
			if tasks[0]["status"] == "failed" {
				if em, ok := tasks[0]["error_message"].(string); !ok || em == "" {
					t.Fatalf("expected error_message to be set, got: %v", tasks[0]["error_message"])
				}
				return
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
}
