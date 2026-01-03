package services

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DownloadToTemp downloads the given URL to a temp file and returns its path.
// cookie is optional and is used only for that single request (not stored).
// The implementation does a few retries and attempts to pick a sensible filename.
func DownloadToTemp(rawurl string, cookie string) (string, error) {
	const maxAttempts = 3
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		client := &http.Client{Timeout: 120 * time.Second}
		req, err := http.NewRequest("GET", rawurl, nil)
		if err != nil {
			return "", err
		}
		if cookie != "" {
			req.Header.Set("Cookie", cookie)
		}

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			// simple backoff
			time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			lastErr = errors.New("non-2xx response")
			resp.Body.Close()
			// backoff
			time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
			continue
		}

		// determine filename from Content-Disposition or URL
		filename := "download"
		if cd := resp.Header.Get("Content-Disposition"); cd != "" {
			if _, params, err := mime.ParseMediaType(cd); err == nil {
				if v, ok := params["filename"]; ok {
					filename = v
				}
			}
		} else {
			if u, err := url.Parse(rawurl); err == nil {
				if base := filepath.Base(u.Path); base != "" && base != "/" {
					filename = base
				}
			}
		}
		// sanitize
		filename = filepath.Base(strings.Split(filename, "?")[0])

		f, err := os.CreateTemp(os.TempDir(), "music-download-*"+"-"+filename)
		if err != nil {
			return "", err
		}
		defer f.Close()

		_, err = io.Copy(f, resp.Body)
		if err != nil {
			lastErr = err
			// remove partial
			os.Remove(f.Name())
			// backoff
			time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
			continue
		}

		return f.Name(), nil
	}
	return "", lastErr
}
