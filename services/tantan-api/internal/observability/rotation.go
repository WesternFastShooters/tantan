package observability

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type RotatingWriterConfig struct {
	Path     string
	MaxBytes int64
	Backups  int
}

type RotatingWriter struct {
	mutex    sync.Mutex
	path     string
	maxBytes int64
	backups  int
	file     *os.File
	size     int64
}

func NewRotatingWriter(config RotatingWriterConfig) (*RotatingWriter, error) {
	if config.Path == "" || config.MaxBytes < 1 || config.Backups < 0 || config.Backups > 20 {
		return nil, errors.New("valid log path, size and retention are required")
	}
	path, err := filepath.Abs(config.Path)
	if err != nil {
		return nil, errors.New("resolve log path failed")
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, errors.New("create log directory failed")
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return nil, errors.New("secure log directory failed")
	}
	writer := &RotatingWriter{path: path, maxBytes: config.MaxBytes, backups: config.Backups}
	if err := writer.open(); err != nil {
		return nil, err
	}
	return writer, nil
}

func (writer *RotatingWriter) Write(contents []byte) (int, error) {
	writer.mutex.Lock()
	defer writer.mutex.Unlock()
	if writer.file == nil {
		return 0, errors.New("log writer is closed")
	}
	if writer.size > 0 && writer.size+int64(len(contents)) > writer.maxBytes {
		if err := writer.rotate(); err != nil {
			return 0, err
		}
	}
	written, err := writer.file.Write(contents)
	writer.size += int64(written)
	return written, err
}

func (writer *RotatingWriter) Close() error {
	writer.mutex.Lock()
	defer writer.mutex.Unlock()
	if writer.file == nil {
		return nil
	}
	err := writer.file.Close()
	writer.file = nil
	return err
}

func (writer *RotatingWriter) Sync() error {
	writer.mutex.Lock()
	defer writer.mutex.Unlock()
	if writer.file == nil {
		return errors.New("log writer is closed")
	}
	return writer.file.Sync()
}

func (writer *RotatingWriter) open() error {
	if info, err := os.Lstat(writer.path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return errors.New("log target must be a regular file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return errors.New("inspect log target failed")
	}
	file, err := os.OpenFile(writer.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return errors.New("open log file failed")
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = file.Close()
		return errors.New("log target must be a regular file")
	}
	pathInfo, err := os.Lstat(writer.path)
	if err != nil || pathInfo.Mode()&os.ModeSymlink != 0 || !os.SameFile(info, pathInfo) {
		_ = file.Close()
		return errors.New("log target changed while opening")
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return errors.New("secure log file failed")
	}
	writer.file = file
	writer.size = info.Size()
	return nil
}

func (writer *RotatingWriter) rotate() error {
	if err := writer.file.Sync(); err != nil {
		return errors.New("sync log before rotation failed")
	}
	if err := writer.file.Close(); err != nil {
		return errors.New("close log before rotation failed")
	}
	writer.file = nil
	if writer.backups > 0 {
		oldest := fmt.Sprintf("%s.%d", writer.path, writer.backups)
		if err := os.Remove(oldest); err != nil && !errors.Is(err, os.ErrNotExist) {
			return errors.New("remove expired log failed")
		}
		for index := writer.backups - 1; index >= 1; index-- {
			source := fmt.Sprintf("%s.%d", writer.path, index)
			destination := fmt.Sprintf("%s.%d", writer.path, index+1)
			if err := os.Rename(source, destination); err != nil && !errors.Is(err, os.ErrNotExist) {
				return errors.New("rotate retained log failed")
			}
		}
		if err := os.Rename(writer.path, writer.path+".1"); err != nil && !errors.Is(err, os.ErrNotExist) {
			return errors.New("rotate current log failed")
		}
	} else if err := os.Remove(writer.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.New("truncate rotated log failed")
	}
	return writer.open()
}
