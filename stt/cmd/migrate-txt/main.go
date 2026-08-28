// Command migrate-txt rewrites already-generated transcript artifacts to the new
// layout: a single "<mp4-base>-transcription.txt" file whose lines carry integer
// millisecond timestamps ("[startMs - endMs] text"). It deletes the old
// transcript.json / transcript.srt / transcript.txt files.
//
// Usage: go run ./cmd/migrate-txt [outputDir]   (default: data/output)
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var (
	oldLine  = regexp.MustCompile(`^\[(\d{2}):(\d{2}):(\d{2})\.(\d{3}) - (\d{2}):(\d{2}):(\d{2})\.(\d{3})\] ?(.*)$`)
	newLine  = regexp.MustCompile(`^\[\d+ - \d+\] `)
	fileName = regexp.MustCompile(`"fileName"\s*:\s*"([^"]*)"`)
)

func main() {
	root := "data/output"
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read output dir:", err)
		os.Exit(1)
	}
	var migrated, skipped int
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		if err := migrateDir(dir); err != nil {
			fmt.Printf("SKIP  %s: %v\n", dir, err)
			skipped++
			continue
		}
		migrated++
	}
	fmt.Printf("done: %d migrated, %d skipped\n", migrated, skipped)
}

func migrateDir(dir string) error {
	txtPath := filepath.Join(dir, "transcript.txt")
	raw, err := os.ReadFile(txtPath)
	if err != nil {
		return err
	}

	source := "transcript"
	if jsonRaw, err := os.ReadFile(filepath.Join(dir, "transcript.json")); err == nil {
		if m := fileName.FindSubmatch(jsonRaw); m != nil {
			source = string(m[1])
		}
	}
	base := strings.TrimSuffix(filepath.Base(source), filepath.Ext(source))
	if base == "" || base == "." {
		base = "transcript"
	}
	outName := base + "-transcription.txt"

	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	for i, line := range lines {
		if line == "" || newLine.MatchString(line) {
			continue
		}
		m := oldLine.FindStringSubmatch(line)
		if m == nil {
			return fmt.Errorf("line %d not in a known format: %q", i+1, line)
		}
		start := hmsToMS(m[1], m[2], m[3], m[4])
		end := hmsToMS(m[5], m[6], m[7], m[8])
		lines[i] = fmt.Sprintf("[%d - %d] %s", start, end, m[9])
	}

	if err := os.WriteFile(filepath.Join(dir, outName), []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		return err
	}
	for _, name := range []string{"transcript.txt", "transcript.json", "transcript.srt"} {
		p := filepath.Join(dir, name)
		if p == filepath.Join(dir, outName) {
			continue
		}
		_ = os.Remove(p)
	}
	fmt.Printf("OK    %s -> %s\n", dir, outName)
	return nil
}

func hmsToMS(h, m, s, ms string) int64 {
	hi, _ := strconv.ParseInt(h, 10, 64)
	mi, _ := strconv.ParseInt(m, 10, 64)
	si, _ := strconv.ParseInt(s, 10, 64)
	msi, _ := strconv.ParseInt(ms, 10, 64)
	return ((hi*60+mi)*60+si)*1000 + msi
}
