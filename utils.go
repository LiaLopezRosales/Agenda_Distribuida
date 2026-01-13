// util.go
package agendadistribuida

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// -----------------------------
// Context helpers para UserID
// -----------------------------

type ctxKeyUserID struct{}

func SetUserContext(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, ctxKeyUserID{}, userID)
}

func GetUserIDFromContext(ctx context.Context) (string, bool) {
	uid, ok := ctx.Value(ctxKeyUserID{}).(string)
	return uid, ok
}

// -----------------------------
// Parse helpers
// -----------------------------

// parseID normaliza IDs (UUID/strings) provenientes de path.
// Para B1, los IDs son strings deterministas; si viene vacío, retorna "".
func parseID(s string) string {
	return strings.TrimSpace(s)
}

// parseTimeRange lee ?start= y ?end= en formato RFC3339
// Si no se pasan, da un rango por defecto (hoy -> +7 días)
func parseTimeRange(r *http.Request) (time.Time, time.Time) {
	q := r.URL.Query()
	now := time.Now()

	// default: agenda de hoy a +7 días
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	end := start.Add(7 * 24 * time.Hour)

	if s := q.Get("start"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			start = t
		}
	}
	if s := q.Get("end"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			end = t
		}
	}
	return start, end
}

// -----------------------------
// HMAC helpers S2S
// -----------------------------

func computeHMACSHA256Hex(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func verifyHMACSHA256Hex(body []byte, secret, hexSig string) bool {
	expect := computeHMACSHA256Hex(body, secret)
	// constant time compare
	return hmac.Equal([]byte(expect), []byte(hexSig))
}

// -----------------------------
// Deterministic IDs (B1)
// -----------------------------

// stableID returns a deterministic UUIDv5-like identifier encoded as 32 lowercase hex chars.
// We don't strictly enforce RFC4122 formatting because we only require:
// - deterministic mapping from stable keys
// - low collision probability for project usage
func stableID(kind, key string) string {
	base := strings.ToLower(strings.TrimSpace(key))
	h := sha1.Sum([]byte(kind + ":" + base))
	// Use the first 16 bytes (128 bits) of SHA1 as the ID.
	return hex.EncodeToString(h[:16])
}

func UserIDFromUsername(username string) string {
	return stableID("user", username)
}

func GroupIDFromSignature(groupType GroupType, creatorUsername, groupName string) string {
	return stableID("group", fmt.Sprintf("%s:%s:%s", groupType, creatorUsername, groupName))
}

func AppointmentIDFromSignature(ownerUsername string, groupSignature string, start, end time.Time, title string) string {
	// groupSignature should be "" for personal appointments.
	return stableID("appointment", fmt.Sprintf("%s:%s:%s:%s:%s", ownerUsername, groupSignature, start.UTC().Format(time.RFC3339), end.UTC().Format(time.RFC3339), title))
}
