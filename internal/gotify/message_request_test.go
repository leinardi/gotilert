/*
 * MIT License
 *
 * Copyright (c) 2025 Roberto Leinardi
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */

package gotify

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseMessageRequestJSONDefaultsPriority(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "http://example.local/message",
		strings.NewReader(`{"message":"hello"}`),
	)
	req.Header.Set("Content-Type", "application/json")

	msg, err := ParseMessageRequest(req)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if msg.Message != "hello" {
		t.Fatalf("expected message %q, got %q", "hello", msg.Message)
	}

	if msg.Priority != 5 {
		t.Fatalf("expected default priority %d, got %d", 5, msg.Priority)
	}

	if msg.Title != "" {
		t.Fatalf("expected empty title, got %q", msg.Title)
	}
}

func TestParseMessageRequestJSONMissingMessage(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "http://example.local/message",
		strings.NewReader(`{"title":"t"}`),
	)
	req.Header.Set("Content-Type", "application/json")

	_, err := ParseMessageRequest(req)
	if !errors.Is(err, ErrMessageRequired) {
		t.Fatalf("expected ErrMessageRequired, got: %v", err)
	}
}

func TestParseMessageRequestForm(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "http://example.local/message",
		strings.NewReader("message=hello&title=test&priority=7"),
	)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	msg, err := ParseMessageRequest(req)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if msg.Message != "hello" {
		t.Fatalf("expected message %q, got %q", "hello", msg.Message)
	}

	if msg.Title != "test" {
		t.Fatalf("expected title %q, got %q", "test", msg.Title)
	}

	if msg.Priority != 7 {
		t.Fatalf("expected priority %d, got %d", 7, msg.Priority)
	}
}

func TestParseMessageRequestUnsupportedContentType(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "http://example.local/message",
		strings.NewReader("message=hello"),
	)
	req.Header.Set("Content-Type", "text/plain")

	_, err := ParseMessageRequest(req)
	if !errors.Is(err, ErrUnsupportedContentType) {
		t.Fatalf("expected ErrUnsupportedContentType, got: %v", err)
	}
}

func TestParseMessageRequestJSONNegativePriority(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(
		http.MethodPost,
		"http://example.local/message",
		strings.NewReader(`{"message":"hello","priority":-1}`),
	)
	request.Header.Set("Content-Type", "application/json")

	_, err := ParseMessageRequest(request)
	if !errors.Is(err, ErrInvalidPriority) {
		t.Fatalf("expected ErrInvalidPriority, got: %v", err)
	}
}

func TestParseMessageRequestFormNegativePriority(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(
		http.MethodPost,
		"http://example.local/message",
		strings.NewReader("message=hello&priority=-1"),
	)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	_, err := ParseMessageRequest(request)
	if !errors.Is(err, ErrInvalidPriority) {
		t.Fatalf("expected ErrInvalidPriority, got: %v", err)
	}
}
