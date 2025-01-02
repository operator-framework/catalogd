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
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/containers/image/v5/types"
	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	k8slabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	apimachineryrand "k8s.io/apimachinery/pkg/util/rand"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/metadata"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/textlogger"
	ctrl "sigs.k8s.io/controller-runtime"
	crcache "sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/certwatcher"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	crwebhook "sigs.k8s.io/controller-runtime/pkg/webhook"

	catalogdv1 "github.com/operator-framework/catalogd/api/v1"
	corecontrollers "github.com/operator-framework/catalogd/internal/controllers/core"
	"github.com/operator-framework/catalogd/internal/features"
	"github.com/operator-framework/catalogd/internal/garbagecollection"
	"github.com/operator-framework/catalogd/internal/serverutil"
	"github.com/operator-framework/catalogd/internal/source"
	"github.com/operator-framework/catalogd/internal/storage"
	"github.com/operator-framework/catalogd/internal/version"
	"github.com/operator-framework/catalogd/internal/webhook"
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

func validateTLSConfig(cfg *config) error {
	if (cfg.certFile != "" && cfg.keyFile == "") || (cfg.certFile == "" && cfg.keyFile != "") {
		return fmt.Errorf("tls-cert and tls-key flags must be used together")
	}

	if cfg.metricsAddr != "" && cfg.certFile == "" && cfg.keyFile == "" {
		return fmt.Errorf("metrics-bind-address requires tls-cert and tls-key flags")
	}

	if cfg.certFile != "" && cfg.keyFile != "" && cfg.metricsAddr == "" {
		cfg.metricsAddr = ":7443"
	}

	return nil
}

func configureMetricsServer(cfg *config, tlsOpts func(*tls.Config)) metricsserver.Options {
	options := metricsserver.Options{}

	if cfg.certFile != "" && cfg.keyFile != "" {
		setupLog.Info("Starting metrics server with TLS enabled",
			"addr", cfg.metricsAddr,
			"tls-cert", cfg.certFile,
			"tls-key", cfg.keyFile)

		options.BindAddress = cfg.metricsAddr
		options.SecureServing = true
		options.FilterProvider = filters.WithAuthenticationAndAuthorization
		options.TLSOpts = append(options.TLSOpts, tlsOpts)
	} else {
		options.BindAddress = "0"
		setupLog.Info("WARNING: Metrics Server is disabled. " +
			"Metrics will not be served since the TLS certificate and key file are not provided.")
	}

	return options
}

func configureCacheOptions(globalPullSecretKey *k8stypes.NamespacedName) crcache.Options {
	options := crcache.Options{
		ByObject: map[client.Object]crcache.ByObject{},
	}

	if globalPullSecretKey != nil {
		options.ByObject[&corev1.Secret{}] = crcache.ByObject{
			Namespaces: map[string]crcache.Config{
				globalPullSecretKey.Namespace: {
					LabelSelector: k8slabels.Everything(),
					FieldSelector: fields.SelectorFromSet(map[string]string{
						"metadata.name": globalPullSecretKey.Name,
					}),
				},
			},
		}
	}

	return options
}

func createManager(k8sCfg *rest.Config, cfg *config, webhookServer crwebhook.Server,
	metricsOpts metricsserver.Options, cacheOpts crcache.Options) (ctrl.Manager, error) {
	return ctrl.NewManager(k8sCfg, ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsOpts,
		PprofBindAddress:       cfg.pprofAddr,
		HealthProbeBindAddress: cfg.probeAddr,
		LeaderElection:         cfg.enableLeaderElection,
		LeaderElectionID:       "catalogd-operator-lock",
		WebhookServer:          webhookServer,
		Cache:                  cacheOpts,
	})
}

func initializeCacheDirectories(cacheDir string) error {
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		return fmt.Errorf("unable to create cache directory: %w", err)
	}

	unpackCacheBasePath := filepath.Join(cacheDir, source.UnpackCacheDir)
	if err := os.MkdirAll(unpackCacheBasePath, 0770); err != nil {
		return fmt.Errorf("unable to create cache directory for unpacking: %w", err)
	}

	return nil
}

