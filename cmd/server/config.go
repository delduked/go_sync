package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/TypeTerrors/go_sync/conf"
)

func parseConfigFlags(flags *flag.FlagSet, args []string) error {
	var config string
	for i, arg := range args {
		if strings.HasPrefix(arg, "--config=") || strings.HasPrefix(arg, "-config=") {
			config = strings.SplitN(arg, "=", 2)[1]
		}
		if (arg == "--config" || arg == "-config") && i+1 < len(args) {
			config = args[i+1]
		}
	}
	if config != "" {
		data, err := os.ReadFile(config)
		if err != nil {
			return err
		}
		var values map[string]json.RawMessage
		if err = json.Unmarshal(data, &values); err != nil {
			return fmt.Errorf("config: %w", err)
		}
		for name, raw := range values {
			if name == "config" || flags.Lookup(name) == nil {
				return fmt.Errorf("unknown config setting %q", name)
			}
			value := string(raw)
			if len(value) > 0 && value[0] == '"' {
				if err = json.Unmarshal(raw, &value); err != nil {
					return err
				}
			} else if value == "null" || len(value) == 0 || value[0] == '[' || value[0] == '{' {
				return fmt.Errorf("invalid config value for %s", name)
			}
			switch name {
			case "sync-folder", "state-dir", "tls-cert", "tls-key", "tls-ca":
				if value != "" && !filepath.IsAbs(value) {
					value = filepath.Join(filepath.Dir(config), value)
				}
			}
			if err = flags.Set(name, value); err != nil {
				return fmt.Errorf("config %s: %w", name, err)
			}
		}
	}
	return flags.Parse(args)
}
func initCommand(args []string) error {
	flags := flag.NewFlagSet("init", flag.ContinueOnError)
	config := flags.String("config", "", "New JSON configuration path")
	folder := flags.String("sync-folder", "", "Folder to synchronize")
	state := flags.String("state-dir", "", "Private state directory outside shared folder")
	identity := flags.String("identity", "", "Peer identity bundle directory")
	folderID := flags.String("folder-id", "", "Existing shared folder identity, or generate one")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *config == "" || *folder == "" || *state == "" || *identity == "" || flags.NArg() != 0 {
		return fmt.Errorf("init requires --config, --sync-folder, --state-dir and --identity")
	}
	root, err := filepath.Abs(*folder)
	if err != nil {
		return err
	}
	statePath, err := filepath.Abs(*state)
	if err != nil {
		return err
	}
	identityPath, err := filepath.Abs(*identity)
	if err != nil {
		return err
	}
	cfg := conf.Config{TLSCert: filepath.Join(identityPath, "peer.pem"), TLSKey: filepath.Join(identityPath, "peer-key.pem"), TLSCA: filepath.Join(identityPath, "ca.pem")}
	if _, _, err = transport(cfg); err != nil {
		return err
	}
	if *folderID == "" {
		bytes := make([]byte, 16)
		if _, err = rand.Read(bytes); err != nil {
			return err
		}
		*folderID = hex.EncodeToString(bytes)
	}
	data, err := json.MarshalIndent(map[string]any{"sync-folder": root, "state-dir": statePath, "folder-id": *folderID, "tls-cert": cfg.TLSCert, "tls-key": cfg.TLSKey, "tls-ca": cfg.TLSCA, "sync-interval": "1s", "fanout": 2}, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(*config), 0700); err != nil {
		return err
	}
	out, err := os.OpenFile(*config, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = out.Write(append(data, '\n'))
	closeErr := out.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	fmt.Printf("Created %s for folder %s; use this folder ID on the other peers\n", *config, *folderID)
	return nil
}
