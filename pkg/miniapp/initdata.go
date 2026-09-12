// Package miniapp authenticates requests that come from the Telegram Mini App.
package miniapp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	ErrMissingHash = errors.New("init data has no hash")
	ErrInvalidHash = errors.New("init data hash does not match")
	ErrExpired     = errors.New("init data is too old")
	ErrMalformed   = errors.New("init data is malformed")
)

// User is the Telegram account that opened the Mini App.
type User struct {
	ID           int64  `json:"id"`
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name,omitempty"`
	Username     string `json:"username,omitempty"`
	LanguageCode string `json:"language_code,omitempty"`
	IsPremium    bool   `json:"is_premium,omitempty"`
}

// InitData is the verified launch data of one Mini App session.
type InitData struct {
	User       User
	AuthDate   time.Time
	QueryID    string
	StartParam string
}

// ValidateInitData verifies raw init data — the tgWebAppData launch parameter,
// the same string as Telegram.WebApp.initData — against the bot token, as
// described at https://core.telegram.org/bots/webapps.
//
// Data signed longer than maxAge before now is rejected; a zero maxAge turns
// that check off. The clock is a parameter so callers and tests control it.
//
// Every field except hash takes part in the check, including the signature
// field Bot API 8.0 added for third-party Ed25519 validation: the official
// wording covers "all received fields", and init-data-golang skips only hash.
func ValidateInitData(raw string, botToken string, maxAge time.Duration, now time.Time) (InitData, error) {
	values, err := url.ParseQuery(raw)
	if err != nil {
		return InitData{}, fmt.Errorf("%w: %v", ErrMalformed, err)
	}

	received := values.Get("hash")
	if received == "" {
		return InitData{}, ErrMissingHash
	}

	authSeconds, err := strconv.ParseInt(values.Get("auth_date"), 10, 64)
	if err != nil {
		return InitData{}, fmt.Errorf("%w: auth_date: %v", ErrMalformed, err)
	}

	if !hashMatches(values, received, botToken) {
		return InitData{}, ErrInvalidHash
	}

	authDate := time.Unix(authSeconds, 0)
	if maxAge > 0 && now.Sub(authDate) > maxAge {
		return InitData{}, ErrExpired
	}

	data := InitData{
		AuthDate:   authDate,
		QueryID:    values.Get("query_id"),
		StartParam: values.Get("start_param"),
	}

	if rawUser := values.Get("user"); rawUser != "" {
		if err := json.Unmarshal([]byte(rawUser), &data.User); err != nil {
			return InitData{}, fmt.Errorf("%w: user: %v", ErrMalformed, err)
		}
	}

	return data, nil
}

// hashMatches rebuilds the data-check-string — key=value lines sorted by key,
// values decoded exactly once — and compares its HMAC in constant time.
func hashMatches(values url.Values, received string, botToken string) bool {
	keys := make([]string, 0, len(values))
	for key := range values {
		if key != "hash" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)

	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		lines = append(lines, key+"="+values.Get(key))
	}

	secret := hmac.New(sha256.New, []byte("WebAppData"))
	secret.Write([]byte(botToken))

	mac := hmac.New(sha256.New, secret.Sum(nil))
	mac.Write([]byte(strings.Join(lines, "\n")))

	got, err := hex.DecodeString(received)
	if err != nil {
		return false
	}

	return hmac.Equal(got, mac.Sum(nil))
}
