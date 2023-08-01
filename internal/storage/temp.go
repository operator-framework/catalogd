package storage

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"testing/fstest"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ Storage = &TempStorage{}

var _ http.Handler = &TempStorage{}

type TempStorage struct{}

func (ts *TempStorage) Store(ctx context.Context, owner client.Object, bundle fs.FS) error {
	log.Printf("Storing contents for %s", owner.GetName())
	return nil
}

func (ts *TempStorage) Delete(ctx context.Context, owner client.Object) error {
	log.Printf("Deleting contents for %s", owner.GetName())
	return nil
}

func (ts *TempStorage) URLFor(ctx context.Context, owner client.Object) (string, error) {
	return fmt.Sprintf("%s-tempstorage-url", owner.GetName()), nil
}

func (ts *TempStorage) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	// Eat any errors since this is temporary
	rw.Write([]byte("TempStorage serving some HTTP!"))
}

func (ts *TempStorage) Load(ctx context.Context, owner client.Object) (fs.FS, error) {
	return &fstest.MapFS{}, nil
}
