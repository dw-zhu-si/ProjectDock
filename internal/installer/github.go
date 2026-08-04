package installer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var repositoryPart = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

type GitHubRepository struct {
	Owner    string `json:"owner"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	CloneURL string `json:"-"`
}

type GitHubInstaller struct{}

func ParseGitHubURL(raw string) (GitHubRepository, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || !strings.EqualFold(parsed.Hostname(), "github.com") {
		return GitHubRepository{}, errors.New("只支持 https://github.com/所有者/仓库 格式的 GitHub 地址")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Port() != "" {
		return GitHubRepository{}, errors.New("GitHub 地址不能包含账号、端口、查询参数或片段")
	}
	parts := strings.Split(strings.Trim(parsed.EscapedPath(), "/"), "/")
	if len(parts) != 2 {
		return GitHubRepository{}, errors.New("GitHub 地址必须精确指向一个仓库")
	}
	owner, ownerErr := url.PathUnescape(parts[0])
	name, nameErr := url.PathUnescape(parts[1])
	name = strings.TrimSuffix(name, ".git")
	if ownerErr != nil || nameErr != nil || !repositoryPart.MatchString(owner) || !repositoryPart.MatchString(name) || owner == "." || owner == ".." || name == "." || name == ".." {
		return GitHubRepository{}, errors.New("GitHub 所有者或仓库名称无效")
	}
	canonical := "https://github.com/" + owner + "/" + name
	return GitHubRepository{Owner: owner, Name: name, URL: canonical, CloneURL: canonical + ".git"}, nil
}

func (GitHubInstaller) Clone(ctx context.Context, repository GitHubRepository, installRoot string) (string, error) {
	root, err := canonicalDirectory(installRoot)
	if err != nil {
		return "", fmt.Errorf("安装目录无效: %w", err)
	}
	destination := filepath.Join(root, repository.Name)
	if relative, err := filepath.Rel(root, destination); err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("目标目录离开了指定安装目录")
	}
	if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
		if err == nil {
			return "", fmt.Errorf("目标目录已存在: %s", destination)
		}
		return "", fmt.Errorf("检查目标目录: %w", err)
	}
	command := exec.CommandContext(ctx, "git", "clone", "--depth", "1", "--", repository.CloneURL, destination)
	output, err := command.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if len(message) > 1200 {
			message = message[len(message)-1200:]
		}
		if message == "" {
			message = err.Error()
		}
		return destination, fmt.Errorf("克隆 GitHub 仓库失败: %s", message)
	}
	return destination, nil
}

func canonicalDirectory(raw string) (string, error) {
	path, err := filepath.Abs(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", errors.New("目录不存在或不可访问")
	}
	return filepath.Clean(resolved), nil
}

func CollectMetadata(projectPath string) string {
	var result bytes.Buffer
	for _, name := range []string{"README.md", "README", "package.json", "go.mod", "Package.swift", "pyproject.toml", "Cargo.toml", "compose.yml", "compose.yaml", "docker-compose.yml", "Makefile", ".projectdock.json"} {
		path := filepath.Join(projectPath, name)
		info, err := os.Stat(path)
		if err != nil || info.IsDir() || info.Size() > 24<<10 {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if result.Len()+len(data) > 48<<10 {
			remaining := (48 << 10) - result.Len()
			if remaining <= 0 {
				break
			}
			data = data[:remaining]
		}
		result.WriteString("\n--- " + name + " ---\n")
		result.Write(data)
		if result.Len() >= 48<<10 {
			break
		}
	}
	if result.Len() == 0 {
		return "仓库中没有找到常见项目说明或依赖清单。"
	}
	return result.String()
}
