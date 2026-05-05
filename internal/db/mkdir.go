package db

import "os"

func mkdir(p string) error {
	return os.MkdirAll(p, 0o755)
}
