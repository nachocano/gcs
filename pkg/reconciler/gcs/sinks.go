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

package gcs

import (
	"fmt"

	duckapis "github.com/knative/pkg/apis"
	"github.com/knative/pkg/apis/duck"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
)

// v1beta1AddressableType is copied from knative/pkg. Hack used in the demo.
// Couldn't make dependencies work to get the real one.
type v1beta1AddressableType struct {
	Status struct {
		Address *struct {
			URL *string `json:"url,omitempty"`
		}
	}
}

// GetSinkURI retrieves the sink URI from the object referenced by the given
// ObjectReference.
func GetSinkURI(dc dynamic.Interface, sink *corev1.ObjectReference, namespace string) (string, error) {
	if sink == nil {
		return "", fmt.Errorf("sink ref is nil")
	}

	obj, err := fetchObjectReference(dc, sink, namespace)
	if err != nil {
		return "", err
	}
	t := v1beta1AddressableType{}
	err = duck.FromUnstructured(obj, &t)
	if err != nil {
		return "", fmt.Errorf("failed to deserialize sink: %v", err)
	}

	if t.Status.Address == nil {
		return "", fmt.Errorf("sink does not contain address")
	}

	if t.Status.Address.URL == nil || *t.Status.Address.URL == "" {
		return "", fmt.Errorf("sink contains an empty URL")
	}

	return *t.Status.Address.URL, nil
}

func fetchObjectReference(dc dynamic.Interface, ref *corev1.ObjectReference, namespace string) (duck.Marshalable, error) {
	resourceClient, err := createResourceInterface(dc, ref, namespace)
	if err != nil {
		return nil, err
	}

	return resourceClient.Get(ref.Name, metav1.GetOptions{})
}

func createResourceInterface(dc dynamic.Interface, ref *corev1.ObjectReference, namespace string) (dynamic.ResourceInterface, error) {
	rc := dc.Resource(duckapis.KindToResource(ref.GroupVersionKind()))
	if rc == nil {
		return nil, fmt.Errorf("failed to create dynamic client resource")
	}
	return rc.Namespace(namespace), nil
}
