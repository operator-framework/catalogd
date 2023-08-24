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
	"context"
	"flag"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"oras.land/oras-go/v2/content/oci"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/operator-framework/catalogd/api/core/v1alpha1"
	"github.com/operator-framework/catalogd/internal/source"
	"github.com/operator-framework/catalogd/internal/version"
	corecontrollers "github.com/operator-framework/catalogd/pkg/controllers/core"
	"github.com/operator-framework/catalogd/pkg/features"
	"github.com/operator-framework/catalogd/pkg/profile"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(v1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var (
		metricsAddr          string
		enableLeaderElection bool
		probeAddr            string
		unpackImage          string
		profiling            bool
		catalogdVersion      bool
		systemNamespace      string
		cacheDir             string
	)
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	// TODO: should we move the unpacker to some common place? Or... hear me out... should catalogd just be a rukpak provisioner?
	flag.StringVar(&unpackImage, "unpack-image", "quay.io/operator-framework/rukpak:v0.12.0", "The unpack image to use when unpacking catalog images. Only used if feature gate DirectImageRegistrySource is not enabled")
	flag.StringVar(&systemNamespace, "system-namespace", "", "The namespace catalogd uses for internal state, configuration, and workloads")
	flag.StringVar(&cacheDir, "cache-dir", "/var/cache", "The directory to use for various caches")
	flag.BoolVar(&profiling, "profiling", false, "enable profiling endpoints to allow for using pprof")
	flag.BoolVar(&catalogdVersion, "version", false, "print the catalogd version and exit")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)

	// Combine both flagsets and parse them
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	features.CatalogdFeatureGate.AddFlag(pflag.CommandLine)
	pflag.Parse()

	if catalogdVersion {
		fmt.Printf("%#v\n", version.Version())
		os.Exit(0)
	}

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	for f, fSpec := range features.CatalogdFeatureGate.GetAll() {
		setupLog.Info("feature",
			"name", f,
			"enabled", features.CatalogdFeatureGate.Enabled(f),
			"default", fSpec.Default,
			"lockToDefault", fSpec.LockToDefault,
			"prerelease", fSpec.PreRelease,
		)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "catalogd-operator-lock",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if systemNamespace == "" {
		systemNamespace = podNamespace()
	}

	ctx := ctrl.SetupSignalHandler()

	var imageSource source.Unpacker
	if features.CatalogdFeatureGate.Enabled(features.DirectImageRegistrySource) {
		imageSource, err = directImageSource(ctx, mgr.GetClient(), systemNamespace, cacheDir)
	} else {
		imageSource, err = podImageSource(mgr, systemNamespace, unpackImage)
	}
	if err != nil {
		setupLog.Error(err, "unable to create image source")
		os.Exit(1)
	}
	unpacker := source.NewUnpacker(map[v1alpha1.SourceType]source.Unpacker{
		v1alpha1.SourceTypeImage: imageSource,
	})

	if err := mgr.AddMetricsExtraHandler("/catalogs/", catalogsHandler(filepath.Join(cacheDir, "wwwroot"))); err != nil {
		setupLog.Error(err, "unable to add metrics extra handler")
		os.Exit(1)
	}

	if err = (&corecontrollers.CatalogReconciler{
		Client:   mgr.GetClient(),
		Unpacker: unpacker,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Catalog")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	if profiling {
		pprofer := profile.NewPprofer()
		if err := pprofer.ConfigureControllerManager(mgr); err != nil {
			setupLog.Error(err, "unable to setup pprof configuration")
			os.Exit(1)
		}
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func podNamespace() string {
	namespace, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		return "catalogd-system"
	}
	return string(namespace)
}

func directImageSource(ctx context.Context, cl client.Client, systemNamespace, cacheDir string) (source.Unpacker, error) {
	imageDir := filepath.Join(cacheDir, "images")
	contentRoot := filepath.Join(cacheDir, "content")
	serveRoot := filepath.Join(cacheDir, "wwwroot")
	tmpRoot := filepath.Join(cacheDir, "tmp")

	for _, dir := range []string{imageDir, contentRoot, serveRoot, tmpRoot} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, fmt.Errorf("unable to create directory %q: %w", dir, err)
		}
	}

	imageStore, err := oci.NewWithContext(ctx, imageDir)
	if err != nil {
		return nil, fmt.Errorf("unable to create image store: %w", err)
	}
	return &source.ImageDirect{
		GetSecret: func(ctx context.Context, s string) (*corev1.Secret, error) {
			var secret corev1.Secret
			if err := cl.Get(ctx, types.NamespacedName{Namespace: systemNamespace, Name: s}, &secret); err != nil {
				return nil, err
			}
			return &secret, nil
		},
		ImageCache: imageStore,

		ContentRoot: contentRoot,
		ServeRoot:   serveRoot,
		TmpRoot:     tmpRoot,
	}, nil
}

func podImageSource(mgr manager.Manager, systemNamespace, unpackImage string) (source.Unpacker, error) {
	kubeClient, err := kubernetes.NewForConfig(mgr.GetConfig())
	if err != nil {
		return nil, fmt.Errorf("unable to create kubernetes client: %w", err)
	}
	return &source.Image{
		Client:       mgr.GetClient(),
		KubeClient:   kubeClient,
		PodNamespace: systemNamespace,
		UnpackImage:  unpackImage,
	}, nil
}

func catalogsHandler(wwwRoot string) http.Handler {
	fsHandler := http.FileServer(http.FS(&noDirsFS{os.DirFS(wwwRoot)}))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var etag string
		contentPath, err := os.Readlink(filepath.Join(wwwRoot, r.URL.Path))
		if err == nil {
			etag = strings.TrimSuffix(filepath.Base(contentPath), ".json")
		}
		if etag != "" {
			if r.Header.Get("If-None-Match") == etag {
				w.Header().Set("ETag", etag)
				w.WriteHeader(http.StatusNotModified)
				return
			}
			w = etagWriter{w: w, etag: etag}
		}
		fsHandler.ServeHTTP(w, r)
	})
}

type etagWriter struct {
	w           http.ResponseWriter
	etag        string
	wroteHeader bool
}

func (s etagWriter) WriteHeader(code int) {
	if s.wroteHeader == false {
		s.w.Header().Set("ETag", s.etag)
		s.wroteHeader = true
	}
	s.w.WriteHeader(code)
}

func (s etagWriter) Write(b []byte) (int, error) {
	return s.w.Write(b)
}

func (s etagWriter) Header() http.Header {
	return s.w.Header()
}

type noDirsFS struct {
	fsys fs.FS
}

func (n noDirsFS) Open(name string) (fs.File, error) {
	stat, err := fs.Stat(n.fsys, name)
	if err != nil {
		return nil, err
	}
	if stat.IsDir() {
		return nil, os.ErrNotExist
	}
	return n.fsys.Open(name)
}
