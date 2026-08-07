package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"projectdock/internal/ai"
	"projectdock/internal/apiprobe"
	"projectdock/internal/model"
	"projectdock/internal/ports"
	"projectdock/internal/projects"
	"projectdock/internal/store"
	"projectdock/internal/widget"
)

type emptyScanner struct{}

func (emptyScanner) List(context.Context) ([]model.PortListener, error) {
	return []model.PortListener{}, nil
}

type countingScanner struct {
	calls atomic.Int32
}

func (scanner *countingScanner) List(context.Context) ([]model.PortListener, error) {
	scanner.calls.Add(1)
	return []model.PortListener{{Port: 43110, PID: 42, Process: "projectctl"}}, nil
}

type fakePicker struct {
	paths []string
	err   error
}

type fakeSecrets struct{ value string }

func (f *fakeSecrets) Get(context.Context) (string, error) { return f.value, nil }
func (f *fakeSecrets) Set(_ context.Context, value string) error {
	f.value = value
	return nil
}

func (f fakePicker) Pick(context.Context) ([]string, error) {
	return f.paths, f.err
}

func testServer(t *testing.T) *Server {
	t.Helper()
	return testServerWithScanner(t, emptyScanner{})
}

func testServerWithScanner(t *testing.T, scanner ports.Scanner) *Server {
	t.Helper()
	st := store.New(t.TempDir())
	portService := ports.NewService(st, scanner)
	projectService := projects.NewService(st, portService)
	result, err := New(st, portService, projectService, apiprobe.NewService(), log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func testAppStoreServer(t *testing.T, scanner ports.Scanner) *Server {
	t.Helper()
	st := store.New(t.TempDir())
	portService := ports.NewService(st, scanner)
	projectService := projects.NewService(st, portService)
	result, err := NewWithOptions(st, portService, projectService, apiprobe.NewService(), log.New(io.Discard, "", 0), Options{AppStore: true})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestAppStoreSnapshotSkipsPortScannerAndReportsCapabilities(t *testing.T) {
	scanner := &countingScanner{}
	server := testAppStoreServer(t, scanner)
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/snapshot", nil)
	request.Host = "127.0.0.1"
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	if scanner.calls.Load() != 0 {
		t.Fatal("App Store snapshot invoked the system port scanner")
	}
	var snapshot Snapshot
	if err := json.Unmarshal(response.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if !snapshot.Capabilities.AppStore || snapshot.Capabilities.PortMonitoring || snapshot.Capabilities.ProjectLifecycle || snapshot.Capabilities.FullDelete {
		t.Fatalf("unexpected App Store capabilities: %#v", snapshot.Capabilities)
	}
}

func TestAppStoreServerRejectsUnavailableOperations(t *testing.T) {
	server := testAppStoreServer(t, emptyScanner{})
	for _, test := range []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/api/ports", ""},
		{http.MethodPost, "/api/projects/demo/start", `{}`},
		{http.MethodPost, "/api/projects/demo/delete", `{"removeFiles":true,"confirmation":"demo"}`},
		{http.MethodPost, "/api/projects/sync-registry", `{}`},
	} {
		request := httptest.NewRequest(test.method, "http://127.0.0.1"+test.path, bytes.NewBufferString(test.body))
		request.Host = "127.0.0.1"
		if test.body != "" {
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("X-ProjectDock-Token", server.token)
		}
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusNotImplemented {
			t.Fatalf("%s returned %d instead of 501: %s", test.path, response.Code, response.Body.String())
		}
	}
}

func TestAppStoreServerDisablesAIAndRemoteInstall(t *testing.T) {
	server := testAppStoreServer(t, emptyScanner{})
	for _, test := range []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/api/settings/ai", ""},
		{http.MethodPut, "/api/settings/ai", `{}`},
		{http.MethodPost, "/api/settings/ai/verify", `{}`},
		{http.MethodPost, "/api/github/install", `{}`},
	} {
		request := httptest.NewRequest(test.method, "http://127.0.0.1"+test.path, bytes.NewBufferString(test.body))
		request.Host = "127.0.0.1"
		if test.body != "" {
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("X-ProjectDock-Token", server.token)
		}
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusNotFound {
			t.Fatalf("%s returned %d instead of 404: %s", test.path, response.Code, response.Body.String())
		}
	}
}

func TestReadSnapshotsShareOnePortObservation(t *testing.T) {
	scanner := &countingScanner{}
	server := testServerWithScanner(t, scanner)

	for _, path := range []string{"/api/snapshot", "/api/widget-snapshot"} {
		request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1"+path, nil)
		request.Host = "127.0.0.1"
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s returned %d: %s", path, response.Code, response.Body.String())
		}
	}
	if got := scanner.calls.Load(); got != 1 {
		t.Fatalf("dashboard and widget snapshots should share one lsof observation, got %d scans", got)
	}
}

func TestHealthAndSecurityHeaders(t *testing.T) {
	server := testServer(t)
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/health", nil)
	request.Host = "127.0.0.1"
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
	if response.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("missing CSP")
	}
	if !bytes.Contains(response.Body.Bytes(), []byte(`"version":"development"`)) {
		t.Fatalf("health response missing service version: %s", response.Body.String())
	}
}

