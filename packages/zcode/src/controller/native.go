package controller

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"unicode"
)

type NativeCommand struct {
	Account     string `json:"account"`
	Generation  uint64 `json:"generation"`
	Operation   string `json:"operation"`
	Workspace   string `json:"workspace"`
	Session     string `json:"session"`
	Command     string `json:"command"`
	RequestHash string `json:"requestHash"`
	State       string `json:"state"`
	Terminal    string `json:"terminal,omitempty"`
}

func NativeKey(workspace, session, command string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{workspace, session, command}, "\x00")))
	return hex.EncodeToString(sum[:])
}

func validateNative(commands map[string]NativeCommand) error {
	if len(commands) > 1024 {
		return ErrJournal
	}
	for key, command := range commands {
		if !validHash(command.Workspace) || !validHash(command.RequestHash) ||
			key != NativeKey(command.Workspace, command.Session, command.Command) ||
			command.Account == "" || command.Generation == 0 || command.Operation == "" ||
			!nativeIdentity(command.Session) || !nativeIdentity(command.Command) {
			return ErrJournal
		}
		if command.State == "pending" && command.Terminal == "" {
			continue
		}
		if command.State != "completed" || command.Terminal != "completedSuccess" &&
			command.Terminal != "completedInterrupted" && command.Terminal != "failed" {
			return ErrJournal
		}
	}
	return nil
}

func nativeIdentity(value string) bool {
	return value != "" && len(value) <= 256 && !strings.ContainsFunc(value, unicode.IsControl)
}
