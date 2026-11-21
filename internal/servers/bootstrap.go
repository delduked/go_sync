package servers

import (
	"errors"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/dgraph-io/badger/v3"
)

const localPrefix = ".gosync-local-"

func privateName(name string) bool {
	return strings.HasPrefix(name, stagePrefix) || strings.HasPrefix(name, localPrefix)
}

type Option func(*Engine) error

func WithBootstrap(policy string) Option {
	return func(e *Engine) error {
		if policy != "" && policy != "seed" && policy != "join" {
			return fmt.Errorf("bootstrap must be seed or join")
		}
		e.bootstrap = policy
		return nil
	}
}
func (e *Engine) bootstrapExisting() error {
	root, err := e.root.Open(".")
	if err != nil {
		return err
	}
	entries, err := root.ReadDir(-1)
	root.Close()
	if err != nil {
		return err
	}
	var names []string
	for _, entry := range entries {
		if !privateName(entry.Name()) {
			names = append(names, entry.Name())
		}
	}
	if len(names) == 0 {
		return nil
	}
	if e.bootstrap == "seed" {
		return nil
	}
	if e.bootstrap != "join" {
		return fmt.Errorf("existing files with no sync history: choose --bootstrap seed to publish them, or --bootstrap join to archive them locally before joining")
	}
	archive := ""
	if err = e.db.View(func(tx *badger.Txn) error {
		item, err := tx.Get([]byte("bootstrap/archive"))
		if err == badger.ErrKeyNotFound {
			return nil
		}
		if err != nil {
			return err
		}
		return item.Value(func(b []byte) error { archive = string(b); return nil })
	}); err != nil {
		return err
	}
	if archive == "" {
		archive = localPrefix + "prejoin-" + randomID()
		if err = e.db.Update(func(tx *badger.Txn) error { return tx.Set([]byte("bootstrap/archive"), []byte(archive)) }); err != nil {
			return err
		}
	}
	if !strings.HasPrefix(archive, localPrefix) || path.Base(archive) != archive {
		return fmt.Errorf("invalid bootstrap archive")
	}
	if err = e.root.MkdirAll(archive, 0700); err != nil {
		return err
	}
	for _, name := range names {
		target := path.Join(archive, name)
		if _, err = e.root.Lstat(target); err == nil {
			target = path.Join(archive, randomID()+"-"+name)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		// Same-filesystem rename preserves symlinks and directory trees without
		// following them. Stop writers while bootstrapping; this is per top-level item.
		if err = e.root.Rename(name, target); err != nil {
			return err
		}
	}
	return nil
}
