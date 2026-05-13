package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"

	"cpal/internal/auth"
	"cpal/internal/store"
)

// idempotency enforces idempotency keys on mutating requests. Replaying a
// request with the same key returns the stored response without re-executing.
// GET/HEAD/OPTIONS pass through untouched.
func (s *Server) idempotency(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}

		claims, ok := auth.FromContext(r.Context())
		if !ok {
			writeErr(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
			return
		}
		key := r.Header.Get("Idempotency-Key")
		if key == "" {
			writeErr(w, http.StatusBadRequest, "idempotency_key_required",
				"this request requires an Idempotency-Key header")
			return
		}

		// Read and restore the body so we can hash it and the handler can read it.
		body, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		r.Body = io.NopCloser(bytes.NewReader(body))
		sum := sha256.Sum256(append([]byte(r.Method+" "+r.URL.Path+"\n"), body...))
		reqHash := hex.EncodeToString(sum[:])

		// Replay a stored response if this key was seen before.
		rec, err := s.store.GetIdempotency(r.Context(), claims.IdentityID, key)
		if err == nil {
			if rec.RequestHash != reqHash {
				writeErr(w, http.StatusConflict, "idempotency_key_reuse",
					"this Idempotency-Key was used with a different request")
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Idempotent-Replayed", "true")
			w.WriteHeader(rec.StatusCode)
			_, _ = w.Write(rec.ResponseBody)
			return
		} else if err != store.ErrNotFound {
			writeErr(w, http.StatusInternalServerError, "internal", "idempotency lookup failed")
			return
		}

		// First time: capture the response, then persist it (only if < 500 so
		// transient server errors remain retryable).
		cap := &captureWriter{ResponseWriter: w, status: http.StatusOK, buf: &bytes.Buffer{}}
		next.ServeHTTP(cap, r)

		if cap.status < 500 {
			_ = s.store.SaveIdempotency(r.Context(), store.IdempotencyRecord{
				IdentityID:   claims.IdentityID,
				Key:          key,
				Method:       r.Method,
				Path:         r.URL.Path,
				RequestHash:  reqHash,
				StatusCode:   cap.status,
				ResponseBody: cap.buf.Bytes(),
			})
		}
	})
}

// captureWriter records the status and body while still streaming to the client.
type captureWriter struct {
	http.ResponseWriter
	status      int
	buf         *bytes.Buffer
	wroteHeader bool
}

func (c *captureWriter) WriteHeader(status int) {
	if c.wroteHeader {
		return
	}
	c.status = status
	c.wroteHeader = true
	c.ResponseWriter.WriteHeader(status)
}

func (c *captureWriter) Write(b []byte) (int, error) {
	if !c.wroteHeader {
		c.WriteHeader(http.StatusOK)
	}
	c.buf.Write(b)
	return c.ResponseWriter.Write(b)
}
