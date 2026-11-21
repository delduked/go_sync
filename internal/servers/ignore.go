package servers

import (
	"fmt"
	"io"
	"os"
	"path"
	"reflect"
	"strings"
)

const ignoreFile = ".gosyncignore"

type ignoreRule struct {
	pattern             string
	anchored, directory bool
}

// This deliberately small glob language is shared with the folder. Exclusions
// never delete local contents; ignored paths cannot be served or overwritten.
func (e *Engine) loadIgnoreLocked() error {
	info, err := e.root.Lstat(ignoreFile)
	if os.IsNotExist(err) {
		if len(e.ignores) > 0 {
			e.generation++
		}
		e.ignores = nil
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return fmt.Errorf("%s must be a regular file smaller than 1 MiB", ignoreFile)
	}
	f, err := e.root.Open(ignoreFile)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil {
		return err
	}
	var rules []ignoreRule
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "!") || strings.Contains(line, "\\") {
			return fmt.Errorf("%s:%d: negation and backslash escapes are unsupported", ignoreFile, i+1)
		}
		r := ignoreRule{anchored: strings.HasPrefix(line, "/"), directory: strings.HasSuffix(line, "/")}
		r.pattern = strings.Trim(line, "/")
		if r.pattern == "" || strings.Contains(r.pattern, "//") {
			return fmt.Errorf("%s:%d: empty path component", ignoreFile, i+1)
		}
		for _, part := range strings.Split(r.pattern, "/") {
			if part == "." || part == ".." {
				return fmt.Errorf("%s:%d: invalid path", ignoreFile, i+1)
			}
			if _, err := path.Match(part, ""); err != nil {
				return fmt.Errorf("%s:%d: %w", ignoreFile, i+1, err)
			}
		}
		rules = append(rules, r)
	}
	if !reflect.DeepEqual(e.ignores, rules) {
		e.generation++
	}
	e.ignores = rules
	return nil
}
func globParts(pattern, name []string) bool {
	if len(pattern) == 0 {
		return len(name) == 0
	}
	if pattern[0] == "**" {
		return globParts(pattern[1:], name) || len(name) > 0 && globParts(pattern, name[1:])
	}
	if len(name) == 0 {
		return false
	}
	matched, _ := path.Match(pattern[0], name[0])
	return matched && globParts(pattern[1:], name[1:])
}
func (e *Engine) ignoredLocked(name string, directory bool) bool {
	if name == ignoreFile || name == ".gosync-conflicts" || strings.HasPrefix(name, ".gosync-conflicts/") {
		return false
	}
	parts := strings.Split(name, "/")
	for _, rule := range e.ignores {
		for n := 1; n <= len(parts); n++ {
			if rule.directory && n == len(parts) && !directory {
				continue
			}
			if !rule.anchored && !strings.Contains(rule.pattern, "/") {
				if match, _ := path.Match(rule.pattern, parts[n-1]); match {
					return true
				}
			} else if globParts(strings.Split(rule.pattern, "/"), parts[:n]) {
				return true
			}
		}
	}
	return false
}
func (e *Engine) excluded(name string, directory bool) (bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.loadIgnoreLocked(); err != nil {
		return true, err
	}
	return e.ignoredLocked(name, directory), nil
}
