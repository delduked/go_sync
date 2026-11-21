package pkg

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const CompletionSuffix = ".gosync-complete.json"

type Completion struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

func completionFor(name string) (Completion, error) {
	file, err := os.Open(name)
	if err != nil {
		return Completion{}, err
	}
	defer file.Close()
	before, err := file.Stat()
	if err != nil {
		return Completion{}, err
	}
	if !before.Mode().IsRegular() {
		return Completion{}, fmt.Errorf("completion requires a regular file")
	}
	h := sha256.New()
	n, err := io.CopyN(h, file, before.Size())
	if err != nil {
		return Completion{}, err
	}
	after, err := os.Stat(name)
	if err != nil {
		return Completion{}, err
	}
	if !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return Completion{}, fmt.Errorf("file changed while marking completion")
	}
	return Completion{Name: filepath.Base(name), Size: n, SHA256: hex.EncodeToString(h.Sum(nil))}, nil
}

// MarkComplete is an explicit producer assertion. Call only after closing the
// writer. The sidecar travels independently and is never proof by itself.
func MarkComplete(name string) (string, error) {
	record, err := completionFor(name)
	if err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return "", err
	}
	temp, err := os.CreateTemp(filepath.Dir(name), ".gosync-stage-completion-")
	if err != nil {
		return "", err
	}
	defer os.Remove(temp.Name())
	_, err = temp.Write(data)
	if err == nil {
		err = temp.Sync()
	}
	closeErr := temp.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return "", err
	}
	marker := name + CompletionSuffix
	if err = os.Rename(temp.Name(), marker); err != nil {
		return "", err
	}
	return marker, nil
}

// VerifyComplete always hashes the local data. A marker may arrive before its
// file; old markers fail after subsequent edits instead of falsely completing.
func VerifyComplete(name string) error {
	raw, err := os.ReadFile(name + CompletionSuffix)
	if err != nil {
		return err
	}
	var expected Completion
	if err = json.Unmarshal(raw, &expected); err != nil {
		return err
	}
	if expected.Name != filepath.Base(name) || expected.Size < 0 {
		return fmt.Errorf("invalid completion marker")
	}
	actual, err := completionFor(name)
	if err != nil {
		return err
	}
	if actual != expected {
		return fmt.Errorf("file does not match completion marker")
	}
	return nil
}
