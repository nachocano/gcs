/*
Copyright 2018 The Knative Authors

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

package resources

import (
	"github.com/knative/pkg/kmeta"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	pubsubv1alpha1 "github.com/knative/eventing-sources/pkg/apis/sources/v1alpha1"
	"github.com/vaikas-google/gcs/pkg/apis/gcs/v1alpha1"
)

// MakePubSub creates the spec for, but does not create, a GcpPubSource
// (Receive Adapter) for a given GCSSource.
func MakePubSub(source *v1alpha1.GCSSource, topic string) *pubsubv1alpha1.GcpPubSubSource {
	labels := map[string]string{
		"receive-adapter": "gcssource",
	}

	pubsubSecret := v1.SecretKeySelector{
		LocalObjectReference: v1.LocalObjectReference{
			Name: source.Spec.GCSCredsSecret,
		},
		Key: "key.json",
	}

	if source.Spec.GcpCredsSecret != nil {
		pubsubSecret = *source.Spec.GcpCredsSecret
	}

	return &pubsubv1alpha1.GcpPubSubSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      source.Name,
			Namespace: source.Namespace,
			Labels:    labels,
			OwnerReferences: []metav1.OwnerReference{
				*kmeta.NewControllerRef(source),
			},
		},
		Spec: pubsubv1alpha1.GcpPubSubSourceSpec{
			GcpCredsSecret:     pubsubSecret,
			GoogleCloudProject: source.Spec.GoogleCloudProject,
			Topic:              source.Status.Topic,
			Sink:               source.Spec.Sink,
			ServiceAccountName: source.Spec.ServiceAccountName,
		},
	}
}
