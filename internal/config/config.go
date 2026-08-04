package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const EnvHome = "PROJECTDOCK_HOME"

func DataDir() (string, error) {
	if custom := os.Getenv(EnvHome); custom != "" {
		if !filepath.IsAbs(custom) {
			return "", fmt.Errorf("%s 必须是绝对路径", EnvHome)
		}
		return filepath.Clean(custom), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("读取用户目录: %w", err)
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Application Support", "ProjectDock"), nil
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("读取配置目录: %w", err)
	}
	return filepath.Join(configDir, "ProjectDock"), nil
}

func ProjectRegistryPath() string {
	if configured, exists := os.LookupEnv("PROJECTDOCK_PROJECTS_FILE"); exists {
		configured = strings.TrimSpace(configured)
		if configured == "" || configured == "-" || strings.EqualFold(configured, "off") {
			return ""
		}
		if absolute, err := filepath.Abs(configured); err == nil {
			return absolute
		}
		return configured
	}
	return ""
}
