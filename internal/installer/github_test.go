package installer

import (
	"archive/zip"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestParseGitHubURL(t *testing.T) {
	repository, err := ParseGitHubURL("https://github.com/openai/openai-go.git")
	if err != nil {
		t.Fatal(err)
	}
	if repository.Owner != "openai" || repository.Name != "openai-go" || repository.URL != "https://github.com/openai/openai-go" {
		t.Fatalf("unexpected repository: %#v", repository)
	}
}

func TestZIPInstallerExtractsIntoExactRepositoryDirectory(t *testing.T) {
	archive := testArchive(t, map[string]string{
		"owner-repo-hash/README.md":        "hello",
		"owner-repo-hash/sub/package.json": "{}",
	})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Write(archive)
	}))
	defer server.Close()
	client := server.Client()
	client.Transport = rewriteTransport{base: client.Transport, target: server.URL}
	root := t.TempDir()
	destination, err := (ZIPInstaller{Client: client}).Install(context.Background(), GitHubRepository{Owner: "owner", Name: "repo"}, root)
	if err != nil {
		t.Fatal(err)
	}
	resolvedRoot, _ := filepath.EvalSymlinks(root)
	if destination != filepath.Join(resolvedRoot, "repo") {
		t.Fatalf("unexpected destination: %s", destination)
	}
	data, err := os.ReadFile(filepath.Join(destination, "sub", "package.json"))
	if err != nil || string(data) != "{}" {
		t.Fatalf("archive was not extracted correctly: %q, %v", data, err)
	}
}

func TestZIPInstallerRejectsUnsafeEntriesAndCleansStage(t *testing.T) {
	archive := testArchive(t, map[string]string{"owner-repo-hash/../../escape": "bad"})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) { writer.Write(archive) }))
	defer server.Close()
	client := server.Client()
	client.Transport = rewriteTransport{base: client.Transport, target: server.URL}
	root := t.TempDir()
	if _, err := (ZIPInstaller{Client: client}).Install(context.Background(), GitHubRepository{Owner: "owner", Name: "repo"}, root); err == nil {
		t.Fatal("expected unsafe archive to be rejected")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("failed install left files behind: %#v", entries)
	}
}

type rewriteTransport struct {
	base   http.RoundTripper
	target string
}

func (transport rewriteTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	target := clone.URL
	parsed, _ := http.NewRequest(http.MethodGet, transport.target, nil)
	target.Scheme = parsed.URL.Scheme
	target.Host = parsed.URL.Host
	return transport.base.RoundTrip(clone)
}

func testArchive(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var data bytes.Buffer
	writer := zip.NewWriter(&data)
	for name, body := range entries {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func TestParseGitHubURLRejectsUnsafeOrNonRepositoryURLs(t *testing.T) {
	for _, raw := range []string{
		"http://github.com/openai/openai-go",
		"https://github.com/openai/openai-go/issues",
		"https://github.com/openai/openai-go?tab=readme",
		"https://user:secret@github.com/openai/openai-go",
		"https://example.com/openai/openai-go",
	} {
		if _, err := ParseGitHubURL(raw); err == nil {
			t.Fatalf("expected %q to be rejected", raw)
		}
	}
}