func TestMutationRequiresToken(t *testing.T) {
	server := testServer(t)
	payload, _ := json.Marshal(model.Project{})
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/projects", bytes.NewReader(payload))
	request.Host = "127.0.0.1"
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", response.Code)
	}
}

func TestRejectsNonLoopbackHost(t *testing.T) {
	server := testServer(t)
	request := httptest.NewRequest(http.MethodGet, "http://attacker.test/api/health", nil)
	request.Host = "attacker.test"
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", response.Code)
	}
}

func TestStaticIndex(t *testing.T) {
	server := testServer(t)
	request := httptest.NewRequest(http.MethodGet, "http://localhost/", nil)
	request.Host = "localhost"
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
	if !bytes.Contains(response.Body.Bytes(), []byte("ProjectDock")) {
		t.Fatal("index does not contain product name")
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("static assets must not survive app upgrades in cache: %q", response.Header().Get("Cache-Control"))
	}
}

func TestFolderPickerImportsProject(t *testing.T) {
	server := testServer(t)
	projectPath := t.TempDir()
	server.picker = fakePicker{paths: []string{projectPath}}
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/projects/pick", bytes.NewReader([]byte(`{}`)))
	request.Host = "127.0.0.1"
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-ProjectDock-Token", server.token)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var report importReport
	if err := json.Unmarshal(response.Body.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	resolvedProjectPath, _ := filepath.EvalSymlinks(projectPath)
	if report.Imported != 1 || report.Items[0].Result.Project.Path != resolvedProjectPath {
		t.Fatalf("unexpected import report: %#v", report)
	}
}

func TestSnapshotOnlyReturnsProjectsWithLifecycleControl(t *testing.T) {
	server := testServer(t)
	ctx := context.Background()
	for _, project := range []model.Project{
		{ID: "ready-project", Name: "Ready", Path: t.TempDir(), Source: "manual", SyncMode: "manual", StartCommand: "printf ready", LaunchSource: "manual"},
		{ID: "registered-only", Name: "Registered", Path: t.TempDir(), Source: "manual", SyncMode: "manual"},
	} {
		if _, err := server.projects.Upsert(ctx, project); err != nil {
			t.Fatal(err)
		}
	}

	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/snapshot", nil)
	request.Host = "127.0.0.1"
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var snapshot Snapshot
	if err := json.Unmarshal(response.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Projects) != 1 || snapshot.Projects[0].ID != "ready-project" {
		t.Fatalf("snapshot exposed projects without lifecycle control: %#v", snapshot.Projects)
	}
}

func TestDeleteEndpointRemovesRegistrationWithoutDeletingFolder(t *testing.T) {
	server := testServer(t)
	projectPath := t.TempDir()
	marker := filepath.Join(projectPath, "keep.txt")
	if err := os.WriteFile(marker, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := server.projects.Upsert(context.Background(), model.Project{
		ID: "delete-demo", Name: "Delete demo", Path: projectPath, Source: "manual",
		SyncMode: "manual", StartCommand: "printf ready", LaunchSource: "manual",
	}); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodDelete, "http://127.0.0.1/api/projects/delete-demo", nil)
	request.Host = "127.0.0.1"
	request.Header.Set("X-ProjectDock-Token", server.token)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("delete endpoint changed project files: %v", err)
	}
	if _, err := server.projects.Get(context.Background(), "delete-demo"); err == nil {
		t.Fatal("project registration still exists after delete")
	}
	registry, err := server.store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(registry.IgnoredPaths) != 1 || registry.IgnoredPaths[0].Path != projectPath {
		t.Fatalf("delete did not create the expected ignore tombstone: %#v", registry.IgnoredPaths)
	}
}

func TestDeleteChoiceEndpointCanRemoveExactProjectDirectory(t *testing.T) {
	server := testServer(t)
	projectPath := filepath.Join(t.TempDir(), "owner", "full-delete")
	if err := os.MkdirAll(projectPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectPath, "package.json"), []byte(`{"scripts":{"dev":"vite"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := server.projects.SyncPath(context.Background(), projects.SyncInput{Path: projectPath, Name: "Full delete", Source: "manual", Revive: true})
	if err != nil {
		t.Fatal(err)
	}
	payload := bytes.NewBufferString(`{"removeFiles":true,"confirmation":"Full delete"}`)
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/projects/"+result.Project.ID+"/delete", payload)
	request.Host = "127.0.0.1"
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-ProjectDock-Token", server.token)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	if _, err := os.Stat(result.Project.Path); !os.IsNotExist(err) {
		t.Fatalf("project directory still exists: %v", err)
	}
}

func TestScanEndpointReturnsManageableProject(t *testing.T) {
	server := testServer(t)
	root := t.TempDir()
	projectPath := filepath.Join(root, "demo")
	if err := os.MkdirAll(projectPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectPath, "package.json"), []byte(`{"scripts":{"dev":"vite"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]string{"root": root})
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/projects/scan", bytes.NewReader(payload))
	request.Host = "127.0.0.1"
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-ProjectDock-Token", server.token)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"manageable":true`)) {
		t.Fatalf("unexpected scan response %d: %s", response.Code, response.Body.String())
	}
}

