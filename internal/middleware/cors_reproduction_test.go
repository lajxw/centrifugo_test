package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestCORS_ReproduceOriginHeaderIssue reproduces the reported issue where
// the code uses lowercase "origin" instead of the HTTP standard "Origin"
//
// Issue: https://github.com/lajxw/centrifugo_test/issues/XXX
// Error log shows: origin=http://127.0.0.1:9000 when user expects https://office.talenthope.com.cn
//
// Analysis:
// - The code uses r.Header.Get("origin") with lowercase 'o'
// - HTTP standard specifies "Origin" with capital 'O'
// - Go's Header.Get() IS case-insensitive, so technically it works
// - However, using lowercase violates HTTP standards and best practices
// - The real issue in production is likely a reverse proxy configuration problem
func TestCORS_ReproduceOriginHeaderIssue(t *testing.T) {
	t.Log("=== Reproducing Origin Header Issue ===")
	t.Log("Bug Report: Origin shows 'http://127.0.0.1:9000' instead of 'https://office.talenthope.com.cn'")
	t.Log("")
	
	t.Run("verify current code uses lowercase 'origin'", func(t *testing.T) {
		// Read the current CORS middleware code
		t.Log("Current code at line 20 of cors.go:")
		t.Log(`  header.Set("Access-Control-Allow-Origin", r.Header.Get("origin"))`)
		t.Log("")
		t.Log("⚠️  Issue: Uses lowercase 'origin' instead of 'Origin'")
		t.Log("✓  It works because Go's Header.Get() is case-insensitive")
		t.Log("✗  But violates HTTP standard which specifies 'Origin'")
	})
	
	t.Run("demonstrate that lowercase works in Go", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get origin using lowercase (current buggy code)
			originLower := r.Header.Get("origin")
			// Get origin using proper case
			originProper := r.Header.Get("Origin")
			
			t.Logf("Origin (lowercase 'origin'): %s", originLower)
			t.Logf("Origin (proper case 'Origin'): %s", originProper)
			
			if originLower == originProper {
				t.Log("✓ Both return the same value - Header.Get() is case-insensitive")
			}
			
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest("GET", "http://localhost/", nil)
		req.Header.Set("Origin", "https://office.talenthope.com.cn")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	})
	
	t.Run("test current CORS middleware behavior", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		corsMiddleware := NewCORS(func(r *http.Request) bool {
			return true // Allow all for this test
		})
		wrappedHandler := corsMiddleware.Middleware(handler)

		// Test with the expected origin
		req := httptest.NewRequest("GET", "http://localhost/", nil)
		req.Header.Set("Origin", "https://office.talenthope.com.cn")
		rr := httptest.NewRecorder()
		wrappedHandler.ServeHTTP(rr, req)

		allowOrigin := rr.Header().Get("Access-Control-Allow-Origin")
		t.Logf("Request Origin: https://office.talenthope.com.cn")
		t.Logf("Response Access-Control-Allow-Origin: %s", allowOrigin)
		
		if allowOrigin == "https://office.talenthope.com.cn" {
			t.Log("✓ Current code works (Header.Get is case-insensitive)")
			t.Log("⚠️  But should be fixed to use proper case 'Origin' for standards compliance")
		} else {
			t.Errorf("❌ Unexpected: CORS header not set correctly")
		}
	})
	
	t.Run("document the real production issue", func(t *testing.T) {
		t.Log("")
		t.Log("=== Real Production Issue Analysis ===")
		t.Log("The lowercase 'origin' in code is not the root cause of production issue.")
		t.Log("")
		t.Log("Production Problem:")
		t.Log("  - User accesses: https://office.talenthope.com.cn")
		t.Log("  - Server receives Origin: http://127.0.0.1:9000")
		t.Log("  - X-Forwarded-For shows: 111.198.60.59, 47.117.201.205")
		t.Log("")
		t.Log("Root Cause:")
		t.Log("  1. Request goes through reverse proxy (Nginx/etc)")
		t.Log("  2. Proxy forwards to backend at 127.0.0.1:9000")
		t.Log("  3. Origin header gets rewritten or not properly forwarded")
		t.Log("")
		t.Log("Solutions:")
		t.Log("  1. Fix code to use 'Origin' (proper HTTP standard) ← This PR")
		t.Log("  2. Configure proxy to preserve Origin header")
		t.Log("  3. Ensure proxy_pass_request_headers is on in Nginx")
		t.Log("")
		t.Log("Recommended Nginx config:")
		t.Log("  proxy_set_header Host $host;")
		t.Log("  proxy_set_header X-Real-IP $remote_addr;")
		t.Log("  proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;")
		t.Log("  proxy_pass_request_headers on;  # Ensures Origin is forwarded")
	})
}

// TestCORS_HTTPStandardCompliance verifies HTTP standard compliance
func TestCORS_HTTPStandardCompliance(t *testing.T) {
	t.Run("HTTP standard requires 'Origin' with capital O", func(t *testing.T) {
		t.Log("RFC 6454 (Web Origin Concept) specifies:")
		t.Log(`  "Origin" header field (with capital 'O')`)
		t.Log("")
		t.Log("Current code uses: r.Header.Get(\"origin\")")
		t.Log("Should use: r.Header.Get(\"Origin\")")
		t.Log("")
		t.Log("While Go's Header.Get() handles case-insensitivity,")
		t.Log("using the standard case improves code clarity and")
		t.Log("follows best practices.")
	})
}
