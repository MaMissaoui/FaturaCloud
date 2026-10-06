package api

import (
	"context"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/MaMissaoui/fatura-cloud/db"
)

// The activity history: every route registered through NewRouter's mux is
// wrapped by auditMux.Handle, and a change (POST, PUT, PATCH, DELETE) that
// succeeds writes one audit_events row after its handler returns
// (db/audit_event.go). Nothing per handler: the user and organization come
// from the request info the auth layers fill (api/request_log.go), the
// document from the route's path and a read of the document itself, and a
// created document's id from the start of the response. Sign-in, sign-out
// and browser error reports are not changes and aren't recorded. A failed
// write is logged and never fails the request.

// auditMux registers routes on the ServeMux like Handle always did, wrapping
// each change route so it is recorded. router.go keeps calling mux.Handle,
// which the route-coverage tests read.
type auditMux struct {
	*http.ServeMux
	h *handler
}

func (m *auditMux) Handle(pattern string, next http.Handler) {
	m.ServeMux.Handle(pattern, m.h.audited(pattern, next))
}

// auditSkipped are change routes that aren't changes to anyone's data.
func auditSkipped(path string) bool {
	return strings.HasPrefix(path, "/api/auth/") || path == "/api/client-errors"
}

// auditTarget reads, from a route's path, the resource it acts on (the path
// segment naming it, "invoices") and the name of the path value holding the
// document's id ("id"), "" when the route has none (a create).
func auditTarget(path string) (resource, idParam string) {
	segs := strings.Split(strings.TrimPrefix(path, "/api/"), "/")
	rest := segs
	// /api/organizations/{orgId}/clients/... is about clients, but
	// /api/organizations/{id} is about the organization.
	if len(segs) >= 3 && segs[0] == "organizations" && isWildcard(segs[1]) {
		rest = segs[2:]
	}
	resource = rest[0]
	for _, seg := range rest[1:] {
		if isWildcard(seg) {
			return resource, strings.Trim(seg, "{}.")
		}
	}
	if resource == "organizations" && len(segs) == 2 && isWildcard(segs[1]) {
		return resource, strings.Trim(segs[1], "{}.")
	}
	return resource, ""
}

func isWildcard(seg string) bool { return strings.HasPrefix(seg, "{") }

var createdIDPattern = regexp.MustCompile(`"id"\s*:\s*"([^"]+)"`)

func (h *handler) audited(pattern string, next http.Handler) http.Handler {
	method, path := splitPattern(pattern)
	if method == "" || method == http.MethodGet || method == http.MethodHead || auditSkipped(path) {
		return next
	}
	resource, idParam := auditTarget(path)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Normally RequestLogger already set up the request info and the
		// status-recording writer; a router used on its own (tests) gets
		// its own.
		info := infoFrom(r.Context())
		if info == nil {
			info = &requestInfo{id: newRequestID()}
			r = r.WithContext(context.WithValue(r.Context(), requestInfoKey{}, info))
			w = &loggingResponseWriter{ResponseWriter: w, info: info, status: http.StatusOK}
		}
		entityID := ""
		if idParam != "" {
			entityID = r.PathValue(idParam)
		} else {
			info.captureHead = true
		}
		// What the document was before: its state, and for a deletion its
		// label, which won't be readable afterwards.
		var beforeLabel, beforeState string
		if entityID != "" {
			h.dbMu.RLock()
			beforeLabel, beforeState = h.db.AuditDocumentInfo(resource, entityID)
			h.dbMu.RUnlock()
		}

		next.ServeHTTP(w, r)

		if info.status < 200 || info.status >= 300 {
			return
		}
		if entityID == "" {
			if m := createdIDPattern.FindSubmatch(info.head); m != nil {
				entityID = string(m[1])
			}
		}
		event := db.AuditEvent{
			CreatedAt: time.Now().UnixMilli(),
			UserEmail: info.email,
			Method:    method,
			Route:     path,
			Resource:  resource,
			EntityID:  entityID,
			RequestID: info.id,
		}
		if info.userID != "" {
			event.UserID = &info.userID
		}
		if info.orgID != "" {
			event.OrganizationID = &info.orgID
		}
		// Restore routes swap the database under the write lock; this
		// read lock waits for that and writes into the database now live.
		h.dbMu.RLock()
		defer h.dbMu.RUnlock()
		afterLabel, afterState := "", ""
		if entityID != "" && method != http.MethodDelete {
			afterLabel, afterState = h.db.AuditDocumentInfo(resource, entityID)
		}
		event.EntityLabel = afterLabel
		if event.EntityLabel == "" {
			event.EntityLabel = beforeLabel
		}
		if beforeState != afterState && afterState != "" {
			event.FromState, event.ToState = beforeState, afterState
		}
		if err := h.db.InsertAuditEvent(event); err != nil {
			slog.Warn("activity history: could not record a change", "route", pattern, "request_id", info.id, "err", err)
		}
	})
}

// GET /api/organizations/{orgId}/audit-events — the organization's activity
// history, for its admins. GET /api/audit-events — the platform's (users,
// backups, restores), for platform admins. Both newest first, a page at a
// time: pass the response's next cursor as ?before= for the following page.
func (h *handler) listOrganizationAuditEvents(w http.ResponseWriter, r *http.Request) {
	h.writeAuditEvents(w, r, db.AuditEventFilter{OrganizationID: r.PathValue("orgId")})
}

func (h *handler) listPlatformAuditEvents(w http.ResponseWriter, r *http.Request) {
	h.writeAuditEvents(w, r, db.AuditEventFilter{Platform: true})
}

func (h *handler) writeAuditEvents(w http.ResponseWriter, r *http.Request, f db.AuditEventFilter) {
	q := r.URL.Query()
	f.UserID = q.Get("userId")
	f.EntityID = q.Get("entityId")
	f.From = int64(parseIntParam(r, "from"))
	f.To = int64(parseIntParam(r, "to"))
	f.Limit = parseIntParam(r, "limit")
	if before := q.Get("before"); before != "" {
		at, id, ok := strings.Cut(before, ":")
		ms, err := strconv.ParseInt(at, 10, 64)
		if !ok || err != nil {
			writeError(w, http.StatusBadRequest, "invalid cursor")
			return
		}
		f.Before, f.BeforeID = ms, id
	}
	if f.Limit <= 0 {
		f.Limit = 100
	}
	events, err := h.db.ListAuditEvents(f)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	next := ""
	if len(events) == f.Limit {
		last := events[len(events)-1]
		next = strconv.FormatInt(last.CreatedAt, 10) + ":" + last.ID
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events, "next": next})
}

// runAuditPruning drops the history older than db.AuditRetention, a minute
// after startup and then daily.
func (h *handler) runAuditPruning() {
	for {
		time.Sleep(time.Minute)
		h.dbMu.RLock()
		if h.db != nil {
			if n, err := h.db.PruneAuditEvents(time.Now()); err != nil {
				slog.Warn("activity history: pruning failed", "err", err)
			} else if n > 0 {
				slog.Info("activity history: pruned old entries", "count", n)
			}
		}
		h.dbMu.RUnlock()
		time.Sleep(24*time.Hour - time.Minute)
	}
}
