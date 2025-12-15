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
	"strings"
)

// Well-known Gotify extras we map into Alertmanager annotations.
const (
	AnnotationGotifyContentType        = "gotify_content_type"
	AnnotationGotifyClickURL           = "gotify_click_url"
	AnnotationGotifyBigImageURL        = "gotify_big_image_url"
	AnnotationGotifyOnReceiveIntentURL = "gotify_on_receive_intent_url"
)

// ExtrasAnnotations extracts a small set of well-known Gotify extras and converts them into
// string annotations suitable for Alertmanager.
// Unknown extras are ignored. Non-string values are ignored.
func ExtrasAnnotations(extras map[string]any) map[string]string {
	if len(extras) == 0 {
		return map[string]string{}
	}

	annotations := make(map[string]string)

	// client::display.contentType
	if contentType, ok := extrasStringAtPath(extras, "client::display", "contentType"); ok {
		annotations[AnnotationGotifyContentType] = contentType
	}

	// client::notification.click.url
	if clickURL, ok := extrasStringAtPath(extras, "client::notification", "click", "url"); ok {
		annotations[AnnotationGotifyClickURL] = clickURL
	}

	// client::notification.bigImageUrl
	if bigImageURL, ok := extrasStringAtPath(extras, "client::notification", "bigImageUrl"); ok {
		annotations[AnnotationGotifyBigImageURL] = bigImageURL
	}

	// android::action.onReceive.intentUrl
	if intentURL, ok := extrasStringAtPath(extras, "android::action", "onReceive", "intentUrl"); ok {
		annotations[AnnotationGotifyOnReceiveIntentURL] = intentURL
	}

	return annotations
}

func extrasStringAtPath(extras map[string]any, path ...string) (string, bool) {
	if len(extras) == 0 || len(path) == 0 {
		return "", false
	}

	var current any = extras

	for index := range path {
		key := path[index]

		currentMap, ok := current.(map[string]any)
		if !ok {
			return "", false
		}

		next, exists := currentMap[key]
		if !exists {
			return "", false
		}

		current = next
	}

	stringValue, ok := current.(string)
	if !ok {
		return "", false
	}

	stringValue = strings.TrimSpace(stringValue)
	if stringValue == "" {
		return "", false
	}

	return stringValue, true
}
