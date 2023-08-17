package catalogserver

import (
	"context"
	"net/http"
	"os"
	"time"

	"golang.org/x/sync/errgroup"
)

// Instance is a manager.Runnable catalog server instance,
// that serves the FBC content of the extension catalogs
// added to the cluster
type Instance struct {
	Dir             string
	Addr            string
	ShutdownTimeout time.Duration
}

// New returns an Instance of a catalog server that serves
// the FBC content stored in the given directory on the given
// http address.
func New(dir, addr string) Instance {
	return Instance{
		Dir:  dir,
		Addr: addr,
	}
}

func (s Instance) Start(ctx context.Context) error {
	server := &http.Server{
		Handler:           ServerHandler(s.Dir),
		Addr:              s.Addr,
		ReadHeaderTimeout: 3 * time.Second,
	}
	eg, ctx := errgroup.WithContext(ctx)
	// run a server in a go routine
	// server.ListenAndServer() returns under two circumstances
	// 1. If there was an error starting the server
	// 2. If the server was shut down (ErrServerClosed)
	// i.e both are non-nil errors
	eg.Go(func() error { return server.ListenAndServe() })
	// waiting for one of two things
	// 1. a error is returned from the go routine
	// 2. the runnable's context is cancelled
	if err := eg.Wait(); err != nil && ctx.Err() == nil {
		// we only get here if we're in case 1 (both case 1s)
		return err
	}
	// if the ShutdownTimeout is zero, wait forever to shutdown
	// otherwise force shut down when timeout expires
	sc := context.Background()
	if s.ShutdownTimeout > 0 {
		var scc context.CancelFunc
		sc, scc = context.WithTimeout(context.Background(), s.ShutdownTimeout)
		defer scc()
	}
	// if the runnable's context was cancelled, shut down the server
	return server.Shutdown(sc)
}

func ServerHandler(dir string) http.Handler {
	fs := http.FileServer(http.FS(os.DirFS(dir)))
	return fs
}
