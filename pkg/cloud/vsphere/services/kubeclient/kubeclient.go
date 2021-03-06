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

package kubeclient

import (
	"context"

	"github.com/pkg/errors"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	remotev1 "sigs.k8s.io/cluster-api/pkg/controller/remote"
	controllerClient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Context is used to get a Kubernetes client for a target cluster.
type Context interface {
	context.Context
	GetCluster() *clusterv1.Cluster
	GetControllerClient() controllerClient.Client
}

// New returns a new client for the target cluster using the KubeConfig
// secret stored in the management cluster.
func New(ctx Context) (corev1.CoreV1Interface, error) {
	client, err := remotev1.NewClusterClient(ctx.GetControllerClient(), ctx.GetCluster())
	if err != nil {
		return nil, errors.Wrap(err, "unable to get client for target cluster")
	}
	coreClient, err := client.CoreV1()
	if err != nil {
		return nil, errors.Wrap(err, "unable to get core client for target cluster")
	}
	return coreClient, nil
}
