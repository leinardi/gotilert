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

package alertmanager

import "errors"

var (
	ErrClientNil            = errors.New("alertmanager client is nil")
	ErrBaseURLMissing       = errors.New("alertmanager base url is missing")
	ErrUpstreamNon2xx       = errors.New("alertmanager returned non-2xx status")
	ErrEncodeRequest        = errors.New("encode request failed")
	ErrCreateRequest        = errors.New("create request failed")
	ErrDoRequest            = errors.New("perform request failed")
	ErrReadResponseBody     = errors.New("read response body failed")
	ErrInvalidConfiguration = errors.New("invalid alertmanager configuration")
	ErrNotReady             = errors.New("alertmanager not ready")
)
