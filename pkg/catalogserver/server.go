package catalogserver

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Instance is a manager.Runnable catalog server instance,
// that serves the FBC content of the extension catalogs
// added to the cluster
type Instance struct {
	Dir  string
	Port string
	Mux  *http.ServeMux
}

// New returns an Instance of a catalog server that serves
// the FBC content stored in the given directory on the given
// http address.
func New(dir, port string, mux *http.ServeMux) Instance {
	return Instance{
		Dir:  dir,
		Port: port,
		Mux:  mux,
	}
}

func (s Instance) Start(ctx context.Context) error {
	s.Mux.HandleFunc("/catalogs", func(w http.ResponseWriter, r *http.Request) {
		files, err := os.ReadDir(s.Dir)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "error reading catalog store directory: %v", err)
			return
		}
		for _, file := range files {
			name := file.Name()
			fmt.Fprintf(w, "%v\n", name[:len(name)-len(filepath.Ext(name))])
		}
	})
	server := &http.Server{
		Handler:           s.Mux,
		Addr:              s.Port,
		ReadHeaderTimeout: 3 * time.Second,
	}
	e := make(chan error)
	go func(server *http.Server, e chan error) {
		err := server.ListenAndServe()
		e <- err
		close(e)
	}(server, e)
	err := <-e
	if err != nil {
		return err
	}

	<-ctx.Done()
	return server.Shutdown(context.TODO())
}
