package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
)

type Index struct {
	GeneratedAt string     `json:"generated_at"`
	Root        string     `json:"root"`
	Nodes       []Node     `json:"nodes"`
	Quotes      []Quote    `json:"quotes"`
	Conflicts   []Conflict `json:"conflicts"`
}

type Node struct {
	ID              string   `json:"id"`
	Title           string   `json:"title"`
	Status          string   `json:"status,omitempty"`
	Tags            []string `json:"tags,omitempty"`
	Layers          []string `json:"layers,omitempty"`
	Claim           string   `json:"claim,omitempty"`
	SourceReceipts  []string `json:"source_receipts,omitempty"`
	KnownConflicts  []string `json:"known_conflicts,omitempty"`
	PressurePhrases []string `json:"pressure_phrases,omitempty"`
	NextHooks       []string `json:"next_hooks,omitempty"`
	Path            string   `json:"path"`
}

type Quote struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Source    string   `json:"source,omitempty"`
	Text      string   `json:"text"`
	Reasons   []string `json:"reasons,omitempty"`
	TestValue bool     `json:"test_value"`
	Path      string   `json:"path"`
}

type Conflict struct {
	ID     string   `json:"id"`
	Title  string   `json:"title"`
	Status string   `json:"status"`
	Points []string `json:"points,omitempty"`
	Path   string   `json:"path"`
}

type document struct {
	title    string
	status   string
	sections map[string][]string
}

func main() {
	root := flag.String("root", "ri", "RI root directory")
	out := flag.String("out", "", "output JSON path; stdout when empty")
	format := flag.String("format", "json", "output format: json or lines")
	flag.Parse()

	idx, err := compileIndex(*root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ri-compile: %v\n", err)
		os.Exit(1)
	}
	data, err := encodeIndex(idx, *format)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ri-compile: encode: %v\n", err)
		os.Exit(1)
	}
	if *out == "" {
		_, _ = os.Stdout.Write(data)
		return
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "ri-compile: mkdir: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, data, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "ri-compile: write: %v\n", err)
		os.Exit(1)
	}
}

func encodeIndex(idx Index, format string) ([]byte, error) {
	switch format {
	case "json":
		data, err := json.MarshalIndent(idx, "", "  ")
		if err != nil {
			return nil, err
		}
		return append(data, '\n'), nil
	case "lines":
		return []byte(encodeLines(idx)), nil
	default:
		return nil, fmt.Errorf("unknown format %q", format)
	}
}

func encodeLines(idx Index) string {
	var b strings.Builder
	fmt.Fprintf(&b, "ri\tgenerated_at=%s\troot=%s\n", idx.GeneratedAt, escapeField(idx.Root))
	for _, node := range idx.Nodes {
		fmt.Fprintf(&b, "node\tid=%s\ttitle=%s\tstatus=%s\ttags=%s\tlayers=%s\tpath=%s\n",
			escapeField(node.ID),
			escapeField(node.Title),
			escapeField(node.Status),
			escapeField(strings.Join(node.Tags, ",")),
			escapeField(strings.Join(node.Layers, ",")),
			escapeField(node.Path),
		)
		for _, p := range node.PressurePhrases {
			fmt.Fprintf(&b, "pressure\tnode=%s\ttext=%s\n", escapeField(node.ID), escapeField(p))
		}
		for _, hook := range node.NextHooks {
			fmt.Fprintf(&b, "hook\tnode=%s\ttext=%s\n", escapeField(node.ID), escapeField(hook))
		}
	}
	for _, q := range idx.Quotes {
		fmt.Fprintf(&b, "quote\tid=%s\ttest=%t\tsource=%s\ttext=%s\n",
			escapeField(q.ID),
			q.TestValue,
			escapeField(q.Source),
			escapeField(q.Text),
		)
	}
	for _, c := range idx.Conflicts {
		fmt.Fprintf(&b, "conflict\tid=%s\tstatus=%s\ttitle=%s\n",
			escapeField(c.ID),
			escapeField(c.Status),
			escapeField(c.Title),
		)
	}
	return b.String()
}

