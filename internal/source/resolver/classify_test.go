package resolver

import (
	"strings"
	"testing"
)

func TestClassifyYouTubeBotCheck(t *testing.T) {
	t.Parallel()
	msgs := []string{
		`yt-dlp: ERROR: [youtube] ssuM6NJQ2no: Sign in to confirm you’re not a bot. Use --cookies-from-browser or --cookies for the authentication.`,
		`ERROR: [youtube] x: Sign in to confirm you're not a bot`,
	}
	for _, msg := range msgs {
		code, user := Classify(msg)
		if code != CodeYouTubeBotCheck {
			t.Fatalf("Classify(%q) code = %q", msg, code)
		}
		if user != youtubeBotCheckMessage {
			t.Fatalf("user %q", user)
		}
		if strings.Contains(user, "ERROR:") || strings.Contains(strings.ToLower(user), "ssum6") {
			t.Fatalf("UI message leaked extractor dump: %q", user)
		}
	}
}

func TestClassifyYouTubeAuth(t *testing.T) {
	t.Parallel()
	msgs := []string{
		`Please sign in to continue`,
		`ERROR: [youtube] x: This video is members-only`,
		`ERROR: [youtube] x: Sign in to confirm your age`,
	}
	for _, msg := range msgs {
		code, user := Classify(msg)
		if code != CodeYouTubeAuth {
			t.Fatalf("Classify(%q) code = %q want %s", msg, code, CodeYouTubeAuth)
		}
		if user != youtubeAuthMessage {
			t.Fatalf("user %q", user)
		}
	}
}

func TestClassifyUnavailableAndMissing(t *testing.T) {
	t.Parallel()
	code, _ := Classify("yt-dlp is not installed")
	if code != CodeToolMissing {
		t.Fatalf("code %q", code)
	}
	code, user := Classify("ERROR: [youtube] x: Private video")
	if code != CodeYouTubeUnavailable {
		t.Fatalf("code %q", code)
	}
	if !strings.Contains(strings.ToLower(user), "private") {
		t.Fatalf("user %q", user)
	}
	code, user = Classify("Requested format is not available")
	if code != "" || user != "" {
		t.Fatalf("generic should leave the message to the caller, got %q %q", code, user)
	}
}
