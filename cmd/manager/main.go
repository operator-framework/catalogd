/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	apimachineryrand "k8s.io/apimachinery/pkg/util/rand"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/textlogger"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/certwatcher"
	crwebhook "sigs.k8s.io/controller-runtime/pkg/webhook"

	catalogdv1 "github.com/operator-framework/catalogd/api/v1"
	"github.com/operator-framework/catalogd/internal/features"
	"github.com/operator-framework/catalogd/internal/source"
	"github.com/operator-framework/catalogd/internal/version"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

const (
	storageDir     = "catalogs"
	authFilePrefix = "catalogd-global-pull-secret"
)

type config struct {
	metricsAddr          string
	enableLeaderElection bool
	probeAddr            string
	pprofAddr            string
	systemNamespace      string
	catalogServerAddr    string
	externalAddr         string
	cacheDir             string
	gcInterval           time.Duration
	certFile             string
	keyFile              string
	webhookPort          int
	caCertDir            string
	globalPullSecret     string
}

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(catalogdv1.AddToScheme(scheme))
}

func main() {
	cfg := &config{}
	cmd := newRootCmd(cfg)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func newRootCmd(cfg *config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "catalogd",
		Short: "Catalogd is a Kubernetes operator for managing operator catalogs",
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cfg)
		},
	}

	flags := cmd.PersistentFlags()
	flags.StringVar(&cfg.metricsAddr, "metrics-bind-address", "", "The address for the metrics endpoint. Requires tls-cert and tls-key. (Default: ':7443')")
	flags.StringVar(&cfg.probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flags.StringVar(&cfg.pprofAddr, "pprof-bind-address", "0", "The address the pprof endpoint binds to. an empty string or 0 disables pprof")
	flags.BoolVar(&cfg.enableLeaderElection, "leader-elect", false, "Enable leader election for controller manager")
	flags.StringVar(&cfg.systemNamespace, "system-namespace", "", "The namespace catalogd uses for internal state")
	flags.StringVar(&cfg.catalogServerAddr, "catalogs-server-addr", ":8443", "The address where catalogs' content will be accessible")
	flags.StringVar(&cfg.externalAddr, "external-address", "catalogd-service.olmv1-system.svc", "External address for http(s) server")
	flags.StringVar(&cfg.cacheDir, "cache-dir", "/var/cache/", "Directory for file based caching")
	flags.DurationVar(&cfg.gcInterval, "gc-interval", 12*time.Hour, "Garbage collection interval")
	flags.StringVar(&cfg.certFile, "tls-cert", "", "Certificate file for TLS")
	flags.StringVar(&cfg.keyFile, "tls-key", "", "Key file for TLS")
	flags.IntVar(&cfg.webhookPort, "webhook-server-port", 9443, "Webhook server port")
	flags.StringVar(&cfg.caCertDir, "ca-certs-dir", "", "Directory of CA certificates")
	flags.StringVar(&cfg.globalPullSecret, "global-pull-secret", "", "Global pull secret (<namespace>/<name>)")

	cmd.AddCommand(newVersionCmd())
	klog.InitFlags(nil)
	flags.AddGoFlagSet(flag.CommandLine)
	features.CatalogdFeatureGate.AddFlag(flags)

	return cmd
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("%#v\n", version.Version())
		},
	}
}

func run(cfg *config) error {
	ctrl.SetLogger(textlogger.NewLogger(textlogger.NewConfig()))

	authFilePath := filepath.Join(os.TempDir(), fmt.Sprintf("%s-%s.json", authFilePrefix, apimachineryrand.String(8)))
	var globalPullSecretKey *k8stypes.NamespacedName
	if cfg.globalPullSecret != "" {
		secretParts := strings.Split(cfg.globalPullSecret, "/")
		if len(secretParts) != 2 {
			return fmt.Errorf("incorrect global-pull-secret format: should be <namespace>/<name>")
		}
		globalPullSecretKey = &k8stypes.NamespacedName{
			Name:      secretParts[1],
			Namespace: secretParts[0],
		}
	}

	if err := validateTLSConfig(cfg); err != nil {
		return err
	}

	protocol := "http://"
	if cfg.certFile != "" && cfg.keyFile != "" {
		protocol = "https://"
	}
	cfg.externalAddr = protocol + cfg.externalAddr

	k8sCfg := ctrl.GetConfigOrDie()

	cw, err := certwatcher.New(cfg.certFile, cfg.keyFile)
	if err != nil {
		return fmt.Errorf("failed to initialize certificate watcher: %w", err)
	}

	tlsOpts := func(config *tls.Config) {
		config.GetCertificate = cw.GetCertificate
		config.NextProtos = []string{"http/1.1"}
	}

	webhookServer := crwebhook.NewServer(crwebhook.Options{
		Port:    cfg.webhookPort,
		TLSOpts: []func(*tls.Config){tlsOpts},
	})

	metricsServerOptions := configureMetricsServer(cfg, tlsOpts)

	cacheOptions := configureCacheOptions(globalPullSecretKey)

	mgr, err := createManager(k8sCfg, cfg, webhookServer, metricsServerOptions, cacheOptions)
	if err != nil {
		return err
	}

	if err := mgr.Add(cw); err != nil {
		return fmt.Errorf("unable to add certificate watcher: %w", err)
	}

	if cfg.systemNamespace == "" {
		cfg.systemNamespace = podNamespace()
	}

	if err := initializeCacheDirectories(cfg.cacheDir); err != nil {
		return err
	}

	unpackCacheBasePath := filepath.Join(cfg.cacheDir, source.UnpackCacheDir)
	if err := initializeCacheDirectories(cfg.cacheDir); err != nil {
		return err
	}

	unpacker := configureUnpacker(cfg.cacheDir, cfg.caCertDir, authFilePath)

	localStorage, err := configureStorage(cfg)
	if err != nil {
		return err
	}

	if err := configureCatalogServer(mgr, cfg, localStorage, cw); err != nil {
		return err
	}

	if err := setupControllers(mgr, cfg, unpacker, localStorage, globalPullSecretKey, authFilePath); err != nil {
		return err
	}

	if err := setupHealthChecks(mgr); err != nil {
		return err
	}

	if err := setupGarbageCollector(mgr, k8sCfg, cfg, unpackCacheBasePath); err != nil {
		return err
	}

	if err := setupWebhook(mgr); err != nil {
		return err
	}

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		return fmt.Errorf("problem running manager: %w", err)
	}

	if err := os.Remove(authFilePath); err != nil {
		log.Printf("failed to cleanup temporary auth file: %v", err)
	}

	return nil
}
