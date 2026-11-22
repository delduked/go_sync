package main

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/TypeTerrors/go_sync/internal/servers"
)

func TestYAMLConfigWithGonfigAndOverrides(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GOSYNC_TEST_FOLDER", "folder with spaces")
	t.Setenv("GOSYNC_TEST_INTERVAL", "250ms")
	config := filepath.Join(dir, "peer.yaml")
	data := "sync-folder: '${GOSYNC_TEST_FOLDER}'\nsync-interval: '${GOSYNC_TEST_INTERVAL:-1s}'\nfanout: 2\nno-mdns: true\n"
	if err := os.WriteFile(config, []byte(data), 0600); err != nil {
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
	if *folder != filepath.Join(dir, "folder with spaces") || *interval != 250*time.Millisecond || *fanout != 3 || !*noMDNS {
		t.Fatal("YAML/env/relative path/override not applied")
	}
}

func TestYAMLConfigRejectsInvalidSettings(t *testing.T) {
	for _, data := range []string{"typo: true", "fanout: [1, 2]", "fanout: null", "fanout: 2\nfanout: 3", "fanout: '${GOSYNC_TEST_NEVER_SET_489}'", "config: other.yaml"} {
		t.Run(data, func(t *testing.T) {
			config := filepath.Join(t.TempDir(), "peer.yml")
			if err := os.WriteFile(config, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			flags := flag.NewFlagSet("test", flag.ContinueOnError)
			flags.Int("fanout", 2, "")
			flags.String("config", "", "")
			if err := parseConfigFlags(flags, []string{"--config", config}); err == nil {
				t.Fatal("accepted invalid configuration")
			}
		})
	}
}

func TestJSONKeepsLiteralEnvironmentPlaceholders(t *testing.T) {
	config := filepath.Join(t.TempDir(), "legacy.json")
	if err := os.WriteFile(config, []byte(`{"sync-folder":"${GOSYNC_TEST_LITERAL}"}`), 0600); err != nil {
		t.Fatal(err)
	}
	values, err := readConfigValues(config)
	if err != nil {
		t.Fatal(err)
	}
	if values["sync-folder"] != "${GOSYNC_TEST_LITERAL}" {
		t.Fatal("changed legacy JSON semantics")
	}
}

func TestLANInitPublishesExistingFolderOnce(t *testing.T) {
	root := t.TempDir()
	folder, config := filepath.Join(root, "files"), filepath.Join(root, "peer.yaml")
	if err := os.Mkdir(folder, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "existing"), []byte("keep this"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := initCommand([]string{"--config", config, "--sync-folder", folder}); err != nil {
		t.Fatal(err)
	}
	values, err := readConfigValues(config)
	if err != nil {
		t.Fatal(err)
	}
	if values["insecure"] != "true" || values["folder-id"] != lanFolderID || values["bootstrap"] != "" || values["tls-key"] != "" {
		t.Fatalf("unexpected config: %v", values)
	}
	if values["state-dir"] != config+".state" {
		t.Fatal("state was not selected automatically")
	}
	engine, err := servers.NewEngine(folder, values["state-dir"], servers.WithFolderID(lanFolderID))
	if err != nil {
		t.Fatal(err)
	}
	id := engine.ID()
	manifest, err := engine.Manifest()
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Files) != 1 || manifest.Files[0].FileName != "existing" {
		t.Fatal("existing file not adopted")
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	engine, err = servers.NewEngine(folder, values["state-dir"], servers.WithFolderID(lanFolderID))
	if err != nil {
		t.Fatal(err)
	}
	if engine.ID() != id {
		t.Fatal("normal restart changed identity")
	}
	engine.Close()
	// Losing history must still require an explicit recovery decision.
	if err := os.Rename(values["state-dir"], values["state-dir"]+".saved"); err != nil {
		t.Fatal(err)
	}
	if e, err := servers.NewEngine(folder, values["state-dir"], servers.WithFolderID(lanFolderID)); err == nil {
		e.Close()
		t.Fatal("silently reseeded after state loss")
	}
	data, err := os.ReadFile(filepath.Join(folder, "existing"))
	if err != nil || string(data) != "keep this" {
		t.Fatal("changed source contents")
	}
}

func TestLANInitDoesNotOverwriteConfigOrPublishPrivateFiles(t *testing.T) {
	base := t.TempDir()
	config := filepath.Join(base, "existing.yaml")
	before := []byte("existing configuration")
	if err := os.WriteFile(config, before, 0600); err != nil {
		t.Fatal(err)
	}
	if err := initCommand([]string{"--config", config, "--sync-folder", filepath.Join(base, "new")}); err == nil {
		t.Fatal("overwrote existing config")
	}
	after, _ := os.ReadFile(config)
	if string(before) != string(after) {
		t.Fatal("changed existing config")
	}
	if _, err := os.Stat(filepath.Join(base, "new")); !os.IsNotExist(err) {
		t.Fatal("initialized root despite existing config")
	}
	folder := filepath.Join(base, "files")
	for _, args := range [][]string{
		{"--config", filepath.Join(folder, "peer.yaml"), "--sync-folder", folder},
		{"--config", filepath.Join(base, "peer.yaml"), "--sync-folder", folder, "--state-dir", filepath.Join(folder, "state")},
	} {
		if err := initCommand(args); err == nil || !strings.Contains(err.Error(), "outside") {
			t.Fatalf("accepted private files inside root: %v", err)
		}
	}
}

func TestInitRejectsSymlinkedPrivatePaths(t *testing.T) {
	base := t.TempDir()
	folder := filepath.Join(base, "files")
	if err := os.Mkdir(folder, 0755); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(base, "alias")
	if err := os.Symlink(folder, alias); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := initCommand([]string{"--config", filepath.Join(alias, "new", "config.yaml"), "--sync-folder", folder}); err == nil {
		t.Fatal("accepted config through symlink inside root")
	}
	if _, err := os.Stat(filepath.Join(folder, "new")); !os.IsNotExist(err) {
		t.Fatal("created private path inside root")
	}
}

func TestInvalidIdentityNeverFallsBackToLAN(t *testing.T) {
	base := t.TempDir()
	config := filepath.Join(base, "peer.yaml")
	if err := initCommand([]string{"--config", config, "--sync-folder", filepath.Join(base, "files"), "--identity", filepath.Join(base, "missing")}); err == nil {
		t.Fatal("invalid TLS identity fell back to LAN")
	}
	if _, err := os.Stat(config); !os.IsNotExist(err) {
		t.Fatal("created config despite invalid TLS identity")
	}
}

func TestDefaultStartupAndLegacyJSONFallback(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("isolates the native config directory using XDG_CONFIG_HOME")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if _, err := startupArgs(nil); err == nil || !strings.Contains(err.Error(), "gosync init") {
		t.Fatal("missing setup guidance")
	}
	folder := filepath.Join(t.TempDir(), "selected")
	if err := initCommand([]string{"--sync-folder", folder}); err != nil {
		t.Fatal(err)
	}
	config, _ := defaultConfigPath()
	args, err := startupArgs(nil)
	if err != nil || len(args) != 2 || args[1] != config {
		t.Fatalf("default startup: %v %v", args, err)
	}
	// Arguments remain explicit: never mix a saved config into a raw flag launch.
	args, err = startupArgs([]string{"--help"})
	if err != nil || len(args) != 1 || args[0] != "--help" {
		t.Fatal("intercepted explicit invocation")
	}
	legacy := strings.TrimSuffix(config, ".yaml") + ".json"
	values, err := readConfigValues(config)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(config); err != nil {
		t.Fatal(err)
	}
	args, err = startupArgs(nil)
	if err != nil || args[1] != legacy {
		t.Fatal("did not find saved legacy JSON configuration")
	}
	if err := initCommand([]string{"--sync-folder", folder}); err == nil {
		t.Fatal("created a new LAN config that shadows an existing JSON config")
	}
}

func TestInitFailureDoesNotLeaveBrokenConfig(t *testing.T) {
	base := t.TempDir()
	folder, config := filepath.Join(base, "files"), filepath.Join(base, "peer.yml")
	if err := os.WriteFile(folder, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := initCommand([]string{"--config", config, "--sync-folder", folder}); err == nil {
		t.Fatal("accepted non-directory root")
	}
	if _, err := os.Stat(config); !os.IsNotExist(err) {
		t.Fatal("left incomplete config")
	}
}

func TestYAMLInitCannotRedirectALiteralFolderThroughEnvironment(t *testing.T) {
	base := t.TempDir()
	t.Setenv("GOSYNC_TEST_LITERAL_PATH", "different-folder")
	folder := filepath.Join(base, "${GOSYNC_TEST_LITERAL_PATH}")
	config := filepath.Join(base, "peer.yaml")
	if err := initCommand([]string{"--config", config, "--sync-folder", folder}); err == nil {
		t.Fatal("generated YAML would synchronize a different folder after expansion")
	}
	if _, err := os.Stat(config); !os.IsNotExist(err) {
		t.Fatal("left an ambiguous generated YAML config")
	}
	if err := initCommand([]string{"--config", filepath.Join(base, "peer.json"), "--sync-folder", folder}); err != nil {
		t.Fatal("literal JSON path should remain supported:", err)
	}
}

func TestInitJoinArchivesInsteadOfPublishingExistingFiles(t *testing.T) {
	base := t.TempDir()
	folder, config := filepath.Join(base, "files"), filepath.Join(base, "peer.yaml")
	if err := os.Mkdir(folder, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "old"), []byte("old copy"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := initCommand([]string{"--config", config, "--sync-folder", folder, "--bootstrap", "join"}); err != nil {
		t.Fatal(err)
	}
	archives, err := filepath.Glob(filepath.Join(folder, ".gosync-local-prejoin-*", "old"))
	if err != nil || len(archives) != 1 {
		t.Fatal("join did not retain the old contents privately")
	}
	data, err := os.ReadFile(archives[0])
	if err != nil || string(data) != "old copy" {
		t.Fatal("archived contents changed")
	}
	engine, err := servers.NewEngine(folder, config+".state", servers.WithFolderID(lanFolderID))
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	manifest, err := engine.Manifest()
	if err != nil || len(manifest.Files) != 0 {
		t.Fatal("join published archived contents")
	}
}
