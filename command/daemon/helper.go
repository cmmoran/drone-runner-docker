package daemon

import (
	"io"
	"os"
	"path/filepath"
)

func installOutputHelper() (string, error) {
	src, err := os.Executable()
	if err != nil {
		return "", err
	}

	in, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer in.Close()

	dir, err := os.MkdirTemp("", "drone-output-*")
	if err != nil {
		return "", err
	}
	dst := filepath.Join(dir, "drone-output")

	out, err := os.Create(dst)
	if err != nil {
		return "", err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return "", err
	}
	if err := out.Sync(); err != nil {
		return "", err
	}
	if err := os.Chmod(dst, 0o755); err != nil {
		return "", err
	}
	return dst, nil
}
