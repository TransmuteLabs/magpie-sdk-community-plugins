package controller

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const MaxJournalBytes = 4 << 20

var ErrJournal = errors.New("ZCode durable controller state is invalid or unavailable")

type Execution struct {
	Account    string            `json:"account"`
	Login      string            `json:"login"`
	Generation uint64            `json:"generation"`
	Task       string            `json:"task"`
	Lane       string            `json:"lane"`
	Model      string            `json:"model"`
	Operation  string            `json:"operation"`
	Reference  string            `json:"reference"`
	State      string            `json:"state"`
	Ticket     string            `json:"ticket,omitempty"`
	Card       string            `json:"card,omitempty"`
	ExpiresAt  int64             `json:"expiresAt,omitempty"`
	InFlight   string            `json:"inFlight,omitempty"`
	Outcomes   map[string]string `json:"outcomes,omitempty"`
}

type Journal struct {
	Schema         int                      `json:"schema"`
	Principal      string                   `json:"principal"`
	Executions     map[string]Execution     `json:"executions"`
	Claims         map[string]Claim         `json:"claims"`
	Reset          map[string]Reset         `json:"reset"`
	DrawAfter      map[string]int64         `json:"drawAfter,omitempty"`
	Checkin        *Checkin                 `json:"checkin,omitempty"`
	Native         map[string]NativeCommand `json:"native,omitempty"`
	ClaimHoldUntil int64                    `json:"claimHoldUntil,omitempty"`
	ClaimHoldDelay int64                    `json:"claimHoldDelay,omitempty"`
}

