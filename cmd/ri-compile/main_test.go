package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompileIndex(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "nodes"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "nodes", "bridge.md"), `# Limpha -> RI Bridge

Status: active

## Claim

RI turns receipts into structure.

## Current Receipts

- Metal smoke: ok.

## Rules From The First Failure

1. Raw memory is dangerous when shown as speech to continue.

## Next Hooks

- ri to kk
`)
	writeFile(t, filepath.Join(root, "quotes.md"), `# RI Quotes

## Boundary Quote

Source: `+"`/tmp/log`"+`

> Don't become me.

Why it matters:

- Useful as a future regression phrase.
`)
	writeFile(t, filepath.Join(root, "conflicts.md"), `# RI Conflicts

## Prompt Stuffing

- Risk: RAG leak.

## Breath Cap

Resolved by receipt.

- Receipt: one dream.
`)

	idx, err := compileIndex(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Nodes) != 1 || idx.Nodes[0].ID != "bridge" {
		t.Fatalf("bad nodes: %+v", idx.Nodes)
	}
	if idx.Nodes[0].Claim != "RI turns receipts into structure." {
		t.Fatalf("bad claim: %q", idx.Nodes[0].Claim)
	}
	if len(idx.Quotes) != 1 || !idx.Quotes[0].TestValue {
		t.Fatalf("bad quote extraction: %+v", idx.Quotes)
	}
	if len(idx.Conflicts) != 2 {
		t.Fatalf("bad conflict count: %+v", idx.Conflicts)
	}
	data, err := encodeIndex(idx, "lines")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "pressure\tnode=bridge") ||
		!strings.Contains(text, "quote\tid=boundary-quote") {
		t.Fatalf("line output missing expected records:\n%s", text)
	}
}

func TestContainsTermDoesNotMatchInsideWords(t *testing.T) {
	if containsTerm("first failure", "ri") {
		t.Fatal("short term ri matched inside first")
	}
	if !containsTerm("ri failure", "ri") {
		t.Fatal("short term ri should match as its own token")
	}
}

func writeFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}
