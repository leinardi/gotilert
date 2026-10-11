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

package dockerfiles_test

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/leinardi/gotilert/test/dockerfiles"
	"github.com/leinardi/gotilert/test/internal/toolgate"
)

const (
	shippedDir        = "deployments/docker"
	shippedDockerfile = "deployments/docker/Dockerfile"
	releaseWorkflow   = ".github/workflows/release.yaml"
	hadolintConfig    = ".hadolint.yaml"
	runtimeUser       = "65532:65532"
)

// TestShippedDockerfileIsTheOnlyOne pins that the repository holds exactly the image that ships. A
// second Dockerfile would look shipped to every reader and be built by nothing.
func TestShippedDockerfileIsTheOnlyOne(t *testing.T) {
	t.Parallel()

	if found := allDockerfiles(t); !slices.Equal(found, []string{shippedDockerfile}) {
		t.Errorf("the repository holds the Dockerfiles %v, want exactly [%s]", found,
			shippedDockerfile)
	}
}

// TestSyntaxDirectiveIsPinned pins the BuildKit frontend that reads the Dockerfile. A bare
// docker/dockerfile:1.7 is a tag that can be moved under a build, and Dependabot never updates the
// line, so it is a literal reference pinned by digest.
func TestSyntaxDirectiveIsPinned(t *testing.T) {
	t.Parallel()

	syntax, ok := dockerfiles.Syntax(readRepoFile(t, shippedDockerfile))
	if !ok {
		t.Fatalf("%s has no syntax parser directive on its first line", shippedDockerfile)
	}

	if dockerfiles.Repository(syntax) != "docker/dockerfile" {
		t.Errorf("%s reads with the frontend %q, want docker/dockerfile", shippedDockerfile, syntax)
	}

	if !dockerfiles.DigestPinned(syntax) {
		t.Errorf("%s reads with the frontend %q, which is not pinned by digest", shippedDockerfile,
			syntax)
	}
}

// TestStagesArePinnedByTag pins the two stages, each from a literal base with an explicit tag. The
// bases are pinned by tag on purpose, not by digest: dhi.io republishes its tags with security
// fixes, and a rebuild picks them up (the Dockerfile's header). A base on latest, with no tag or
// behind a build argument is never what this repository ships.
func TestStagesArePinnedByTag(t *testing.T) {
	t.Parallel()

	stages := shippedStages(t)

	names := make([]string, 0, len(stages))
	for _, stage := range stages {
		names = append(names, stage.Name)

		if !dockerfiles.TagPinned(stage.Image) {
			t.Errorf("stage %q is FROM %q, which is not a literal reference with an explicit tag",
				stage.Name, stage.Image)
		}
	}

	if !slices.Equal(names, []string{"build", ""}) {
		t.Errorf("the stages are %q, want build and then the unnamed runtime stage", names)
	}
}

// TestBuildStage pins where the binary is compiled and how: on the build platform, since Go
// cross-compiles; static; reproducible (no paths, no VCS stamp); and with the version stamped into
// package main, where cmd/gotilert/version.go declares it.
func TestBuildStage(t *testing.T) {
	t.Parallel()

	build := shippedStages(t)[0]

	if build.Platform != "$BUILDPLATFORM" {
		t.Errorf("the build stage runs on --platform=%q, want $BUILDPLATFORM", build.Platform)
	}

	if got := dockerfiles.Repository(build.Image); got != "dhi.io/golang" {
		t.Errorf("the build stage is FROM %s, want dhi.io/golang", got)
	}

	instructions := joined(build)
	for _, want := range []string{
		"ARG VERSION", "ARG COMMIT", "ARG DATE", "ARG TARGETOS", "ARG TARGETARCH",
		"CGO_ENABLED=0", "-trimpath", "-buildvcs=false",
		"-X 'main.version=${VERSION}'", "-X 'main.commit=${COMMIT}'", "-X 'main.date=${DATE}'",
		"-o /out/gotilert ./cmd/gotilert",
	} {
		if !strings.Contains(instructions, want) {
			t.Errorf("the build stage lacks %q:\n%s", want, instructions)
		}
	}
}

