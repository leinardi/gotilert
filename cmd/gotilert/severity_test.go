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

package main

import "testing"

func TestSeverityForPriorityExactMatch(t *testing.T) {
	t.Parallel()

	mapping := map[int]string{
		0: "info",
		2: "warning",
		5: "critical",
	}

	got := severityForPriority(mapping, 2)
	if got != "warning" {
		t.Fatalf("expected %q, got %q", "warning", got)
	}
}

func TestSeverityForPriorityClosestLower(t *testing.T) {
	t.Parallel()

	mapping := map[int]string{
		0: "info",
		2: "warning",
		5: "critical",
	}

	got := severityForPriority(mapping, 3)
	if got != "warning" {
		t.Fatalf("expected %q, got %q", "warning", got)
	}
}

func TestSeverityForPriorityBelowAllChoosesSmallestKey(t *testing.T) {
	t.Parallel()

	mapping := map[int]string{
		5:  "critical",
		10: "warning",
	}

	got := severityForPriority(mapping, 1)
	if got != "critical" {
		t.Fatalf("expected %q, got %q", "critical", got)
	}
}
