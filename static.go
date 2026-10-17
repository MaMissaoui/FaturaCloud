package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"
)

// The embedded frontend is served from memory with caching and compression
// (audit F171). An embed.FS has no modification times, so http.FileServerFS
// sent no Last-Modified, ETag or Cache-Control, and the browser downloaded
// every script again on every visit.
//
//   - /assets/* names carry a content hash, so they are cached for a year as
//     immutable. One that doesn't exist is a 404, never the SPA shell: a tab
//     left open across a deploy asks for chunks of the old release, and an
//     index.html cached as immutable under a script's name would break that
//     name for a year.
//   - index.html, the SPA fallback and the files outside assets/ are
//     no-cache: revalidated on every load through their ETag, so a deploy is
//     picked up immediately.
//   - Text files are gzipped once at startup and the gzipped copy is sent to
//     a browser that accepts it, so nothing is compressed per request.

const immutableCacheControl = "public, max-age=31536000, immutable"

// staticFile is one embedded file, kept in memory with its gzipped copy
// (nil when compressing it doesn't pay) and the ETag of each.
type staticFile struct {
	contentType string
	body        []byte
	etag        string
	gzipped     []byte
	gzipETag    string
}

// compressibleTypes are the extensions worth gzipping; images and fonts are
// compressed already.
var compressibleTypes = map[string]bool{
	".js": true, ".mjs": true, ".css": true, ".html": true, ".svg": true,
	".json": true, ".txt": true, ".xml": true, ".webmanifest": true, ".ico": true,
}

// loadStaticFiles reads every file of fsys into memory, keyed by its path.
func loadStaticFiles(fsys fs.FS) (map[string]*staticFile, error) {
	files := map[string]*staticFile{}
	err := fs.WalkDir(fsys, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		body, err := fs.ReadFile(fsys, name)
		if err != nil {
			return err
		}
		ext := path.Ext(name)
		contentType := mime.TypeByExtension(ext)
		if contentType == "" {
			contentType = http.DetectContentType(body)
		}
		sum := sha256.Sum256(body)
		tag := hex.EncodeToString(sum[:8])
		f := &staticFile{contentType: contentType, body: body, etag: `"` + tag + `"`}
		if compressibleTypes[ext] && len(body) > 256 {
			var buf bytes.Buffer
			zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
			if _, err := zw.Write(body); err != nil {
				return err
			}
			if err := zw.Close(); err != nil {
				return err
			}
			// Keep the gzipped copy only when it saves at least a tenth.
			if buf.Len() < len(body)*9/10 {
				f.gzipped = buf.Bytes()
				f.gzipETag = `"` + tag + `-gz"`
			}
		}
		files[name] = f
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("load static files: %w", err)
	}
	return files, nil
}

// spaHandler serves the embedded frontend and falls back to index.html for
// any path that isn't a file, letting React Router handle client-side
// navigation. Unmatched /api/* paths (no registered handler matched them)
// get a JSON 404 instead of index.html, and no path ever lists a directory.
func spaHandler(fsys fs.FS) (http.Handler, error) {
	files, err := loadStaticFiles(fsys)
	if err != nil {
		return nil, err
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeJSONNotFound(w)
			return
		}
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		f, ok := files[name]
		switch {
		case ok && strings.HasPrefix(name, "assets/"):
			w.Header().Set("Cache-Control", immutableCacheControl)
		case ok:
			w.Header().Set("Cache-Control", "no-cache")
		case strings.HasPrefix(name, "assets/"):
			http.NotFound(w, r)
			return
		default:
			f = files["index.html"]
			if f == nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Cache-Control", "no-cache")
		}
		serveStaticFile(w, r, f)
	}), nil
}

// serveStaticFile writes f, gzipped when the browser accepts it, and lets
// http.ServeContent answer If-None-Match with a 304. A range request gets
// the plain bytes, so a range always refers to the file itself.
func serveStaticFile(w http.ResponseWriter, r *http.Request, f *staticFile) {
	h := w.Header()
	h.Set("Content-Type", f.contentType)
	body, etag := f.body, f.etag
	if f.gzipped != nil {
		h.Add("Vary", "Accept-Encoding")
		if r.Header.Get("Range") == "" && acceptsGzip(r) {
			body, etag = f.gzipped, f.gzipETag
			h.Set("Content-Encoding", "gzip")
		}
	}
	h.Set("ETag", etag)
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(body))
}

// acceptsGzip reports whether the request's Accept-Encoding allows gzip
// (named, or through "*", and not with q=0).
func acceptsGzip(r *http.Request) bool {
	for _, part := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		coding, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		coding = strings.ToLower(strings.TrimSpace(coding))
		if coding != "gzip" && coding != "*" {
			continue
		}
		q := strings.ReplaceAll(strings.ToLower(params), " ", "")
		if q == "q=0" || q == "q=0.0" || q == "q=0.00" || q == "q=0.000" {
			return false
		}
		return true
	}
	return false
}
