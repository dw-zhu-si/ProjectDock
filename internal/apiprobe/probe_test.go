package apiprobe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRejectsRemoteURL(t *testing.T) {
	service := NewService()
	_, err := service.Do(context.Background(), Request{Method: "GET", URL: "https://example.com"})
	if err == nil || !strings.Contains(err.Error(), "回环") {
		t.Fatalf("expected loopback error, got %v", err)
	}
}

func TestProbeLocalServerAndRedactsCookie(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Set-Cookie", "session=secret")
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusCreated)
		_, _ = writer.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	service := NewService()
	response, err := service.Do(context.Background(), Request{Method: "POST", URL: server.URL, Body: "{}"})
	if err != nil {
		t.Fatal(err)
	}
	if response.Status != http.StatusCreated || response.Body != `{"ok":true}` {
		t.Fatalf("unexpected response: %#v", response)
	}
	if response.Headers["Set-Cookie"][0] != "[已隐藏]" {
		t.Fatalf("cookie was not redacted: %#v", response.Headers)
	}
}

func TestRejectsAuthorizationHeader(t *testing.T) {
	service := NewService()
	_, err := service.Do(context.Background(), Request{
		Method: "GET",
		URL:    "http://127.0.0.1:1",
		Headers: map[string]string{
			"Authorization": "Bearer secret",
		},
	})
	if err == nil {
		t.Fatal("expected blocked header error")
	}
}
