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

package gotify_test

import (
	"testing"

	"github.com/leinardi/gotilert/internal/gotify"
)

func TestExtrasAnnotationsExtractsWellKnownKeys(t *testing.T) {
	t.Parallel()

	extras := map[string]any{
		"client::display": map[string]any{
			"contentType": "text/markdown",
		},
		"client::notification": map[string]any{
			"click": map[string]any{
				"url": "https://example.local/details",
			},
			"bigImageUrl": "https://example.local/image.png",
		},
		"android::action": map[string]any{
			"onReceive": map[string]any{
				"intentUrl": "intent://example.local/#Intent;scheme=https;end",
			},
		},
	}

	annotations := gotify.ExtrasAnnotations(extras)

	if got := annotations[gotify.AnnotationGotifyContentType]; got != "text/markdown" {
		t.Fatalf("expected %q, got %q", "text/markdown", got)
	}

	if got := annotations[gotify.AnnotationGotifyClickURL]; got != "https://example.local/details" {
		t.Fatalf("expected %q, got %q", "https://example.local/details", got)
	}

	if got := annotations[gotify.AnnotationGotifyBigImageURL]; got != "https://example.local/image.png" {
		t.Fatalf("expected %q, got %q", "https://example.local/image.png", got)
	}

	if got := annotations[gotify.AnnotationGotifyOnReceiveIntentURL]; got != "intent://example.local/#Intent;scheme=https;end" {
		t.Fatalf("expected %q, got %q", "intent://example.local/#Intent;scheme=https;end", got)
	}
}

func TestExtrasAnnotationsIgnoresNonStringValues(t *testing.T) {
	t.Parallel()

	extras := map[string]any{
		"client::display": map[string]any{
			"contentType": 123,
		},
		"client::notification": map[string]any{
			"click": map[string]any{
				"url": false,
			},
			"bigImageUrl": []any{"nope"},
		},
		"android::action": map[string]any{
			"onReceive": map[string]any{
				"intentUrl": map[string]any{"value": "nope"},
			},
		},
	}

	annotations := gotify.ExtrasAnnotations(extras)
	if len(annotations) != 0 {
		t.Fatalf("expected no annotations, got %v", annotations)
	}
}

func TestExtrasAnnotationsEmptyExtras(t *testing.T) {
	t.Parallel()

	annotations := gotify.ExtrasAnnotations(nil)
	if len(annotations) != 0 {
		t.Fatalf("expected no annotations, got %v", annotations)
	}
}