// TestRuntimeStage pins the posture of the image that ships: no shell to run anything in, an
// unprivileged numeric user that does not follow the base's default, the binary as the entrypoint,
// and the seven OCI labels, fed by the build arguments the binary is stamped with.
func TestRuntimeStage(t *testing.T) {
	t.Parallel()

	runtime := shippedStages(t)[1]

	if got := dockerfiles.Repository(runtime.Image); got != "dhi.io/static" {
		t.Errorf("the runtime stage is FROM %s, want dhi.io/static", got)
	}

	if runtime.Platform != "" {
		t.Errorf("the runtime stage pins --platform=%s; it has to follow the target platform",
			runtime.Platform)
	}

	want := map[string][]string{
		"USER":       {runtimeUser},
		"EXPOSE":     {"8008"},
		"ENTRYPOINT": {`["/usr/local/bin/gotilert"]`},
		"COPY":       {"--from=build /out/gotilert /usr/local/bin/gotilert"},
		"ARG":        {`VERSION="dev"`, `COMMIT="none"`, `DATE="unknown"`},
	}

	seen := map[string][]string{}

	for _, instruction := range runtime.Instructions {
		switch instruction.Keyword {
		case "RUN", "SHELL", "ADD":
			t.Errorf("the runtime stage carries %s %s; it has no shell", instruction.Keyword,
				instruction.Args)
		}

		seen[instruction.Keyword] = append(seen[instruction.Keyword], instruction.Args)
	}

	for keyword, args := range want {
		if !slices.Equal(seen[keyword], args) {
			t.Errorf("the runtime stage has %s %q, want exactly %q", keyword, seen[keyword], args)
		}
	}

	labels := dockerfiles.Labels(runtime)
	wantLabels := map[string]string{
		"org.opencontainers.image.title":       "gotilert",
		"org.opencontainers.image.description": labels["org.opencontainers.image.description"],
		"org.opencontainers.image.source":      "https://github.com/leinardi/gotilert",
		"org.opencontainers.image.version":     "${VERSION}",
		"org.opencontainers.image.revision":    "${COMMIT}",
		"org.opencontainers.image.created":     "${DATE}",
		"org.opencontainers.image.licenses":    "MIT",
	}

	if labels["org.opencontainers.image.description"] == "" {
		t.Error("the runtime stage has no org.opencontainers.image.description")
	}

	for key, value := range wantLabels {
		if labels[key] != value {
			t.Errorf("label %s is %q, want %q", key, labels[key], value)
		}
	}

	if len(labels) != len(wantLabels) {
		t.Errorf("the runtime stage carries labels %v, want exactly the seven OCI labels", labels)
	}
}

