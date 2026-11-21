//go:build !linux

package servers

import "os"

func cloneFile(dst, src *os.File) bool { return false }
