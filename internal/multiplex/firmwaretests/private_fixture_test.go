package firmwaretests_test

import (
	"os"
	"path/filepath"
	"testing"
)

func copyPrivateNVRAM(t *testing.T, name string) string {
	t.Helper()
	source := filepath.Join("..", "..", "..", ".artifacts", name)
	contents, err := os.ReadFile(source) // #nosec G304 -- named private diagnostic fixture
	if os.IsNotExist(err) {
		t.Skipf("private %s fixture is not installed", name)
	}
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(target, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	return target
}