func escapeField(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\t", "\\t")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}

func compileIndex(root string) (Index, error) {
	var idx Index
	idx.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	idx.Root = root

	nodes, err := compileNodes(filepath.Join(root, "nodes"))
	if err != nil {
		return idx, err
	}
	idx.Nodes = nodes

	quotes, err := compileQuotes(filepath.Join(root, "quotes.md"))
	if err != nil {
		return idx, err
	}
	idx.Quotes = quotes

	conflicts, err := compileConflicts(filepath.Join(root, "conflicts.md"))
	if err != nil {
		return idx, err
	}
	idx.Conflicts = conflicts
	return idx, nil
}

func compileNodes(dir string) ([]Node, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	nodes := make([]Node, 0, len(paths))
	for _, path := range paths {
		doc, err := readDoc(path)
		if err != nil {
			return nil, err
		}
		body := strings.ToLower(strings.Join(allLines(doc), "\n"))
		id := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		node := Node{
			ID:              id,
			Title:           doc.title,
			Status:          doc.status,
			Tags:            inferTags(body),
			Layers:          inferLayers(body),
			Claim:           prose(doc.sections["Claim"]),
			SourceReceipts:  bullets(doc.sections["Current Receipts"], doc.sections["Source Receipts"]),
			KnownConflicts:  bullets(doc.sections["Conflicts"]),
			PressurePhrases: numbered(doc.sections["Rules From The First Failure"]),
			NextHooks:       bullets(doc.sections["Next Hooks"]),
			Path:            path,
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func compileQuotes(path string) ([]Quote, error) {
	doc, err := readDoc(path)
	if err != nil {
		return nil, err
	}
	var quotes []Quote
	keys := sortedSectionKeys(doc.sections)
	for _, title := range keys {
		lines := doc.sections[title]
		source := ""
		var quoteLines, reasons []string
		inReasons := false
		for _, line := range lines {
			trim := strings.TrimSpace(line)
			switch {
			case strings.HasPrefix(trim, "Source:"):
				source = strings.TrimSpace(strings.TrimPrefix(trim, "Source:"))
				source = strings.Trim(source, "`")
			case strings.HasPrefix(trim, "Why it matters:"):
				inReasons = true
			case strings.HasPrefix(trim, ">"):
				quoteLines = append(quoteLines, strings.TrimSpace(strings.TrimPrefix(trim, ">")))
			case inReasons && strings.HasPrefix(trim, "- "):
				reasons = append(reasons, strings.TrimSpace(strings.TrimPrefix(trim, "- ")))
			case inReasons && len(reasons) > 0 && trim != "" && !strings.HasPrefix(trim, "#"):
				reasons[len(reasons)-1] += " " + trim
			}
		}
		text := strings.TrimSpace(strings.Join(quoteLines, " "))
		if text == "" {
			continue
		}
		allReason := strings.ToLower(strings.Join(reasons, " "))
		quotes = append(quotes, Quote{
			ID:        slug(title),
			Title:     title,
			Source:    source,
			Text:      text,
			Reasons:   reasons,
			TestValue: strings.Contains(allReason, "test") || strings.Contains(allReason, "regression"),
			Path:      path,
		})
	}
	return quotes, nil
}

func compileConflicts(path string) ([]Conflict, error) {
	doc, err := readDoc(path)
	if err != nil {
		return nil, err
	}
	var conflicts []Conflict
	for _, title := range sortedSectionKeys(doc.sections) {
		lines := doc.sections[title]
		body := strings.ToLower(strings.Join(lines, "\n"))
		status := "open"
		if strings.Contains(body, "resolved") {
			status = "resolved"
		}
		conflicts = append(conflicts, Conflict{
			ID:     slug(title),
			Title:  title,
			Status: status,
			Points: bullets(lines),
			Path:   path,
		})
	}
	return conflicts, nil
}

func readDoc(path string) (document, error) {
	f, err := os.Open(path)
	if err != nil {
		return document{}, err
	}
	defer f.Close()

	doc := document{sections: map[string][]string{}}
	current := ""
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		trim := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trim, "# ") && doc.title == "":
			doc.title = strings.TrimSpace(strings.TrimPrefix(trim, "# "))
		case strings.HasPrefix(trim, "Status:"):
			doc.status = strings.TrimSpace(strings.TrimPrefix(trim, "Status:"))
		case strings.HasPrefix(trim, "## "):
			current = strings.TrimSpace(strings.TrimPrefix(trim, "## "))
			doc.sections[current] = nil
		case current != "":
			doc.sections[current] = append(doc.sections[current], line)
		}
	}
	if err := sc.Err(); err != nil {
		return document{}, err
	}
	if doc.title == "" {
		return document{}, errors.New("missing title in " + path)
	}
	return doc, nil
}

func prose(lines []string) string {
	var parts []string
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") || strings.HasPrefix(trim, "- ") || isNumbered(trim) {
			continue
		}
		parts = append(parts, trim)
	}
	return strings.Join(parts, " ")
}

func bullets(groups ...[]string) []string {
	var out []string
	for _, lines := range groups {
		for _, line := range lines {
			trim := strings.TrimSpace(line)
			if strings.HasPrefix(trim, "- ") {
				out = append(out, strings.TrimSpace(strings.TrimPrefix(trim, "- ")))
				continue
			}
			if len(out) > 0 && trim != "" && !strings.HasPrefix(trim, "#") && !isNumbered(trim) {
				out[len(out)-1] += " " + trim
			}
		}
	}
	return out
}

func numbered(lines []string) []string {
	var out []string
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if isNumbered(trim) {
			out = append(out, strings.TrimSpace(trim[strings.Index(trim, ".")+1:]))
			continue
		}
		if len(out) > 0 && trim != "" && !strings.HasPrefix(trim, "#") && !strings.HasPrefix(trim, "- ") {
			out[len(out)-1] += " " + trim
		}
	}
	return out
}

func isNumbered(s string) bool {
	i := strings.Index(s, ".")
	if i <= 0 {
		return false
	}
	for _, r := range s[:i] {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func inferTags(body string) []string {
	candidates := []string{"limpha", "innerworld", "ri", "kk", "dario", "aml", "router", "memory", "recall", "pressure", "quotes", "conflicts"}
	return present(candidates, body)
}

func inferLayers(body string) []string {
	candidates := []string{"limpha", "innerworld", "ri", "kk/dario", "aml", "router"}
	layers := make([]string, 0, len(candidates))
	for _, c := range candidates {
		if containsTerm(body, c) {
			layers = append(layers, c)
		}
	}
	return layers
}

func present(candidates []string, body string) []string {
	var out []string
	for _, c := range candidates {
		if containsTerm(body, c) {
			out = append(out, c)
		}
	}
	return out
}

func containsTerm(body, term string) bool {
	if strings.Contains(term, "/") {
		for _, part := range strings.Split(term, "/") {
			if containsTerm(body, part) {
				return true
			}
		}
		return false
	}
	if len(term) > 3 {
		return strings.Contains(body, term)
	}
	for _, tok := range strings.FieldsFunc(body, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	}) {
		if tok == term {
			return true
		}
	}
	return false
}

func allLines(doc document) []string {
	var out []string
	for _, key := range sortedSectionKeys(doc.sections) {
		out = append(out, key)
		out = append(out, doc.sections[key]...)
	}
	return out
}

func sortedSectionKeys(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func slug(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}
