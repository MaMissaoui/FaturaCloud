package api

import (
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// POST /api/client-errors — the browser reports an uncaught error (a crash,
// an unhandled promise rejection) so it lands in the server log next to the
// requests around it, since the published image ships without Sentry. Any
// signed-in user may report; nothing is stored, the report is logged as an
// error-level request line (event=client_error) and dropped. Fields are
// truncated, the page URL is reduced to its path, and each user is limited
// to clientErrorLimit reports per clientErrorWindow so a render loop can't
// flood the log. Always 204, so a page never retries or reports its own
// reporting.

const (
	clientErrorLimit  = 20
	clientErrorWindow = 10 * time.Minute
)

type clientErrorReport struct {
	Message        string `json:"message"`
	Name           string `json:"name"`
	Stack          string `json:"stack"`
	ComponentStack string `json:"componentStack"`
	Source         string `json:"source"`
	URL            string `json:"url"`
	Release        string `json:"release"`
}

var clientErrorBuckets = struct {
	sync.Mutex
	byUser map[string]*clientErrorBucket
}{byUser: map[string]*clientErrorBucket{}}

type clientErrorBucket struct {
	start time.Time
	count int
}

// allowClientError counts a report against the user's window.
func allowClientError(userID string, now time.Time) bool {
	clientErrorBuckets.Lock()
	defer clientErrorBuckets.Unlock()
	b := clientErrorBuckets.byUser[userID]
	if b == nil || now.Sub(b.start) >= clientErrorWindow {
		// Drop expired windows while here, so the map stays as small as the
		// set of users reporting right now.
		for id, old := range clientErrorBuckets.byUser {
			if now.Sub(old.start) >= clientErrorWindow {
				delete(clientErrorBuckets.byUser, id)
			}
		}
		b = &clientErrorBucket{start: now}
		clientErrorBuckets.byUser[userID] = b
	}
	b.count++
	return b.count <= clientErrorLimit
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	// Cut on a rune boundary.
	for n > 0 && n < len(s) && s[n]&0xC0 == 0x80 {
		n--
	}
	return s[:n] + "…"
}

// pagePath keeps only the path of the page the error happened on: the query
// string can carry search text.
func pagePath(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return truncate(u.Path, 200)
}

func (h *handler) reportClientError(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	var report clientErrorReport
	if err := decodeJSON(w, r, &report); err != nil {
		return
	}
	claims := getClaims(r)
	if claims == nil || !allowClientError(claims.UserID, time.Now()) {
		noteRequest(r, nil, slog.String("event", "client_error_dropped"))
		w.WriteHeader(http.StatusNoContent)
		return
	}
	noteRequest(r, &levelError,
		slog.String("event", "client_error"),
		slog.String("error", truncate(report.Name+": "+report.Message, 500)),
		slog.String("source", truncate(report.Source, 40)),
		slog.String("page", pagePath(report.URL)),
		slog.String("release", truncate(report.Release, 40)),
		slog.String("browser", truncate(r.UserAgent(), 200)),
		slog.String("stack", truncate(report.Stack, 4000)),
		slog.String("component_stack", truncate(report.ComponentStack, 2000)),
	)
	w.WriteHeader(http.StatusNoContent)
}
