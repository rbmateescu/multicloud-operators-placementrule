// Copyright 2019 The Kubernetes Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package utils

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	clusterv1alpha1 "k8s.io/cluster-registry/pkg/apis/clusterregistry/v1alpha1"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appv1alpha1 "github.com/open-cluster-management/multicloud-operators-placementrule/pkg/apis/apps/v1"
)

func ToPlaceLocal(placement *appv1alpha1.Placement) bool {
	if placement == nil || placement.Local == nil {
		return false
	}

	return *placement.Local
}

// PlaceByGenericPlacmentFields search with basic placement criteria
// Top priority: clusterNames, ignore selector
// Bottomline: Use label selector
func PlaceByGenericPlacmentFields(kubeclient client.Client, placement appv1alpha1.GenericPlacementFields,
	authclient kubernetes.Interface, object runtime.Object) (map[string]*clusterv1alpha1.Cluster, error) {
	clmap := make(map[string]*clusterv1alpha1.Cluster)

	var labelSelector *metav1.LabelSelector

	// MCM Assumption: clusters are always labeled with name
	if len(placement.Clusters) != 0 {
		namereq := metav1.LabelSelectorRequirement{}
		namereq.Key = "name"
		namereq.Operator = metav1.LabelSelectorOpIn

		for _, cl := range placement.Clusters {
			namereq.Values = append(namereq.Values, cl.Name)
		}

		labelSelector = &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{namereq},
		}
	} else {
		labelSelector = placement.ClusterSelector
	}

	clSelector, err := ConvertLabels(labelSelector)

	if err != nil {
		return nil, err
	}

	klog.V(10).Info("Using Cluster LabelSelector ", clSelector)

	cllist := &clusterv1alpha1.ClusterList{}

	err = kubeclient.List(context.TODO(), cllist, &client.ListOptions{LabelSelector: clSelector})

	if err != nil && !errors.IsNotFound(err) {
		klog.Error("Listing clusters and found error: ", err)
		return nil, err
	}

	klog.V(10).Info("listed clusters:", cllist.Items)

	for _, cl := range cllist.Items {
		clmap[cl.Name] = cl.DeepCopy()
	}

	return clmap, nil
}

func InstanceDeepCopy(a, b interface{}) error {
	byt, err := json.Marshal(a)

	if err == nil {
		err = json.Unmarshal(byt, b)
	}

	return err
}

// IsReadyACMClusterRegistry check if ACM Cluster API service is ready or not.
func IsReadyACMClusterRegistry(clReader client.Reader) bool {
	cllist := &clusterv1alpha1.ClusterList{}

	listopts := &client.ListOptions{}

	err := clReader.List(context.TODO(), cllist, listopts)

	if err == nil {
		klog.Error("ACM Cluster API service ready")
		return true
	}

	klog.Error("ACM Cluster API service NOT ready: ", err)

	return false
}

// DetectClusterRegistry - Detect the ACM cluster API service every 10 seconds. the controller will be exited when it is ready
// The controller will be auto restarted by the multicluster-operators-application deployment CR later.
func DetectClusterRegistry(clReader client.Reader, s <-chan struct{}) {
	if !IsReadyACMClusterRegistry(clReader) {
		go wait.Until(func() {
			if IsReadyACMClusterRegistry(clReader) {
				os.Exit(1)
			}
		}, time.Duration(10)*time.Second, s)
	}
}
