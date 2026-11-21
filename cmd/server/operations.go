package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/TypeTerrors/go_sync/internal/servers"
)

func maintenanceCommand(command string, args []string) error {
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	folder := flags.String("sync-folder", "", "Shared root (offline commands)")
	state := flags.String("state-dir", "", "Existing private state directory")
	name := flags.String("path", "", "Relative original file path")
	version := flags.String("version", "", "Retained version ID")
	conflict := flags.String("conflict", "", "Conflict ID to restore")
	timeout := flags.Duration("wait", 30*time.Second, "Rescan acknowledgement timeout")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *state == "" || flags.NArg() != 0 {
		return fmt.Errorf("%s requires --state-dir", command)
	}
	if command == "rescan" {
		return requestRescan(*state, *timeout)
	}
	if *folder == "" {
		return fmt.Errorf("%s requires --sync-folder", command)
	}
	if _, err := os.Stat(filepath.Join(*state, "MANIFEST")); err != nil {
		return fmt.Errorf("existing state required: %w", err)
	}
	e, err := servers.NewEngine(*folder, *state, servers.WithStoredState())
	if err != nil {
		return err
	}
	defer e.Close()
	out := json.NewEncoder(os.Stdout)
	switch command {
	case "inspect":
		return out.Encode(e.StoredManifest())
	case "verify":
		issues, err := e.Verify()
		if err != nil {
			return err
		}
		if err = out.Encode(issues); err != nil {
			return err
		}
		if len(issues) > 0 {
			return fmt.Errorf("verification found %d differences; inspect before resuming sync", len(issues))
		}
		return nil
	case "history":
		if *name == "" {
			return fmt.Errorf("history requires --path")
		}
		versions, err := e.History(*name)
		if err != nil {
			return err
		}
		return out.Encode(versions)
	case "restore":
		if *conflict != "" && *version == "" && *name == "" {
			return e.RestoreConflict(*conflict)
		}
		if *name == "" || *version == "" || *conflict != "" {
			return fmt.Errorf("restore requires either --path and --version, or --conflict")
		}
		return e.RestoreVersion(*name, *version)
	}
	return fmt.Errorf("unknown maintenance command")
}
func requestRescan(state string, timeout time.Duration) error {
	if timeout <= 0 {
		return fmt.Errorf("--wait must be positive")
	}
	data, err := os.ReadFile(filepath.Join(state, "status.json"))
	if err != nil {
		return err
	}
	var status servers.StatusSnapshot
	if err = json.Unmarshal(data, &status); err != nil {
		return err
	}
	if !status.Running || time.Since(status.Updated) > 5*time.Second {
		return fmt.Errorf("service is not reporting a fresh running status; start it before requesting a rescan")
	}
	dir := filepath.Join(state, "commands")
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	request, err := os.CreateTemp(dir, "rescan-*.request")
	if err != nil {
		return err
	}
	request.Close()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err = os.Stat(request.Name()); os.IsNotExist(err) {
			fmt.Println("rescan completed")
			return nil
		} else if err != nil {
			return err
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("rescan is still pending; check status for scan errors")
}
