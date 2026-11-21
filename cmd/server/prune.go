package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/TypeTerrors/go_sync/internal/servers"
	"os"
	"path/filepath"
	"time"
)

func pruneCommand(args []string) error {
	flags := flag.NewFlagSet("prune", flag.ContinueOnError)
	folder := flags.String("sync-folder", "", "Synchronized folder")
	state := flags.String("state-dir", "", "Existing state directory (service must be stopped)")
	age := flags.Duration("older-than", 30*24*time.Hour, "Age of history to retire")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *folder == "" || *state == "" || *age <= 0 {
		return fmt.Errorf("prune requires --sync-folder, --state-dir and a positive --older-than")
	}
	if _, err := os.Stat(filepath.Join(*state, "MANIFEST")); err != nil {
		return fmt.Errorf("existing state required: %w", err)
	}
	e, err := servers.NewEngine(*folder, *state)
	if err != nil {
		return err
	}
	defer e.Close()
	result, err := e.Prune(time.Now().Add(-*age))
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}
