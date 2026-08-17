//go:build darwin

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func moveFileToTrash(path string) error {
	trashDir, err := trashDirectory(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(trashDir, 0o700); err != nil {
		return err
	}
	destination, err := availableTrashPath(trashDir, filepath.Base(path))
	if err != nil {
		return err
	}
	return os.Rename(path, destination)
}

func trashDirectory(path string) (string, error) {
	clean := filepath.Clean(path)
	const volumesRoot = "/Volumes/"
	if strings.HasPrefix(clean, volumesRoot) {
		parts := strings.Split(strings.TrimPrefix(clean, volumesRoot), string(filepath.Separator))
		if len(parts) > 1 && parts[0] != "" {
			return filepath.Join(volumesRoot, parts[0], ".Trashes", fmt.Sprint(os.Getuid())), nil
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if clean != home && !strings.HasPrefix(clean, home+string(filepath.Separator)) {
		return "", errors.New("file is outside the home volume")
	}
	return filepath.Join(home, ".Trash"), nil
}

func availableTrashPath(dir string, name string) (string, error) {
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for suffix := 0; suffix < 10_000; suffix++ {
		candidateName := name
		if suffix > 0 {
			candidateName = fmt.Sprintf("%s %d%s", stem, suffix, ext)
		}
		candidate := filepath.Join(dir, candidateName)
		_, err := os.Lstat(candidate)
		if errors.Is(err, os.ErrNotExist) {
			return candidate, nil
		}
		if err != nil {
			return "", err
		}
	}
	return "", errors.New("could not create a unique trash path")
}
