package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	keychainService = "com.zhusi.ProjectDock.ai-api-key"
	keychainAccount = "ProjectDock AI"
	maxAIResponse   = 1 << 20
)

type Settings struct {
	BaseURL string `json:"baseUrl"`
	Model   string `json:"model"`
}

type PublicSettings struct {
	Settings
	Configured          bool   `json:"configured"`
	RequiresAPIKey      bool   `json:"requiresApiKey"`
	Usable              bool   `json:"usable"`
	VerificationStatus  string `json:"verificationStatus"`
	VerificationMessage string `json:"verificationMessage,omitempty"`
	VerifiedAt          string `json:"verifiedAt,omitempty"`
}

type settingsState struct {
	Settings
	VerificationStatus  string `json:"verificationStatus,omitempty"`
	VerificationMessage string `json:"verificationMessage,omitempty"`
	VerifiedAt          string `json:"verifiedAt,omitempty"`
}

type Analysis struct {
	Summary    string   `json:"summary"`
	Runtime    string   `json:"runtime"`
	SetupSteps []string `json:"setupSteps"`
	Warnings   []string `json:"warnings"`
}

type SecretStore interface {
	Get(context.Context) (string, error)
	Set(context.Context, string) error
}

type Service struct {
	settingsPath string
	secrets      SecretStore
	client       *http.Client
	settingsMu   sync.Mutex
}

func NewService(dataDir string) *Service {
	return NewServiceWithDependencies(filepath.Join(dataDir, "ai-settings.json"), KeychainStore{}, &http.Client{Timeout: 45 * time.Second})
}

func NewServiceWithDependencies(settingsPath string, secrets SecretStore, client *http.Client) *Service {
	return &Service{settingsPath: settingsPath, secrets: secrets, client: client}
}

func (s *Service) Get(ctx context.Context) (PublicSettings, error) {
	state, err := s.loadState()
	if err != nil {
		return PublicSettings{}, err
	}
	settings := state.Settings
	configured := false
	requiresKey := settings.BaseURL != "" && requiresAPIKey(settings.BaseURL)
	if settings.BaseURL != "" && settings.Model != "" {
		if requiresKey {
			key, keyErr := s.secrets.Get(ctx)
			configured = keyErr == nil && strings.TrimSpace(key) != ""
		} else {
			configured = true
		}
	}
	status := state.VerificationStatus
	if status == "" {
		status = "unverified"
	}
	message := state.VerificationMessage
	if compatibilityErr := validateSettingsCompatibility(settings); compatibilityErr != nil {
		status = "failed"
		message = compatibilityErr.Error()
	}
	return PublicSettings{
		Settings: settings, Configured: configured, RequiresAPIKey: requiresKey,
		Usable: configured && status == "verified", VerificationStatus: status,
		VerificationMessage: message, VerifiedAt: state.VerifiedAt,
	}, nil
}

