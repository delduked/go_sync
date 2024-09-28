// Contents of ./internal/servers/meta_db.go
package servers

import (
	"encoding/json"
	"fmt"

	"github.com/TypeTerrors/go_sync/pkg"
	"github.com/charmbracelet/log"
	"github.com/dgraph-io/badger/v3"
)

// LoadMetaDataFromDB loads all metadata from BadgerDB into the in-memory MetaData map.
func (m *Meta) LoadMetaDataFromDB() error {
	err := m.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			key := item.Key()
			err := item.Value(func(val []byte) error {
				var meta MetaData
				err := json.Unmarshal(val, &meta)
				if err != nil {
					return err
				}
				m.MetaData[string(key)] = meta
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	return err
}

// saveMetaDataToDB saves the metadata to BadgerDB.
func (m *Meta) saveMetaDataToDB(file string, metadata MetaData) error {
	if pkg.IsTemporaryFile(file) {
		return nil
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %v", err)
	}
	err = m.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(file), data)
	})
	if err != nil {
		return fmt.Errorf("failed to save metadata to BadgerDB: %v", err)
	}
	log.Printf("Saved metadata to DB for file: %s", file)
	return nil
}

// DeleteFileMetaData removes the metadata of a file from both in-memory and BadgerDB.
func (m *Meta) DeleteFileMetaData(file string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.MetaData, file)

	err := m.db.Update(func(txn *badger.Txn) error {
		return txn.Delete([]byte(file))
	})
	if err != nil {
		log.Errorf("failed to delete metadata from BadgerDB: %v", err)
	}
}
