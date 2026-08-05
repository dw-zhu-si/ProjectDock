package sandboxbookmark

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type Manager struct {
	mu      sync.Mutex
	path    string
	records []string
	handles []Handle
	paths   []string
}

type bookmarkFile struct {
	Bookmarks []string `json:"bookmarks"`
}

func New(dataDir string) (*Manager, error) {
	manager := &Manager{path: filepath.Join(dataDir, "security-scoped-bookmarks.json")}
	data, err := os.ReadFile(manager.path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("读取安全作用域书签: %w", err)
	}
	if len(data) == 0 {
		return manager, nil
	}
	var saved bookmarkFile
	if err := json.Unmarshal(data, &saved); err != nil {
		return nil, fmt.Errorf("解析安全作用域书签: %w", err)
	}
	for _, encoded := range saved.Bookmarks {
		raw, decodeErr := base64.StdEncoding.DecodeString(encoded)
		if decodeErr != nil {
			continue
		}
		handle, resolveErr := Resolve(raw)
		if resolveErr != nil {
			continue
		}
		manager.records = append(manager.records, encoded)
		manager.handles = append(manager.handles, handle)
		manager.paths = append(manager.paths, filepath.Clean(handle.Path()))
	}
	return manager, nil
}

func (m *Manager) Authorize(encoded string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil || len(raw) == 0 {
		return "", errors.New("安全作用域书签格式无效")
	}
	handle, err := Resolve(raw)
	if err != nil {
		return "", err
	}
	clean := filepath.Clean(handle.Path())
	info, err := os.Stat(clean)
	if err != nil || !info.IsDir() {
		handle.Close()
		return "", errors.New("授权的目录不存在或不可访问")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for index, existing := range m.paths {
		if existing == clean {
			handle.Close()
			return m.paths[index], nil
		}
	}
	m.records = append(m.records, strings.TrimSpace(encoded))
	m.handles = append(m.handles, handle)
	m.paths = append(m.paths, clean)
	if err := m.saveLocked(); err != nil {
		m.records = m.records[:len(m.records)-1]
		m.handles = m.handles[:len(m.handles)-1]
		m.paths = m.paths[:len(m.paths)-1]
		handle.Close()
		return "", err
	}
	return clean, nil
}

func (m *Manager) Allows(path string) bool {
	abs, err := filepath.Abs(strings.TrimSpace(path))
	if err != nil {
		return false
	}
	clean := filepath.Clean(abs)
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, root := range m.paths {
		relative, relErr := filepath.Rel(root, clean)
		if relErr == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, handle := range m.handles {
		handle.Close()
	}
	m.handles = nil
	m.paths = nil
}

func (m *Manager) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(m.path), 0o700); err != nil {
		return fmt.Errorf("创建书签目录: %w", err)
	}
	data, err := json.MarshalIndent(bookmarkFile{Bookmarks: m.records}, "", "  ")
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(m.path), ".bookmarks-")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, m.path); err != nil {
		return fmt.Errorf("保存安全作用域书签: %w", err)
	}
	return nil
}
