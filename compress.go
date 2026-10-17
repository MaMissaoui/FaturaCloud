package main

import (
	"bufio"
	"compress/gzip"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
)

// compressAPIResponses gzips /api JSON and text responses for a browser
// that accepts it (audit F171): the big lists shrink 8–10×. Responses under
// minCompressSize go out as they are, and anything that isn't JSON or text
// (Excel and PDF exports, backups, logos) is passed through untouched — it
// is either compressed already or binary.
//
// It must wrap the outermost handler, outside api.RequestLogger: the
// activity history learns a created document's id from the first bytes of
// the response the logger captures (api/request_log.go), and wrapped inside
// it would see gzip instead of JSON. TestServerHandlerKeepsActivityHistory
// guards that placement.
func compressAPIResponses(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") || r.Method == http.MethodHead {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Add("Vary", "Accept-Encoding")
		if !acceptsGzip(r) {
			next.ServeHTTP(w, r)
			return
		}
		gw := &gzipResponseWriter{ResponseWriter: w}
		defer gw.finish()
		next.ServeHTTP(gw, r)
	})
}

// minCompressSize is the smallest response worth gzipping; below it the
// gzip framing costs more than it saves.
const minCompressSize = 1024

var gzipWriters = sync.Pool{New: func() any {
	zw, _ := gzip.NewWriterLevel(io.Discard, gzip.DefaultCompression)
	return zw
}}

// gzipResponseWriter holds back the status and the first minCompressSize
// bytes until it knows whether to compress: a response that ends smaller,
// or turns out not to be JSON/text, is written exactly as the handler wrote
// it.
type gzipResponseWriter struct {
	http.ResponseWriter
	status   int
	buf      []byte
	decided  bool // headers sent: either passing through or compressing
	zw       *gzip.Writer
	hijacked bool
}

func (g *gzipResponseWriter) WriteHeader(status int) {
	if g.decided || g.status != 0 {
		return
	}
	g.status = status
	// Bodiless or informational responses, and ones that can't be
	// compressed, go straight through.
	if status < 200 || status == http.StatusNoContent || status == http.StatusNotModified || !g.compressible() {
		g.passThrough()
	}
}

func (g *gzipResponseWriter) Write(p []byte) (int, error) {
	if g.status == 0 {
		g.WriteHeader(http.StatusOK)
	}
	if g.decided {
		if g.zw != nil {
			return g.zw.Write(p)
		}
		return g.ResponseWriter.Write(p)
	}
	g.buf = append(g.buf, p...)
	if len(g.buf) >= minCompressSize {
		if err := g.startGzip(); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

// compressible reports whether the response's own headers allow gzipping:
// JSON or text, and not already encoded.
func (g *gzipResponseWriter) compressible() bool {
	h := g.Header()
	if h.Get("Content-Encoding") != "" {
		return false
	}
	ct := strings.ToLower(h.Get("Content-Type"))
	return strings.HasPrefix(ct, "application/json") || strings.HasPrefix(ct, "text/")
}

func (g *gzipResponseWriter) passThrough() {
	g.decided = true
	g.ResponseWriter.WriteHeader(g.status)
}

func (g *gzipResponseWriter) startGzip() error {
	g.decided = true
	h := g.Header()
	h.Set("Content-Encoding", "gzip")
	h.Del("Content-Length")
	g.ResponseWriter.WriteHeader(g.status)
	g.zw = gzipWriters.Get().(*gzip.Writer)
	g.zw.Reset(g.ResponseWriter)
	_, err := g.zw.Write(g.buf)
	g.buf = nil
	return err
}

// finish writes whatever is still held back — a small response as it is —
// and closes the gzip stream.
func (g *gzipResponseWriter) finish() {
	if g.hijacked {
		return
	}
	if !g.decided {
		if g.status == 0 {
			// The handler wrote nothing at all; the server sends 200.
			return
		}
		g.passThrough()
		if len(g.buf) > 0 {
			_, _ = g.ResponseWriter.Write(g.buf)
		}
		return
	}
	if g.zw != nil {
		_ = g.zw.Close()
		g.zw.Reset(io.Discard)
		gzipWriters.Put(g.zw)
		g.zw = nil
	}
}

// Flush sends what has been written so far; a held-back response is
// decided first.
func (g *gzipResponseWriter) Flush() {
	if !g.decided && g.status != 0 {
		if g.compressible() && len(g.buf) > 0 {
			_ = g.startGzip()
		} else {
			g.passThrough()
			if len(g.buf) > 0 {
				_, _ = g.ResponseWriter.Write(g.buf)
				g.buf = nil
			}
		}
	}
	if g.zw != nil {
		_ = g.zw.Flush()
	}
	if f, ok := g.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack is passed through for completeness; nothing in the API upgrades a
// connection today.
func (g *gzipResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hj, ok := g.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("hijacking not supported")
	}
	g.hijacked = true
	return hj.Hijack()
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (g *gzipResponseWriter) Unwrap() http.ResponseWriter { return g.ResponseWriter }