func (s *Service) Save(ctx context.Context, settings Settings, apiKey string) (PublicSettings, error) {
	settings.BaseURL = strings.TrimRight(strings.TrimSpace(settings.BaseURL), "/")
	settings.Model = strings.TrimSpace(settings.Model)
	if err := validateBaseURL(settings.BaseURL); err != nil {
		return PublicSettings{}, err
	}
	if settings.Model == "" || len(settings.Model) > 160 {
		return PublicSettings{}, errors.New("AI 模型名称不能为空且不能超过 160 个字符")
	}
	if err := validateSettingsCompatibility(settings); err != nil {
		return PublicSettings{}, err
	}
	if strings.TrimSpace(apiKey) != "" {
		if err := s.secrets.Set(ctx, strings.TrimSpace(apiKey)); err != nil {
			return PublicSettings{}, fmt.Errorf("保存 AI 密钥到 macOS 钥匙串: %w", err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(s.settingsPath), 0o700); err != nil {
		return PublicSettings{}, fmt.Errorf("创建 AI 设置目录: %w", err)
	}
	state := settingsState{
		Settings: settings, VerificationStatus: "unverified",
		VerificationMessage: "配置已保存，尚未验证连接",
	}
	if err := s.saveState(state); err != nil {
		return PublicSettings{}, err
	}
	return s.Get(ctx)
}

func (s *Service) Verify(ctx context.Context) (PublicSettings, error) {
	state, err := s.loadState()
	if err != nil {
		return PublicSettings{}, err
	}
	settings := state.Settings
	if err := validateSettingsCompatibility(settings); err != nil {
		_ = s.recordVerification("failed", err.Error(), "")
		current, getErr := s.Get(ctx)
		if getErr != nil {
			return PublicSettings{}, getErr
		}
		return current, err
	}
	key, err := s.apiKey(ctx, settings)
	if err != nil {
		_ = s.recordVerification("failed", err.Error(), "")
		current, getErr := s.Get(ctx)
		if getErr != nil {
			return PublicSettings{}, getErr
		}
		return current, err
	}
	payload := map[string]any{
		"model": settings.Model,
		"messages": []map[string]string{
			{"role": "user", "content": "Reply with OK."},
		},
	}
	responseBody, err := s.chatCompletion(ctx, settings, key, payload)
	if err == nil {
		var envelope struct {
			Choices []json.RawMessage `json:"choices"`
		}
		if json.Unmarshal(responseBody, &envelope) != nil || len(envelope.Choices) == 0 {
			err = errors.New("AI 模型响应格式无效，请检查接口兼容性")
		}
	}
	if err != nil {
		_ = s.recordVerification("failed", err.Error(), "")
		settings, getErr := s.Get(ctx)
		if getErr != nil {
			return PublicSettings{}, getErr
		}
		return settings, err
	}
	if err := s.recordVerification("verified", "连接验证成功", time.Now().Format(time.RFC3339)); err != nil {
		return PublicSettings{}, err
	}
	return s.Get(ctx)
}

func (s *Service) saveState(state settingsState) error {
	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	return s.writeState(state)
}

func (s *Service) writeState(state settingsState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(s.settingsPath), "ai-settings-*.tmp")
	if err != nil {
		return fmt.Errorf("创建 AI 设置临时文件: %w", err)
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempName, s.settingsPath); err != nil {
		return fmt.Errorf("保存 AI 设置: %w", err)
	}
	return nil
}

func (s *Service) AnalyzeRepository(ctx context.Context, repositoryURL, metadata string) (Analysis, error) {
	settings, err := s.load()
	if err != nil {
		return Analysis{}, err
	}
	if err := validateSettingsCompatibility(settings); err != nil {
		return Analysis{}, err
	}
	key, err := s.apiKey(ctx, settings)
	if err != nil {
		return Analysis{}, err
	}
	if len(metadata) > 48<<10 {
		metadata = metadata[:48<<10]
	}
	prompt := "你是本地项目安装分析器。只根据提供的仓库元数据返回 JSON，不要输出 Markdown，不要生成或建议危险、提权、删除命令。字段必须是 summary(string)、runtime(string)、setupSteps(string[])、warnings(string[])。setupSteps 只写需要用户人工确认的依赖准备步骤，不会自动执行。\n仓库：" + repositoryURL + "\n元数据：\n" + metadata
	payload := map[string]any{
		"model":           settings.Model,
		"temperature":     0.1,
		"response_format": map[string]string{"type": "json_object"},
		"messages": []map[string]string{
			{"role": "system", "content": "输出严格 JSON。不得索取或复述密钥，不得声称已执行任何命令。"},
			{"role": "user", "content": prompt},
		},
	}
	responseBody, err := s.chatCompletion(ctx, settings, key, payload)
	if err != nil {
		_ = s.recordVerification("failed", err.Error(), "")
		return Analysis{}, err
	}
	var envelope struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(responseBody, &envelope); err != nil || len(envelope.Choices) == 0 {
		_ = s.recordVerification("failed", "AI 模型响应格式无效", "")
		return Analysis{}, errors.New("AI 模型响应格式无效")
	}
	var analysis Analysis
	if err := json.Unmarshal([]byte(envelope.Choices[0].Message.Content), &analysis); err != nil {
		_ = s.recordVerification("failed", "AI 模型没有返回有效的项目分析 JSON", "")
		return Analysis{}, errors.New("AI 模型没有返回有效的项目分析 JSON")
	}
	analysis.Summary = strings.TrimSpace(analysis.Summary)
	analysis.Runtime = strings.TrimSpace(analysis.Runtime)
	analysis.SetupSteps = boundedStrings(analysis.SetupSteps, 8, 500)
	analysis.Warnings = boundedStrings(analysis.Warnings, 8, 500)
	_ = s.recordVerification("verified", "连接验证成功", time.Now().Format(time.RFC3339))
	return analysis, nil
}

func (s *Service) apiKey(ctx context.Context, settings Settings) (string, error) {
	if settings.BaseURL == "" || settings.Model == "" {
		return "", errors.New("请先填写 AI 接口基础地址和模型名称")
	}
	if !requiresAPIKey(settings.BaseURL) {
		return "", nil
	}
	key, err := s.secrets.Get(ctx)
	if err != nil || strings.TrimSpace(key) == "" {
		return "", errors.New("远程 AI 接口需要有效 API 密钥")
	}
	return strings.TrimSpace(key), nil
}

func (s *Service) chatCompletion(ctx context.Context, settings Settings, key string, payload map[string]any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, settings.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if key != "" {
		request.Header.Set("Authorization", "Bearer "+key)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("调用 AI 模型失败：%w", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxAIResponse))
	if err != nil {
		return nil, fmt.Errorf("读取 AI 响应失败：%w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, aiStatusError(response.StatusCode)
	}
	return responseBody, nil
}

func aiStatusError(status int) error {
	switch status {
	case http.StatusUnauthorized:
		return errors.New("AI 鉴权失败（HTTP 401）：请重新填写有效 API 密钥")
	case http.StatusForbidden:
		return errors.New("AI 服务拒绝访问（HTTP 403）：请确认账号和模型权限")
	case http.StatusNotFound:
		return errors.New("AI 接口或模型不存在（HTTP 404）：请检查基础地址和模型名称")
	case http.StatusTooManyRequests:
		return errors.New("AI 服务限流或额度不足（HTTP 429）：请稍后重试或检查额度")
	default:
		return fmt.Errorf("AI 模型返回 HTTP %d", status)
	}
}

func (s *Service) recordVerification(status, message, verifiedAt string) error {
	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	state, err := s.readState()
	if err != nil {
		return err
	}
	state.VerificationStatus = status
	state.VerificationMessage = strings.TrimSpace(message)
	state.VerifiedAt = verifiedAt
	return s.writeState(state)
}

func (s *Service) load() (Settings, error) {
	state, err := s.loadState()
	return state.Settings, err
}

func (s *Service) loadState() (settingsState, error) {
	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	return s.readState()
}

func (s *Service) readState() (settingsState, error) {
	file, err := os.Open(s.settingsPath)
	if errors.Is(err, os.ErrNotExist) {
		return settingsState{}, nil
	}
	if err != nil {
		return settingsState{}, fmt.Errorf("打开 AI 设置: %w", err)
	}
	defer file.Close()
	var state settingsState
	decoder := json.NewDecoder(io.LimitReader(file, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return settingsState{}, fmt.Errorf("读取 AI 设置: %w", err)
	}
	return state, nil
}

func validateBaseURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" {
		return errors.New("AI 接口基础地址无效")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("AI 接口基础地址不能包含账号、查询参数或片段")
	}
	host := strings.ToLower(parsed.Hostname())
	loopback := host == "localhost" || host == "127.0.0.1" || host == "::1" || net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback()
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && loopback) {
		return errors.New("AI 接口只允许 HTTPS，或本机回环 HTTP 地址")
	}
	return nil
}

