package riindex

import (
	"strings"
	"testing"
)

const sampleLines = `ri	generated_at=2026-06-30T00:00:00Z	root=ri
node	id=n1	title=Node One	status=active	tags=ri,kk	layers=ri,path	path=ri/nodes/n1.md
pressure	node=n1	text=Past inner monologues should reach generation as pressure, not command.
pressure	node=n1	text=RI must not become a larger prompt. It must become structure.
quote	id=q1	test=true	source=/tmp/log	text=Don't become me. We're already two.
quote	id=q2	test=false	source=/tmp/log	text=Now you. Now you're real.
conflict	id=c1	status=open	title=KK/Dario vs Prompt Stuffing
conflict	id=c2	status=resolved	title=Autonomous Breath vs Receipt Bounds
`

func TestParseEscapes(t *testing.T) {
	records, err := Parse(strings.NewReader("pressure\ttext=a\\tb\\ncc\\\\d\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("want 1 record, got %d", len(records))
	}
	if got := records[0].Fields["text"]; got != "a\tb\ncc\\d" {
		t.Fatalf("bad unescape: %q", got)
	}
}

func TestRuntimeSelectIsBoundedSurface(t *testing.T) {
	records, err := Parse(strings.NewReader(sampleLines))
	if err != nil {
		t.Fatal(err)
	}
	got, err := Select(records, "runtime", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("runtime should select 2 pressure + 1 test quote + 1 open conflict, got %d", len(got))
	}
	for _, rec := range got {
		if rec.Kind == "quote" && rec.Fields["test"] == "false" {
			t.Fatalf("runtime leaked non-test quote: %+v", rec)
		}
		if rec.Kind == "conflict" && rec.Fields["status"] != "open" {
			t.Fatalf("runtime leaked non-open conflict: %+v", rec)
		}
	}
}

func TestSelectMaxCapsRecords(t *testing.T) {
	records, err := Parse(strings.NewReader(sampleLines))
	if err != nil {
		t.Fatal(err)
	}
	got, err := Select(records, "runtime", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("max cap failed, got %d", len(got))
	}
}

func TestEncodeLines(t *testing.T) {
	records, err := Parse(strings.NewReader(sampleLines))
	if err != nil {
		t.Fatal(err)
	}
	selected, err := Select(records, "test-quotes", 0)
	if err != nil {
		t.Fatal(err)
	}
	data, err := Encode(Packet{Input: "x", Mode: "test-quotes", Records: selected}, "lines")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "packet\tmode=test-quotes\tinput=x\tcount=1") {
		t.Fatalf("missing packet header: %s", text)
	}
	if strings.Contains(text, "Now you. Now you're real.") {
		t.Fatalf("line output leaked non-test quote: %s", text)
	}
}
