package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/Lzdlh1/music-backend/internal/config"
	"github.com/Lzdlh1/music-backend/internal/models"
	"github.com/Lzdlh1/music-backend/internal/services"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRegisterLoginCreateTaskFlow(t *testing.T) {
	// small HTTP server to serve download content
	fileServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("ok"))
	}))
	defer fileServer.Close()

	// fake rclone script
	script, err := os.CreateTemp("", "fake-rclone-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(script.Name())
	script.WriteString("#!/bin/sh\necho Success\nexit 0\n")
	script.Chmod(0755)
	script.Close()

	// ensure /bin/sh exists for the test
	if _, err := exec.LookPath("/bin/sh"); err != nil {
		t.Skip("/bin/sh not available")
	}

	// configure services to use fake rclone
	old := services.RcloneCmd
	services.RcloneCmd = script.Name()
	defer func() { services.RcloneCmd = old }()

	cfg := &config.Config{SecretKey: "test-secret", RcloneRemote: "myremote:base", Port: "12233", FrontendOrigin: "http://localhost:12233"}
	// init an in-memory DB
	dbConn, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	if err := dbConn.AutoMigrate(&models.User{}, &models.Task{}); err != nil {
		t.Fatalf("automigrate failed: %v", err)
	}

	// setup router
	r := gin.Default()
	h := &Handler{DB: dbConn, Cfg: cfg}
	r.POST("/api/register", h.Register)
	r.POST("/api/login", h.Login)
	r.POST("/api/tasks", AuthMiddleware(cfg.SecretKey), h.CreateTask)
	r.GET("/api/tasks", AuthMiddleware(cfg.SecretKey), h.ListTasks)

	// register
	regBody := map[string]string{"username": "user1", "password": "password"}
	b, _ := json.Marshal(regBody)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/register", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
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

	// create task (will run background worker), include cookie
	w = httptest.NewRecorder()
	payload := map[string]string{"title": "t1", "url": fileServer.URL, "cookie": "CM=abc123"}
	b, _ = json.Marshal(payload)
	req = httptest.NewRequest("POST", "/api/tasks", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("create task failed: %d %s", w.Code, w.Body.String())
	}

	// poll tasks until done or timeout
	timeout := time.After(5 * time.Second)
	for {
		select {
		case <-timeout:
			t.Fatal("task did not complete in time")
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
			if len(tasks) > 0 {
				if tasks[0]["status"] == "done" {
					return
				}
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
}
