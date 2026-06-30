// Command sartre-limpha-ingest stores SARTRE utility JSONL receipts in limpha.
//
// It is the standalone seam between the SARTRE body organ and Yent's shared
// memory: utilities can be run by the SARTRE slot supervisor, their stdout can be
// captured as JSONL, and this command turns that receipt into typed limpha seams
// without starting the full innerworld dock.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	yent "github.com/ariannamethod/yent/yent/go"
)

type ingestResult struct {
	Kind   string `json:"kind"`
	Events int    `json:"events"`
	SeamID int64  `json:"seam_id"`
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr, os.Stdin); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer, stdin io.Reader) error {
	fs := flag.NewFlagSet("sartre-limpha-ingest", flag.ContinueOnError)
	fs.SetOutput(stderr)
	eventsPath := fs.String("events", strings.TrimSpace(os.Getenv("YENT_SARTRE_EVENTS")), "SARTRE JSONL receipt path; defaults to stdin")
	dbPath := fs.String("db", strings.TrimSpace(os.Getenv("YENT_LIMPHA_DB")), "limpha DB path; defaults to ~/.yent/limpha.db")
	if err := fs.Parse(args); err != nil {
		return err
	}

	data, err := readEvents(*eventsPath, stdin)
	if err != nil {
		return err
	}
	events := yent.ParseSartreEventsJSONL(string(data))
	if len(events) == 0 {
		return fmt.Errorf("no SARTRE utility events found")
	}

	var lc *yent.LimphaClient
	if strings.TrimSpace(*dbPath) != "" {
		lc, err = yent.NewLimphaClientAt(*dbPath)
	} else {
		lc, err = yent.NewLimphaClient()
	}
	if err != nil {
		return fmt.Errorf("open limpha: %w", err)
	}
	defer lc.Close()

	seamID, err := lc.StoreSartreEvents(events, yent.LimphaState{})
	if err != nil {
		return fmt.Errorf("store SARTRE events: %w", err)
	}
	res := ingestResult{Kind: "sartre_limphed", Events: len(events), SeamID: seamID}
	enc := json.NewEncoder(stdout)
	return enc.Encode(res)
}

func readEvents(path string, stdin io.Reader) ([]byte, error) {
	if strings.TrimSpace(path) != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read events: %w", err)
		}
		return data, nil
	}
	data, err := io.ReadAll(stdin)
	if err != nil {
		return nil, fmt.Errorf("read stdin: %w", err)
	}
	return data, nil
}
