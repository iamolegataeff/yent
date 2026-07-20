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
	for _, script := range []string{
		filepath.Join(root, "DoE", "worldmodel", "interface_session.test.cjs"),
		filepath.Join(root, "DoE", "worldmodel", "event_stream.test.cjs"),
	} {
		cmd := exec.Command("node", script)
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s failed: %v\n%s", filepath.Base(script), err, string(out))
		}
	}
}

func TestWorldmodelInterfaceSessionContract(t *testing.T) {
	root := repoRootForTest(t)
	yentHTML := readTextFile(t, filepath.Join(root, "yent.html"))
	worldHTML := readTextFile(t, filepath.Join(root, "worldmodel.html"))
	doeC := readTextFile(t, filepath.Join(root, "DoE", "doe.c"))
	yentJS := readTextFile(t, filepath.Join(root, "DoE", "worldmodel", "yent.js"))
	worldJS := readTextFile(t, filepath.Join(root, "DoE", "worldmodel", "worldmodel.js"))

	assertScriptOrder(t, "yent.html", yentHTML,
		"/worldmodel/interface_session.js",
		"/worldmodel/event_stream.js",
		"/worldmodel/yent.js")
	assertScriptOrder(t, "worldmodel.html", worldHTML,
		"/worldmodel/interface_session.js",
		"/worldmodel/event_stream.js",
		"/worldmodel/worldmodel.js")

	if !strings.Contains(doeC, `"/worldmodel/interface_session.js"`) ||
		!strings.Contains(doeC, `"worldmodel/interface_session.js not found"`) {
		t.Fatalf("DoE server does not explicitly whitelist interface_session.js")
	}
	if !strings.Contains(doeC, `"/worldmodel/event_stream.js"`) ||
		!strings.Contains(doeC, `"worldmodel/event_stream.js not found"`) {
		t.Fatalf("DoE server does not explicitly whitelist event_stream.js")
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
		if !strings.Contains(tc.src, "window.YentEventStream") {
			t.Fatalf("%s does not use the shared event stream helper", tc.name)
		}
		if strings.Contains(tc.src, "messages = restored") {
			t.Fatalf("%s repopulates prompt messages from restored UI receipt", tc.name)
		}
		if strings.Contains(tc.src, "function parseSseEvents") || strings.Contains(tc.src, "sseBuffer") {
			t.Fatalf("%s still carries a page-local SSE parser", tc.name)
		}
	}
}

func assertScriptOrder(t *testing.T, name, html string, scripts ...string) {
	t.Helper()
	prevAt := -1
	for _, script := range scripts {
		at := strings.Index(html, script)
		if at < 0 {
			t.Fatalf("%s missing script %s", name, script)
		}
		if prevAt >= 0 && prevAt > at {
			t.Fatalf("%s loads script out of order near %s", name, script)
		}
		prevAt = at
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
