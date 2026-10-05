package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"runtime/debug"
	"strings"
	"time"
)

// Request logging: one structured line per /api request, written by
// RequestLogger once the response is done. The middleware layers below it
// fill in what they learn on the way through — authMiddleware the user,
// orgAuthorized and requireOrgMember/Role the organization and role,
// writeInternalError the error behind a 500, a handler an extra attribute —
// through the *requestInfo the logger puts in the request context.
//
// Never logged: request or response bodies (backups, uploads, clients'
// identity numbers and phones), the query string (the Cash Book search sends
// names, phones and CINs), cookies or any header. The route is the
// registered pattern ("PATCH /api/invoices/{id}/state"), not the raw path.

type requestInfoKey struct{}

// requestInfo is what a request's log line reports beyond method, route,
// status and duration. Filled by whichever layer knows each part.
type requestInfo struct {
	id     string
	userID string
	email  string
	orgID  string
	role   string
	err    error
	stack  []byte
	// level, when set, raises the line above what its status alone gives:
	// a failed login or a rejected SSO callback is a security event even
	// though its status is an ordinary 401 or redirect.
	level *slog.Level
	attrs []slog.Attr
}

func infoFrom(ctx context.Context) *requestInfo {
	info, _ := ctx.Value(requestInfoKey{}).(*requestInfo)
	return info
}

// requestID is the id of the request r belongs to, "" outside RequestLogger
// (a unit test calling a handler directly).
func requestID(r *http.Request) string {
	if info := infoFrom(r.Context()); info != nil {
		return info.id
	}
	return ""
}

// noteRequest adds attributes to r's log line, and with level set raises it
// to at least that level.
func noteRequest(r *http.Request, level *slog.Level, attrs ...slog.Attr) {
	info := infoFrom(r.Context())
	if info == nil {
		return
	}
	if level != nil && (info.level == nil || *level > *info.level) {
		info.level = level
	}
	info.attrs = append(info.attrs, attrs...)
}

var (
	levelWarn  = slog.LevelWarn
	levelError = slog.LevelError
)

// noteSecurityEvent marks r's log line as a security event (warn level):
// a failed login, a rate-limit hit, a rejected SSO callback.
func noteSecurityEvent(r *http.Request, event string, attrs ...slog.Attr) {
	noteRequest(r, &levelWarn, append([]slog.Attr{slog.String("event", event)}, attrs...)...)
}

func setRequestUser(r *http.Request, userID, email string) {
	if info := infoFrom(r.Context()); info != nil {
		info.userID, info.email = userID, email
	}
}

func setRequestOrg(r *http.Request, orgID, role string) {
	if info := infoFrom(r.Context()); info != nil {
		info.orgID, info.role = orgID, role
	}
}

// loggingResponseWriter records the status and size of a response. Unwrap
// keeps http.ResponseController (flush, deadlines) working through it.
type loggingResponseWriter struct {
	http.ResponseWriter
	info        *requestInfo
	status      int
	bytes       int64
	wroteHeader bool
}

func (lw *loggingResponseWriter) WriteHeader(status int) {
	if !lw.wroteHeader {
		lw.status, lw.wroteHeader = status, true
	}
	lw.ResponseWriter.WriteHeader(status)
}

func (lw *loggingResponseWriter) Write(b []byte) (int, error) {
	if !lw.wroteHeader {
		lw.status, lw.wroteHeader = http.StatusOK, true
	}
	n, err := lw.ResponseWriter.Write(b)
	lw.bytes += int64(n)
	return n, err
}

func (lw *loggingResponseWriter) Unwrap() http.ResponseWriter { return lw.ResponseWriter }

// requestInfoOf finds the request info behind a ResponseWriter, for the
// helpers (writeInternalError) that only get the writer.
func requestInfoOf(w http.ResponseWriter) *requestInfo {
	for w != nil {
		if lw, ok := w.(*loggingResponseWriter); ok {
			return lw.info
		}
		u, ok := w.(interface{ Unwrap() http.ResponseWriter })
		if !ok {
			return nil
		}
		w = u.Unwrap()
	}
	return nil
}

func newRequestID() string {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

// RequestLogger wraps the whole server. For every /api request it assigns a
// request id (returned as X-Request-ID), turns a panic into a logged 500, and
// writes one log line when the response is done. Static files pass straight
// through, unlogged — the server's log is a few rotated files on a Pi.
func RequestLogger(next http.Handler, trustedProxies []netip.Prefix) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		info := &requestInfo{id: newRequestID()}
		w.Header().Set("X-Request-ID", info.id)
		lw := &loggingResponseWriter{ResponseWriter: w, info: info, status: http.StatusOK}
		// The mux sets the matched pattern on the request it is given, so
		// keep this copy to read it back afterwards.
		r2 := r.WithContext(context.WithValue(r.Context(), requestInfoKey{}, info))

		defer func() {
			if p := recover(); p != nil {
				if p == http.ErrAbortHandler {
					panic(p)
				}
				info.err = fmt.Errorf("panic: %v", p)
				info.stack = debug.Stack()
				if !lw.wroteHeader {
					writeError(lw, http.StatusInternalServerError, internalErrorMessage(info.id))
				}
			}
			logRequest(r2, lw, info, time.Since(start), trustedProxies)
		}()
		next.ServeHTTP(lw, r2)
	})
}

// requestLevel picks a line's level: errors for 5xx, warnings for refusals
// (403, 429) and security events, info for changes and other client errors,
// debug for successful reads — the bulk of the traffic, off by default.
func requestLevel(method string, status int, info *requestInfo) slog.Level {
	level := slog.LevelDebug
	switch {
	case status >= 500:
		level = slog.LevelError
	case status == http.StatusForbidden || status == http.StatusTooManyRequests:
		level = slog.LevelWarn
	case status >= 400 || (method != http.MethodGet && method != http.MethodHead):
		level = slog.LevelInfo
	}
	if info.level != nil && *info.level > level {
		level = *info.level
	}
	return level
}

func logRequest(r *http.Request, lw *loggingResponseWriter, info *requestInfo, elapsed time.Duration, trustedProxies []netip.Prefix) {
	level := requestLevel(r.Method, lw.status, info)
	ctx := r.Context()
	if !slog.Default().Enabled(ctx, level) {
		return
	}
	route := r.Pattern
	if route == "" {
		route = r.Method + " (unmatched)"
	}
	attrs := []slog.Attr{
		slog.String("route", route),
		slog.Int("status", lw.status),
		slog.Int64("ms", elapsed.Milliseconds()),
		slog.String("request_id", info.id),
		slog.String("ip", clientIPFor(r, trustedProxies)),
	}
	if info.userID != "" {
		attrs = append(attrs, slog.String("user", info.email), slog.String("user_id", info.userID))
	}
	if info.orgID != "" {
		attrs = append(attrs, slog.String("org", info.orgID), slog.String("role", info.role))
	}
	if lw.bytes > 0 {
		attrs = append(attrs, slog.Int64("bytes", lw.bytes))
	}
	attrs = append(attrs, info.attrs...)
	if info.err != nil {
		attrs = append(attrs, slog.String("err", info.err.Error()))
	}
	if info.stack != nil {
		attrs = append(attrs, slog.String("stack", string(info.stack)))
	}
	slog.LogAttrs(ctx, level, "request", attrs...)
}

// internalErrorMessage is a 500's body: generic, plus the request id so a
// user can quote it and the matching log line can be found.
func internalErrorMessage(id string) string {
	if id == "" {
		return "internal error"
	}
	return "internal error (ref " + id + ")"
}
