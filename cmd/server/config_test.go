package main

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPersistentConfigAndExplicitOverrides(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "config.json")
	if err := os.WriteFile(config, []byte(`{"sync-folder":"files","sync-interval":"250ms","fanout":2,"no-mdns":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	flags := flag.NewFlagSet("test", flag.ContinueOnError)
	folder := flags.String("sync-folder", "", "")
	interval := flags.Duration("sync-interval", time.Second, "")
	fanout := flags.Int("fanout", 0, "")
	noMDNS := flags.Bool("no-mdns", false, "")
	flags.String("config", "", "")
	if err := parseConfigFlags(flags, []string{"--config", config, "--fanout=3"}); err != nil {
		t.Fatal(err)
	}
	if *folder != filepath.Join(dir, "files") || *interval != 250*time.Millisecond || *fanout != 3 || !*noMDNS {
		t.Fatal("config/override not applied")
	}
	if err := os.WriteFile(config, []byte(`{"typo":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := parseConfigFlags(flags, []string{"--config", config}); err == nil {
		t.Fatal("unknown config key accepted")
	}
}