func validateSettingsCompatibility(settings Settings) error {
	parsed, err := url.Parse(settings.BaseURL)
	if err != nil {
		return nil
	}
	host := strings.ToLower(parsed.Hostname())
	model := strings.ToLower(strings.TrimSpace(settings.Model))
	if (host == "api.openai.com" || strings.HasSuffix(host, ".api.openai.com")) && strings.HasPrefix(model, "qwen") {
		return errors.New("AI 配置不匹配：千问模型不能使用 OpenAI 官方接口；请选择“阿里云百炼”或填写实际提供该模型的兼容接口")
	}
	return nil
}

func requiresAPIKey(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return true
	}
	host := strings.ToLower(parsed.Hostname())
	return !(host == "localhost" || host == "127.0.0.1" || host == "::1" || net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback())
}

func boundedStrings(values []string, limit, maxLength int) []string {
	if len(values) > limit {
		values = values[:limit]
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if len(value) > maxLength {
			value = value[:maxLength]
		}
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}

type KeychainStore struct{}

func (KeychainStore) Get(ctx context.Context) (string, error) {
	if runtime.GOOS != "darwin" {
		return "", errors.New("AI 密钥存储当前只支持 macOS 钥匙串")
	}
	output, err := exec.CommandContext(ctx, "/usr/bin/security", "find-generic-password", "-a", keychainAccount, "-s", keychainService, "-w").Output()
	if err != nil {
		return "", errors.New("macOS 钥匙串中没有 AI 密钥")
	}
	return strings.TrimSpace(string(output)), nil
}

func (KeychainStore) Set(ctx context.Context, value string) error {
	if runtime.GOOS != "darwin" {
		return errors.New("AI 密钥存储当前只支持 macOS 钥匙串")
	}
	command := exec.CommandContext(ctx, "/usr/bin/security", "add-generic-password", "-U", "-a", keychainAccount, "-s", keychainService, "-w", value)
	if err := command.Run(); err != nil {
		return errors.New("无法写入 macOS 钥匙串")
	}
	return nil
}
