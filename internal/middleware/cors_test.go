package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCORSMiddleware_OriginHeader(t *testing.T) {
	tests := []struct {
		name           string
		originHeader   string
		checkOriginFn  OriginCheck
		expectedOrigin string
	}{
		{
			name:         "allows valid origin with proper casing",
			originHeader: "https://example.com",
			checkOriginFn: func(r *http.Request) bool {
				return true
			},
			expectedOrigin: "https://example.com",
		},
		{
			name:         "allows origin from behind proxy",
			originHeader: "https://office.talenthope.com.cn",
			checkOriginFn: func(r *http.Request) bool {
				return true
			},
			expectedOrigin: "https://office.talenthope.com.cn",
		},
		{
			name:         "denies invalid origin",
			originHeader: "https://evil.com",
			checkOriginFn: func(r *http.Request) bool {
				return false
			},
			expectedOrigin: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			corsMiddleware := NewCORS(tt.checkOriginFn)
			wrappedHandler := corsMiddleware.Middleware(handler)

			req := httptest.NewRequest("GET", "http://localhost/", nil)
			req.Header.Set("Origin", tt.originHeader)

			rr := httptest.NewRecorder()
			wrappedHandler.ServeHTTP(rr, req)

			allowOrigin := rr.Header().Get("Access-Control-Allow-Origin")
			require.Equal(t, tt.expectedOrigin, allowOrigin)

			if tt.expectedOrigin != "" {
				require.Equal(t, "true", rr.Header().Get("Access-Control-Allow-Credentials"))
			}
		})
	}
}

func TestCORSMiddleware_RequestHeaders(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	corsMiddleware := NewCORS(func(r *http.Request) bool {
		return true
	})
	wrappedHandler := corsMiddleware.Middleware(handler)

	req := httptest.NewRequest("GET", "http://localhost/", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Headers", "Content-Type, Authorization")

	rr := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(rr, req)

	require.Equal(t, "https://example.com", rr.Header().Get("Access-Control-Allow-Origin"))
	require.Equal(t, "Content-Type, Authorization", rr.Header().Get("Access-Control-Allow-Headers"))
	require.Equal(t, "true", rr.Header().Get("Access-Control-Allow-Credentials"))
}
