package installer

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
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

type RepositoryInstaller interface {
	Install(context.Context, GitHubRepository, string) (string, error)
}

// ZIPInstaller avoids invoking /usr/bin/git in the Mac App Store sandbox.
// It downloads GitHub's source archive and extracts only regular files under
// the exact user-authorized installation root.
type ZIPInstaller struct {
	Client             *http.Client
	MaxCompressedBytes int64
	MaxExtractedBytes  int64
	MaxEntries         int
}

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

func (installer GitHubInstaller) Install(ctx context.Context, repository GitHubRepository, installRoot string) (string, error) {
	return installer.Clone(ctx, repository, installRoot)
}

func (installer ZIPInstaller) Install(ctx context.Context, repository GitHubRepository, installRoot string) (destination string, resultErr error) {
	root, err := canonicalDirectory(installRoot)
	if err != nil {
		return "", fmt.Errorf("安装目录无效: %w", err)
	}
	destination = filepath.Join(root, repository.Name)
	if relative, relErr := filepath.Rel(root, destination); relErr != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("目标目录离开了指定安装目录")
	}
	if _, statErr := os.Lstat(destination); !errors.Is(statErr, os.ErrNotExist) {
		if statErr == nil {
			return destination, fmt.Errorf("目标目录已存在: %s", destination)
		}
		return destination, fmt.Errorf("检查目标目录: %w", statErr)
	}

	maxCompressed := installer.MaxCompressedBytes
	if maxCompressed <= 0 {
		maxCompressed = 128 << 20
	}
	maxExtracted := installer.MaxExtractedBytes
	if maxExtracted <= 0 {
		maxExtracted = 512 << 20
	}
	maxEntries := installer.MaxEntries
	if maxEntries <= 0 {
		maxEntries = 20_000
	}
	client := installer.Client
	if client == nil {
		client = http.DefaultClient
	}

	stage, err := os.MkdirTemp(root, ".projectdock-install-")
	if err != nil {
		return destination, fmt.Errorf("创建临时安装目录: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.RemoveAll(stage)
		}
	}()
	archivePath := filepath.Join(stage, ".source.zip")
	requestURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/zipball", url.PathEscape(repository.Owner), url.PathEscape(repository.Name))
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return destination, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "ProjectDock/0.10")
	response, err := client.Do(request)
	if err != nil {
		return destination, fmt.Errorf("下载 GitHub 源码包: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
		return destination, fmt.Errorf("GitHub 返回 HTTP %d", response.StatusCode)
	}
	if response.ContentLength > maxCompressed {
		return destination, errors.New("GitHub 源码包超过 128 MiB 安全上限")
	}
	archive, err := os.OpenFile(archivePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return destination, fmt.Errorf("创建临时源码包: %w", err)
	}
	written, copyErr := io.Copy(archive, io.LimitReader(response.Body, maxCompressed+1))
	closeErr := archive.Close()
	if copyErr != nil {
		return destination, fmt.Errorf("保存 GitHub 源码包: %w", copyErr)
	}
	if closeErr != nil {
		return destination, fmt.Errorf("关闭 GitHub 源码包: %w", closeErr)
	}
	if written > maxCompressed {
		return destination, errors.New("GitHub 源码包超过 128 MiB 安全上限")
	}

	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return destination, fmt.Errorf("打开 GitHub 源码包: %w", err)
	}
	defer reader.Close()
	if len(reader.File) > maxEntries {
		return destination, fmt.Errorf("源码包条目超过 %d 个安全上限", maxEntries)
	}
	var extracted int64
	for _, entry := range reader.File {
		if strings.HasPrefix(entry.Name, "/") {
			return destination, fmt.Errorf("源码包包含不安全路径: %s", entry.Name)
		}
		parts := strings.Split(strings.TrimSuffix(entry.Name, "/"), "/")
		unsafePath := false
		for _, part := range parts {
			if part == "" || part == "." || part == ".." {
				unsafePath = true
				break
			}
		}
		if unsafePath {
			return destination, fmt.Errorf("源码包包含不安全路径: %s", entry.Name)
		}
		if len(parts) < 2 {
			continue
		}
		relative := path.Clean(strings.Join(parts[1:], "/"))
		if relative == "." || relative == ".." || strings.HasPrefix(relative, "../") || path.IsAbs(relative) {
			return destination, fmt.Errorf("源码包包含不安全路径: %s", entry.Name)
		}
		mode := entry.Mode()
		if mode&os.ModeSymlink != 0 || (!mode.IsRegular() && !mode.IsDir()) {
			return destination, fmt.Errorf("源码包包含不允许的文件类型: %s", entry.Name)
		}
		extracted += int64(entry.UncompressedSize64)
		if extracted > maxExtracted {
			return destination, errors.New("解压内容超过 512 MiB 安全上限")
		}
		target := filepath.Join(stage, filepath.FromSlash(relative))
		if rel, relErr := filepath.Rel(stage, target); relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return destination, fmt.Errorf("源码包路径离开安装目录: %s", entry.Name)
		}
		if mode.IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return destination, err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return destination, err
		}
		source, err := entry.Open()
		if err != nil {
			return destination, err
		}
		permissions := mode.Perm() & 0o755
		if permissions == 0 {
			permissions = 0o644
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, permissions)
		if err != nil {
			source.Close()
			return destination, err
		}
		_, copyErr = io.Copy(output, io.LimitReader(source, int64(entry.UncompressedSize64)+1))
		outputCloseErr := output.Close()
		sourceCloseErr := source.Close()
		if copyErr != nil || outputCloseErr != nil || sourceCloseErr != nil {
			return destination, errors.New("解压 GitHub 源码包失败")
		}
	}
	if err := os.Remove(archivePath); err != nil {
		return destination, fmt.Errorf("清理临时源码包: %w", err)
	}
	if err := os.Rename(stage, destination); err != nil {
		return destination, fmt.Errorf("提交安装目录: %w", err)
	}
	committed = true
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
