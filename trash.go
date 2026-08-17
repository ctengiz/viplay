package main

import "os"

func deleteFile(path string) error {
	return deleteFileWithTrash(path, moveFileToTrash)
}

func deleteFileWithTrash(path string, moveToTrash func(string) error) error {
	if err := moveToTrash(path); err == nil {
		return nil
	}
	return os.Remove(path)
}
