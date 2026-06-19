package ignitionmerge_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runHelper(t *testing.T, root string) (string, error) {
	t.Helper()
	cmd := exec.Command("bash", "elemental-ignition-merge.sh")
	cmd.Dir = "."
	cmd.Env = append(os.Environ(),
		"ELEMENTAL_IGNITION_MERGE_ROOT="+root,
		"ELEMENTAL_IGNITION_MERGE_LOG_TO_STDERR=1",
	)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestNoMarkerExitsWithoutChanges(t *testing.T) {
	root := t.TempDir()

	out, err := runHelper(t, root)

	if err != nil {
		t.Fatalf("helper failed: %v\n%s", err, out)
	}
	if pathExists(filepath.Join(root, "usr/lib/ignition/base.d/10-elemental-base.ign")) {
		t.Fatal("base ignition should not be staged without marker")
	}
	if !strings.Contains(out, "merge marker not found") {
		t.Fatalf("expected merge-marker-not-found diagnostic, got:\n%s", out)
	}
	if pathExists(filepath.Join(root, "usr/lib/ignition/base.d")) {
		t.Fatal("base.d directory should not be created when no merge is needed")
	}
}

func TestCopiesEmbeddedBaseIgnitionWhenMarkerExists(t *testing.T) {
	root := t.TempDir()
	base := `{"ignition":{"version":"3.5.0"}}`
	writeFile(t, filepath.Join(root, "ignition/elemental-merge"), "enabled\n")
	writeFile(t, filepath.Join(root, "ignition/config.ign"), base)
	writeFile(t, filepath.Join(root, "usr/lib/ignition/user.ign"), base)

	out, err := runHelper(t, root)

	if err != nil {
		t.Fatalf("helper failed: %v\n%s", err, out)
	}
	staged := readFile(t, filepath.Join(root, "usr/lib/ignition/base.d/10-elemental-base.ign"))
	if staged != base {
		t.Fatalf("unexpected staged base ignition: %s", staged)
	}
	if pathExists(filepath.Join(root, "usr/lib/ignition/user.ign")) {
		t.Fatal("merge helper should remove embedded config copied as user.ign")
	}
}

func TestCopiesEmbeddedBaseIgnitionFromMountedIgnitionPartitionLayout(t *testing.T) {
	root := t.TempDir()
	base := `{"ignition":{"version":"3.5.0"}}`

	writeFile(t, filepath.Join(root, "ignition/ignition/elemental-merge"), "enabled\n")
	writeFile(t, filepath.Join(root, "ignition/ignition/config.ign"), base)

	out, err := runHelper(t, root)
	if err != nil {
		t.Fatalf("helper failed: %v\n%s", err, out)
	}

	staged := readFile(t, filepath.Join(root, "usr/lib/ignition/base.d/10-elemental-base.ign"))
	if staged != base {
		t.Fatalf("unexpected staged base ignition: %s", staged)
	}
}

func TestFailsWhenMarkerExistsButBaseIgnitionMissing(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "ignition/elemental-merge"), "enabled\n")

	out, err := runHelper(t, root)

	if err == nil {
		t.Fatal("expected helper to fail")
	}
	if !strings.Contains(out, "missing embedded base ignition") {
		t.Fatalf("expected missing base diagnostic, got:\n%s", out)
	}
}
