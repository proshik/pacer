package miniapp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"
)

// A real init data string and the bot token that signed it, published at
// https://docs.telegram-mini-apps.com/platform/init-data. It anchors the
// algorithm to Telegram's output rather than to this package's own idea of it.
const (
	exampleToken    = "5768337691:AAH5YkoiEuPk8-FZa32hStHTqXiLPtAEhx8"
	exampleInitData = "query_id=AAHdF6IQAAAAAN0XohDhrOrc" +
		"&user=%7B%22id%22%3A279058397%2C%22first_name%22%3A%22Vladislav%22%2C%22last_name%22%3A%22Kibenko%22" +
		"%2C%22username%22%3A%22vdkfrost%22%2C%22language_code%22%3A%22ru%22%2C%22is_premium%22%3Atrue%7D" +
		"&auth_date=1662771648" +
		"&hash=c501b71e775f74ce10e377dea85a7ea24ecd640b223ea86dfe453e0eaed2e2b2"
)

// The example was signed in 2022, so every test passes its own clock.
var exampleSignedAt = time.Unix(1662771648, 0)

func TestValidateInitDataAcceptsPublishedExample(t *testing.T) {
	data, err := ValidateInitData(exampleInitData, exampleToken, time.Hour, exampleSignedAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("published example rejected: %v", err)
	}

	user := data.User
	if user.ID != 279058397 || user.FirstName != "Vladislav" || user.Username != "vdkfrost" ||
		user.LanguageCode != "ru" || !user.IsPremium {
		t.Errorf("user = %+v, want the published Vladislav Kibenko", user)
	}

	if !data.AuthDate.Equal(exampleSignedAt) {
		t.Errorf("auth date = %v, want %v", data.AuthDate, exampleSignedAt)
	}

	if data.QueryID != "AAHdF6IQAAAAAN0XohDhrOrc" {
		t.Errorf("query id = %q", data.QueryID)
	}
}

func TestValidateInitDataRejectsAnotherBotsToken(t *testing.T) {
	_, err := ValidateInitData(exampleInitData, "123456:another-bot", time.Hour, exampleSignedAt)

	if !errors.Is(err, ErrInvalidHash) {
		t.Errorf("err = %v, want %v", err, ErrInvalidHash)
	}
}

func TestValidateInitDataRejectsTamperedField(t *testing.T) {
	tampered := strings.Replace(exampleInitData, "279058397", "279058398", 1)

	_, err := ValidateInitData(tampered, exampleToken, time.Hour, exampleSignedAt)

	if !errors.Is(err, ErrInvalidHash) {
		t.Errorf("err = %v, want %v", err, ErrInvalidHash)
	}
}

func TestValidateInitDataRejectsStaleData(t *testing.T) {
	_, err := ValidateInitData(exampleInitData, exampleToken, time.Hour, exampleSignedAt.Add(2*time.Hour))

	if !errors.Is(err, ErrExpired) {
		t.Errorf("err = %v, want %v", err, ErrExpired)
	}
}

func TestValidateInitDataRejectsMissingHash(t *testing.T) {
	withoutHash := exampleInitData[:strings.Index(exampleInitData, "&hash=")]

	_, err := ValidateInitData(withoutHash, exampleToken, time.Hour, exampleSignedAt)

	if !errors.Is(err, ErrMissingHash) {
		t.Errorf("err = %v, want %v", err, ErrMissingHash)
	}
}

func TestValidateInitDataRejectsMalformedInput(t *testing.T) {
	for name, raw := range map[string]string{
		"broken escape":     "user=%zz&auth_date=1662771648&hash=00",
		"missing auth date": "user=%7B%22id%22%3A1%7D&hash=00",
	} {
		if _, err := ValidateInitData(raw, exampleToken, time.Hour, exampleSignedAt); !errors.Is(err, ErrMalformed) {
			t.Errorf("%s: err = %v, want %v", name, err, ErrMalformed)
		}
	}
}

// sign produces init data the way Telegram does. It is an independent
// reference for cases the published example cannot cover; the example above
// keeps both implementations honest.
func sign(token string, fields map[string]string) string {
	secret := hmac.New(sha256.New, []byte("WebAppData"))
	secret.Write([]byte(token))

	pairs := make([]string, 0, len(fields))
	values := url.Values{}
	for key, value := range fields {
		pairs = append(pairs, key+"="+value)
		values.Set(key, value)
	}
	sort.Strings(pairs)

	mac := hmac.New(sha256.New, secret.Sum(nil))
	mac.Write([]byte(strings.Join(pairs, "\n")))
	values.Set("hash", hex.EncodeToString(mac.Sum(nil)))

	return values.Encode()
}

// The go-telegram/bot validator unescaped values that url.Values had already
// decoded, so a name containing "+" or "%" never validated.
func TestValidateInitDataKeepsPlusAndPercentInValues(t *testing.T) {
	raw := sign(exampleToken, map[string]string{
		"auth_date": "1662771648",
		"user":      `{"id":1,"first_name":"C++ 100%"}`,
	})

	data, err := ValidateInitData(raw, exampleToken, time.Hour, exampleSignedAt)
	if err != nil {
		t.Fatalf("validly signed data rejected: %v", err)
	}

	if data.User.FirstName != "C++ 100%" {
		t.Errorf("first name = %q, want %q", data.User.FirstName, "C++ 100%")
	}
}

// Telegram adds a "signature" field for Ed25519 checks by third parties (Bot
// API 8.0). For the bot-token hash it is an ordinary field: the official
// wording covers "all received fields", and init-data-golang, which modern
// clients exercise constantly, skips only "hash". Only a real launch inside
// Telegram can confirm this for certain.
func TestValidateInitDataTreatsSignatureAsOrdinaryField(t *testing.T) {
	raw := sign(exampleToken, map[string]string{
		"auth_date": "1662771648",
		"user":      `{"id":1,"first_name":"Ann"}`,
		"signature": "c2lnbmF0dXJl",
	})

	if _, err := ValidateInitData(raw, exampleToken, time.Hour, exampleSignedAt); err != nil {
		t.Fatalf("data signed over its signature field rejected: %v", err)
	}
}
