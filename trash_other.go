//go:build !darwin

package main

import "errors"

func moveFileToTrash(string) error {
	return errors.New("native trash is unavailable")
}
