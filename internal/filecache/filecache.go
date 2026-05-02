package filecache

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mudittt/validatasaurus/internal/platform"
)

const cacheRoot = "/tmp/validatasaurus"

// FetchWithCache returns cached SQL files for the given URL if already
// downloaded, otherwise calls fetch, persists the results, and returns them.
func FetchWithCache(ticketURL string, fetch func(string) ([]platform.SQLFile, error)) ([]platform.SQLFile, error) {
	url := strings.TrimSpace(ticketURL)
	dir := cacheDir(url)

	if files, ok := loadFromCache(dir); ok {
		return files, nil
	}

	files, err := fetch(url)
	if err != nil {
		return nil, err
	}

	_ = saveToCache(dir, files) // best-effort; don't fail the caller if cache write fails
	return files, nil
}

func cacheDir(url string) string {
	sum := sha256.Sum256([]byte(url))
	return filepath.Join(cacheRoot, fmt.Sprintf("%x", sum))
}

func loadFromCache(dir string) ([]platform.SQLFile, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) == 0 {
		return nil, false
	}

	var files []platform.SQLFile
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, false
		}
		files = append(files, platform.SQLFile{Name: e.Name(), Content: content})
	}
	if len(files) == 0 {
		return nil, false
	}
	return files, true
}

func saveToCache(dir string, files []platform.SQLFile) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, f := range files {
		path := filepath.Join(dir, filepath.Base(f.Name))
		if err := os.WriteFile(path, f.Content, 0o644); err != nil {
			return err
		}
	}
	return nil
}
