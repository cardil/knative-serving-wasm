package fs

import (
	"path"
	"path/filepath"
	"runtime"
)

func RootDir() string {
	pth := filepath.ToSlash(here())
	return path.Clean(path.Join(path.Dir(pth), "..", "..", ".."))
}

func here() string {
	_, file, _, _ := runtime.Caller(0)
	return file
}