func configureUnpacker(cacheDir, caCertDir, authFilePath string) *source.ContainersImageRegistry {
	unpackCacheBasePath := filepath.Join(cacheDir, source.UnpackCacheDir)
	return &source.ContainersImageRegistry{
		BaseCachePath: unpackCacheBasePath,
		SourceContextFunc: func(logger logr.Logger) (*types.SystemContext, error) {
			srcContext := &types.SystemContext{
				DockerCertPath: caCertDir,
				OCICertPath:    caCertDir,
			}
			if _, err := os.Stat(authFilePath); err == nil {
				logger.Info("using available authentication information for pulling image")
				srcContext.AuthFilePath = authFilePath
			}
			return srcContext, nil
		},
	}
}

func configureStorage(cfg *config) (storage.Instance, error) {
	storeDir := filepath.Join(cfg.cacheDir, storageDir)
	if err := os.MkdirAll(storeDir, 0700); err != nil {
		return nil, fmt.Errorf("unable to create storage directory for catalogs: %w", err)
	}

	baseStorageURL, err := url.Parse(fmt.Sprintf("%s/catalogs/", cfg.externalAddr))
	if err != nil {
		return nil, fmt.Errorf("unable to create base storage URL: %w", err)
	}

	return storage.LocalDirV1{RootDir: storeDir, RootURL: baseStorageURL}, nil
}

func configureCatalogServer(mgr ctrl.Manager, cfg *config, localStorage storage.Instance, cw *certwatcher.CertWatcher) error {
	catalogServerConfig := serverutil.CatalogServerConfig{
		ExternalAddr: cfg.externalAddr,
		CatalogAddr:  cfg.catalogServerAddr,
		CertFile:     cfg.certFile,
		KeyFile:      cfg.keyFile,
		LocalStorage: localStorage,
	}

	return serverutil.AddCatalogServerToManager(mgr, catalogServerConfig, cw)
}

func setupControllers(mgr ctrl.Manager, cfg *config, unpacker *source.ContainersImageRegistry, localStorage storage.Instance,
	globalPullSecretKey *k8stypes.NamespacedName, authFilePath string) error {
	if err := (&corecontrollers.ClusterCatalogReconciler{
		Client:   mgr.GetClient(),
		Unpacker: unpacker,
		Storage:  localStorage,
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create controller: %w", err)
	}

	if globalPullSecretKey != nil {
		setupLog.Info("creating SecretSyncer controller for watching secret", "Secret", cfg.globalPullSecret)
		err := (&corecontrollers.PullSecretReconciler{
			Client:       mgr.GetClient(),
			AuthFilePath: authFilePath,
			SecretKey:    *globalPullSecretKey,
		}).SetupWithManager(mgr)
		if err != nil {
			return fmt.Errorf("unable to create controller: %w", err)
		}
	}

	return nil
}

func setupHealthChecks(mgr ctrl.Manager) error {
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return fmt.Errorf("unable to set up health check: %w", err)
	}

	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return fmt.Errorf("unable to set up ready check: %w", err)
	}

	return nil
}

func setupGarbageCollector(mgr ctrl.Manager, k8sCfg *rest.Config, cfg *config, unpackCacheBasePath string) error {
	metaClient, err := metadata.NewForConfig(k8sCfg)
	if err != nil {
		return fmt.Errorf("unable to setup client for garbage collection: %w", err)
	}

	gc := &garbagecollection.GarbageCollector{
		CachePath:      unpackCacheBasePath,
		Logger:         ctrl.Log.WithName("garbage-collector"),
		MetadataClient: metaClient,
		Interval:       cfg.gcInterval,
	}
	if err := mgr.Add(gc); err != nil {
		return fmt.Errorf("unable to add garbage collector to manager: %w", err)
	}

	return nil
}

func setupWebhook(mgr ctrl.Manager) error {
	if err := (&webhook.ClusterCatalog{}).SetupWebhookWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create webhook: %w", err)
	}

	return nil
}

func podNamespace() string {
	namespace, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		return "olmv1-system"
	}
	return string(namespace)
}
