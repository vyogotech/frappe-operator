/*
Copyright 2023 Vyogo Technologies.

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
	"os"
	"strconv"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	routev1 "github.com/openshift/api/route/v1"
	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
	"github.com/vyogotech/frappe-operator/controllers"
	//+kubebuilder:scaffold:imports
)

//+kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(vyogotechv1alpha1.AddToScheme(scheme))
	utilruntime.Must(routev1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

const defaultMaxConcurrentSiteReconciles = 10

// effectiveMaxFromBenches returns the effective max concurrent site reconciles from env value and bench list.
// Used by getMaxConcurrentSiteReconciles; exported for testing.
func effectiveMaxFromBenches(fromEnv int, items []vyogotechv1alpha1.FrappeBench) int {
	benchMax := 0
	for i := range items {
		if items[i].Spec.SiteReconcileConcurrency != nil && *items[i].Spec.SiteReconcileConcurrency > 0 {
			if int(*items[i].Spec.SiteReconcileConcurrency) > benchMax {
				benchMax = int(*items[i].Spec.SiteReconcileConcurrency)
			}
		}
	}
	if benchMax > fromEnv {
		fromEnv = benchMax
	}
	if fromEnv < 1 {
		fromEnv = 1
	}
	return fromEnv
}

// getMaxConcurrentSiteReconciles returns the effective max concurrent site reconciles:
// max(operatorConfig from env FRAPPE_MAX_CONCURRENT_SITE_RECONCILES, max(spec.siteReconcileConcurrency across benches)).
// Operator config is from frappe-operator-config ConfigMap (e.g. maxConcurrentSiteReconciles), passed via env when using Helm.
func getMaxConcurrentSiteReconciles(mgr ctrl.Manager) int {
	fromEnv := defaultMaxConcurrentSiteReconciles
	if s := os.Getenv("FRAPPE_MAX_CONCURRENT_SITE_RECONCILES"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			fromEnv = n
		}
	}
	var items []vyogotechv1alpha1.FrappeBench
	cl, err := client.New(mgr.GetConfig(), client.Options{Scheme: mgr.GetScheme()})
	if err == nil {
		var list vyogotechv1alpha1.FrappeBenchList
		// Omit InNamespace to list FrappeBenches across all namespaces (bench-level override).
		if err := cl.List(context.Background(), &list); err == nil {
			items = list.Items
		}
	}
	return effectiveMaxFromBenches(fromEnv, items)
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: metricsAddr},
		WebhookServer:          webhook.NewServer(webhook.Options{Port: 9443}),
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "bd4753fa.vyogo.tech",
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Detect OpenShift
	isOpenShift := controllers.IsRouteAPIAvailable(mgr.GetConfig())
	if isOpenShift {
		setupLog.Info("OpenShift platform detected")
	} else {
		setupLog.Info("Standard Kubernetes platform detected")
	}

	if err = (&controllers.FrappeBenchReconciler{
		Client:      mgr.GetClient(),
		Scheme:      mgr.GetScheme(),
		Recorder:    mgr.GetEventRecorderFor("frappebench-controller"),
		IsOpenShift: isOpenShift,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "FrappeBench")
		os.Exit(1)
	}
	maxSiteReconciles := getMaxConcurrentSiteReconciles(mgr)
	setupLog.Info("FrappeSite controller concurrency", "maxConcurrentReconciles", maxSiteReconciles)
	if err = (&controllers.FrappeSiteReconciler{
		Client:                  mgr.GetClient(),
		Scheme:                  mgr.GetScheme(),
		Recorder:                mgr.GetEventRecorderFor("frappesite-controller"),
		IsOpenShift:             isOpenShift,
		MaxConcurrentReconciles: maxSiteReconciles,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "FrappeSite")
		os.Exit(1)
	}
	if err = (&controllers.SiteUserReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("siteuser-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "SiteUser")
		os.Exit(1)
	}
	if err = (&controllers.FrappeWorkpaceReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("frappeworkpace-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "FrappeWorkpace")
		os.Exit(1)
	}
	if err = (&controllers.SiteWorkspaceReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("siteworkspace-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "SiteWorkspace")
		os.Exit(1)
	}
	if err = (&controllers.SiteDashboardChartReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("sitedashboardchart-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "SiteDashboardChart")
		os.Exit(1)
	}
	if err = (&controllers.SiteDashboardReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("sitedashboard-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "SiteDashboard")
		os.Exit(1)
	}
	if err = (&controllers.SiteJobReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("sitejob-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "SiteJob")
		os.Exit(1)
	}
	if err = (&controllers.SiteBackupReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("sitebackup-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "SiteBackup")
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

	setupLog.Info("starting manager", "version", "v2.6.3")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