// TestHadolintEnforcesTheLabels runs hadolint, with the repository's configuration, on the shipped
// Dockerfile and on copies that drop a required label or add an unknown one: the label schema is
// only a gate if hadolint fails the last two.
func TestHadolintEnforcesTheLabels(t *testing.T) {
	t.Parallel()

	hadolint := toolgate.Lookup(t, toolgate.Hadolint)
	config := repoPath(t, hadolintConfig)
	readRepoFile(t, hadolintConfig)

	shipped := readRepoFile(t, shippedDockerfile)
	created := `	org.opencontainers.image.created="${DATE}" \` + "\n"

	if !strings.Contains(shipped, created) {
		t.Fatalf("%s has no created label line to drop", shippedDockerfile)
	}

	for name, test := range map[string]struct {
		content string
		pass    bool
		want    string
	}{
		"the shipped Dockerfile": {content: shipped, pass: true},
		"a required label dropped": {
			content: strings.Replace(shipped, created, "", 1),
			want:    "DL3049",
		},
		"an unknown label added": {
			content: strings.Replace(shipped, created,
				created+`	org.opencontainers.image.vendor="someone" \`+"\n", 1),
			want: "DL3050",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "Dockerfile")

			err := os.WriteFile(path, []byte(test.content), 0o600)
			if err != nil {
				t.Fatal(err)
			}

			cmd := exec.CommandContext(t.Context(), hadolint, "--config", config, "--no-color",
				path)

			out, err := cmd.CombinedOutput()
			if test.pass && err != nil {
				t.Errorf("hadolint rejected it: %v\n%s", err, out)
			}

			if !test.pass && (err == nil || !strings.Contains(string(out), test.want)) {
				t.Errorf("hadolint did not fail it with %s: %v\n%s", test.want, err, out)
			}
		})
	}
}

// TestDockerignoreIsAtTheContextRoot pins where the build context is filtered. Every build sends
// the repository root (make docker-build's DOCKER_CONTEXT, release.yaml's context), and BuildKit
// reads only <context>/.dockerignore or <Dockerfile>.dockerignore; a .dockerignore anywhere else
// filters nothing.
func TestDockerignoreIsAtTheContextRoot(t *testing.T) {
	t.Parallel()

	ignore := readRepoFile(t, ".dockerignore")
	for _, want := range []string{".git", ".github", ".claude", ".agents", ".serena", "test"} {
		if !slices.Contains(strings.Split(ignore, "\n"), want) {
			t.Errorf(".dockerignore does not exclude %s", want)
		}
	}

	if _, ok := readOptional(t, shippedDir+"/.dockerignore"); ok {
		t.Errorf("%s/.dockerignore exists, and BuildKit never reads it", shippedDir)
	}

	if !regexp.MustCompile(`(?m)^DOCKER_CONTEXT\s*\?=\s*\.$`).
		MatchString(readRepoFile(t, "Makefile")) {
		t.Error("the Makefile's DOCKER_CONTEXT is not the repository root")
	}

	release := readRepoFile(t, releaseWorkflow)
	if !strings.Contains(release, "context: .\n") ||
		!strings.Contains(release, "file: "+shippedDockerfile+"\n") {
		t.Errorf("%s does not build %s with the repository root as the context", releaseWorkflow,
			shippedDockerfile)
	}
}

// TestParseRejectsWhatTheAssertionsMustNotMiss is the negative control for the reader: each of
// these inputs has to come out as something the assertions above refuse.
func TestParseRejectsWhatTheAssertionsMustNotMiss(t *testing.T) {
	t.Parallel()

	const digest = "@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	for name, image := range map[string]string{
		"no tag":                  "dhi.io/static",
		"latest":                  "dhi.io/static:latest",
		"a variable tag":          "dhi.io/golang:${GO_LINE}-alpine3.23-dev",
		"a variable image":        "$BASE",
		"a digest with no tag":    "dhi.io/static" + digest,
		"a stage name, not a ref": "build",
	} {
		if dockerfiles.TagPinned(image) {
			t.Errorf("%s (%q) passes as tag-pinned", name, image)
		}
	}

	for _, image := range []string{"dhi.io/static:20250419", "dhi.io/static:20250419" + digest} {
		if !dockerfiles.TagPinned(image) {
			t.Errorf("%q does not pass as tag-pinned", image)
		}
	}

	for name, image := range map[string]string{
		"a tag only":           "docker/dockerfile:1.7",
		"a short digest":       "docker/dockerfile:1.7@sha256:0123",
		"a digest with no tag": "docker/dockerfile" + digest,
	} {
		if dockerfiles.DigestPinned(image) {
			t.Errorf("%s (%q) passes as digest-pinned", name, image)
		}
	}

	stages, err := dockerfiles.Parse(
		"FROM a:1 AS one\nRUN echo \\\n  two\n# note\nFROM b:2\nUSER 1:1\n",
	)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if len(stages) != 2 || stages[0].Name != "one" || stages[1].Name != "" ||
		joined(stages[0]) != "RUN echo two" || joined(stages[1]) != "USER 1:1" {
		t.Errorf("parsed %+v", stages)
	}

	for name, content := range map[string]string{
		"no directive":         "FROM a:1\n",
		"after a comment":      "# note\n# syntax=docker/dockerfile:1.7" + digest + "\n",
		"after a blank line":   "\n# syntax=docker/dockerfile:1.7" + digest + "\n",
		"after an instruction": "FROM a:1\n# syntax=docker/dockerfile:1.7" + digest + "\n",
	} {
		if syntax, ok := dockerfiles.Syntax(content); ok {
			t.Errorf("%s: read the syntax directive %q, which BuildKit ignores", name, syntax)
		}
	}

	_, err = dockerfiles.Parse("FROM a:1\nRUN echo \\\n")
	if err == nil {
		t.Error("a file ending inside a continuation parsed")
	}
}

func shippedStages(t *testing.T) []dockerfiles.Stage {
	t.Helper()

	stages, err := dockerfiles.Parse(readRepoFile(t, shippedDockerfile))
	if err != nil {
		t.Fatalf("%s: %v", shippedDockerfile, err)
	}

	if len(stages) != 2 {
		t.Fatalf("%s has %d stages, want 2", shippedDockerfile, len(stages))
	}

	return stages
}

func joined(stage dockerfiles.Stage) string {
	lines := make([]string, 0, len(stage.Instructions))
	for _, instruction := range stage.Instructions {
		lines = append(lines, instruction.Keyword+" "+instruction.Args)
	}

	return strings.Join(lines, "\n")
}

func isDockerfile(name string) bool {
	return name == "Dockerfile" || strings.HasSuffix(name, ".Dockerfile")
}

// allDockerfiles walks the repository for Dockerfiles. Walking rather than asking git keeps a new
// file in the test cache key.
func allDockerfiles(t *testing.T) []string {
	t.Helper()

	root, err := os.OpenRoot(repoPath(t, "."))
	if err != nil {
		t.Fatalf("open the repository: %v", err)
	}

	defer root.Close()

	var found []string

	err = fs.WalkDir(root.FS(), ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "node_modules", "dist", ".cache":
				return fs.SkipDir
			}

			return nil
		}

		if isDockerfile(entry.Name()) {
			found = append(found, path)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walk the repository: %v", err)
	}

	if !slices.Contains(found, shippedDockerfile) {
		t.Fatalf("the walk did not find %s, so it would find nothing else either",
			shippedDockerfile)
	}

	return found
}

func repoPath(t *testing.T, rel string) string {
	t.Helper()

	return filepath.Join(toolgate.RepoRoot(t), filepath.FromSlash(rel))
}

func readRepoFile(t *testing.T, rel string) string {
	t.Helper()

	content, ok := readOptional(t, rel)
	if !ok {
		t.Fatalf("%s does not exist", rel)
	}

	return content
}

// readOptional reads a file that may not exist. Reading, or failing to, records the path in the
// test cache key either way.
func readOptional(t *testing.T, rel string) (string, bool) {
	t.Helper()

	data, err := os.ReadFile(repoPath(t, rel))
	if os.IsNotExist(err) {
		return "", false
	}

	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}

	return string(data), true
}
