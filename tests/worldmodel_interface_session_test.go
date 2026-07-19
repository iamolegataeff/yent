package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorldmodelInterfaceSessionHelper(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skipf("node not found: %v", err)
	}
	root := repoRootForTest(t)
	cmd := exec.Command("node", filepath.Join(root, "DoE", "worldmodel", "interface_session.test.cjs"))
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("interface session helper test failed: %v\n%s", err, string(out))
	}
}

func TestWorldmodelInterfaceSessionContract(t *testing.T) {
	root := repoRootForTest(t)
	yentHTML := readTextFile(t, filepath.Join(root, "yent.html"))
	worldHTML := readTextFile(t, filepath.Join(root, "worldmodel.html"))
	doeC := readTextFile(t, filepath.Join(root, "DoE", "doe.c"))
	yentJS := readTextFile(t, filepath.Join(root, "DoE", "worldmodel", "yent.js"))
	worldJS := readTextFile(t, filepath.Join(root, "DoE", "worldmodel", "worldmodel.js"))

	assertScriptOrder(t, "yent.html", yentHTML, "/worldmodel/interface_session.js", "/worldmodel/yent.js")
	assertScriptOrder(t, "worldmodel.html", worldHTML, "/worldmodel/interface_session.js", "/worldmodel/worldmodel.js")

	if !strings.Contains(doeC, `"/worldmodel/interface_session.js"`) ||
		!strings.Contains(doeC, `"worldmodel/interface_session.js not found"`) {
		t.Fatalf("DoE server does not explicitly whitelist interface_session.js")
	}

	for _, tc := range []struct {
		name string
		src  string
	}{
		{"yent.js", yentJS},
		{"worldmodel.js", worldJS},
	} {
		if !strings.Contains(tc.src, "window.YentInterfaceSession") {
			t.Fatalf("%s does not use the shared interface session helper", tc.name)
		}
		if strings.Contains(tc.src, "messages = restored") {
			t.Fatalf("%s repopulates prompt messages from restored UI receipt", tc.name)
		}
	}
}

func assertScriptOrder(t *testing.T, name, html, first, second string) {
	t.Helper()
	firstAt := strings.Index(html, first)
	secondAt := strings.Index(html, second)
	if firstAt < 0 {
		t.Fatalf("%s missing script %s", name, first)
	}
	if secondAt < 0 {
		t.Fatalf("%s missing script %s", name, second)
	}
	if firstAt > secondAt {
		t.Fatalf("%s loads %s after %s", name, first, second)
	}
}

func readTextFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
