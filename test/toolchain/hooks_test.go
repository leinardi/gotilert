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

package toolchain_test

import (
	"fmt"
	"slices"
	"testing"
)

// wholeTreeHooks read the whole repository rather than the files a commit stages, so a commit that
// touches none of their files must still run them.
var wholeTreeHooks = []string{"check-license-headers", "check-suppressions", "docs-links"}

// TestWholeTreeHooksAlwaysRun: each whole-tree hook exists once, always runs, takes no file list,
// and is pinned to the pre-commit stage, since always_run alone makes it eligible at commit-msg too.
func TestWholeTreeHooksAlwaysRun(t *testing.T) {
	t.Parallel()

	hooks := preCommitHooks(t)

	for _, id := range wholeTreeHooks {
		found := hooks[id]
		if len(found) != 1 {
			t.Errorf("hook %s is defined %d times, want once", id, len(found))

			continue
		}

		hook := found[0]
		stages, _ := hook["stages"].([]any)

		if hook["always_run"] != true || hook["pass_filenames"] != false ||
			!slices.Equal(stages, []any{"pre-commit"}) {
			t.Errorf("hook %s has always_run %v, pass_filenames %v, stages %v; want true, false "+
				"and [pre-commit]", id, hook["always_run"], hook["pass_filenames"],
				fmt.Sprint(stages))
		}
	}
}
