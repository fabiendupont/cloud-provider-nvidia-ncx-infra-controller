/*
Copyright 2026 Fabien Dupont.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cloudprovider

import (
	"context"
	"net/http"
	"time"

	"k8s.io/klog/v2"
)

// retryConfig holds parameters for API call retry behavior.
type retryConfig struct {
	maxRetries     int
	initialBackoff time.Duration
}

// defaultRetryConfig returns the default retry configuration:
// 3 retries with 1-second initial backoff.
func defaultRetryConfig() retryConfig {
	return retryConfig{maxRetries: 3, initialBackoff: 1 * time.Second}
}

// isTransient returns true if the HTTP status code indicates a transient
// error that should be retried.
func isTransient(statusCode int) bool {
	switch statusCode {
	case http.StatusTooManyRequests,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	}
	return false
}

// isTerminal returns true if the HTTP status code indicates a terminal
// error that should not be retried.
func isTerminal(statusCode int) bool {
	switch statusCode {
	case http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusNotFound:
		return true
	}
	return false
}

// retryDo executes fn with exponential backoff for transient errors.
// It records latency via apiLatency and errors via apiErrors for the
// given endpoint name.
func retryDo[T any](
	ctx context.Context,
	endpoint string,
	cfg retryConfig,
	fn func() (T, *http.Response, error),
) (T, *http.Response, error) {
	start := time.Now()
	var lastResult T
	var lastResp *http.Response
	var lastErr error

	for attempt := 0; attempt <= cfg.maxRetries; attempt++ {
		lastResult, lastResp, lastErr = fn()

		// Record latency for each attempt
		elapsed := time.Since(start).Seconds()

		if lastErr == nil && lastResp != nil && lastResp.StatusCode >= 200 && lastResp.StatusCode < 300 {
			apiLatency.WithLabelValues(endpoint).Observe(elapsed)
			return lastResult, lastResp, nil
		}

		// Determine error type for metrics
		errorType := "unknown"
		statusCode := 0
		if lastResp != nil {
			statusCode = lastResp.StatusCode
		}

		if statusCode > 0 {
			if isTerminal(statusCode) {
				errorType = "terminal"
				apiErrors.WithLabelValues(endpoint, errorType).Inc()
				apiLatency.WithLabelValues(endpoint).Observe(elapsed)
				klog.V(4).Infof("Terminal error on %s (status %d), not retrying", endpoint, statusCode)
				return lastResult, lastResp, lastErr
			}
			if isTransient(statusCode) {
				errorType = "transient"
			}
		} else if lastErr != nil {
			// Network errors (no response) are treated as transient
			errorType = "transient"
		}

		apiErrors.WithLabelValues(endpoint, errorType).Inc()

		if attempt < cfg.maxRetries {
			backoff := cfg.initialBackoff * (1 << uint(attempt))
			klog.V(4).Infof("Transient error on %s (status %d, err=%v), retrying in %v (attempt %d/%d)",
				endpoint, statusCode, lastErr, backoff, attempt+1, cfg.maxRetries)

			select {
			case <-ctx.Done():
				apiLatency.WithLabelValues(endpoint).Observe(time.Since(start).Seconds())
				return lastResult, lastResp, ctx.Err()
			case <-time.After(backoff):
			}
		}
	}

	apiLatency.WithLabelValues(endpoint).Observe(time.Since(start).Seconds())
	klog.V(2).Infof("All retries exhausted for %s: %v", endpoint, lastErr)
	return lastResult, lastResp, lastErr
}
