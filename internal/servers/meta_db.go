package servers

import (
	"encoding/json"
	"fmt"

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
	data, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %v", err)
	}
	err = m.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(file), data)
	})
	return err
}

func (m *Meta) saveMetaDataToRam(file string, metadata MetaData) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.MetaData[file] = metadata
}

func (m *Meta) saveMetaData(file string, metadata MetaData) {
	err := m.saveMetaDataToDB(file, metadata)
	if err != nil {
		log.Errorf("failed to save metadata to DB: %v", err)
	}
	m.saveMetaDataToRam(file, metadata)
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
		log.Errorf("failed to delete metadata from badger DB: %v", err)
	}
}

// func (m *Meta) saveToBadger(file string) error {
// 	err := m.db.Update(func(txn *badger.Txn) error {
// 		meta := m.MetaData[file]

// 		// Serialize the MetaData (use a suitable serialization method like JSON or Gob)
// 		val, err := json.Marshal(meta)
// 		if err != nil {
// 			return err
// 		}

// 		// Save the metadata in BadgerDB with the file name as the key
// 		return txn.Set([]byte(file), val)
// 	})
// 	return err
// }
