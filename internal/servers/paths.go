package servers

import (
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

func portableComponent(part string) error {
	if len(part) > 255 || !utf8.ValidString(part) || !norm.NFC.IsNormalString(part) || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") || strings.ContainsAny(part, "<>\"|?*") {
		return fmt.Errorf("nonportable path component %q", part)
	}
	for _, r := range part {
		if r < 32 || r == 127 {
			return fmt.Errorf("control character in path")
		}
	}
	base := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
	if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9' {
		return fmt.Errorf("reserved device name %q", part)
	}
	return nil
}
func foldedPath(name string) string { return cases.Fold().String(name) }

// Check the real directory entries, including untracked files, before staging.
// This prevents a case-insensitive host from silently overwriting another name.
func (e *Engine) checkPortableTarget(name string) error {
	parent := "."
	for _, part := range strings.Split(name, "/") {
		dir, err := e.root.Open(parent)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		entries, err := dir.ReadDir(-1)
		dir.Close()
		if err != nil {
			return err
		}
		key := foldedPath(part)
		for _, entry := range entries {
			if entry.Name() != part && foldedPath(entry.Name()) == key {
				return fmt.Errorf("case-fold collision: %s and %s", part, entry.Name())
			}
		}
		if parent == "." {
			parent = part
		} else {
			parent += "/" + part
		}
	}
	return nil
}
