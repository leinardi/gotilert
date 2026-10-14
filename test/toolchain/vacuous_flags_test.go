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
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/leinardi/gotilert/test/internal/toolgate"
)

// vacuousFlags turn a gate with nothing to check, or a failing one, into a pass. None of them may
// appear where gates are defined.
var vacuousFlags = []struct {
	name    string
	pattern *regexp.Regexp
}{
	{"--passWithNoTests", regexp.MustCompile(`--pass-?[Ww]ith-?[Nn]o-?[Tt]ests`)},
	{"--no-error-on-unmatched-pattern", regexp.MustCompile(`--no-error-on-unmatched-pattern`)},
	{"|| true", regexp.MustCompile(`\|\|\s*(true\b|:(\s|;|$)|exit\s+0\b)`)},
	{"make's - recipe prefix", regexp.MustCompile(`^\t[@+]*-`)},
	{".IGNORE", regexp.MustCompile(`^\.IGNORE\b`)},
	{
		"make --ignore-errors",
		regexp.MustCompile(`\$\(MAKE\).*\s(-[a-zA-Z]*i[a-zA-Z]*|--ignore-errors)\b`),
	},
	{"continue-on-error", regexp.MustCompile(`continue-on-error`)},
	{
		"if: false",
		regexp.MustCompile(`\bif:\s*(["']?false["']?\s*$|["']?\$\{\{\s*false\s*\}\}["']?)`),
	},
}

// allowedHit is a line that matches a vacuous flag on purpose: a shell idiom, not a gate that
// passes on nothing.
type allowedHit struct {
	file string
	// line matches the whole trimmed line.
	line *regexp.Regexp
	why  string
}

// allowedHits is the closed list of deliberate matches. Each entry must still match a line, so a
// stale entry fails rather than lingering as a hole.
var allowedHits = []allowedHit{
	{
		file: ".github/workflows/release.yaml",
		line: regexp.MustCompile(`\{ grep -E "\$RELEASE_TAG_REGEX" \|\| true; \}`),
		why: "grep exits 1 when no tag matches yet, which is an empty list, not a failure; " +
			"the braces scope the || true to that grep alone",
	},
	{
		file: ".github/workflows/release.yaml",
		line: regexp.MustCompile(
			`^PREVIOUS=\$\(\{ git tag --merged "HEAD\^" 2>/dev/null \|\| true; \} \| highest_release\)$`,
		),
		why: "the first commit has no HEAD^, and then there is no previous release",
	},
}

// TestNoVacuousFlags scans the files that define gates: the Makefile and make fragments, the
// pre-commit configuration and every YAML file under .github, the workflows included.
func TestNoVacuousFlags(t *testing.T) {
	t.Parallel()

	root := toolgate.RepoRoot(t)

	fragments, err := filepath.Glob(filepath.Join(root, ".mk", "*.mk"))
	if err != nil || len(fragments) == 0 {
		t.Fatalf("no make fragments found under .mk/ (%v); the scan would check nothing", err)
	}

	workflows := githubYAML(t)
	files := make([]string, 0, 2+len(fragments)+len(workflows))
	files = append(files, "Makefile", ".pre-commit-config.yaml")

	for _, fragment := range fragments {
		files = append(
			files,
			filepath.ToSlash(strings.TrimPrefix(fragment, root+string(os.PathSeparator))),
		)
	}

	files = append(files, workflows...)
	used := make([]bool, len(allowedHits))

	for _, rel := range files {
		for _, hit := range vacuousHits(readRepoFile(t, rel)) {
			if index := allowedIndex(rel, hit.line); index >= 0 {
				used[index] = true

				continue
			}

			t.Errorf("%s:%d: %s: %s", rel, hit.number, hit.flag, hit.line)
		}
	}

	for index, entry := range allowedHits {
		if !used[index] {
			t.Errorf("allowed hit %s %q (%s) matches nothing any more; remove it",
				entry.file, entry.line, entry.why)
		}
	}
}

// TestVacuousFlagsAreRecognized is the negative control: each flag, written as a gate would carry
// it, is found, and an allowed idiom in another file is not allowed.
func TestVacuousFlagsAreRecognized(t *testing.T) {
	t.Parallel()

	for _, sample := range []string{
		`"test": "vitest run --passWithNoTests"`,
		`"lint": "eslint src --no-error-on-unmatched-pattern"`,
		"\tgo vet ./... || true",
		"\tgo vet ./... ||:",
		"\tgo vet ./... || exit 0",
		"\t-go vet ./...",
		"\t@-go vet ./...",
		".IGNORE: go-vet",
		"\t$(MAKE) -i go-test",
		"\t$(MAKE) --ignore-errors go-test",
		"        continue-on-error: true",
		"    if: false",
		"    if: ${{ false }}",
		"    if: 'false'",
		"    if: \"${{ false }}\"",
	} {
		if len(vacuousHits(sample)) == 0 {
			t.Errorf("no vacuous flag found in %q", sample)
		}
	}

	idiom := `| { grep -E "$RELEASE_TAG_REGEX" || true; } \`
	if allowedIndex(".github/workflows/ci.yaml", idiom) >= 0 {
		t.Error("an allowed release.yaml idiom is allowed in ci.yaml too")
	}

	for _, line := range []string{
		"make audit-deps || true",
		`{ grep -E "$RELEASE_TAG_REGEX" || true; } | make audit-deps || true`,
	} {
		if allowedIndex(".github/workflows/release.yaml", line) >= 0 {
			t.Errorf("%q is allowed in release.yaml", line)
		}
	}
}

// vacuousHit is one line that carries a vacuous flag.
type vacuousHit struct {
	number int
	flag   string
	line   string
}

func vacuousHits(content string) []vacuousHit {
	var hits []vacuousHit

	for index, line := range strings.Split(content, "\n") {
		for _, flag := range vacuousFlags {
			if flag.pattern.MatchString(line) {
				hits = append(hits, vacuousHit{index + 1, flag.name, strings.TrimSpace(line)})
			}
		}
	}

	return hits
}

// allowedIndex returns the index of the allowedHits entry that covers line in file, or -1. A line
// with a second || is never covered: the idiom must be the only fallback on it.
func allowedIndex(file, line string) int {
	if strings.Count(line, "||") != 1 {
		return -1
	}

	for index, entry := range allowedHits {
		if entry.file == file && entry.line.MatchString(strings.TrimSpace(line)) {
			return index
		}
	}

	return -1
}
