package read

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
)

// ReadFileInRange reads a file with offset/limit support and cancellation
// Checks ctx.Done() during reading to support cancellation
func ReadFileInRange(
	ctx context.Context,
	filePath string,
	offset int,
	limit int,
	maxBytes int64,
) (content string, lineCount int, totalLines int, totalBytes int64, readBytes int, mtimeMs int64, err error) {
	// Check for cancellation at start
	select {
	case <-ctx.Done():
		return "", 0, 0, 0, 0, 0, fmt.Errorf("file read cancelled: %w", ctx.Err())
	default:
	}

	file, err := os.Open(filePath)
	if err != nil {
		return "", 0, 0, 0, 0, 0, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Get file stats
	fileInfo, err := file.Stat()
	if err != nil {
		return "", 0, 0, 0, 0, 0, fmt.Errorf("failed to stat file: %w", err)
	}

	mtimeMs = fileInfo.ModTime().UnixMilli()
	totalBytes = fileInfo.Size()

	// Read line by line with cancellation checks
	scanner := bufio.NewScanner(file)
	lineNum := 0
	var lines []string
	var currentBytes int

	// Increase buffer size for long lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		// Check for cancellation before each line
		select {
		case <-ctx.Done():
			return "", 0, 0, 0, 0, 0, fmt.Errorf("file read cancelled: %w", ctx.Err())
		default:
		}

		lineNum++
		line := scanner.Text()

		// Skip until offset
		if lineNum < offset {
			continue
		}

		// Apply limit if set
		if limit > 0 && len(lines) >= limit {
			break
		}

		// Check byte limit
		newBytes := currentBytes + len(line) + 1 // +1 for newline
		if maxBytes > 0 && newBytes > int(maxBytes) {
			break
		}

		lines = append(lines, line)
		currentBytes = newBytes
	}

	if err := scanner.Err(); err != nil {
		if err == context.Canceled {
			return "", 0, 0, 0, 0, 0, fmt.Errorf("file read cancelled: %w", ctx.Err())
		}
		return "", 0, 0, 0, 0, 0, fmt.Errorf("error reading file: %w", err)
	}

	content = strings.Join(lines, "\n")
	lineCount = len(lines)
	readBytes = len(content)
	totalLines = lineNum

	return content, lineCount, totalLines, totalBytes, readBytes, mtimeMs, nil
}

// CountFileLines counts a text file's total lines without buffering their
// content, so a tail request (see readTextFile's negative-offset handling)
// can resolve "N lines before EOF" to an absolute line number cheaply even
// for large files.
func CountFileLines(ctx context.Context, filePath string) (int, error) {
	select {
	case <-ctx.Done():
		return 0, fmt.Errorf("file read cancelled: %w", ctx.Err())
	default:
	}

	file, err := os.Open(filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	count := 0
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return 0, fmt.Errorf("file read cancelled: %w", ctx.Err())
		default:
		}
		count++
	}
	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("error counting lines: %w", err)
	}
	return count, nil
}
