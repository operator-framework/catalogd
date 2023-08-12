package catalogserver

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Server is a manager.Runnable Server, that serves the FBC
// content of the extension catalogs added to the cluster
type Server struct {
	Dir  string
	Port string
	Mux  *http.ServeMux
}

// NewServer takes directory and port number, and returns
// a Server that serves the FBC content stored in the
// directory on the given port number.
func NewServer(dir, port string) Server {
	return Server{
		Dir:  dir,
		Port: port,
		Mux:  &http.ServeMux{},
	}
}

func (s Server) Start(_ context.Context) error {
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
	return server.ListenAndServe()
}
