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

func statusCommand(args []string) error {
	flags := flag.NewFlagSet("status", flag.ContinueOnError)
	state := flags.String("state-dir", "", "Running service's state directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *state == "" || flags.NArg() != 0 {
		return fmt.Errorf("status requires --state-dir")
	}
	data, err := os.ReadFile(filepath.Join(*state, "status.json"))
	if err != nil {
		return err
	}
	var snapshot servers.StatusSnapshot
	if err = json.Unmarshal(data, &snapshot); err != nil {
		return err
	}
	report := struct {
		servers.StatusSnapshot
		Stale bool `json:"stale"`
	}{snapshot, snapshot.Running && time.Since(snapshot.Updated) > 5*time.Second}
	return json.NewEncoder(os.Stdout).Encode(report)
}
