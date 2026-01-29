package git

import (
	"bufio"
	"errors"
	"strconv"
	"strings"
)

type DiffLineKind int

const (
	DiffLineContext DiffLineKind = iota
	DiffLineAdd
	DiffLineDel
)

type DiffFile struct {
	Path  string
	Hunks []DiffHunk
}

type DiffHunk struct {
	Header   string
	Lines    []DiffLine
	OldStart int
	OldLines int
	NewStart int
	NewLines int
}

type DiffLine struct {
	Kind    DiffLineKind
	OldLine int
	NewLine int
	Text    string
}

func ParseUnifiedDiff(diff string) ([]DiffFile, error) {
	if strings.TrimSpace(diff) == "" {
		return nil, errors.New("diff is empty")
	}

	scanner := bufio.NewScanner(strings.NewReader(diff))
	files := make([]DiffFile, 0)

	var currentFile *DiffFile
	var currentHunk *DiffHunk
	var oldLine int
	var newLine int

	flushFile := func() {
		if currentFile != nil {
			files = append(files, *currentFile)
			currentFile = nil
			currentHunk = nil
		}
	}

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "diff --git ") {
			flushFile()
			currentFile = &DiffFile{}
			continue
		}

		if currentFile == nil {
			continue
		}

		if strings.HasPrefix(line, "+++ ") {
			path := strings.TrimSpace(strings.TrimPrefix(line, "+++ "))
			path = strings.TrimPrefix(path, "b/")
			if path != "/dev/null" {
				currentFile.Path = path
			}
			continue
		}

		if strings.HasPrefix(line, "@@") {
			header, oldStart, oldLines, newStart, newLines, err := parseHunkHeader(line)
			if err != nil {
				return nil, err
			}
			hunk := DiffHunk{
				Header:   header,
				OldStart: oldStart,
				OldLines: oldLines,
				NewStart: newStart,
				NewLines: newLines,
			}
			currentFile.Hunks = append(currentFile.Hunks, hunk)
			currentHunk = &currentFile.Hunks[len(currentFile.Hunks)-1]
			oldLine = oldStart
			newLine = newStart
			continue
		}

		if currentHunk == nil {
			continue
		}

		if line == `\ No newline at end of file` {
			continue
		}

		switch {
		case strings.HasPrefix(line, "+"):
			currentHunk.Lines = append(currentHunk.Lines, DiffLine{
				Kind:    DiffLineAdd,
				OldLine: 0,
				NewLine: newLine,
				Text:    strings.TrimPrefix(line, "+"),
			})
			newLine++
		case strings.HasPrefix(line, "-"):
			currentHunk.Lines = append(currentHunk.Lines, DiffLine{
				Kind:    DiffLineDel,
				OldLine: oldLine,
				NewLine: 0,
				Text:    strings.TrimPrefix(line, "-"),
			})
			oldLine++
		case strings.HasPrefix(line, " "):
			currentHunk.Lines = append(currentHunk.Lines, DiffLine{
				Kind:    DiffLineContext,
				OldLine: oldLine,
				NewLine: newLine,
				Text:    strings.TrimPrefix(line, " "),
			})
			oldLine++
			newLine++
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	flushFile()

	return files, nil
}

func parseHunkHeader(line string) (string, int, int, int, int, error) {
	header := line
	trimmed := strings.TrimPrefix(line, "@@")
	trimmed = strings.TrimSuffix(trimmed, "@@")
	trimmed = strings.TrimSpace(trimmed)
	parts := strings.Split(trimmed, " ")
	if len(parts) < 2 {
		return "", 0, 0, 0, 0, errors.New("invalid hunk header")
	}

	oldStart, oldLines, err := parseRange(parts[0])
	if err != nil {
		return "", 0, 0, 0, 0, err
	}
	newStart, newLines, err := parseRange(parts[1])
	if err != nil {
		return "", 0, 0, 0, 0, err
	}

	return header, oldStart, oldLines, newStart, newLines, nil
}

func parseRange(value string) (int, int, error) {
	value = strings.TrimPrefix(value, "-")
	value = strings.TrimPrefix(value, "+")

	parts := strings.Split(value, ",")
	start, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, err
	}

	lines := 1
	if len(parts) > 1 {
		lines, err = strconv.Atoi(parts[1])
		if err != nil {
			return 0, 0, err
		}
	}

	return start, lines, nil
}
