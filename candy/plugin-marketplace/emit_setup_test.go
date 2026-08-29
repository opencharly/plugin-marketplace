package marketplace

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// emit_setup_test.go — the launcher is a shell script nothing executed, which is how it kept
// pointing at the `./charly` submodule for a full cutover after that submodule was removed
// (declaring one dragged the whole charly repo into consumer fetch trees). These tests RUN it
// against a stub `charly` on PATH, so the arguments it would really pass are asserted, not
// eyeballed.

// writeLauncher lays the emitted launcher down as an executable ./setup inside a fake
// marketplace root, and returns that root.
func writeLauncher(t *testing.T) string {
	t.Helper()
	em := emissions{}
	emitSetup(em)
	body, ok := em["plugins/setup"]
	if !ok {
		t.Fatal("emitSetup did not emit plugins/setup")
	}
	root := filepath.Join(t.TempDir(), "marketplace")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "setup")
	if err := os.WriteFile(path, body, 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

// stubCharly puts a `charly` on PATH that records its argv and cwd instead of running.
func stubCharly(t *testing.T) (binDir, logPath string) {
	t.Helper()
	binDir = t.TempDir()
	logPath = filepath.Join(binDir, "argv.log")
	stub := "#!/usr/bin/env bash\n" +
		"{ printf 'cwd=%s\\n' \"$PWD\"; printf 'argv=%s\\n' \"$*\"; } > " + logPath + "\n"
	if err := os.WriteFile(filepath.Join(binDir, "charly"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	return binDir, logPath
}

func runLauncher(t *testing.T, root, binDir string, env []string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(filepath.Join(root, "setup"), args...)
	cmd.Env = append(append(os.Environ(),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH")), env...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// The defect this file exists for: the launcher must not require a `./charly` checkout
// INSIDE the marketplace repo, because that submodule was deliberately removed.
func TestSetupLauncher_DoesNotRequireANestedCharlySubmodule(t *testing.T) {
	if strings.Contains(setupLauncher, `cd "$(dirname "$0")/charly"`) {
		t.Error("launcher cd's into a nested ./charly checkout; that submodule was removed " +
			"because declaring it dragged the whole charly repo into consumer fetch trees")
	}
	if strings.Contains(setupLauncher, "--out ..") {
		t.Error("launcher passes `--out ..`, which is the marketplace only when charly is " +
			"nested inside it — the arrangement that no longer exists")
	}
}

// A sibling charly checkout is the umbrella layout, and it must be found with no configuration.
func TestSetupLauncher_UsesASiblingCharlyCheckoutByDefault(t *testing.T) {
	root := writeLauncher(t)
	sibling := filepath.Join(filepath.Dir(root), "charly")
	if err := os.MkdirAll(sibling, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sibling, "charly.yml"), []byte("x:\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	binDir, logPath := stubCharly(t)

	out, err := runLauncher(t, root, binDir, nil, "--check")
	if err != nil {
		t.Fatalf("launcher failed with a sibling checkout present: %v\n%s", err, out)
	}
	rec, readErr := os.ReadFile(logPath)
	if readErr != nil {
		t.Fatalf("stub charly was never invoked: %v\n%s", readErr, out)
	}
	got := string(rec)
	// --check must reach the drift gate, not the generator.
	if !strings.Contains(got, "argv=marketplace drift ") {
		t.Errorf("--check did not dispatch to the drift gate; got:\n%s", got)
	}
	// Both paths absolute, --out pointing back at the marketplace root.
	if !strings.Contains(got, "--out "+root) {
		t.Errorf("--out is not the marketplace root %q; got:\n%s", root, got)
	}
	if !strings.Contains(got, "--root "+sibling) {
		t.Errorf("--root is not the charly checkout %q; got:\n%s", sibling, got)
	}
	// The verb is prescanned from the project root the CLI runs in, so cwd must be charly.
	if !strings.Contains(got, "cwd="+sibling) {
		t.Errorf("launcher did not run from the charly checkout; got:\n%s", got)
	}
}

func TestSetupLauncher_CharlySrcOverrides(t *testing.T) {
	root := writeLauncher(t)
	custom := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.MkdirAll(custom, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(custom, "charly.yml"), []byte("x:\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	binDir, logPath := stubCharly(t)

	out, err := runLauncher(t, root, binDir, []string{"CHARLY_SRC=" + custom})
	if err != nil {
		t.Fatalf("launcher failed with CHARLY_SRC set: %v\n%s", err, out)
	}
	rec, readErr := os.ReadFile(logPath)
	if readErr != nil {
		t.Fatalf("stub charly was never invoked: %v\n%s", readErr, out)
	}
	got := string(rec)
	// No --check: the generator, not the drift gate.
	if !strings.Contains(got, "argv=marketplace generate ") {
		t.Errorf("bare ./setup did not dispatch to the generator; got:\n%s", got)
	}
	if !strings.Contains(got, "--root "+custom) {
		t.Errorf("CHARLY_SRC was not honoured; got:\n%s", got)
	}
}

// With no checkout anywhere, the launcher must fail with an actionable message rather than a
// bare `cd: ./charly: No such file or directory`.
func TestSetupLauncher_FailsActionablyWithNoCharlyCheckout(t *testing.T) {
	root := writeLauncher(t)
	binDir, _ := stubCharly(t)

	out, err := runLauncher(t, root, binDir, []string{"CHARLY_SRC=" + filepath.Join(t.TempDir(), "absent")})
	if err == nil {
		t.Fatalf("launcher succeeded with no charly checkout; output:\n%s", out)
	}
	for _, want := range []string{"no charly checkout at", "CHARLY_SRC="} {
		if !strings.Contains(out, want) {
			t.Errorf("failure message lacks %q; got:\n%s", want, out)
		}
	}
}
