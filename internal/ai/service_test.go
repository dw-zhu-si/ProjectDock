package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type memorySecrets struct{ value string }

func (m *memorySecrets) Get(context.Context) (string, error) { return m.value, nil }
func (m *memorySecrets) Set(_ context.Context, value string) error {
	m.value = value
	return nil
}

func TestSaveDoesNotWriteAPIKeyIntoSettingsFile(t *testing.T) {
	secrets := &memorySecrets{}
	path := filepath.Join(t.TempDir(), "ai-settings.json")
	service := NewServiceWithDependencies(path, secrets, http.DefaultClient)
	settings, err := service.Save(context.Background(), Settings{BaseURL: "https://api.example.com/v1", Model: "test-model"}, "top-secret")
	if err != nil {
		t.Fatal(err)
	}
	if !settings.Configured || secrets.value != "top-secret" {
		t.Fatalf("unexpected saved settings: %#v", settings)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "top-secret") {
		t.Fatal("settings file contains API key")
	}
}

func TestAnalyzeRepositoryUsesOpenAICompatibleJSONResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/chat/completions" || request.Header.Get("Authorization") != "" {
			t.Fatalf("unexpected request: %s %q", request.URL.Path, request.Header.Get("Authorization"))
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": `{"summary":"demo","runtime":"Go","setupSteps":["安装 Go"],"warnings":[]}`}}}})
	}))
	defer server.Close()
	secrets := &memorySecrets{}
	service := NewServiceWithDependencies(filepath.Join(t.TempDir(), "settings.json"), secrets, server.Client())
	if _, err := service.Save(context.Background(), Settings{BaseURL: server.URL + "/v1", Model: "demo"}, ""); err != nil {
		t.Fatal(err)
	}
	analysis, err := service.AnalyzeRepository(context.Background(), "https://github.com/a/b", "go.mod")
	if err != nil {
		t.Fatal(err)
	}
	if analysis.Summary != "demo" || analysis.Runtime != "Go" {
		t.Fatalf("unexpected analysis: %#v", analysis)
	}
}

func TestVerifyMarksRejectedCredentialsAsFailedAndNotUsable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusUnauthorized)
		_, _ = writer.Write([]byte(`{"error":{"message":"secret upstream detail"}}`))
	}))
	defer server.Close()

	secrets := &memorySecrets{}
	service := NewServiceWithDependencies(filepath.Join(t.TempDir(), "settings.json"), secrets, server.Client())
	if _, err := service.Save(context.Background(), Settings{BaseURL: server.URL + "/v1", Model: "demo"}, "test-key"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Verify(context.Background()); err == nil || !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("expected friendly 401 verification failure, got %v", err)
	}
	settings, err := service.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if settings.Usable || settings.VerificationStatus != "failed" {
		t.Fatalf("rejected credentials must not be usable: %#v", settings)
	}
	if strings.Contains(settings.VerificationMessage, "secret upstream detail") {
		t.Fatalf("verification status leaked upstream response: %#v", settings)
	}
}

func TestVerifySuccessPersistsAndSettingsChangeInvalidatesIt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]string{"content": "OK"}}},
		})
	}))
	defer server.Close()

	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	secrets := &memorySecrets{}
	service := NewServiceWithDependencies(settingsPath, secrets, server.Client())
	if _, err := service.Save(context.Background(), Settings{BaseURL: server.URL + "/v1", Model: "demo"}, ""); err != nil {
		t.Fatal(err)
	}
	verified, err := service.Verify(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !verified.Usable || verified.VerificationStatus != "verified" || verified.VerifiedAt == "" {
		t.Fatalf("unexpected verified settings: %#v", verified)
	}

	reloaded := NewServiceWithDependencies(settingsPath, secrets, server.Client())
	persisted, err := reloaded.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !persisted.Usable || persisted.VerificationStatus != "verified" {
		t.Fatalf("verification did not persist: %#v", persisted)
	}
	changed, err := reloaded.Save(context.Background(), Settings{BaseURL: server.URL + "/v1", Model: "another-model"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if changed.Usable || changed.VerificationStatus != "unverified" {
		t.Fatalf("changed settings must require re-verification: %#v", changed)
	}
}

func TestVerifyReturnsErrorWhenRemoteAPIKeyIsMissing(t *testing.T) {
	secrets := &memorySecrets{}
	service := NewServiceWithDependencies(filepath.Join(t.TempDir(), "settings.json"), secrets, http.DefaultClient)
	if _, err := service.Save(context.Background(), Settings{BaseURL: "https://api.example.com/v1", Model: "demo"}, ""); err != nil {
		t.Fatal(err)
	}
	settings, err := service.Verify(context.Background())
	if err == nil || !strings.Contains(err.Error(), "API 密钥") {
		t.Fatalf("expected missing key verification error, got settings=%#v err=%v", settings, err)
	}
	if settings.Usable || settings.VerificationStatus != "failed" {
		t.Fatalf("missing key must remain unusable: %#v", settings)
	}
}

func TestQwenModelCannotUseOfficialOpenAIEndpoint(t *testing.T) {
	secrets := &memorySecrets{value: "test-key"}
	path := filepath.Join(t.TempDir(), "settings.json")
	service := NewServiceWithDependencies(path, secrets, http.DefaultClient)
	_, err := service.Save(context.Background(), Settings{
		BaseURL: "https://api.openai.com/v1",
		Model:   "qwen3.6-35b-a3b",
	}, "")
	if err == nil || !strings.Contains(err.Error(), "配置不匹配") {
		t.Fatalf("expected provider/model mismatch, got %v", err)
	}
}

func TestExistingMismatchedSettingsAreReportedAsUnusable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{
  "baseUrl": "https://api.openai.com/v1",
  "model": "qwen3.6-35b-a3b",
  "verificationStatus": "verified",
  "verificationMessage": "连接验证成功",
  "verifiedAt": "2026-08-04T00:00:00Z"
}`), 0o600); err != nil {
		t.Fatal(err)
	}
	service := NewServiceWithDependencies(path, &memorySecrets{value: "test-key"}, http.DefaultClient)
	settings, err := service.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if settings.Usable || settings.VerificationStatus != "failed" || !strings.Contains(settings.VerificationMessage, "配置不匹配") {
		t.Fatalf("mismatched legacy settings must be blocked: %#v", settings)
	}
}
