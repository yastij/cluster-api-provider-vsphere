/*
Copyright 2019 The Kubernetes Authors.

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
	"net/http"
	"net/http/pprof"
	"os"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/klog"
	clusterapis "sigs.k8s.io/cluster-api/pkg/apis"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/common"
	"sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset"
	capicluster "sigs.k8s.io/cluster-api/pkg/controller/cluster"
	capimachine "sigs.k8s.io/cluster-api/pkg/controller/machine"
	configv1 "sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"

	"sigs.k8s.io/cluster-api-provider-vsphere/pkg/apis"
	"sigs.k8s.io/cluster-api-provider-vsphere/pkg/cloud/vsphere/actuators/cluster"
	"sigs.k8s.io/cluster-api-provider-vsphere/pkg/cloud/vsphere/actuators/machine"
	"sigs.k8s.io/cluster-api-provider-vsphere/pkg/cloud/vsphere/config"
	"sigs.k8s.io/cluster-api-provider-vsphere/pkg/deployer"
	"sigs.k8s.io/cluster-api-provider-vsphere/pkg/record"
)

const (
	profilerAddrEnvVar  = "PROFILER_ADDR"
	syncPeriodEnvVar    = "SYNC_PERIOD"
	requeuePeriodEnvVar = "REQUEUE_PERIOD"
)

var (
	defaultProfilerAddr  = os.Getenv(profilerAddrEnvVar)
	defaultSyncPeriod    = 10 * time.Minute
	defaultRequeuePeriod = 20 * time.Second
)

func init() {
	if v, err := time.ParseDuration(os.Getenv(syncPeriodEnvVar)); err == nil {
		defaultSyncPeriod = v
	}
	if v, err := time.ParseDuration(os.Getenv(requeuePeriodEnvVar)); err == nil {
		defaultRequeuePeriod = v
	}
}

func main() {
	klog.InitFlags(nil)
	flag.Set("logtostderr", "true")
	watchNamespace := flag.String("namespace", "",
		"Namespace that the controller watches to reconcile cluster-api objects. If unspecified, the controller watches for cluster-api objects across all namespaces.")
	profilerAddr := flag.String("profiler-address", defaultProfilerAddr,
		"The address to expose the pprof profiler")
	syncPeriod := flag.Duration("sync-period", defaultSyncPeriod,
		"The interval at which cluster-api objects are synchronized")
	flag.DurationVar(&config.DefaultRequeue, "requeue-period", defaultRequeuePeriod,
		"The default amount of time to wait before an operation is requeued.")

	flag.Parse()

	if addr := *profilerAddr; addr != "" {
		klog.Infof("Starting pprof at %s", *profilerAddr)
		go runProfiler(addr)
	}

	cfg := configv1.GetConfigOrDie()

	// Setup a Manager
	opts := manager.Options{
		SyncPeriod: syncPeriod,
	}
	klog.Infof("Cluster-api objects are synchronized every %s", *syncPeriod)
	klog.Infof("The default requeue period is %s", config.DefaultRequeue)

	if *watchNamespace != "" {
		opts.Namespace = *watchNamespace
		klog.Infof("Watching cluster-api objects only in namespace %q for reconciliation.", opts.Namespace)
	}

	mgr, err := manager.New(cfg, opts)
	if err != nil {
		klog.Fatalf("Failed to set up overall controller manager: %v", err)
	}

	cs, err := clientset.NewForConfig(cfg)
	if err != nil {
		klog.Fatalf("Failed to create client from configuration: %v", err)
	}

	coreClient, err := corev1.NewForConfig(cfg)
	if err != nil {
		klog.Fatalf("Failed to create corev1 client from configuration: %v", err)
	}

	// Initialize event recorder.
	record.InitFromRecorder(mgr.GetRecorder("vsphere-controller"))

	// Register the deployer.
	common.RegisterClusterProvisioner("vsphere", deployer.Deployer{})

	// Initialize cluster actuator.
	clusterActuator := cluster.NewActuator(cs.ClusterV1alpha1(), coreClient, mgr.GetClient())

	// Initialize machine actuator.
	machineActuator := machine.NewActuator(cs.ClusterV1alpha1(), coreClient, mgr.GetClient())

	if err := apis.AddToScheme(mgr.GetScheme()); err != nil {
		klog.Fatal(err)
	}

	if err := clusterapis.AddToScheme(mgr.GetScheme()); err != nil {
		klog.Fatal(err)
	}

	capimachine.AddWithActuator(mgr, machineActuator)
	capicluster.AddWithActuator(mgr, clusterActuator)

	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		klog.Fatalf("Failed to run manager: %v", err)
	}
}

func runProfiler(addr string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	if err := http.ListenAndServe(addr, mux); err != nil {
		klog.Error(errors.Wrap(err, "error running pprof"))
	}
}
