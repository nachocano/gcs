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
	"context"
	"flag"
	"github.com/knative/pkg/logging"
	"github.com/knative/pkg/signals"
	"github.com/knative/pkg/system"
	"github.com/knative/pkg/webhook"
	"github.com/vaikas-google/gcs/pkg/apis/gcs/v1alpha1"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	masterURL  = flag.String("kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	kubeconfig = flag.String("master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	// TODO(mattmoor): Move into a configmap and use the watcher.
	raImage = flag.String("raimage", "", "The name of the Receive Adapter image, see //cmd/receivedapter")
)

func main() {
	flag.Parse()

	// set up signals so we handle the first shutdown signal gracefully
	stopCh := signals.SetupSignalHandler()

	logger := logging.FromContext(context.TODO()).Named("webhook")

	cfg, err := clientcmd.BuildConfigFromFlags(*masterURL, *kubeconfig)
	if err != nil {
		logger.Fatalf("Error building kubeconfig: %s", err.Error())
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		logger.Fatalf("Error building kubernetes clientset: %s", err.Error())
	}

	options := webhook.ControllerOptions{
		ServiceName:    "gcssource-webhook",
		DeploymentName: "gcssource-webhook",
		Namespace:      system.Namespace(),
		Port:           8443,
		SecretName:     "gcssource-webhook-certs",
		WebhookName:    "webhook.sources.aikas.org",
	}
	controller := webhook.AdmissionController{
		Client:  kubeClient,
		Options: options,
		Handlers: map[schema.GroupVersionKind]webhook.GenericCRD{
			v1alpha1.SchemeGroupVersion.WithKind("GCSSource"): &v1alpha1.GCSSource{},
		},
		Logger: logger,
	}
	if err != nil {
		logger.Fatalw("Failed to create the Kafka admission controller", zap.Error(err))
	}
	if err = controller.Run(stopCh); err != nil {
		logger.Errorw("controller.Run() failed", zap.Error(err))
	}

	<-stopCh
}
