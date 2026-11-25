package kimiwork

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

const (
	maxStoragePointerBytes = 64 * 1024
	maxKeysFileBytes       = 1024 * 1024
	maxDesktopConfigBytes  = 8 * 1024 * 1024
)

var (
	deviceIDPattern    = regexp.MustCompile(`^[\x21-\x7e]{1,128}$`)
	windowsPathPattern = regexp.MustCompile(`^(?:[A-Za-z]:[\\/]|\\\\)`)
)

type DesktopPathOptions struct {
	Home     string
	Platform string
	AppData  string
	Read     func(string) ([]byte, error)
}

type DesktopSession struct {
	WorkToken  string
	WebToken   string
	WebRefresh string
	DeviceID   string
	UID        string
	Keys       map[string]string
	Away       bool
}

type DesktopError struct {
	Code         string
	SignIn       string
	Message      string
	File         string
	Missing      bool
	ShareMissing bool
}

func (err *DesktopError) Error() string { return err.Message }

func expiredError(file string, missing bool) *DesktopError {
	return &DesktopError{
		Code:    "kimi_work_login_expired",
		SignIn:  "expired",
		Message: "Kimi Work sign-in has expired. Sign in to Kimi desktop Work if needed, then reconnect Kimi Work (magpie plugin login kimi-work).",
		File:    file,
		Missing: missing,
	}
}

func shareMissingError(dir string) *DesktopError {
	return &DesktopError{
		Code:         "kimi_work_share_missing",
		SignIn:       "kept",
		Message:      "Kimi Work 的数据目录不存在：" + dir + "。如果移动或删除过它，把它放回原处即可恢复，无需重新登录；否则请完全退出并重新打开 Kimi 桌面版（它会重建这个目录），再登录 Work。",
		File:         dir,
		ShareMissing: true,
	}
}

func notHereError(revoked bool) *DesktopError {
	msg := "这个 Kimi Work 账号已不是 Kimi 桌面版当前登录的账号，本机也没有它的 Work 密钥。请在 Kimi 桌面版切换到这个账号进入一次 Work，之后切回别的账号它也能继续用。"
	if revoked {
		msg = "这个 Kimi Work 账号在本机的 Work 密钥已失效。请在 Kimi 桌面版切换到这个账号进入一次 Work，Kimi 会换一把新的。"
	}
	return &DesktopError{
		Code:    "kimi_work_account_not_here",
		SignIn:  "expired",
		Message: msg,
	}
}

func stripBOM(data []byte) []byte {
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		return data[3:]
	}
	return data
}

func readSmallFile(filePath string, limit int64) ([]byte, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, fmt.Errorf("file too large or not regular")
	}
	return os.ReadFile(filePath)
}

