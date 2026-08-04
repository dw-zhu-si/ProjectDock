package installer

import "testing"

func TestParseGitHubURL(t *testing.T) {
	repository, err := ParseGitHubURL("https://github.com/openai/openai-go.git")
	if err != nil {
		t.Fatal(err)
	}
	if repository.Owner != "openai" || repository.Name != "openai-go" || repository.URL != "https://github.com/openai/openai-go" {
		t.Fatalf("unexpected repository: %#v", repository)
	}
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
