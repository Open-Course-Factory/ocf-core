package utils

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	maxExtractedSize = 50 * 1024 * 1024 // 50MB
	maxFileCount     = 500
)

// ExtractArchive extracts a .zip or .tar.gz archive to destDir.
// It includes path traversal protection, skips symlinks,
// limits total extracted size to 50MB, and limits file count to 500.
func ExtractArchive(archivePath, destDir string) error {
	ext := strings.ToLower(archivePath)
	switch {
	case strings.HasSuffix(ext, ".zip"):
		return extractZip(archivePath, destDir)
	case strings.HasSuffix(ext, ".tar.gz"), strings.HasSuffix(ext, ".tgz"):
		return extractTarGz(archivePath, destDir)
	default:
		return fmt.Errorf("unsupported archive format: %s", filepath.Ext(archivePath))
	}
}

// FindIndexJSON locates the directory containing index.json.
// Checks destDir first, then immediate subdirectories (common when zipping a folder).
// Returns the directory path containing index.json, or error if not found.
func FindIndexJSON(dir string) (string, error) {
	// Check root
	if _, err := os.Stat(filepath.Join(dir, "index.json")); err == nil {
		return dir, nil
	}

	// Check immediate subdirectories
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("failed to read directory: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		subDir := filepath.Join(dir, entry.Name())
		if _, err := os.Stat(filepath.Join(subDir, "index.json")); err == nil {
			return subDir, nil
		}
	}

	return "", fmt.Errorf("index.json not found in archive")
}

func extractZip(archivePath, destDir string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open zip: %w", err)
	}
	defer r.Close()

	var totalBytes int64
	var fileCount int

	cleanDest := filepath.Clean(destDir)

	for _, f := range r.File {
		// Skip symlinks
		if f.FileInfo().Mode()&os.ModeSymlink != 0 {
			continue
		}

		// Path traversal protection
		target := filepath.Join(destDir, f.Name)
		cleanTarget := filepath.Clean(target)
		if !strings.HasPrefix(cleanTarget, cleanDest+string(filepath.Separator)) && cleanTarget != cleanDest {
			return fmt.Errorf("path traversal detected: %s", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(cleanTarget, 0755); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}
			continue
		}

		fileCount++
		if fileCount > maxFileCount {
			return fmt.Errorf("file count exceeds limit of %d", maxFileCount)
		}

		// Ensure parent directory exists
		if err := os.MkdirAll(filepath.Dir(cleanTarget), 0755); err != nil {
			return fmt.Errorf("failed to create parent directory: %w", err)
		}

		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("failed to open file in zip: %w", err)
		}

		written, err := writeFile(rc, cleanTarget, totalBytes)
		rc.Close()
		if err != nil {
			return err
		}
		totalBytes += written
	}

	return nil
}

func extractTarGz(archivePath, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open archive: %w", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)

	var totalBytes int64
	var fileCount int

	cleanDest := filepath.Clean(destDir)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar entry: %w", err)
		}

		// Skip symlinks
		if header.Typeflag == tar.TypeSymlink || header.Typeflag == tar.TypeLink {
			continue
		}

		// Path traversal protection
		target := filepath.Join(destDir, header.Name)
		cleanTarget := filepath.Clean(target)
		if !strings.HasPrefix(cleanTarget, cleanDest+string(filepath.Separator)) && cleanTarget != cleanDest {
			return fmt.Errorf("path traversal detected: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(cleanTarget, 0755); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}
		case tar.TypeReg:
			fileCount++
			if fileCount > maxFileCount {
				return fmt.Errorf("file count exceeds limit of %d", maxFileCount)
			}

			// Ensure parent directory exists
			if err := os.MkdirAll(filepath.Dir(cleanTarget), 0755); err != nil {
				return fmt.Errorf("failed to create parent directory: %w", err)
			}

			written, err := writeFile(tr, cleanTarget, totalBytes)
			if err != nil {
				return err
			}
			totalBytes += written
		}
	}

	return nil
}

// writeFile writes data from reader to target path, enforcing the total size limit.
// Returns the number of bytes written.
func writeFile(r io.Reader, target string, currentTotal int64) (int64, error) {
	out, err := os.Create(target)
	if err != nil {
		return 0, fmt.Errorf("failed to create file: %w", err)
	}
	defer out.Close()

	limited := io.LimitReader(r, maxExtractedSize-currentTotal+1)
	written, err := io.Copy(out, limited)
	if err != nil {
		return 0, fmt.Errorf("failed to write file: %w", err)
	}
	if currentTotal+written > maxExtractedSize {
		return 0, fmt.Errorf("total extracted size exceeds limit of %d bytes", maxExtractedSize)
	}
	return written, nil
}
