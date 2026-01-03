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

// Test retrying a failed task clears error_message and re-runs to completion
func TestRetryTask(t *testing.T) {
	// test server
	fileServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))
	defer fileServer.Close()

	// failing rclone first
	fFail, err := os.CreateTemp("", "fake-rclone-fail-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(fFail.Name())
	fFail.WriteString("#!/bin/sh\necho 'ERROR upload failed' >&2\nexit 1\n")
	fFail.Chmod(0755)
	fFail.Close()
	// verify content
	if data, err := os.ReadFile(fFail.Name()); err != nil {
		t.Fatal(err)
	} else if !bytes.Contains(data, []byte("ERROR upload failed")) {
		t.Fatalf("fFail content mismatch: %s", string(data))
	}

	// then success rclone for retry
	fSucceed, err := os.CreateTemp("", "fake-rclone-ok-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(fSucceed.Name())
	fSucceed.WriteString("#!/bin/sh\necho 'OK'\nexit 0\n")
	fSucceed.Chmod(0755)
	fSucceed.Close()
	// verify content
	if data, err := os.ReadFile(fSucceed.Name()); err != nil {
		t.Fatal(err)
	} else if !bytes.Contains(data, []byte("OK")) {
		t.Fatalf("fSucceed content mismatch: %s", string(data))
	}

	// prepare a success sentinel for retry run
	succSentinelFile, err := os.CreateTemp("", "succ-sentinel-*")
	if err != nil {
		t.Fatal(err)
	}
	os.Remove(succSentinelFile.Name())
	succSentinel := succSentinelFile.Name()
	// rewrite succeed script to touch sentinel when run
	if err := os.WriteFile(fSucceed.Name(), []byte("#!/bin/sh\n echo 'OK'\n echo 'invoked' > \""+succSentinel+"\"\n exit 0\n"), 0755); err != nil {
		t.Fatalf("could not write succeed script: %v", err)
	}

	old := services.RcloneCmd
	defer func() { services.RcloneCmd = old }()

	// start with failing rclone
	services.RcloneCmd = fFail.Name()

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
	r.POST("/api/tasks/:id/retry", AuthMiddleware(cfg.SecretKey), h.RetryTask)
	r.GET("/api/tasks", AuthMiddleware(cfg.SecretKey), h.ListTasks)

	// register & login
	regBody := map[string]string{"username": "retryuser", "password": "password"}
	b, _ := json.Marshal(regBody)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/register", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("register failed: %d %s", w.Code, w.Body.String())
	}

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

	// create task (will fail)
	w = httptest.NewRecorder()
	payload := map[string]string{"title": "retrytask", "url": fileServer.URL, "cookie": "CM=abc123"}
	b, _ = json.Marshal(payload)
	req = httptest.NewRequest("POST", "/api/tasks", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("create task failed: %d %s", w.Code, w.Body.String())
	}

	// wait until the failing rclone script is actually invoked (sentinel file)
	// this avoids races where the script might not have run yet
	failSentinelFile, err := os.CreateTemp("", "fail-sentinel-*")
	if err != nil {
		t.Fatal(err)
	}
	os.Remove(failSentinelFile.Name())
	failSentinel := failSentinelFile.Name()

	// rewrite the failing script to touch the sentinel when run
	if err := os.WriteFile(fFail.Name(), []byte("#!/bin/sh\n echo 'ERROR upload failed' >&2\n echo 'invoked' > \""+failSentinel+"\"\n exit 1\n"), 0755); err != nil {
		t.Fatalf("could not write fail script: %v", err)
	}

	// wait for sentinel and for task to reach failed state
	tout := time.After(10 * time.Second)
	for {
		select {
		case <-tout:
			t.Fatalf("task didn't reach failed state in time")
		default:
			// check sentinel
			if _, err := os.Stat(failSentinel); err == nil {
				// sentinel seen; now wait for task to be marked failed
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
				status := tasks[0]["status"].(string)
				if status == "failed" {
					break
				}
			}
			time.Sleep(100 * time.Millisecond)
		}
	}

	// now switch rclone to succeed and POST retry
	services.RcloneCmd = fSucceed.Name()
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/tasks/1/retry", bytes.NewReader([]byte(`{"cookie":"CM=abc123"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("retry request failed: %d %s", w.Code, w.Body.String())
	}

	// wait until done (also wait for success sentinel)
	tout2 := time.After(10 * time.Second)
	for {
		select {
		case <-tout2:
			t.Fatalf("task didn't complete after retry in time")
		default:
			// check success sentinel
			if _, err := os.Stat(succSentinel); err == nil {
				// now check task status
				w = httptest.NewRecorder()
				req = httptest.NewRequest("GET", "/api/tasks", nil)
				req.Header.Set("Authorization", "Bearer "+token)
				r.ServeHTTP(w, req)
				var tasks []map[string]interface{}
				json.Unmarshal(w.Body.Bytes(), &tasks)
				if tasks[0]["status"] == "done" {
					if em := tasks[0]["error_message"].(string); em != "" {
						t.Fatalf("expected error_message to be cleared after retry, got: %s", em)
					}
					return
				}
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}
