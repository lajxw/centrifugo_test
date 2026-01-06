package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
	"github.com/centrifugal/centrifugo/v6/internal/proxyproto"
	"github.com/stretchr/testify/require"
)

func TestCacheEmptyHandlerHTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req proxyproto.NotifyCacheEmptyRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)
		require.Equal(t, "test:channel", req.Channel)

		resp := &proxyproto.NotifyCacheEmptyResponse{
			Result: &proxyproto.NotifyCacheEmptyResult{
				Populated: true,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		err = json.NewEncoder(w).Encode(resp)
		require.NoError(t, err)
	}))
	defer server.Close()

	proxy, err := NewHTTPCacheEmptyProxy(Config{
		Endpoint: server.URL,
		Timeout:  configtypes.Duration(time.Second),
	})
	require.NoError(t, err)

	handler := NewCacheEmptyHandler(CacheEmptyHandlerConfig{
		Proxies: map[string]CacheEmptyProxy{
			"test": proxy,
		},
	})

	resp, err := handler(context.Background(), "test:channel")
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.Result)
	require.True(t, resp.Result.Populated)
}

func TestCacheEmptyHandlerHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	proxy, err := NewHTTPCacheEmptyProxy(Config{
		Endpoint: server.URL,
		Timeout:  configtypes.Duration(time.Second),
	})
	require.NoError(t, err)

	handler := NewCacheEmptyHandler(CacheEmptyHandlerConfig{
		Proxies: map[string]CacheEmptyProxy{
			"test": proxy,
		},
	})

	_, err = handler(context.Background(), "test:channel")
	require.Error(t, err)
}

func TestCacheEmptyHandlerConcurrency(t *testing.T) {
	var callCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		// Simulate some processing time
		time.Sleep(100 * time.Millisecond)

		var req proxyproto.NotifyCacheEmptyRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)

		resp := &proxyproto.NotifyCacheEmptyResponse{
			Result: &proxyproto.NotifyCacheEmptyResult{
				Populated: true,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		err = json.NewEncoder(w).Encode(resp)
		require.NoError(t, err)
	}))
	defer server.Close()

	proxy, err := NewHTTPCacheEmptyProxy(Config{
		Endpoint: server.URL,
		Timeout:  configtypes.Duration(5 * time.Second),
	})
	require.NoError(t, err)

	handler := NewCacheEmptyHandler(CacheEmptyHandlerConfig{
		Proxies: map[string]CacheEmptyProxy{
			"test": proxy,
		},
		LockTimeout: 10 * time.Second, // Long enough for test
	})

	// Launch 10 concurrent requests for the same channel
	const numGoroutines = 10
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			resp, err := handler(context.Background(), "test:channel")
			require.NoError(t, err)
			require.NotNil(t, resp)
			require.NotNil(t, resp.Result)
			require.True(t, resp.Result.Populated)
		}()
	}

	wg.Wait()

	// Verify that only one proxy call was made despite 10 concurrent requests
	require.Equal(t, int32(1), callCount.Load(), "Expected only 1 proxy call for concurrent requests to the same channel")
}

func TestCacheEmptyHandlerLockTimeout(t *testing.T) {
	var callCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := callCount.Add(1)
		// First call takes a very long time (simulating hang)
		if count == 1 {
			time.Sleep(3 * time.Second)
		}

		var req proxyproto.NotifyCacheEmptyRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)

		resp := &proxyproto.NotifyCacheEmptyResponse{
			Result: &proxyproto.NotifyCacheEmptyResult{
				Populated: true,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		err = json.NewEncoder(w).Encode(resp)
		require.NoError(t, err)
	}))
	defer server.Close()

	proxy, err := NewHTTPCacheEmptyProxy(Config{
		Endpoint: server.URL,
		Timeout:  configtypes.Duration(5 * time.Second),
	})
	require.NoError(t, err)

	handler := NewCacheEmptyHandler(CacheEmptyHandlerConfig{
		Proxies: map[string]CacheEmptyProxy{
			"test": proxy,
		},
		LockTimeout: 500 * time.Millisecond, // Short timeout to trigger timeout behavior
	})

	// Start first call that will hang
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		resp, err := handler(context.Background(), "test:channel")
		require.NoError(t, err)
		require.NotNil(t, resp)
	}()

	// Give first call time to start
	time.Sleep(100 * time.Millisecond)

	// Second call should timeout waiting and make its own call
	go func() {
		defer wg.Done()
		resp, err := handler(context.Background(), "test:channel")
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotNil(t, resp.Result)
		require.True(t, resp.Result.Populated)
	}()

	wg.Wait()

	// Should have 2 calls - first one that hung, and second one after timeout
	require.Equal(t, int32(2), callCount.Load(), "Expected 2 proxy calls: one slow initial call and one after timeout")
}

func TestCacheEmptyHandlerDifferentChannels(t *testing.T) {
	var callCount atomic.Int32
	channelsSeen := sync.Map{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)

		var req proxyproto.NotifyCacheEmptyRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)

		channelsSeen.Store(req.Channel, true)

		resp := &proxyproto.NotifyCacheEmptyResponse{
			Result: &proxyproto.NotifyCacheEmptyResult{
				Populated: true,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		err = json.NewEncoder(w).Encode(resp)
		require.NoError(t, err)
	}))
	defer server.Close()

	proxy, err := NewHTTPCacheEmptyProxy(Config{
		Endpoint: server.URL,
		Timeout:  configtypes.Duration(5 * time.Second),
	})
	require.NoError(t, err)

	handler := NewCacheEmptyHandler(CacheEmptyHandlerConfig{
		Proxies: map[string]CacheEmptyProxy{
			"test": proxy,
		},
	})

	// Launch concurrent requests for different channels
	channels := []string{"channel1", "channel2", "channel3"}
	var wg sync.WaitGroup

	for _, ch := range channels {
		wg.Add(1)
		go func(channel string) {
			defer wg.Done()
			resp, err := handler(context.Background(), channel)
			require.NoError(t, err)
			require.NotNil(t, resp)
			require.NotNil(t, resp.Result)
			require.True(t, resp.Result.Populated)
		}(ch)
	}

	wg.Wait()

	// Verify that each channel got its own proxy call
	require.Equal(t, int32(3), callCount.Load(), "Expected 3 proxy calls for 3 different channels")

	// Verify all channels were seen
	for _, ch := range channels {
		_, ok := channelsSeen.Load(ch)
		require.True(t, ok, "Channel %s should have been seen", ch)
	}
}

func TestCacheEmptyHandlerGRPC(t *testing.T) {
	// Skip gRPC test for now - would require more complex setup
	t.Skip("gRPC test not implemented yet")
}
