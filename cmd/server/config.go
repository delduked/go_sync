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
	"github.com/TypeTerrors/go_sync/internal/servers"
	"github.com/TypeTerrors/gonfig"
	"gopkg.in/yaml.v3"
)

func yamlConfig(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".yaml" || ext == ".yml"
}

func readConfigValues(config string) (map[string]string, error) {
	values := make(map[string]string)
	if yamlConfig(config) {
		loaded, err := gonfig.Load[map[string]any](gonfig.WithConfigFile(config), gonfig.WithStrict())
		if err != nil {
			return nil, err
		}
		for name, value := range loaded {
			switch value.(type) {
			case string, bool, int, int64, uint64, float64:
				values[name] = fmt.Sprint(value)
			default:
				return nil, fmt.Errorf("invalid config value for %s: expected a scalar", name)
			}
		}
		return values, nil
	}
	// Keep legacy JSON parsing and literal strings unchanged. Environment
	// interpolation is available in YAML through gonfig, not retrofitted to JSON.
	data, err := os.ReadFile(config)
	if err != nil {
		return nil, err
	}
	var rawValues map[string]json.RawMessage
	if err = json.Unmarshal(data, &rawValues); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	for name, raw := range rawValues {
		value := string(raw)
		if len(value) > 0 && value[0] == '"' {
			if err = json.Unmarshal(raw, &value); err != nil {
				return nil, err
			}
		} else if value == "null" || len(value) == 0 || value[0] == '[' || value[0] == '{' {
			return nil, fmt.Errorf("invalid config value for %s", name)
		}
		values[name] = value
	}
	return values, nil
}

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
		values, err := readConfigValues(config)
		if err != nil {
			return err
		}
		for name, value := range values {
			if name == "config" || flags.Lookup(name) == nil {
				return fmt.Errorf("unknown config setting %q", name)
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
	config := flags.String("config", "", "New YAML or JSON config (default: user config directory/gosync/default.yaml)")
	folder := flags.String("sync-folder", "", "Folder to synchronize")
	state := flags.String("state-dir", "", "Private state outside shared folder (default: CONFIG.state)")
	identity := flags.String("identity", "", "Optional peer identity bundle for mutual TLS instead of automatic LAN access")
	folderID := flags.String("folder-id", "", "Sync group (default: gosync-lan, or a new ID with --identity)")
	bootstrap := flags.String("bootstrap", "", "Prepare first history: seed publishes existing files; join archives them (LAN default: seed)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *folder == "" || flags.NArg() != 0 {
		return fmt.Errorf("choose a folder: gosync init --sync-folder /path/to/shared")
	}
	if *bootstrap != "" && *bootstrap != "seed" && *bootstrap != "join" {
		return fmt.Errorf("--bootstrap must be seed or join")
	}
	if *config == "" {
		var err error
		*config, err = defaultConfigPath()
		if err != nil {
			return err
		}
		legacy := strings.TrimSuffix(*config, ".yaml") + ".json"
		if _, err = os.Stat(legacy); err == nil {
			return fmt.Errorf("saved configuration already exists at %s; start it with gosync or choose a different --config path", legacy)
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	configPath, err := filepath.Abs(*config)
	if err != nil {
		return err
	}
	root, err := filepath.Abs(*folder)
	if err != nil {
		return err
	}
	if *state == "" {
		*state = configPath + ".state"
	}
	statePath, err := filepath.Abs(*state)
	if err != nil {
		return err
	}
	for _, private := range []string{configPath, statePath} {
		if err = outsideFolder(root, private); err != nil {
			return err
		}
	}
	values := map[string]any{"sync-folder": root, "state-dir": statePath, "sync-interval": "1s", "fanout": 2}
	lan := *identity == ""
	if lan {
		values["insecure"] = true
		if *folderID == "" {
			*folderID = lanFolderID
		}
	} else {
		identityPath, err := filepath.Abs(*identity)
		if err != nil {
			return err
		}
		cfg := conf.Config{TLSCert: filepath.Join(identityPath, "peer.pem"), TLSKey: filepath.Join(identityPath, "peer-key.pem"), TLSCA: filepath.Join(identityPath, "ca.pem")}
		if _, _, err = transport(cfg); err != nil {
			return err
		}
		values["tls-cert"], values["tls-key"], values["tls-ca"] = cfg.TLSCert, cfg.TLSKey, cfg.TLSCA
	}
	if *folderID == "" {
		bytes := make([]byte, 16)
		if _, err = rand.Read(bytes); err != nil {
			return err
		}
		*folderID = hex.EncodeToString(bytes)
	}
	values["folder-id"] = *folderID
	var data []byte
	if yamlConfig(configPath) {
		// gonfig expands placeholders before parsing YAML, even inside quoted
		// scalars. Never turn a literal selected path into a different folder.
		for name, value := range values {
			if literal, ok := value.(string); ok && strings.Contains(literal, "${") {
				return fmt.Errorf("%s contains a literal environment placeholder; use --config peer.json to preserve it, or add environment placeholders to YAML after setup", name)
			}
		}
		data, err = yaml.Marshal(values)
	} else {
		data, err = json.MarshalIndent(values, "", "  ")
	}
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		return err
	}
	out, err := os.OpenFile(configPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("create config: %w (existing configurations are never overwritten)", err)
	}
	saved := false
	defer func() {
		out.Close()
		if !saved {
			os.Remove(configPath)
		}
	}()
	if lan || *bootstrap != "" {
		policy := *bootstrap
		if policy == "" {
			policy = "seed"
		}
		// Selecting a folder in LAN init intentionally publishes its contents.
		// Save this decision once, never as a permanent bootstrap=seed setting.
		engine, err := servers.NewEngine(root, statePath, servers.WithFolderID(*folderID), servers.WithBootstrap(policy), servers.WithStoredState())
		if err != nil {
			return err
		}
		if err = engine.Close(); err != nil {
			return err
		}
	}
	_, err = out.Write(append(data, '\n'))
	if err == nil {
		err = out.Sync()
	}
	closeErr := out.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	saved = true
	fmt.Printf("Created %s\nShared folder: %s\nState: %s\n", configPath, root, statePath)
	if lan {
		fmt.Printf("LAN mode: automatic mDNS discovery in group %s; connections are unencrypted and peers are not authenticated.\n", *folderID)
	} else {
		fmt.Printf("Mutual TLS enabled; use folder ID %s on the other peers.\n", *folderID)
	}
	if defaultPath, err := defaultConfigPath(); err == nil && filepath.Clean(configPath) == filepath.Clean(defaultPath) {
		fmt.Println("Start syncing: gosync")
	} else {
		fmt.Printf("Start syncing: gosync --config %q\n", configPath)
	}
	return nil
}
