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
	"regexp"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// pinSource is one place a version is written: a file and the pattern whose first group is the
// version there. Every match counts.
type pinSource struct {
	file    string
	pattern *regexp.Regexp
}

// setupGo is the action whose Go version must come from go.mod.
const setupGo = "actions/setup-go"

// TestGoLineIsOneValue: the go directive and the Dockerfile's build stage name the same Go line
// (major.minor), so the image is never compiled by a Go the module does not declare and CI does not
// test with. Dependabot may move the image's patch release on its own; the line moves with go.mod,
// by hand.
func TestGoLineIsOneValue(t *testing.T) {
	t.Parallel()

	expectOneValue(t, "Go line", []pinSource{
		{file: "go.mod", pattern: regexp.MustCompile(`(?m)^go (\d+\.\d+)(?:\.\d+)?$`)},
		{
			file:    "deployments/docker/Dockerfile",
			pattern: regexp.MustCompile(`(?m)^FROM\s.*dhi\.io/golang:(\d+\.\d+)[.-]`),
		},
	})
}

// TestShellCheckIsOneVersion: droast's ShellCheck bridge runs the shellcheck on PATH, which CI
// installs with setup-shellcheck; it must be the release the shellcheck-py hook wraps (its rev adds
// a fourth component for the wrapper), which Dependabot moves.
func TestShellCheckIsOneVersion(t *testing.T) {
	t.Parallel()

	expectOneValue(t, "ShellCheck", []pinSource{
		{
			file: ".pre-commit-config.yaml",
			pattern: regexp.MustCompile(
				`repo: https://github\.com/shellcheck-py/shellcheck-py\s+rev:\s*(v\d+\.\d+\.\d+)`,
			),
		},
		{
			file:    ".github/actions/setup-shellcheck/action.yaml",
			pattern: regexp.MustCompile(`SHELLCHECK_VERSION:\s*["']?(v[\d.]+)`),
		},
	})
}

// TestDroastIsOneVersion: the droast hook builds its rev, and the go-test job installs the release
// binary that the label meta-test drives; both must be the same release, which Dependabot moves
// through the hook's rev.
func TestDroastIsOneVersion(t *testing.T) {
	t.Parallel()

	expectOneValue(t, "droast", []pinSource{
		{
			file: ".pre-commit-config.yaml",
			pattern: regexp.MustCompile(
				`repo: https://github\.com/immanuwell/dockerfile-roast\s+rev:\s*(v?[\d.]+)`,
			),
		},
		{
			file:    ".github/actions/setup-droast/action.yaml",
			pattern: regexp.MustCompile(`DROAST_VERSION:\s*["']?(v?[\d.]+)`),
		},
	})
}

// TestActionStepsReadGoMod: every setup-go step under .github reads go.mod, and no step writes a
// go-version of its own that the Go line test would not see.
func TestActionStepsReadGoMod(t *testing.T) {
	t.Parallel()

	steps := 0

	for _, file := range githubYAML(t) {
		problems, count, err := goVersionProblems(file, []byte(readRepoFile(t, file)))
		if err != nil {
			t.Fatal(err)
		}

		steps += count

		for _, problem := range problems {
			t.Error(problem)
		}
	}

	if steps == 0 {
		t.Fatal("no setup-go step found under .github; the rule would check nothing")
	}
}

// TestGoVersionRules is the negative control of the step rule: each unpinned form is a problem.
func TestGoVersionRules(t *testing.T) {
	t.Parallel()

	problems, count, err := goVersionProblems("sample.yaml", []byte(`jobs:
  ok:
    steps:
      - uses: actions/setup-go@0123456789abcdef0123456789abcdef01234567 # v7.0.0
        with:
          go-version-file: go.mod
  bad:
    steps:
      - uses: actions/setup-go@0123456789abcdef0123456789abcdef01234567
        with:
          go-version: "1.26"
      - uses: actions/setup-go@0123456789abcdef0123456789abcdef01234567
      - uses: actions/setup-go@0123456789abcdef0123456789abcdef01234567
        with:
          go-version-file: .go-version
      - uses: some/other-action@0123456789abcdef0123456789abcdef01234567
        with:
          go-version: "1.27"
`))
	if err != nil {
		t.Fatal(err)
	}

	// The second step breaks both rules.
	const wantProblems = 5

	if count != 4 || len(problems) != wantProblems {
		t.Errorf("want 4 setup-go steps and %d problems, got %d and %q",
			wantProblems, count, problems)
	}
}

// goVersionProblems finds every mapping with a uses: key anywhere in the document, so job steps and
// composite action steps alike, and returns the Go version rule violations and the number of
// setup-go steps seen.
func goVersionProblems(file string, content []byte) (problems []string, count int, err error) {
	var document any

	err = yaml.Unmarshal(content, &document)
	if err != nil {
		return nil, 0, fmt.Errorf("parse %s: %w", file, err)
	}

	var walk func(node any)

	walk = func(node any) {
		switch value := node.(type) {
		case map[string]any:
			if uses, ok := value["uses"].(string); ok {
				action, _, _ := strings.Cut(uses, "@")
				inputs, _ := value["with"].(map[string]any)

				if version, found := inputs["go-version"]; found {
					problems = append(problems, fmt.Sprintf(
						"%s: %s gets go-version %v; the Go line comes from go.mod",
						file, action, version))
				}

				if action == setupGo {
					count++

					if inputs["go-version-file"] != "go.mod" {
						problems = append(
							problems,
							file+": setup-go must read go-version-file: go.mod",
						)
					}
				}
			}

			for _, child := range value {
				walk(child)
			}
		case []any:
			for _, child := range value {
				walk(child)
			}
		}
	}

	walk(document)

	return problems, count, nil
}

// expectOneValue fails unless every source has the pin and all found values agree.
func expectOneValue(t *testing.T, what string, sources []pinSource) {
	t.Helper()

	found := map[string][]string{}

	for _, source := range sources {
		matches := source.pattern.FindAllStringSubmatch(readRepoFile(t, source.file), -1)
		if len(matches) == 0 {
			t.Errorf("%s: %s does not write it (%s)", what, source.file, source.pattern)
		}

		for _, match := range matches {
			found[match[1]] = append(found[match[1]], source.file)
		}
	}

	if len(found) > 1 {
		values := make([]string, 0, len(found))
		for value, files := range found {
			values = append(values, value+" in "+strings.Join(slices.Compact(files), ", "))
		}

		slices.Sort(values)
		t.Errorf("%s is not one value: %s", what, strings.Join(values, "; "))
	}
}