func joinPlatform(platform string, elem ...string) string {
	if platform == "win32" {
		joined := strings.Join(elem, `\`)
		cleaned := cleanWinPath(joined)
		return cleaned
	}
	return path.Join(elem...)
}

func cleanWinPath(raw string) string {
	normalized := strings.ReplaceAll(raw, "/", `\`)
	unc := strings.HasPrefix(normalized, `\\`)
	prefix := ""
	rest := normalized
	if unc {
		prefix = `\\`
		rest = strings.TrimLeft(normalized, `\`)
	} else if len(normalized) >= 2 && normalized[1] == ':' {
		prefix = normalized[:2]
		rest = normalized[2:]
	}
	posixClean := path.Clean("/" + strings.ReplaceAll(rest, `\`, "/"))
	if unc {
		return prefix + strings.ReplaceAll(strings.TrimPrefix(posixClean, "/"), "/", `\`)
	}
	if prefix != "" {
		return prefix + strings.ReplaceAll(posixClean, "/", `\`)
	}
	return strings.ReplaceAll(strings.TrimPrefix(posixClean, "/"), "/", `\`)
}

func isAbsPlatform(platform, value string) bool {
	if platform == "win32" {
		return windowsPathPattern.MatchString(value)
	}
	return strings.HasPrefix(value, "/")
}

func UserDataDir(opts DesktopPathOptions) string {
	platform := opts.Platform
	if platform == "" {
		platform = runtime.GOOS
		if platform == "windows" {
			platform = "win32"
		}
	}
	home := opts.Home
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	appdata := opts.AppData
	if appdata == "" && opts.Home == "" {
		appdata = os.Getenv("APPDATA")
	}
	var parent string
	switch platform {
	case "win32":
		if appdata != "" {
			parent = appdata
		} else {
			parent = joinPlatform(platform, home, "AppData", "Roaming")
		}
	case "darwin":
		parent = joinPlatform(platform, home, "Library", "Application Support")
	default:
		parent = joinPlatform(platform, home, ".config")
	}
	return joinPlatform(platform, parent, "kimi-desktop")
}

func ShareDir(opts DesktopPathOptions) string {
	platform := opts.Platform
	if platform == "" {
		platform = runtime.GOOS
		if platform == "windows" {
			platform = "win32"
		}
	}
	dataDir := UserDataDir(opts)
	readFn := opts.Read
	if readFn == nil {
		readFn = func(filePath string) ([]byte, error) {
			return readSmallFile(filePath, maxStoragePointerBytes)
		}
	}
	pointerPath := joinPlatform(platform, dataDir, "daimon-storage.json")
	if content, err := readFn(pointerPath); err == nil {
		var pointer struct {
			ShareDir string `json:"shareDir"`
		}
		if json.Unmarshal(stripBOM(content), &pointer) == nil {
			trimmed := strings.TrimSpace(pointer.ShareDir)
			if trimmed != "" && isAbsPlatform(platform, trimmed) {
				if platform == "win32" {
					return cleanWinPath(trimmed)
				}
				return path.Clean(trimmed)
			}
		}
	}
	return joinPlatform(platform, dataDir, "daimon-share")
}

func DesktopFile(opts DesktopPathOptions) string {
	platform := opts.Platform
	if platform == "" {
		platform = runtime.GOOS
		if platform == "windows" {
			platform = "win32"
		}
	}
	return joinPlatform(platform, ShareDir(opts), "daimon", "config.json")
}

func ReadDeviceID(configFile string) string {
	devicePath := filepath.Join(filepath.Dir(configFile), "runtime", "kimi-code", "home", "device_id")
	data, err := readSmallFile(devicePath, 4096)
	if err != nil {
		return ""
	}
	trimmed := strings.TrimSpace(string(data))
	if !deviceIDPattern.MatchString(trimmed) {
		return ""
	}
	return trimmed
}

func ReadKeys(keysFile string) map[string]string {
	out := map[string]string{}
	data, err := readSmallFile(keysFile, maxKeysFileBytes)
	if err != nil {
		return out
	}
	var doc struct {
		Version int `json:"v"`
		Keys    []struct {
			UserID string `json:"userId"`
			APIKey string `json:"apiKey"`
		} `json:"keys"`
	}
	if json.Unmarshal(stripBOM(data), &doc) != nil || doc.Version != 2 {
		return out
	}
	for _, entry := range doc.Keys {
		if entry.UserID == "" || entry.APIKey == "" {
			continue
		}
		uid := AccountUID(entry.UserID)
		if _, exists := out[uid]; !exists {
			out[uid] = entry.APIKey
		}
	}
	return out
}

func readDesktop(configFile, expectedBase string) (DesktopSession, error) {
	daimonDir := filepath.Dir(configFile)
	shareDir := filepath.Dir(daimonDir)
	if _, err := os.Stat(configFile); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if _, shareErr := os.Stat(shareDir); errors.Is(shareErr, os.ErrNotExist) {
				return DesktopSession{}, shareMissingError(shareDir)
			}
			return DesktopSession{}, expiredError(configFile, true)
		}
		return DesktopSession{}, expiredError(configFile, false)
	}
	data, err := readSmallFile(configFile, maxDesktopConfigBytes)
	if err != nil {
		return DesktopSession{}, expiredError(configFile, false)
	}
	var doc struct {
		Credentials struct {
			KimiCode struct {
				APIKey  string `json:"apiKey"`
				BaseURL string `json:"baseUrl"`
			} `json:"kimiCode"`
			KimiWeb struct {
				AccessToken  string `json:"accessToken"`
				RefreshToken string `json:"refreshToken"`
				UserID       string `json:"userId"`
			} `json:"kimiWeb"`
		} `json:"credentials"`
	}
	if json.Unmarshal(stripBOM(data), &doc) != nil {
		return DesktopSession{}, expiredError(configFile, false)
	}
	work := doc.Credentials.KimiCode
	web := doc.Credentials.KimiWeb
	if work.APIKey == "" || web.AccessToken == "" || web.UserID == "" ||
		strings.TrimRight(work.BaseURL, "/") != strings.TrimRight(expectedBase, "/") {
		return DesktopSession{}, expiredError(configFile, false)
	}
	return DesktopSession{
		WorkToken:  work.APIKey,
		WebToken:   web.AccessToken,
		WebRefresh: web.RefreshToken,
		DeviceID:   ReadDeviceID(configFile),
		UID:        AccountUID(web.UserID),
		Keys:       ReadKeys(filepath.Join(daimonDir, "kimi-code-key.json")),
	}, nil
}
