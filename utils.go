// util.go
package agendadistribuida

import (
	"context"
	"net/http"
	"strconv"
	"time"
)

// -----------------------------
// Context helpers para UserID
// -----------------------------

type ctxKeyUserID struct{}

func SetUserContext(ctx context.Context, userID int64) context.Context {
	return context.WithValue(ctx, ctxKeyUserID{}, userID)
}

func GetUserIDFromContext(ctx context.Context) (int64, bool) {
	uid, ok := ctx.Value(ctxKeyUserID{}).(int64)
	return uid, ok
}

// -----------------------------
// Parse helpers
// -----------------------------

// parseID convierte string a int64 con fallback 0
func parseID(s string) int64 {
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return id
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
