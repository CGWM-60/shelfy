package storage

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

type File interface {
	io.Reader
	io.Writer
	io.Closer
	Stat() (fs.FileInfo, error)
	Seek(offset int64, whence int) (int64, error)
}

type FileSystem interface {
	MkdirAll(path string) error
	OpenFile(name string, flag int, perm os.FileMode) (File, error)
	Stat(name string) (fs.FileInfo, error)
	Remove(name string) error
}

type LocalFS struct{}

func (LocalFS) MkdirAll(path string) error {
	return os.MkdirAll(path, 0o755)
}

func (LocalFS) OpenFile(name string, flag int, perm os.FileMode) (File, error) {
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		return nil, err
	}
	return os.OpenFile(name, flag, perm)
}

func (LocalFS) Stat(name string) (fs.FileInfo, error) {
	return os.Stat(name)
}

func (LocalFS) Remove(name string) error {
	return os.Remove(name)
}
