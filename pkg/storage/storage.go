package storage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/operator-framework/operator-registry/alpha/declcfg"
)

// Instance is a storage instance that stores FBC content of catalogs
// added to a cluster. It can be used to Store or Delete FBC in the
// host's filesystem
type Instance struct {
	RootDirectory string
}

func (s *Instance) Store(owner string, fbc *declcfg.DeclarativeConfig) error {
	fbcFile, err := os.Create(s.fbcPath(owner))
	if err != nil {
		return err
	}
	defer fbcFile.Close()
	return declcfg.WriteJSON(*fbc, fbcFile)
}

func (s *Instance) Delete(owner string) error {
	err := os.Remove(s.fbcPath(owner))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (s *Instance) fbcPath(catalogName string) string {
	return filepath.Join(s.RootDirectory, fmt.Sprintf("%s.json", catalogName))
}
