/*
Copyright 2017 The Kubernetes Authors.

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
	"time"

	"github.com/knative/pkg/controller"
	"github.com/knative/pkg/logging"
	"github.com/knative/pkg/signals"
	"k8s.io/client-go/dynamic"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"

	pubsubsourceclientset "github.com/knative/eventing-contrib/contrib/gcppubsub/pkg/client/clientset/versioned"
	pubsubsourceinformers "github.com/knative/eventing-contrib/contrib/gcppubsub/pkg/client/informers/externalversions"
	clientset "github.com/vaikas-google/gcs/pkg/client/clientset/versioned"
	informers "github.com/vaikas-google/gcs/pkg/client/informers/externalversions"

	eventingclientset "github.com/knative/eventing/pkg/client/clientset/versioned"
	eventinginformers "github.com/knative/eventing/pkg/client/informers/externalversions"
	"github.com/vaikas-google/gcs/pkg/reconciler/gcs"
)

const (
	threadsPerController = 2
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

	logger := logging.FromContext(context.TODO()).Named("controller")

	cfg, err := clientcmd.BuildConfigFromFlags(*masterURL, *kubeconfig)
	if err != nil {
		logger.Fatalf("Error building kubeconfig: %s", err.Error())
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		logger.Fatalf("Error building kubernetes clientset: %s", err.Error())
	}

	dynamicClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		logger.Fatalf("Error building dynamic client: %s", err.Error())
	}

	gcsSourceClient, err := clientset.NewForConfig(cfg)
	if err != nil {
		logger.Fatalf("Error building gcsSource clientset: %s", err.Error())
	}

	pubsubsourceClient, err := pubsubsourceclientset.NewForConfig(cfg)
	if err != nil {
		logger.Fatalf("Error building pubsubsource clientset: %s", err.Error())
	}

	eventingClient, err := eventingclientset.NewForConfig(cfg)
	if err != nil {
		logger.Fatalf("Error building eventing clientset: %s", err.Error())
	}

	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeClient, time.Second*30)
	gcsSourceInformerFactory := informers.NewSharedInformerFactory(gcsSourceClient, time.Second*30)
	eventingInformerFactory := eventinginformers.NewSharedInformerFactory(eventingClient, time.Second*30)

	// obtain a reference to a shared index informer for the GCSSource type.
	gcsSourceInformer := gcsSourceInformerFactory.Sources().V1alpha1().GCSSources()

	eventTypeInformer := eventingInformerFactory.Eventing().V1alpha1().EventTypes()

	pubsubsourceInformerFactory := pubsubsourceinformers.NewSharedInformerFactory(pubsubsourceClient, time.Second*30)
	pubsubsourceInformer := pubsubsourceInformerFactory.Sources().V1alpha1().GcpPubSubSources()

	// Add new controllers here.
	controllers := []*controller.Impl{
		gcs.NewController(
			logger,
			kubeClient,
			dynamicClient,
			gcsSourceClient,
			gcsSourceInformer,
			pubsubsourceClient,
			pubsubsourceInformer,
			eventingClient,
			eventTypeInformer,
		),
	}

	go kubeInformerFactory.Start(stopCh)
	go gcsSourceInformerFactory.Start(stopCh)
	go pubsubsourceInformerFactory.Start(stopCh)
	go eventingInformerFactory.Start(stopCh)

	// Wait for the caches to be synced before starting controllers.
	logger.Info("Waiting for informer caches to sync")
	for i, synced := range []cache.InformerSynced{
		gcsSourceInformer.Informer().HasSynced,
		pubsubsourceInformer.Informer().HasSynced,
		eventTypeInformer.Informer().HasSynced,
	} {
		if ok := cache.WaitForCacheSync(stopCh, synced); !ok {
			logger.Fatalf("failed to wait for cache at index %v to sync", i)
		}
	}

	logger.Info("Starting controllers...")
	// Start all of the controllers.
	for _, ctrlr := range controllers {
		go func(ctrlr *controller.Impl) {
			// We don't expect this to return until stop is called,
			// but if it does, propagate it back.
			if err := ctrlr.Run(threadsPerController, stopCh); err != nil {
				logger.Fatalf("Error running controller: %s", err.Error())
			}
		}(ctrlr)
	}

	<-stopCh
}