type Checkin struct {
	Operation string          `json:"operation"`
	Until     int64           `json:"until"`
	Delay     int64           `json:"delay,omitempty"`
	Pending   bool            `json:"pending,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
}

type Claim struct {
	Plan          string          `json:"plan"`
	Operation     string          `json:"operation"`
	Reference     string          `json:"reference"`
	State         string          `json:"state"`
	Generation    uint64          `json:"generation"`
	Baseline      []string        `json:"baseline,omitempty"`
	BaselineKnown bool            `json:"baselineKnown"`
	Result        json.RawMessage `json:"result,omitempty"`
	EndsAt        int64           `json:"endsAt,omitempty"`
	RetryAt       int64           `json:"retryAt,omitempty"`
}

type Reset struct {
	FiveHigh         int64       `json:"fiveHigh,omitempty"`
	WeekHigh         int64       `json:"weekHigh,omitempty"`
	Pending          *ResetClaim `json:"pending,omitempty"`
	OpportunityKey   string      `json:"opportunityKey,omitempty"`
	OpportunityAfter int64       `json:"opportunityAfter,omitempty"`
	AckPending       bool        `json:"ackPending,omitempty"`
}

type ResetClaim struct {
	Key        string `json:"key"`
	Window     string `json:"window"`
	Operation  string `json:"operation"`
	Generation uint64 `json:"generation"`
	Baseline   int64  `json:"baseline"`
	HighWater  int64  `json:"highWater"`
	ObservedAt int64  `json:"observedAt,omitempty"`
	Accepted   bool   `json:"accepted,omitempty"`
}

type Store struct {
	mu     sync.Mutex
	root   *os.Root
	closed bool
}

type Transaction struct {
	Journal   *Journal
	root      *os.Root
	name      string
	principal string
}

func (transaction *Transaction) MergeClaimHold(nowMillis, untilMillis, delayMillis int64) int64 {
	journal := transaction.Journal
	if untilMillis > journal.ClaimHoldUntil {
		journal.ClaimHoldUntil, journal.ClaimHoldDelay = untilMillis, delayMillis
	} else if untilMillis == 0 && journal.ClaimHoldUntil <= nowMillis {
		journal.ClaimHoldUntil, journal.ClaimHoldDelay = 0, 0
	}
	return journal.ClaimHoldUntil
}

func Open(path string) (*Store, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, ErrJournal
	}
	if err := os.MkdirAll(path, 0700); err != nil {
		return nil, ErrJournal
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return nil, ErrJournal
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, ErrJournal
	}
	return &Store{root: root}, nil
}

func Principal(origin, user string) string {
	digest := sha256.Sum256([]byte(origin + "\x00" + user))
	return hex.EncodeToString(digest[:])
}

func EntryKey(account, task string) string {
	digest := sha256.Sum256([]byte(account + "\x00" + task))
	return hex.EncodeToString(digest[:])
}

func (store *Store) Close() error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.closed {
		return nil
	}
	store.closed = true
	return store.root.Close()
}

func validHash(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && strings.ToLower(value) == value
}

func (store *Store) Locked(ctx context.Context, principal string, run func(*Transaction) error) error {
	if !validHash(principal) {
		return ErrJournal
	}
	store.mu.Lock()
	if store.closed {
		store.mu.Unlock()
		return ErrJournal
	}
	lock, err := store.root.OpenFile(principal+".lock", os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if errors.Is(err, os.ErrExist) {
		lock, err = store.root.OpenFile(principal+".lock", os.O_RDWR, 0600)
	}
	store.mu.Unlock()
	if err != nil {
		return ErrJournal
	}
	defer lock.Close()
	info, err := lock.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return ErrJournal
	}
	unlock, err := acquireLock(ctx, lock)
	if err != nil {
		return err
	}
	defer unlock()
	name := principal + ".json"
	journal := &Journal{Schema: 1, Principal: principal, Executions: map[string]Execution{}, Claims: map[string]Claim{},
		Reset: map[string]Reset{}, DrawAfter: map[string]int64{}}
	input, err := store.root.Open(name)
	if err == nil {
		defer input.Close()
		info, statErr := input.Stat()
		if statErr != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > MaxJournalBytes {
			return ErrJournal
		}
		data, readErr := io.ReadAll(io.LimitReader(input, MaxJournalBytes+1))
		if readErr != nil || len(data) > MaxJournalBytes {
			return ErrJournal
		}
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if decoder.Decode(journal) != nil || decoder.Decode(new(any)) != io.EOF {
			return ErrJournal
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return ErrJournal
	}
	if journal.validate(principal) != nil {
		return ErrJournal
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return run(&Transaction{Journal: journal, root: store.root, name: name, principal: principal})
}

func (journal *Journal) validate(principal string) error {
	if journal.Schema != 1 || journal.Principal != principal ||
		len(journal.Executions) > 4096 || len(journal.Claims) > 1024 ||
		journal.Executions == nil || journal.Claims == nil || journal.Reset == nil || journal.DrawAfter == nil {
		return ErrJournal
	}
	if err := validateNative(journal.Native); err != nil {
		return err
	}
	if value := journal.Checkin; value != nil && (value.Operation == "" || value.Until <= 0 || value.Delay < 0 ||
		!value.Pending && len(value.Result) == 0) {
		return ErrJournal
	}
	if journal.ClaimHoldUntil < 0 || journal.ClaimHoldDelay < 0 {
		return ErrJournal
	}
	for key, value := range journal.Executions {
		if key != EntryKey(value.Account, value.Task) || value.Account == "" || value.Login == "" || value.Generation == 0 ||
			value.Task == "" || value.Model == "" || value.Operation == "" || value.Reference == "" ||
			value.Lane != "offpeak" && value.Lane != "highspeed" || value.ExpiresAt < 0 {
			return ErrJournal
		}
		switch value.State {
		case "pending", "unknown", "queued", "ready", "active", "settling", "settled", "expired", "not_found", "abandoned":
		default:
			return ErrJournal
		}
		if len(value.Outcomes) > 2048 {
			return ErrJournal
		}
		for operation, outcome := range value.Outcomes {
			if operation == "" || outcome != "pending" && outcome != "uncertain" && outcome != "completed" {
				return ErrJournal
			}
		}
		if value.InFlight != "" && value.Outcomes[value.InFlight] != "pending" && value.Outcomes[value.InFlight] != "uncertain" {
			return ErrJournal
		}
	}
	for key, value := range journal.Claims {
		if key != value.Plan || value.Plan == "" || value.Operation == "" || value.Reference == "" || value.Generation == 0 || value.EndsAt < 0 || value.RetryAt < 0 {
			return ErrJournal
		}
		switch value.State {
		case "pending", "unknown", "confirmed", "observed", "refused":
		default:
			return ErrJournal
		}
	}
	for account, reset := range journal.Reset {
		if account == "" || reset.FiveHigh < 0 || reset.WeekHigh < 0 || reset.OpportunityAfter < 0 {
			return ErrJournal
		}
		if pending := reset.Pending; pending != nil {
			if pending.Key == "" || pending.Operation == "" || pending.Generation == 0 ||
				pending.Window != "FIVE_HOUR" && pending.Window != "WEEK" ||
				pending.Baseline < 0 || pending.HighWater < 0 || pending.ObservedAt < 0 {
				return ErrJournal
			}
		}
	}
	for account, after := range journal.DrawAfter {
		if account == "" || after < 0 {
			return ErrJournal
		}
	}
	return nil
}

func (transaction *Transaction) Save() error {
	if transaction.Journal.validate(transaction.principal) != nil {
		return ErrJournal
	}
	data, err := json.Marshal(transaction.Journal)
	if err != nil || len(data) > MaxJournalBytes {
		return ErrJournal
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return ErrJournal
	}
	name := "." + hex.EncodeToString(nonce[:]) + ".tmp"
	file, err := transaction.root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return ErrJournal
	}
	defer transaction.root.Remove(name)
	if _, err := file.Write(data); err != nil {
		file.Close()
		return ErrJournal
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return ErrJournal
	}
	if file.Close() != nil || transaction.root.Rename(name, transaction.name) != nil {
		return ErrJournal
	}
	directory, err := transaction.root.Open(".")
	if err != nil {
		return ErrJournal
	}
	defer directory.Close()
	if directory.Sync() != nil {
		return ErrJournal
	}
	return nil
}
