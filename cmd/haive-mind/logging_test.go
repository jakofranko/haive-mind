package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// G18: with a log file, the same events go to the file as JSON and to stdout
// as readable text.
func TestLogger_WritesJSONToFileAndTextToStdout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "haive.log")
	var stdout bytes.Buffer

	logger, closeLog, err := newLogger(path, false, &stdout)
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("participant joined", "name", "alice")
	closeLog()

	b, _ := os.ReadFile(path)
	var rec map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(b), &rec); err != nil {
		t.Fatalf("log file line is not JSON: %v\n%s", err, b)
	}
	if rec["msg"] != "participant joined" || rec["name"] != "alice" {
		t.Errorf("JSON record = %v", rec)
	}
	out := stdout.String()
	if !strings.Contains(out, "participant joined") || !strings.Contains(out, "alice") {
		t.Errorf("stdout lacks the event: %q", out)
	}
	if strings.Contains(out, `"msg"`) {
		t.Errorf("stdout is JSON, want human-readable: %q", out)
	}
}

// Debug detail appears only with --debug, in both destinations.
func TestLogger_DebugOnlyWhenAsked(t *testing.T) {
	for _, debug := range []bool{false, true} {
		path := filepath.Join(t.TempDir(), "haive.log")
		var stdout bytes.Buffer
		logger, closeLog, err := newLogger(path, debug, &stdout)
		if err != nil {
			t.Fatal(err)
		}
		logger.Debug("hub event")
		closeLog()

		b, _ := os.ReadFile(path)
		inFile := strings.Contains(string(b), "hub event")
		inStdout := strings.Contains(stdout.String(), "hub event")
		if inFile != debug || inStdout != debug {
			t.Errorf("debug=%v: in file=%v, in stdout=%v", debug, inFile, inStdout)
		}
	}
}

func TestLogger_WithoutLogFileUsesStdoutOnly(t *testing.T) {
	var stdout bytes.Buffer
	logger, closeLog, err := newLogger("", false, &stdout)
	if err != nil {
		t.Fatal(err)
	}
	defer closeLog()
	logger.Info("hello")
	if !strings.Contains(stdout.String(), "hello") {
		t.Errorf("stdout = %q", stdout.String())
	}
}

func TestLogger_UnwritableLogFileIsAnError(t *testing.T) {
	_, _, err := newLogger(filepath.Join(t.TempDir(), "no", "such", "dir", "x.log"), false, &bytes.Buffer{})
	if err == nil {
		t.Fatal("want an error for an unwritable log file")
	}
}

// With no stdout (the server was started with output going nowhere useful) the
// log file is still written, and logging does not fail.
func TestLogger_WithoutStdoutStillWritesTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "haive.log")

	logger, closeLog, err := newLogger(path, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("participant joined", "name", "alice")
	closeLog()

	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), "alice") {
		t.Errorf("log file lacks the event: %q", b)
	}
}
