package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"

	"github.com/dchote/livestream-viewer/internal/model"
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

var shortPasswordMessage = fmt.Sprintf("password must be at least %d characters", model.MinPasswordLength)

// emptyIfNil normalises a nil slice so collection responses always encode as
// `[]` rather than `null`, which clients would have to special-case.
func emptyIfNil[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}

func runtimePlatform() string {
	return runtime.GOOS + "/" + runtime.GOARCH
}
