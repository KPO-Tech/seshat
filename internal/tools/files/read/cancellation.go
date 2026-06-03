package read

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
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

// ReadFileWithCancellation reads entire file content with cancellation support
func ReadFileWithCancellation(ctx context.Context, filePath string) ([]byte, error) {
	// Check for cancellation at start
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("file read cancelled: %w", ctx.Err())
	default:
	}

	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Get file size for progress tracking
	_, err = file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	// Read in chunks with cancellation checks
	const chunkSize = 32 * 1024 // 32KB chunks
	var result []byte
	buffer := make([]byte, chunkSize)
	totalRead := 0

	for {
		// Check for cancellation before each read
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("file read cancelled: %w", ctx.Err())
		default:
		}

		n, err := file.Read(buffer)
		if n > 0 {
			result = append(result, buffer[:n]...)
			totalRead += n
		}

		if err == io.EOF {
			break
		}

		if err != nil {
			return nil, fmt.Errorf("error reading file: %w", err)
		}
	}

	return result, nil
}

// CancellationGroup allows waiting for multiple operations with cancellation
type CancellationGroup struct {
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	errs   []error
	errMu  sync.Mutex
}

// NewCancellationGroup creates a new cancellation group
func NewCancellationGroup(ctx context.Context) *CancellationGroup {
	childCtx, cancel := context.WithCancel(ctx)
	return &CancellationGroup{
		ctx:    childCtx,
		cancel: cancel,
	}
}

// Go starts a goroutine with cancellation support
func (g *CancellationGroup) Go(fn func(context.Context) error) {
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		if err := fn(g.ctx); err != nil {
			g.errMu.Lock()
			g.errs = append(g.errs, err)
			g.errMu.Unlock()
			// Cancel other operations on error
			g.cancel()
		}
	}()
}

// Wait waits for all operations to complete
func (g *CancellationGroup) Wait() error {
	g.wg.Wait()
	g.cancel()

	g.errMu.Lock()
	defer g.errMu.Unlock()

	if len(g.errs) > 0 {
		return g.errs[0]
	}

	return nil
}

// Cancel cancels all operations in the group
func (g *CancellationGroup) Cancel() {
	g.cancel()
}
