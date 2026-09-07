package handler

import (
	"encoding/json"
	"net/http"
	"runtime"
)

// Version is set from main via SetVersion.
var (
	appVersion   = "dev"
	appCommit    = "unknown"
	appBuildTime = "unknown"
)

func SetVersion(version, commit, buildTime string) {
	appVersion = version
	appCommit = commit
	appBuildTime = buildTime
}

type ErrorBody struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
	Details any    `json:"details,omitempty"`
}

func WriteError(w http.ResponseWriter, status int, code, message string, details any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorBody{Error: code, Message: message, Details: details})
}

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func runtimePlatform() string {
	return runtime.GOOS + "/" + runtime.GOARCH
}
