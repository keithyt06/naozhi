package patrol

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

const (
	defaultMaxLogSize = 10 * 1024 * 1024 // 10 MB
	logFileName       = "logs.jsonl"
)

// LogWriter manages JSONL log files for a single patrol.
type LogWriter struct {
	dir     string
	file    *os.File
	size    int64
	maxSize int64
}

// NewLogWriter opens or creates the JSONL log file for a patrol.
func NewLogWriter(baseDir, patrolName string) (*LogWriter, error) {
	dir := filepath.Join(baseDir, patrolName)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create patrol log dir: %w", err)
	}

	path := filepath.Join(dir, logFileName)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("open patrol log: %w", err)
	}

	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}

	return &LogWriter{
		dir:     dir,
		file:    f,
		size:    info.Size(),
		maxSize: defaultMaxLogSize,
	}, nil
}

// Append writes a RunLog entry as a single JSONL line.
func (lw *LogWriter) Append(rl *RunLog) error {
	if lw.size >= lw.maxSize {
		if err := lw.rotate(); err != nil {
			slog.Warn("patrol log rotate failed", "dir", lw.dir, "err", err)
			// Continue writing even if rotate fails
		}
	}

	data, err := json.Marshal(rl)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	// Seek to end before writing
	if _, err := lw.file.Seek(0, io.SeekEnd); err != nil {
		return err
	}
	n, err := lw.file.Write(data)
	lw.size += int64(n)
	return err
}

// rotate archives the current log file with gzip compression.
func (lw *LogWriter) rotate() error {
	if err := lw.file.Close(); err != nil {
		return err
	}

	src := filepath.Join(lw.dir, logFileName)
	ts := time.Now().Unix()
	archiveName := fmt.Sprintf("logs.%d.jsonl", ts)
	archivePath := filepath.Join(lw.dir, archiveName)

	if err := os.Rename(src, archivePath); err != nil {
		// Reopen original on failure
		lw.file, _ = os.OpenFile(src, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0600)
		return err
	}

	// Gzip compress the archive
	go func() {
		if err := gzipFile(archivePath); err != nil {
			slog.Warn("patrol log gzip failed", "path", archivePath, "err", err)
		}
	}()

	// Create fresh log file
	f, err := os.OpenFile(src, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	lw.file = f
	lw.size = 0
	return nil
}

// gzipFile compresses a file and removes the original.
func gzipFile(path string) error {
	in, err := os.Open(path)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(path + ".gz")
	if err != nil {
		return err
	}
	defer out.Close()

	gz := gzip.NewWriter(out)
	if _, err := io.Copy(gz, in); err != nil {
		gz.Close()
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	in.Close()
	return os.Remove(path)
}

// ReadTail reads the last n log entries from the JSONL file.
// Returns entries in reverse chronological order (newest first).
func (lw *LogWriter) ReadTail(n int) ([]*RunLog, error) {
	if n <= 0 {
		n = 20
	}

	// Read all lines (simple approach for files under maxSize)
	if _, err := lw.file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	var lines [][]byte
	scanner := bufio.NewScanner(lw.file)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024) // up to 1MB per line
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		cp := make([]byte, len(line))
		copy(cp, line)
		lines = append(lines, cp)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// Take last n lines
	start := 0
	if len(lines) > n {
		start = len(lines) - n
	}
	lines = lines[start:]

	// Parse in reverse order (newest first)
	result := make([]*RunLog, 0, len(lines))
	for i := len(lines) - 1; i >= 0; i-- {
		var rl RunLog
		if err := json.Unmarshal(lines[i], &rl); err != nil {
			slog.Debug("skip malformed patrol log line", "err", err)
			continue
		}
		result = append(result, &rl)
	}
	return result, nil
}

// ReadPage reads a page of log entries (offset-based, chronological order).
func (lw *LogWriter) ReadPage(offset, limit int) ([]*RunLog, int, error) {
	if limit <= 0 {
		limit = 20
	}

	if _, err := lw.file.Seek(0, io.SeekStart); err != nil {
		return nil, 0, err
	}

	var all []*RunLog
	scanner := bufio.NewScanner(lw.file)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var rl RunLog
		if err := json.Unmarshal(line, &rl); err != nil {
			continue
		}
		all = append(all, &rl)
	}
	if err := scanner.Err(); err != nil {
		return nil, 0, err
	}

	total := len(all)
	if offset >= total {
		return nil, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return all[offset:end], total, nil
}

// Close closes the underlying log file.
func (lw *LogWriter) Close() error {
	if lw.file != nil {
		return lw.file.Close()
	}
	return nil
}

// CleanOldArchives removes gzipped archive files older than maxAge.
func CleanOldArchives(dir string, maxAge time.Duration) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-maxAge)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if filepath.Ext(name) != ".gz" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			path := filepath.Join(dir, name)
			if err := os.Remove(path); err != nil {
				slog.Warn("remove old patrol archive", "path", path, "err", err)
			}
		}
	}
}