func TestAISettingsNeverReturnOrPersistAPIKeyInResponse(t *testing.T) {
	server := testServer(t)
	secrets := &fakeSecrets{}
	server.ai = ai.NewServiceWithDependencies(filepath.Join(t.TempDir(), "ai.json"), secrets, http.DefaultClient)
	payload := bytes.NewBufferString(`{"baseUrl":"https://api.example.com/v1","model":"demo-model","apiKey":"secret-value"}`)
	request := httptest.NewRequest(http.MethodPut, "http://127.0.0.1/api/settings/ai", payload)
	request.Host = "127.0.0.1"
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-ProjectDock-Token", server.token)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	if bytes.Contains(response.Body.Bytes(), []byte("secret-value")) {
		t.Fatal("AI settings response leaked API key")
	}
}

func TestGitHubInstallRequiresConfiguredAI(t *testing.T) {
	server := testServer(t)
	payload := bytes.NewBufferString(`{"url":"https://github.com/openai/openai-go","installRoot":"/tmp"}`)
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/github/install", payload)
	request.Host = "127.0.0.1"
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-ProjectDock-Token", server.token)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusPreconditionFailed {
		t.Fatalf("expected 412, got %d: %s", response.Code, response.Body.String())
	}
}

func TestGitHubInstallRequiresVerifiedAI(t *testing.T) {
	server := testServer(t)
	secrets := &fakeSecrets{}
	server.ai = ai.NewServiceWithDependencies(filepath.Join(t.TempDir(), "ai.json"), secrets, http.DefaultClient)
	if _, err := server.ai.Save(context.Background(), ai.Settings{BaseURL: "https://api.example.com/v1", Model: "demo-model"}, "secret-value"); err != nil {
		t.Fatal(err)
	}
	payload := bytes.NewBufferString(`{"url":"https://github.com/openai/openai-go","installRoot":"/tmp"}`)
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/github/install", payload)
	request.Host = "127.0.0.1"
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-ProjectDock-Token", server.token)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusPreconditionFailed || !bytes.Contains(response.Body.Bytes(), []byte("连接验证")) {
		t.Fatalf("expected unverified AI to block clone, got %d: %s", response.Code, response.Body.String())
	}
}

func TestAIVerificationEndpointReturnsFriendlyFailureAndPersistsStatus(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusUnauthorized)
		_, _ = writer.Write([]byte(`{"error":{"message":"do not expose this detail"}}`))
	}))
	defer upstream.Close()

	server := testServer(t)
	secrets := &fakeSecrets{}
	server.ai = ai.NewServiceWithDependencies(filepath.Join(t.TempDir(), "ai.json"), secrets, upstream.Client())
	if _, err := server.ai.Save(context.Background(), ai.Settings{BaseURL: upstream.URL + "/v1", Model: "demo-model"}, ""); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/settings/ai/verify", bytes.NewBufferString(`{}`))
	request.Host = "127.0.0.1"
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-ProjectDock-Token", server.token)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusBadGateway || !bytes.Contains(response.Body.Bytes(), []byte("HTTP 401")) {
		t.Fatalf("expected friendly 401 verification response, got %d: %s", response.Code, response.Body.String())
	}
	if bytes.Contains(response.Body.Bytes(), []byte("do not expose this detail")) {
		t.Fatalf("verification response leaked upstream body: %s", response.Body.String())
	}
	settings, err := server.ai.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if settings.Usable || settings.VerificationStatus != "failed" {
		t.Fatalf("verification failure not persisted: %#v", settings)
	}
}

func TestWidgetSnapshotUsesRedactedContract(t *testing.T) {
	server := testServer(t)
	projectPath := t.TempDir()
	_, err := server.projects.Upsert(context.Background(), model.Project{
		ID:           "secret-project",
		Name:         "组件测试项目",
		Path:         projectPath,
		Source:       "codex",
		SyncMode:     "manual",
		StartCommand: "SECRET_COMMAND",
		HealthURL:    "http://127.0.0.1:9999/secret",
		Ports:        []int{5173},
	})
	if err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/widget-snapshot", nil)
	request.Host = "127.0.0.1"
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var snapshot widget.Snapshot
	if err := json.Unmarshal(response.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.SchemaVersion != widget.SchemaVersion || snapshot.ProjectCount != 1 {
		t.Fatalf("unexpected widget snapshot: %#v", snapshot)
	}
	body := response.Body.String()
	for _, secret := range []string{projectPath, "SECRET_COMMAND", "9999/secret", "secret-project"} {
		if bytes.Contains([]byte(body), []byte(secret)) {
			t.Fatalf("widget response leaked %q: %s", secret, body)
		}
	}
}
