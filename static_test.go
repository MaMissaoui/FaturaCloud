package main

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// cacheTestFS has a hashed asset big enough to be gzipped, a small one, a
// file outside assets/ and the SPA shell.
func cacheTestFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html":           {Data: []byte("<html>spa shell " + strings.Repeat("<div></div>", 100) + "</html>")},
		"favicon.svg":          {Data: []byte("<svg></svg>")},
		"assets/index-abc.js":  {Data: []byte(strings.Repeat("console.log('app');\n", 200))},
		"assets/logo-def.png":  {Data: []byte("\x89PNG\r\n\x1a\nnot-really")},
		"assets/small-ghi.css": {Data: []byte("a{}")},
	}
}

func get(t *testing.T, h http.Handler, path string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// TestStaticCaching pins audit F171's caching rules.
func TestStaticCaching(t *testing.T) {
	h := mustSPAHandler(t, cacheTestFS())

	cases := []struct {
		path, wantCache string
		wantStatus      int
	}{
		{"/assets/index-abc.js", immutableCacheControl, 200},
		{"/assets/logo-def.png", immutableCacheControl, 200},
		{"/", "no-cache", 200},
		{"/index.html", "no-cache", 200},
		{"/invoices/abc", "no-cache", 200},
		{"/favicon.svg", "no-cache", 200},
		// A chunk of an older release: a 404, never the SPA shell.
		{"/assets/index-old.js", "", 404},
	}
	for _, tc := range cases {
		rec := get(t, h, tc.path, nil)
		if rec.Code != tc.wantStatus {
			t.Errorf("%s: status %d, want %d", tc.path, rec.Code, tc.wantStatus)
		}
		if got := rec.Header().Get("Cache-Control"); got != tc.wantCache {
			t.Errorf("%s: Cache-Control %q, want %q", tc.path, got, tc.wantCache)
		}
		if tc.wantStatus == 404 && strings.Contains(rec.Body.String(), "spa shell") {
			t.Errorf("%s: answered with the SPA shell", tc.path)
		}
	}
}

// TestStaticCompressionAndRevalidation: a gzip-capable browser gets the
// precompressed copy, the ETag answers a revalidation with 304, and a
// browser without gzip or a range request gets the plain bytes.
func TestStaticCompressionAndRevalidation(t *testing.T) {
	fsys := cacheTestFS()
	h := mustSPAHandler(t, fsys)
	plain := fsys["assets/index-abc.js"].Data

	rec := get(t, h, "/assets/index-abc.js", map[string]string{"Accept-Encoding": "br, gzip"})
	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", rec.Header().Get("Content-Encoding"))
	}
	if !strings.Contains(rec.Header().Get("Vary"), "Accept-Encoding") {
		t.Errorf("Vary = %q, want Accept-Encoding", rec.Header().Get("Vary"))
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Errorf("Content-Type = %q, want javascript", ct)
	}
	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(zr)
	if !bytes.Equal(got, plain) {
		t.Fatal("gzipped body doesn't decompress to the file")
	}
	if rec.Body.Len() >= len(plain) {
		t.Errorf("gzipped %d bytes, plain %d", rec.Body.Len(), len(plain))
	}

	etag := rec.Header().Get("ETag")
	again := get(t, h, "/assets/index-abc.js", map[string]string{"Accept-Encoding": "gzip", "If-None-Match": etag})
	if again.Code != http.StatusNotModified {
		t.Errorf("revalidation: status %d, want 304", again.Code)
	}

	for name, headers := range map[string]map[string]string{
		"no gzip":    {"Accept-Encoding": "identity"},
		"gzip q=0":   {"Accept-Encoding": "gzip;q=0"},
		"range":      {"Accept-Encoding": "gzip", "Range": "bytes=0-9"},
		"no headers": nil,
	} {
		rec := get(t, h, "/assets/index-abc.js", headers)
		if rec.Header().Get("Content-Encoding") != "" {
			t.Errorf("%s: Content-Encoding %q, want none", name, rec.Header().Get("Content-Encoding"))
		}
		if !bytes.HasPrefix(plain, rec.Body.Bytes()) || rec.Body.Len() == 0 {
			t.Errorf("%s: body isn't the plain file", name)
		}
	}

	// Not worth compressing: a tiny file and an image.
	for _, path := range []string{"/assets/small-ghi.css", "/assets/logo-def.png"} {
		if ce := get(t, h, path, map[string]string{"Accept-Encoding": "gzip"}).Header().Get("Content-Encoding"); ce != "" {
			t.Errorf("%s: Content-Encoding %q, want none", path, ce)
		}
	}
}
