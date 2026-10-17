package main

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func serveWith(t *testing.T, handler http.HandlerFunc, path, acceptEncoding string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if acceptEncoding != "" {
		req.Header.Set("Accept-Encoding", acceptEncoding)
	}
	rec := httptest.NewRecorder()
	compressAPIResponses(handler).ServeHTTP(rec, req)
	return rec
}

func writeBody(contentType string, status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		w.WriteHeader(status)
		// Written in pieces, as a JSON encoder does.
		for len(body) > 0 {
			n := min(300, len(body))
			_, _ = io.WriteString(w, body[:n])
			body = body[n:]
		}
	}
}

func TestCompressAPIResponses(t *testing.T) {
	big := `{"rows":"` + strings.Repeat("abc", 2000) + `"}`

	t.Run("large JSON is gzipped", func(t *testing.T) {
		rec := serveWith(t, writeBody("application/json", 200, big), "/api/x", "gzip")
		if rec.Header().Get("Content-Encoding") != "gzip" {
			t.Fatalf("Content-Encoding = %q, want gzip", rec.Header().Get("Content-Encoding"))
		}
		zr, err := gzip.NewReader(rec.Body)
		if err != nil {
			t.Fatal(err)
		}
		got, _ := io.ReadAll(zr)
		if string(got) != big {
			t.Fatal("body doesn't decompress to what the handler wrote")
		}
		if rec.Code != 200 {
			t.Fatalf("status %d", rec.Code)
		}
	})

	t.Run("an error status is kept", func(t *testing.T) {
		rec := serveWith(t, writeBody("application/json", 409, big), "/api/x", "gzip")
		if rec.Code != 409 || rec.Header().Get("Content-Encoding") != "gzip" {
			t.Fatalf("got %d %q, want a gzipped 409", rec.Code, rec.Header().Get("Content-Encoding"))
		}
	})

	passThrough := map[string]struct {
		handler        http.HandlerFunc
		path, encoding string
		want           string
	}{
		"small JSON":    {writeBody("application/json", 200, `{"ok":true}`), "/api/x", "gzip", `{"ok":true}`},
		"no gzip":       {writeBody("application/json", 200, big), "/api/x", "", big},
		"an Excel file": {writeBody("application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", 200, big), "/api/x/export", "gzip", big},
		"a PDF":         {writeBody("application/pdf", 200, big), "/api/x/export", "gzip", big},
		"already gzip": {func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Encoding", "gzip")
			writeBody("application/json", 200, big)(w, r)
		}, "/api/x", "gzip", big},
		"outside /api":   {writeBody("text/html", 200, big), "/invoices", "gzip", big},
		"untyped bytes":  {writeBody("", 200, big), "/api/x", "gzip", big},
		"empty response": {writeBody("application/json", 204, ""), "/api/x", "gzip", ""},
	}
	for name, tc := range passThrough {
		t.Run(name+" passes through", func(t *testing.T) {
			rec := serveWith(t, tc.handler, tc.path, tc.encoding)
			if name != "already gzip" && rec.Header().Get("Content-Encoding") != "" {
				t.Fatalf("Content-Encoding = %q, want none", rec.Header().Get("Content-Encoding"))
			}
			if rec.Body.String() != tc.want {
				t.Fatalf("body changed (%d bytes, want %d)", rec.Body.Len(), len(tc.want))
			}
		})
	}
}
