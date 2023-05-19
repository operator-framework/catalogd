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
	"flag"
	"fmt"
	"net/url"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/operator-framework/catalogd/internal/version"
	"github.com/operator-framework/catalogd/pkg/apis/core/v1beta1"
	corecontrollers "github.com/operator-framework/catalogd/pkg/controllers/core"
	"github.com/operator-framework/catalogd/pkg/profile"
	"github.com/operator-framework/catalogd/pkg/provisioner"
	"github.com/operator-framework/rukpak/api/v1alpha1"
	"github.com/operator-framework/rukpak/pkg/finalizer"
	"github.com/operator-framework/rukpak/pkg/provisioner/bundle"
	"github.com/operator-framework/rukpak/pkg/source"
	"github.com/operator-framework/rukpak/pkg/storage"
	crfinalizer "sigs.k8s.io/controller-runtime/pkg/finalizer"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(v1beta1.AddToScheme(scheme))
	utilruntime.Must(v1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var (
		metricsAddr          string
		enableLeaderElection bool
		probeAddr            string
		opmImage             string
		profiling            bool
		catalogdVersion      bool
	)
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&opmImage, "opm-image", "quay.io/operator-framework/opm:v1.26", "The opm image to use when unpacking catalog images")
	opts := zap.Options{
		Development: true,
	}
	flag.BoolVar(&profiling, "profiling", false, "enable profiling endpoints to allow for using pprof")
	flag.BoolVar(&catalogdVersion, "version", false, "print the catalogd version and exit")
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	if catalogdVersion {
		fmt.Printf("catalogd version: %s", version.ControllerVersion())
		os.Exit(0)
	}

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

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

	if err = (&corecontrollers.CatalogReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Cfg:      mgr.GetConfig(),
		OpmImage: opmImage,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Catalog")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	// TODO: Investigate a storage issue:
	// 	2023-05-19T21:08:41Z	ERROR	Reconciler error	{"controller": "controller.bundle.catalogd-bundle-provisioner", "controllerGroup": "core.rukpak.io", "controllerKind": "Bundle", "Bundle": {"name":"catalog-sample-bundle"}, "namespace": "", "name": "catalog-sample-bundle", "reconcileID": "421a40cf-86e2-4df4-a677-4ed0ff8da111", "error": "persist bundle content: open /var/cache/bundle/catalog-sample-bundle.tgz: no such file or directory"}
	// sigs.k8s.io/controller-runtime/pkg/internal/controller.(*Controller).reconcileHandler
	// 	/home/bpalmer/github/catalogd/vendor/sigs.k8s.io/controller-runtime/pkg/internal/controller/controller.go:329
	// sigs.k8s.io/controller-runtime/pkg/internal/controller.(*Controller).processNextWorkItem
	// 	/home/bpalmer/github/catalogd/vendor/sigs.k8s.io/controller-runtime/pkg/internal/controller/controller.go:274
	// sigs.k8s.io/controller-runtime/pkg/internal/controller.(*Controller).Start.func2.2
	// 	/home/bpalmer/github/catalogd/vendor/sigs.k8s.io/controller-runtime/pkg/internal/controller/controller.go:235

	storageURL, err := url.Parse(fmt.Sprintf("%s/bundles/", "http://localhost:8080"))
	if err != nil {
		setupLog.Error(err, "unable to parse bundle content server URL")
		os.Exit(1)
	}

	localStorage := &storage.LocalDirectory{
		RootDirectory: "/var/cache/bundle",
		URL:           *storageURL,
	}

	kubeClient, err := kubernetes.NewForConfig(mgr.GetConfig())
	if err != nil {
		setupLog.Error(err, "unable to create a kube client")
		os.Exit(1)
	}
	unpacker := source.NewUnpacker(map[v1alpha1.SourceType]source.Unpacker{
		v1alpha1.SourceTypeImage: &source.Image{
			Client:       mgr.GetClient(),
			KubeClient:   kubeClient,
			PodNamespace: "catalogd-system",
			UnpackImage:  "quay.io/operator-framework/rukpak:v0.12.0",
			BundleDir:    "/configs",
		},
	})
	if err != nil {
		setupLog.Error(err, "unable to set up unpacker")
		os.Exit(1)
	}

	httpLoader := storage.NewHTTP(
		storage.WithBearerToken(mgr.GetConfig().BearerToken),
	)
	bundleStorage := storage.WithFallbackLoader(localStorage, httpLoader)

	// This finalizer logic MUST be co-located with this main
	// controller logic because it deals with cleaning up bundle data
	// from the bundle cache when the bundles are deleted. The
	// consequence is that this process MUST remain running in order
	// to process DELETE events for bundles that include this finalizer.
	// If this process is NOT running, deletion of such bundles will
	// hang until $something removes the finalizer.
	//
	// If the bundle cache is backed by a storage implementation that allows
	// multiple writers from different processes (e.g. a ReadWriteMany volume or
	// an S3 bucket), we could have separate processes for finalizer handling
	// and the primary provisioner controllers. For now, the assumption is
	// that we are not using such an implementation.
	bundleFinalizers := crfinalizer.NewFinalizers()
	if err := bundleFinalizers.Register(finalizer.DeleteCachedBundleKey, &finalizer.DeleteCachedBundle{Storage: bundleStorage}); err != nil {
		setupLog.Error(err, "unable to register finalizer", "finalizerKey", finalizer.DeleteCachedBundleKey)
		os.Exit(1)
	}
	// Setup our custom rukpak provisioner
	if err = bundle.SetupProvisioner(mgr, mgr.GetCache(), "catalogd-system",
		bundle.WithProvisionerID("catalogd-bundle-provisioner"),
		bundle.WithUnpacker(unpacker),
		bundle.WithStorage(bundleStorage),
		bundle.WithHandler(&provisioner.CatalogdBundleHandler{Client: mgr.GetClient()}),
		bundle.WithFinalizers(bundleFinalizers),
	); err != nil {
		setupLog.Error(err, "unable to set up catalogd bundle provisioner")
		os.Exit(1)
	}

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
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
