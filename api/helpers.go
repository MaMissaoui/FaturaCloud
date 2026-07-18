package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/MaMissaoui/fatura-cloud/db"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// maxJSONBody caps the size of any JSON request body. It is comfortably larger
// than a multi-MB organization logo (carried as base64 inside the org
// create/update JSON) but small enough to stop an authenticated — or, on the
// login endpoint, unauthenticated — client from exhausting memory with an
// unbounded body. The multipart restore upload has its own, larger cap.
const maxJSONBody = 10 << 20 // 10 MiB

// decodeJSON caps the request body at maxJSONBody, decodes it into v, and on
// failure writes the appropriate error response itself (413 when the cap is
// exceeded, 400 otherwise) so callers need only `return` on a non-nil error.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		} else {
			writeError(w, http.StatusBadRequest, "invalid request body")
		}
		return err
	}
	return nil
}

// writeDBError translates an error from a single-record lookup (GetClient,
// GetProduct, ...) into the right HTTP response: a clean 404 when the record
// simply doesn't exist (sql.ErrNoRows, wrapped by the db package with %w),
// or a generic 500 for anything else — logging the real error server-side
// instead of leaking driver/schema details to the client.
func writeDBError(w http.ResponseWriter, err error, notFoundMsg string) {
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, notFoundMsg)
		return
	}
	writeInternalError(w, err)
}

// writeInternalError logs the real error server-side and returns a generic
// 500 to the client, instead of leaking driver/schema details from err.Error().
func writeInternalError(w http.ResponseWriter, err error) {
	log.Printf("internal error: %v", err)
	writeError(w, http.StatusInternalServerError, "internal error")
}

// writeMutationError handles the error from a state-changing db call: a
// *db.ValidationError carries an already user-safe business-rule message
// (e.g. insufficient stock, invalid status transition) and is returned as a
// 409; anything else is a genuine internal failure.
func writeMutationError(w http.ResponseWriter, err error) {
	if verr, ok := errors.AsType[*db.ValidationError](err); ok {
		writeError(w, http.StatusConflict, verr.Error())
		return
	}
	writeInternalError(w, err)
}
