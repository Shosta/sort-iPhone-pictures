//go:build windows

package devimport

import "os"

func freeSpace(string) (uint64, bool) { return 0, false }

func ownedByCurrentUser(os.FileInfo) bool { return true }
