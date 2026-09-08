package resolver

import (
	"strings"
)

const (
	CodeYouTubeAuth        = "youtube_auth"
	CodeYouTubeBotCheck    = "youtube_bot_check"
	CodeYouTubeUnavailable = "youtube_unavailable"
	CodeToolMissing        = "tool_missing"
)

const youtubeAuthMessage = "This YouTube video needs a signed-in session (private, members-only, or age-restricted). Export cookies from a browser where you can watch it and upload them under Stream Sources."

const youtubeBotCheckMessage = "YouTube is treating this host as a bot. Upload cookies from a browser that can play the stream, under Stream Sources."

// ToolMissingMessage is the probe/UI copy when yt-dlp is absent.
const ToolMissingMessage = "yt-dlp is not installed; install it on PATH to probe YouTube sources"

// Classify maps yt-dlp / resolver errors onto a stable code and a sentence
// the UI can show. Raw extractor dumps stay in logs.
func Classify(msg string) (code, user string) {
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "yt-dlp is not installed"):
		return CodeToolMissing, ToolMissingMessage
	case isYouTubeBotCheck(lower):
		return CodeYouTubeBotCheck, youtubeBotCheckMessage
	case isYouTubeAuth(lower):
		return CodeYouTubeAuth, youtubeAuthMessage
	case isYouTubeUnavailable(lower):
		return CodeYouTubeUnavailable, unavailableMessage(msg)
	default:
		return "", ""
	}
}

func isYouTubeBotCheck(lower string) bool {
	return strings.Contains(lower, "you're not a bot") ||
		strings.Contains(lower, "you’re not a bot") ||
		strings.Contains(lower, "sign in to confirm you're not") ||
		strings.Contains(lower, "sign in to confirm you’re not")
}

func isYouTubeAuth(lower string) bool {
	return strings.Contains(lower, "please sign in") ||
		strings.Contains(lower, "use --cookies") ||
		strings.Contains(lower, "--cookies-from-browser") ||
		strings.Contains(lower, "members-only") ||
		strings.Contains(lower, "members only") ||
		strings.Contains(lower, "join this channel") ||
		strings.Contains(lower, "age-restricted") ||
		strings.Contains(lower, "sign in to confirm your age") ||
		strings.Contains(lower, "login required")
}

func isYouTubeUnavailable(lower string) bool {
	return strings.Contains(lower, "private video") ||
		strings.Contains(lower, "this video is private") ||
		strings.Contains(lower, "video unavailable") ||
		strings.Contains(lower, "has been removed") ||
		strings.Contains(lower, "not currently live") ||
		strings.Contains(lower, "premiere will begin")
}

func unavailableMessage(msg string) string {
	msg = strings.TrimSpace(msg)
	if i := strings.LastIndex(msg, "ERROR:"); i >= 0 {
		msg = strings.TrimSpace(msg[i+6:])
	}
	if len(msg) > 240 {
		msg = msg[:240] + "..."
	}
	if msg == "" {
		return "This YouTube video is unavailable."
	}
	return msg
}
