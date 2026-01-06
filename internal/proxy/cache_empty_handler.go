package proxy

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/proxyproto"

	"github.com/rs/zerolog/log"
)

// CacheEmptyHandlerFunc is a function to handle cache empty events.
type CacheEmptyHandlerFunc func(ctx context.Context, channel string) (*proxyproto.NotifyCacheEmptyResponse, error)

// CacheEmptyHandlerConfig configures CacheEmptyHandler.
type CacheEmptyHandlerConfig struct {
	Proxies map[string]CacheEmptyProxy
	// LockTimeout is the maximum time to wait for a lock on a channel.
	// If not set, defaults to 5 seconds. This prevents deadlocks and indefinite blocking.
	LockTimeout time.Duration
}

var (
	// ErrLockTimeout is returned when unable to acquire lock within timeout.
	ErrLockTimeout = errors.New("timeout waiting for cache empty lock")
)

// channelLock represents a lock for a specific channel's cache empty operation.
type channelLock struct {
	result *proxyproto.NotifyCacheEmptyResponse
	err    error
	done   chan struct{}
}

// CacheEmptyHandler manages cache empty proxy calls with concurrency control.
// This provides single-instance deduplication. For multi-instance setups with Redis,
// the backend should implement idempotency to handle concurrent calls from different instances.
type CacheEmptyHandler struct {
	proxies      map[string]CacheEmptyProxy
	channelLocks sync.Map // map[string]*channelLock
	lockTimeout  time.Duration
}

// NewCacheEmptyHandler creates new CacheEmptyHandler.
func NewCacheEmptyHandler(config CacheEmptyHandlerConfig) CacheEmptyHandlerFunc {
	lockTimeout := config.LockTimeout
	if lockTimeout == 0 {
		lockTimeout = 5 * time.Second // default timeout
	}
	handler := &CacheEmptyHandler{
		proxies:     config.Proxies,
		lockTimeout: lockTimeout,
	}
	return handler.handle
}

func (h *CacheEmptyHandler) handle(ctx context.Context, channel string) (*proxyproto.NotifyCacheEmptyResponse, error) {
	// Try to acquire or wait for the lock for this channel
	lock, isFirstCall := h.getOrCreateLock(channel)

	if isFirstCall {
		// This is the first call for this channel, we should make the proxy call
		defer func() {
			// Clean up the lock after we're done
			h.channelLocks.Delete(channel)
			close(lock.done)
		}()

		req := &proxyproto.NotifyCacheEmptyRequest{
			Channel: channel,
		}
		lock.result, lock.err = handleCacheEmpty(ctx, req, h.proxies)
		return lock.result, lock.err
	}

	// Wait for the first call to complete with timeout to prevent deadlock
	timer := time.NewTimer(h.lockTimeout)
	defer timer.Stop()

	select {
	case <-lock.done:
		return lock.result, lock.err
	case <-timer.C:
		log.Warn().
			Str("channel", channel).
			Dur("timeout", h.lockTimeout).
			Msg("timeout waiting for cache empty lock, making independent call")
		// Timeout occurred - make an independent call to avoid blocking indefinitely.
		// This can happen if the first call hangs or takes too long.
		req := &proxyproto.NotifyCacheEmptyRequest{
			Channel: channel,
		}
		return handleCacheEmpty(ctx, req, h.proxies)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// getOrCreateLock attempts to get or create a lock for the given channel.
// Returns the lock and a boolean indicating if this is the first call (true) or a subsequent call (false).
func (h *CacheEmptyHandler) getOrCreateLock(channel string) (*channelLock, bool) {
	newLock := &channelLock{
		done: make(chan struct{}),
	}

	actual, loaded := h.channelLocks.LoadOrStore(channel, newLock)
	lock := actual.(*channelLock)

	// loaded == false means we stored our new lock, so we're the first call
	return lock, !loaded
}

func handleCacheEmpty(ctx context.Context, req *proxyproto.NotifyCacheEmptyRequest, proxies map[string]CacheEmptyProxy) (*proxyproto.NotifyCacheEmptyResponse, error) {
	for name, cacheEmptyProxy := range proxies {
		if cacheEmptyProxy == nil {
			log.Error().Str("proxy_name", name).Msg("cache empty proxy is nil")
			continue
		}
		resp, err := cacheEmptyProxy.ProxyCacheEmpty(ctx, req)
		if err != nil {
			log.Error().Err(err).Str("proxy_name", name).Str("channel", req.Channel).Msg("error calling cache empty proxy")
			return nil, err
		}
		return resp, nil
	}
	return &proxyproto.NotifyCacheEmptyResponse{
		Result: &proxyproto.NotifyCacheEmptyResult{},
	}, nil
}
