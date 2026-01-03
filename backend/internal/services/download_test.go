package services

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestDownloadToTemp_Simple(t *testing.T) {
	// serve a small test file
	h := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("hello world"))
	}))
	defer h.Close()

	path, err := DownloadToTemp(h.URL, "")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	defer os.Remove(path)
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), "hello world") {
		t.Fatalf("unexpected file contents: %s", string(b))
	}
}

func TestDownloadToTemp_ContentDisposition(t *testing.T) {
	h := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", "attachment; filename=example.txt")
		w.Write([]byte("data"))
	}))
	defer h.Close()

	path, err := DownloadToTemp(h.URL, "")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	defer os.Remove(path)
	if !strings.Contains(path, "example.txt") {
		t.Fatalf("expected filename to contain example.txt, got %s", path)
	}
}

func TestDownloadToTemp_InvalidURL(t *testing.T) {
	_, err := DownloadToTemp("http://localhost:0/nonexistent", "")
	if err == nil {
		t.Fatalf("expected error for invalid url")
	}
}
