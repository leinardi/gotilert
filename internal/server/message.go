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

package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/leinardi/gotilert/internal/gotify"
	"github.com/leinardi/gotilert/internal/logger"
)

var messageID atomic.Uint64

func messageHandler(
	resolve ResolveAppFunc,
	forward ForwardMessageFunc,
	maxBodyBytes int64,
) http.HandlerFunc {
	return func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			writeJSONError(responseWriter, http.StatusMethodNotAllowed, ErrMethodNotAllowed)

			return
		}

		app, ok := authenticate(request, resolve)
		if !ok {
			writeJSONError(responseWriter, http.StatusForbidden, ErrTokenMissingOrInvalid)

			return
		}

		request.Body = http.MaxBytesReader(responseWriter, request.Body, maxBodyBytes)

		msg, err := gotify.ParseMessageRequest(request)
		if err != nil {
			writeParseError(responseWriter, err)

			return
		}

		messageIdentifier := messageID.Add(1)

		if forward == nil {
			writeJSONError(responseWriter, http.StatusInternalServerError, ErrInternalMisconfigured)

			return
		}

		ctx := request.Context()

		err = forward(ctx, app, msg, messageIdentifier)
		if err != nil {
			// Forwarder logs upstream failures with context; return 502.
			writeJSONError(
				responseWriter,
				http.StatusBadGateway,
				fmt.Errorf("%w", ErrUpstreamFailed),
			)

			return
		}

		resp := gotify.MessageResponse{
			ID:       messageIdentifier,
			AppID:    app.ID,
			Message:  msg.Message,
			Title:    msg.Title,
			Priority: msg.Priority,
			Date:     time.Now().UTC(),
			Extras:   msg.Extras,
		}

		writeJSON(responseWriter, http.StatusOK, resp)
	}
}

func authenticate(request *http.Request, resolve ResolveAppFunc) (App, bool) {
	if resolve == nil {
		return App{}, false
	}

	token := extractToken(request)
	if token == "" {
		return App{}, false
	}

	app, ok := resolve(token)

	return app, ok
}

func writeParseError(responseWriter http.ResponseWriter, err error) {
	if errors.Is(err, gotify.ErrMessageRequired) ||
		errors.Is(err, gotify.ErrInvalidPriority) ||
		errors.Is(err, gotify.ErrUnsupportedContentType) {
		writeJSONError(responseWriter, http.StatusBadRequest, err)

		return
	}

	writeJSONError(responseWriter, http.StatusBadRequest, fmt.Errorf("parse message: %w", err))
}

func extractToken(request *http.Request) string {
	// 1) X-Gotify-Key header
	headerToken := strings.TrimSpace(request.Header.Get("X-Gotify-Key"))
	if headerToken != "" {
		return headerToken
	}

	// 2) token query parameter
	queryToken := strings.TrimSpace(request.URL.Query().Get("token"))
	if queryToken != "" {
		return queryToken
	}

	// 3) Authorization: Bearer <token>
	authHeader := strings.TrimSpace(request.Header.Get("Authorization"))
	if authHeader == "" {
		return ""
	}

	lower := strings.ToLower(authHeader)

	const bearerPrefix = "bearer "
	if !strings.HasPrefix(lower, bearerPrefix) {
		return ""
	}

	return strings.TrimSpace(authHeader[len(bearerPrefix):])
}

func writeJSON(responseWriter http.ResponseWriter, status int, payload any) {
	responseWriter.Header().Set("Content-Type", "application/json; charset=utf-8")
	responseWriter.WriteHeader(status)

	encoder := json.NewEncoder(responseWriter)
	encoder.SetEscapeHTML(false)

	err := encoder.Encode(payload)
	if err != nil {
		logger.L().Error("failed to write json response", "err", err)
	}
}

func writeJSONError(responseWriter http.ResponseWriter, status int, err error) {
	type errorBody struct {
		Error string `json:"error"`
	}

	writeJSON(responseWriter, status, errorBody{Error: err.Error()})
}
