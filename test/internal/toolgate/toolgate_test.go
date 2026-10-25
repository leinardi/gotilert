/*
 * MIT License
 *
 * Copyright (c) 2026 Roberto Leinardi
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

package toolgate_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leinardi/gotilert/test/internal/toolgate"
)

// childEnv names the tools TestLookupChild looks up; it is set only in the child process that
// TestLookupDecidesSkipOrFail starts.
const childEnv = "TOOLGATE_CHILD_TOOLS"

// TestLookupChild is not a test of its own: it is the process TestLookupDecidesSkipOrFail runs, so
// that a Lookup which skips or fails ends a test binary whose verdict the parent can read.
func TestLookupChild(t *testing.T) {
	t.Parallel()

	tools := os.Getenv(childEnv)
	if tools == "" {
		t.Skip("runs only as the child of TestLookupDecidesSkipOrFail")
	}

	for tool := range strings.SplitSeq(tools, ",") {
		toolgate.Lookup(t, tool)
	}

	t.Log("toolgate child: every tool found")
}

// TestLookupDecidesSkipOrFail drives Lookup with PATH pointed at a directory that holds none, one
// or all of the gated tools: a missing tool must skip the test locally and fail it when RequireEnv
// is 1, including the case the label test cares about, droast present but shellcheck missing.
func TestLookupDecidesSkipOrFail(t *testing.T) {
	t.Parallel()

	empty := t.TempDir()
	droastOnly := stubDir(t, toolgate.Droast)
	all := stubDir(t, toolgate.Tools...)

	for name, test := range map[string]struct {
		path    string
		require string
		verdict string
		message string
	}{
		"nothing on PATH, not required": {
			path: empty, verdict: "--- SKIP", message: toolgate.Droast + " not found",
		},
		"nothing on PATH, required": {
			path: empty, require: "1", verdict: "--- FAIL",
			message: toolgate.Droast + " is required",
		},
		"droast without shellcheck, not required": {
			path: droastOnly, verdict: "--- SKIP", message: toolgate.ShellCheck + " not found",
		},
		"droast without shellcheck, required": {
			path: droastOnly, require: "1", verdict: "--- FAIL",
			message: toolgate.ShellCheck + " is required",
		},
		"every tool on PATH, required": {
			path: all, require: "1", verdict: "--- PASS", message: "every tool found",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			out, err := runChild(t, test.path, test.require)

			if !strings.Contains(out, test.verdict+": TestLookupChild") ||
				!strings.Contains(out, test.message) {
				t.Errorf("want %s with %q, got:\n%s", test.verdict, test.message, out)
			}

			var exitErr *exec.ExitError
			if failed := errors.As(err, &exitErr); failed != (test.verdict == "--- FAIL") {
				t.Errorf("child exit: %v, want a failure only for %s", err, "--- FAIL")
			}
		})
	}
}

// stubDir returns a directory holding, for each of tools, an executable of that name and nothing
// else. Lookup only has to find it, so each is a symlink to this test binary.
func stubDir(t *testing.T, tools ...string) string {
	t.Helper()

	dir := t.TempDir()

	for _, tool := range tools {
		err := os.Symlink(os.Args[0], filepath.Join(dir, tool))
		if err != nil {
			t.Fatal(err)
		}
	}

	return dir
}

// runChild runs TestLookupChild for every gated tool in a copy of this test binary whose PATH is
// path and whose RequireEnv is require, and returns its verbose output.
func runChild(t *testing.T, path, require string) (string, error) {
	t.Helper()

	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.CommandContext(t.Context(), self, "-test.run=^TestLookupChild$", "-test.v")

	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		if key != "PATH" && key != toolgate.RequireEnv && key != childEnv {
			cmd.Env = append(cmd.Env, kv)
		}
	}

	cmd.Env = append(cmd.Env, "PATH="+path, toolgate.RequireEnv+"="+require,
		childEnv+"="+strings.Join(toolgate.Tools, ","))

	out, err := cmd.CombinedOutput()

	return string(out), err
}
