package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRunProjectActionViaAPIUsesDesktopServiceToken(t *testing.T) {
	var receivedToken string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/health":
			_, _ = writer.Write([]byte(`{"service":"projectdock","status":"ok","version":"0.10.1"}`))
		case "/api/session":
			_, _ = writer.Write([]byte(`{"token":"test-token"}`))
		case "/api/projects/demo-app/start":
			receivedToken = request.Header.Get("X-ProjectDock-Token")
			_, _ = writer.Write([]byte(`{"projectId":"demo-app","state":"running","pid":42}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	originalURL := projectDockAPIBaseURL
	projectDockAPIBaseURL = server.URL
	defer func() { projectDockAPIBaseURL = originalURL }()

	status, err := runProjectActionViaAPI("start", "demo-app")
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "running" || status.PID != 42 || receivedToken != "test-token" {
		t.Fatalf("unexpected API result: status=%#v token=%q", status, receivedToken)
	}
}

func TestRunProjectActionViaAPIRejectsMismatchedServiceVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"service":"projectdock","status":"ok","version":"0.9.2"}`))
	}))
	defer server.Close()

	originalURL := projectDockAPIBaseURL
	projectDockAPIBaseURL = server.URL
	defer func() { projectDockAPIBaseURL = originalURL }()

	_, err := runProjectActionViaAPI("start", "demo-app")
	if err == nil || !strings.Contains(err.Error(), "版本不匹配") {
		t.Fatalf("expected version mismatch, got %v", err)
	}
}

func TestRunProjectActionViaAPIRequiresProjectDockIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"service":"other","status":"ok"}`))
	}))
	defer server.Close()

	originalURL := projectDockAPIBaseURL
	projectDockAPIBaseURL = server.URL
	defer func() { projectDockAPIBaseURL = originalURL }()

	_, err := runProjectActionViaAPI("start", "demo-app")
	if err == nil || !strings.Contains(err.Error(), "不是可用的 ProjectDock") {
		t.Fatalf("expected identity error, got %v", err)
	}
}
