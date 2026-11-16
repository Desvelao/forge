package store

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// copyTree copies src into the existing directory dst. With template set it skips .git
// and rejects symlinks; otherwise it copies everything, symlinks included.
func copyTree(src, dst string, template bool) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil || rel == "." {
			return err
		}
		if template && d.Name() == ".git" {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		t := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case d.IsDir():
			return os.MkdirAll(t, info.Mode().Perm()|0o700)
		case d.Type()&fs.ModeSymlink != 0:
			if template {
				return fmt.Errorf("%s: symlinks are not allowed in templates", rel)
			}
			link, err := os.Readlink(p)
			if err != nil {
				return err
			}
			return os.Symlink(link, t)
		case d.Type().IsRegular():
			return copyFile(p, t, info.Mode().Perm())
		}
		return fmt.Errorf("%s: unsupported file type", rel)
	})
}

func copyFile(src, dst string, mode fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// moveDir renames src to dst, falling back to copy and delete across file systems.
func moveDir(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	if err := copyTree(src, dst, false); err != nil {
		os.RemoveAll(dst)
		return err
	}
	return os.RemoveAll(src)
}
