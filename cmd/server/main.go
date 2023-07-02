package main

import (
	"flag"

	genericapiserver "k8s.io/apiserver/pkg/server"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/operator-framework/catalogd/pkg/catalogserver"
)

func main() {
	stopCh := genericapiserver.SetupSignalHandler()
	options := catalogserver.NewServerOptions()
	cmd := catalogserver.NewCommandStartServer(options, stopCh)
	cmd.Flags().AddGoFlagSet(flag.CommandLine)
	if err := cmd.Execute(); err != nil {
		log.Log.Error(err, "unable to start server ")
	}
}
