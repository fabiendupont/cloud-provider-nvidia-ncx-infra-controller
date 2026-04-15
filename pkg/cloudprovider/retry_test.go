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
	"fmt"
	"net/http"
	"testing"
	"time"
)

func testRetryCfg(maxRetries int) retryConfig {
	return retryConfig{maxRetries: maxRetries, initialBackoff: time.Millisecond}
}

func TestRetryDo_ImmediateSuccess(t *testing.T) {
	calls := 0
	result, _, err := retryDo(
		context.Background(), "test", testRetryCfg(3),
		func() (string, *http.Response, error) {
			calls++
			return "ok", &http.Response{StatusCode: 200}, nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "ok" {
		t.Errorf("result = %q, want %q", result, "ok")
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

func TestRetryDo_TransientThenSuccess(t *testing.T) {
	calls := 0
	result, _, err := retryDo(
		context.Background(), "test", testRetryCfg(3),
		func() (string, *http.Response, error) {
			calls++
			if calls <= 2 {
				return "", &http.Response{StatusCode: 503},
					fmt.Errorf("service unavailable")
			}
			return "recovered", &http.Response{StatusCode: 200}, nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "recovered" {
		t.Errorf("result = %q, want %q", result, "recovered")
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
}

func TestRetryDo_AllRetriesExhausted(t *testing.T) {
	calls := 0
	_, _, err := retryDo(
		context.Background(), "test", testRetryCfg(2),
		func() (string, *http.Response, error) {
			calls++
			return "", &http.Response{StatusCode: 503},
				fmt.Errorf("service unavailable")
		},
	)
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	// 1 initial + 2 retries = 3 calls
	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
}

func TestRetryDo_TerminalNoRetry(t *testing.T) {
	calls := 0
	_, resp, err := retryDo(
		context.Background(), "test", testRetryCfg(3),
		func() (string, *http.Response, error) {
			calls++
			return "", &http.Response{StatusCode: 404},
				fmt.Errorf("not found")
		},
	)
	if err == nil {
		t.Fatal("expected error for terminal status")
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (no retry on terminal)", calls)
	}
	if resp.StatusCode != 404 {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestRetryDo_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	cfg := retryConfig{maxRetries: 3, initialBackoff: 10 * time.Second}
	_, _, err := retryDo(
		ctx, "test", cfg,
		func() (string, *http.Response, error) {
			calls++
			if calls == 1 {
				cancel()
			}
			return "", &http.Response{StatusCode: 503},
				fmt.Errorf("service unavailable")
		},
	)
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestIsTransient(t *testing.T) {
	transient := []int{429, 502, 503, 504}
	for _, code := range transient {
		if !isTransient(code) {
			t.Errorf("isTransient(%d) = false, want true", code)
		}
	}
	nonTransient := []int{200, 400, 404, 500}
	for _, code := range nonTransient {
		if isTransient(code) {
			t.Errorf("isTransient(%d) = true, want false", code)
		}
	}
}

func TestIsTerminal(t *testing.T) {
	terminal := []int{400, 401, 403, 404}
	for _, code := range terminal {
		if !isTerminal(code) {
			t.Errorf("isTerminal(%d) = false, want true", code)
		}
	}
	nonTerminal := []int{200, 429, 500, 503}
	for _, code := range nonTerminal {
		if isTerminal(code) {
			t.Errorf("isTerminal(%d) = true, want false", code)
		}
	}
}
