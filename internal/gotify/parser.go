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
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"strconv"
	"strings"
)

const DefaultPriority = 5

type jsonMessagePayload struct {
	Message  string         `json:"message"`
	Title    string         `json:"title"`
	Priority *int           `json:"priority,omitempty"`
	Extras   map[string]any `json:"extras,omitempty"`
}

// ParseMessageRequest parses a Gotify-like message request. It supports JSON and URL-encoded forms.
func ParseMessageRequest(request *http.Request) (MessageRequest, error) {
	if request == nil {
		return MessageRequest{}, fmt.Errorf("parse request: %w", ErrUnsupportedContentType)
	}

	contentType := request.Header.Get("Content-Type")
	mediaType := ""

	if contentType != "" {
		parsedType, _, err := mime.ParseMediaType(contentType)
		if err != nil {
			return MessageRequest{}, fmt.Errorf(
				"parse content-type %q: %w",
				contentType,
				ErrUnsupportedContentType,
			)
		}

		mediaType = strings.ToLower(strings.TrimSpace(parsedType))
	}

	// Default when header is absent:
	// many clients send x-www-form-urlencoded without explicit content-type,
	// but we keep it strict: if no content-type, try form parsing first.
	switch mediaType {
	case "application/json":
		return parseJSON(request)

	case "application/x-www-form-urlencoded", "":
		return parseForm(request)

	default:
		return MessageRequest{}, fmt.Errorf("%w: %q", ErrUnsupportedContentType, mediaType)
	}
}

func parseJSON(request *http.Request) (MessageRequest, error) {
	var payload jsonMessagePayload

	decoder := json.NewDecoder(request.Body)
	// Compatibility: do NOT DisallowUnknownFields (Gotify clients may send extras, etc.)
	err := decoder.Decode(&payload)
	if err != nil {
		return MessageRequest{}, fmt.Errorf("decode json: %w", err)
	}

	priority := DefaultPriority
	if payload.Priority != nil {
		priority = *payload.Priority
	}

	msg := MessageRequest{
		Message:  strings.TrimSpace(payload.Message),
		Title:    strings.TrimSpace(payload.Title),
		Priority: priority,
		Extras:   payload.Extras,
	}

	return validate(msg)
}

func parseForm(request *http.Request) (MessageRequest, error) {
	err := request.ParseForm()
	if err != nil {
		return MessageRequest{}, fmt.Errorf("parse form: %w", err)
	}

	message := strings.TrimSpace(request.FormValue("message"))
	title := strings.TrimSpace(request.FormValue("title"))
	priority := DefaultPriority

	priorityRaw := strings.TrimSpace(request.FormValue("priority"))
	if priorityRaw != "" {
		parsed, parseErr := strconv.Atoi(priorityRaw)
		if parseErr != nil {
			return MessageRequest{}, fmt.Errorf("%w: %q", ErrInvalidPriority, priorityRaw)
		}

		priority = parsed
	}

	msg := MessageRequest{
		Message:  message,
		Title:    title,
		Priority: priority,
		Extras:   nil,
	}

	return validate(msg)
}

func validate(msg MessageRequest) (MessageRequest, error) {
	if strings.TrimSpace(msg.Message) == "" {
		return MessageRequest{}, ErrMessageRequired
	}

	if msg.Priority < 0 {
		return MessageRequest{}, fmt.Errorf("%w: %d", ErrInvalidPriority, msg.Priority)
	}

	return msg, nil
}
