package model

import "testing"

func TestValidateLoopbackURL(t *testing.T) {
	valid := []string{
		"http://127.0.0.1:5173/health",
		"http://localhost:3000/api",
		"https://[::1]:8443/status",
	}
	for _, value := range valid {
		if err := ValidateLoopbackURL(value); err != nil {
			t.Fatalf("expected %s to be valid: %v", value, err)
		}
	}
	invalid := []string{
		"https://example.com",
		"http://localhost.evil.test:3000",
		"file:///tmp/demo",
		"http://user:pass@localhost:3000",
	}
	for _, value := range invalid {
		if err := ValidateLoopbackURL(value); err == nil {
			t.Fatalf("expected %s to be rejected", value)
		}
	}
}

func TestProjectValidation(t *testing.T) {
	project := Project{
		ID: "demo-app", Name: "Demo", Path: t.TempDir(), Source: "codex",
		SyncMode: "manual", StartCommand: "echo ok", Ports: []int{5173, 5173},
	}
	if err := project.Validate(true); err == nil {
		t.Fatal("expected duplicate ports to be rejected")
	}
	project.Ports = []int{5173}
	if err := project.Validate(true); err != nil {
		t.Fatalf("expected project to be valid: %v", err)
	}
	project.StartCommand = ""
	if err := project.Validate(true); err != nil {
		t.Fatalf("expected discovered project without start command to be valid: %v", err)
	}
}
