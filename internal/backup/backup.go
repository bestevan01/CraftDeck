// Package backup archives and restores an instance's work directory
// (world data, configs, plugins/mods, server jar) as a gzip-compressed tar
// file (requirements.md FR-13). Using the standard library's archive/tar +
// compress/gzip avoids pulling in an external archiving dependency
// (NFR-9).
package backup

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Create archives every file under srcDir into a new gzip-compressed tar at
// destPath, returning the resulting archive's size in bytes.
func Create(srcDir, destPath string) (int64, error) {
	return CreateFiltered(srcDir, destPath, nil)
}

// CreateFiltered is Create, but only archiving top-level entries of srcDir
// for which include returns true (or all of them, if include is nil). Used
// to export just an instance's world folders instead of its entire work
// directory (server jar, plugin configs, etc).
func CreateFiltered(srcDir, destPath string, include func(topLevelName string) bool) (int64, error) {
	f, err := os.Create(destPath)
	if err != nil {
		return 0, fmt.Errorf("create %s: %w", destPath, err)
	}
	defer f.Close()

	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)

	err = filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		if relPath == "." {
			return nil
		}

		if include != nil {
			topLevel := relPath
			if idx := strings.IndexByte(relPath, filepath.Separator); idx != -1 {
				topLevel = relPath[:idx]
			}
			if !include(topLevel) {
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
		}

		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() && !d.IsDir() {
			return nil // skip sockets/symlinks/etc -- not expected under a work dir
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = relPath
		if d.IsDir() {
			header.Name += "/"
		}
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		src, err := os.Open(path)
		if err != nil {
			return err
		}
		defer src.Close()
		_, err = io.Copy(tw, src)
		return err
	})
	if err != nil {
		tw.Close()
		gz.Close()
		f.Close()
		os.Remove(destPath)
		return 0, fmt.Errorf("archive %s: %w", srcDir, err)
	}

	if err := tw.Close(); err != nil {
		return 0, err
	}
	if err := gz.Close(); err != nil {
		return 0, err
	}
	if err := f.Close(); err != nil {
		return 0, err
	}

	stat, err := os.Stat(destPath)
	if err != nil {
		return 0, err
	}
	return stat.Size(), nil
}

// Restore extracts a gzip-compressed tar created by Create into destDir.
// destDir is expected to already exist (and typically be freshly emptied by
// the caller) -- Restore only creates the subdirectories/files the archive
// itself contains.
func Restore(archivePath, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open %s: %w", archivePath, err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("open gzip stream: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read tar entry: %w", err)
		}

		// Guard against a maliciously/corruptly crafted archive escaping
		// destDir via ".." path segments.
		target := filepath.Join(destDir, filepath.Clean("/"+header.Name))
		if header.Typeflag == tar.TypeDir {
			if err := os.MkdirAll(target, 0o750); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
		if err != nil {
			return fmt.Errorf("create %s: %w", target, err)
		}
		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			return fmt.Errorf("write %s: %w", target, err)
		}
		if err := out.Close(); err != nil {
			return err
		}
	}
}
