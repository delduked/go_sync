package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const lanFolderID = "gosync-lan"

func defaultConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "gosync", "default.yaml"), nil
}

func startupArgs(args []string) ([]string, error) {
	if len(args) != 0 {
		return args, nil
	}
	config, err := defaultConfigPath()
	if err != nil {
		return nil, err
	}
	if _, err = os.Stat(config); err != nil {
		if os.IsNotExist(err) {
			legacy := strings.TrimSuffix(config, ".yaml") + ".json"
			if _, legacyErr := os.Stat(legacy); legacyErr == nil {
				return []string{"--config", legacy}, nil
			} else if !os.IsNotExist(legacyErr) {
				return nil, legacyErr
			}
			return nil, fmt.Errorf("no saved configuration at %s; choose a folder first: gosync init --sync-folder /path/to/shared", config)
		}
		return nil, err
	}
	return []string{"--config", config}, nil
}

// Resolve existing ancestors, including symlinks, for a path not yet created.
func resolveNewPath(name string) (string, error) {
	name, err := filepath.Abs(name)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(name)
	if err == nil {
		return resolved, nil
	}
	if !errors.Is(err, os.ErrNotExist) || filepath.Dir(name) == name {
		return "", err
	}
	parent, err := resolveNewPath(filepath.Dir(name))
	if err != nil {
		return "", err
	}
	return filepath.Join(parent, filepath.Base(name)), nil
}

func outsideFolder(folder, name string) error {
	root, err := resolveNewPath(folder)
	if err != nil {
		return err
	}
	target, err := resolveNewPath(name)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(root, target)
	if err != nil {
		if filepath.VolumeName(root) != filepath.VolumeName(target) {
			return nil // Different Windows volumes cannot contain each other.
		}
		return err
	}
	if rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
		return fmt.Errorf("%s must be outside the synchronized folder %s", name, folder)
	}
	return nil
}
