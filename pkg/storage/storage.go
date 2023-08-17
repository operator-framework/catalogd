package storage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/operator-framework/operator-registry/alpha/declcfg"
)

// Storage is a store of FBC content of catalogs added to a cluster.
// It can be used to Store or Delete FBC in the host's filesystem
type Storage struct {
	RootDirectory string
}

func NewStorage(rootDir string) Storage {
	return Storage{
		RootDirectory: rootDir,
	}
}

func (s *Storage) Store(owner string, fbc *declcfg.DeclarativeConfig) error {
	fbcFile, err := os.Create(s.fbcPath(owner))
	if err != nil {
		return err
	}
	defer fbcFile.Close()

	if err := declcfg.WriteJSON(*fbc, fbcFile); err != nil {
		return err
	}
	return nil
}

func (s *Storage) Delete(owner string) error {
	err := os.Remove(s.fbcPath(owner))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (s *Storage) fbcPath(catalogName string) string {
	return filepath.Join(s.RootDirectory, fmt.Sprintf("%s.json", catalogName))
}
