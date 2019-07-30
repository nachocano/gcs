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

package gcs

import (
	"context"
	"fmt"
	"k8s.io/apimachinery/pkg/labels"
	"reflect"

	"github.com/google/uuid"
	"github.com/knative/pkg/controller"
	"github.com/knative/pkg/logging/logkey"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/cache"

	"cloud.google.com/go/pubsub"
	"cloud.google.com/go/storage"

	pubsubsourcev1alpha1 "github.com/knative/eventing-contrib/contrib/gcppubsub/pkg/apis/sources/v1alpha1"
	pubsubsourceclientset "github.com/knative/eventing-contrib/contrib/gcppubsub/pkg/client/clientset/versioned"
	pubsubsourceinformers "github.com/knative/eventing-contrib/contrib/gcppubsub/pkg/client/informers/externalversions/sources/v1alpha1"
	eventingv1alpha1 "github.com/knative/eventing/pkg/apis/eventing/v1alpha1"
	eventingclientset "github.com/knative/eventing/pkg/client/clientset/versioned"
	eventinginformers "github.com/knative/eventing/pkg/client/informers/externalversions/eventing/v1alpha1"
	eventinglisters "github.com/knative/eventing/pkg/client/listers/eventing/v1alpha1"
	"github.com/vaikas-google/gcs/pkg/apis/gcs/v1alpha1"
	clientset "github.com/vaikas-google/gcs/pkg/client/clientset/versioned"
	gcssourcescheme "github.com/vaikas-google/gcs/pkg/client/clientset/versioned/scheme"
	informers "github.com/vaikas-google/gcs/pkg/client/informers/externalversions/gcs/v1alpha1"
	listers "github.com/vaikas-google/gcs/pkg/client/listers/gcs/v1alpha1"
	"github.com/vaikas-google/gcs/pkg/reconciler/gcs/resources"
	"google.golang.org/grpc/codes"
	gstatus "google.golang.org/grpc/status"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/dynamic"
)

const (
	controllerAgentName = "gcs-controller"
	finalizerName       = controllerAgentName
)

// Reconciler is the controller implementation for Gcssource resources
type Reconciler struct {
	// kubeclientset is a standard kubernetes clientset
	kubeclientset kubernetes.Interface
	// gcssourceclientset is a clientset for our own API group
	gcssourceclientset clientset.Interface
	gcssourcesLister   listers.GCSSourceLister

	// We use dynamic client for Duck type related stuff.
	dynamicClient dynamic.Interface

	// For dealing with
	pubsubClient   pubsubsourceclientset.Interface
	pubsubInformer pubsubsourceinformers.GcpPubSubSourceInformer

	eventtypeLister    eventinglisters.EventTypeLister
	eventtypeclientset eventingclientset.Interface
	eventtypeInformer  eventinginformers.EventTypeInformer

	// Sugared logger is easier to use but is not as performant as the
	// raw logger. In performance critical paths, call logger.Desugar()
	// and use the returned raw logger instead. In addition to the
	// performance benefits, raw logger also preserves type-safety at
	// the expense of slightly greater verbosity.
	Logger *zap.SugaredLogger
}

// Check that we implement the controller.Reconciler interface.
var _ controller.Reconciler = (*Reconciler)(nil)

func init() {
	// Add gcssource-controller types to the default Kubernetes Scheme so Events can be
	// logged for gcssource-controller types.
	gcssourcescheme.AddToScheme(scheme.Scheme)
}

// NewController returns a new gcssource controller
func NewController(
	logger *zap.SugaredLogger,
	kubeclientset kubernetes.Interface,
	dynamicClient dynamic.Interface,
	gcssourceclientset clientset.Interface,
	gcssourceInformer informers.GCSSourceInformer,
	pubsubclientset pubsubsourceclientset.Interface,
	pubsubsourceInformer pubsubsourceinformers.GcpPubSubSourceInformer,
	eventingclientset eventingclientset.Interface,
	eventtypeInformer eventinginformers.EventTypeInformer,

) *controller.Impl {

	// Enrich the logs with controller name
	logger = logger.Named(controllerAgentName).With(zap.String(logkey.ControllerType, controllerAgentName))

	r := &Reconciler{
		kubeclientset:      kubeclientset,
		dynamicClient:      dynamicClient,
		gcssourceclientset: gcssourceclientset,
		gcssourcesLister:   gcssourceInformer.Lister(),
		pubsubClient:       pubsubclientset,
		eventtypeclientset: eventingclientset,
		eventtypeLister:    eventtypeInformer.Lister(),
		Logger:             logger,
	}
	impl := controller.NewImpl(r, logger, "GCSSources")

	logger.Info("Setting up event handlers")

	// Set up an event handler for when GCSSource resources change
	gcssourceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    impl.Enqueue,
		UpdateFunc: controller.PassNew(impl.Enqueue),
	})

	// Set up an event handler for when EventType resources change
	eventtypeInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    impl.Enqueue,
		UpdateFunc: controller.PassNew(impl.Enqueue),
	})

	// Set up an event handler for when GCSSource owned Service resources change.
	// Basically whenever a Service controlled by us is chaned, we want to know about it.
	pubsubsourceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    impl.EnqueueControllerOf,
		UpdateFunc: controller.PassNew(impl.EnqueueControllerOf),
		DeleteFunc: impl.EnqueueControllerOf,
	})

	return impl
}

// Reconcile implements controller.Reconciler
func (c *Reconciler) Reconcile(ctx context.Context, key string) error {
	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		runtime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}

	// Get the GCSSource resource with this namespace/name
	original, err := c.gcssourcesLister.GCSSources(namespace).Get(name)
	if errors.IsNotFound(err) {
		// The GCSSource resource may no longer exist, in which case we stop processing.
		runtime.HandleError(fmt.Errorf("gcssource '%s' in work queue no longer exists", key))
		return nil
	} else if err != nil {
		return err
	}

	// Don't modify the informers copy
	csr := original.DeepCopy()

	err = c.reconcileGCSSource(ctx, csr)

	if equality.Semantic.DeepEqual(original.Status, csr.Status) &&
		equality.Semantic.DeepEqual(original.ObjectMeta, csr.ObjectMeta) {
		// If we didn't change anything (status or finalizers) then don't
		// call update.
		// This is important because the copy we loaded from the informer's
		// cache may be stale and we don't want to overwrite a prior update
		// to status with this stale state.
	} else if _, err := c.update(csr); err != nil {
		c.Logger.Warn("Failed to update GCS Source status", zap.Error(err))
		return err
	}
	return err
}

func (c *Reconciler) reconcileGCSSource(ctx context.Context, csr *v1alpha1.GCSSource) error {
	// See if the source has been deleted.
	deletionTimestamp := csr.DeletionTimestamp

	// First try to resolve the sink, and if not found mark as not resolved.
	uri, err := GetSinkURI(c.dynamicClient, csr.Spec.Sink, csr.Namespace)
	if err != nil {
		// TODO: Update status appropriately
		//		csr.Status.MarkNoSink("NotFound", "%s", err)
		c.Logger.Infof("Couldn't resolve Sink URI: %s", err)
		if deletionTimestamp == nil {
			return err
		}
		// we don't care about the URI if we're deleting, so carry on...
		uri = ""
	}
	c.Logger.Infof("Resolved Sink URI to %q", uri)

	if deletionTimestamp != nil {
		err := c.deleteNotification(csr)
		if err != nil {
			c.Logger.Infof("Unable to delete the Notification: %s", err)
			return err
		}
		err = c.deleteTopic(csr.Spec.GoogleCloudProject, csr.Status.Topic)
		if err != nil {
			c.Logger.Infof("Unable to delete the Topic: %s", err)
			return err
		}
		csr.Status.Topic = ""
		c.removeFinalizer(csr)
		return nil
	}

	csr.Status.InitializeConditions()

	err = c.reconcileTopic(csr)
	if err != nil {
		c.Logger.Infof("Failed to reconcile topic %s", err)
		csr.Status.MarkPubSubTopicNotReady(fmt.Sprintf("Failed to create GCP PubSub Topic: %s", err), "")
		return err
	}

	csr.Status.MarkPubSubTopicReady()

	c.addFinalizer(csr)

	csr.Status.SinkURI = uri

	// Make sure PubSubSource is in the state we expect it to be in.
	pubsub, err := c.reconcilePubSub(csr)
	if err != nil {
		// TODO: Update status appropriately
		c.Logger.Infof("Failed to reconcile GCP PubSub Source: %s", err)
		csr.Status.MarkPubSubSourceNotReady(fmt.Sprintf("Failed to create GCP PubSub Source: %s", err), "")
		return err
	}
	c.Logger.Infof("Reconciled pubsub source: %+v", pubsub)
	c.Logger.Infof("using %q as a cluster internal sink", pubsub.Status.SinkURI)

	// Check to see if pubsub source is ready
	if !pubsub.Status.IsReady() {
		c.Logger.Infof("GCP PubSub Source is not ready yet")
		csr.Status.MarkPubSubSourceNotReady("underlying GCP PubSub Source is not ready", "")
	} else {
		csr.Status.MarkPubSubSourceReady()
	}

	notification, err := c.reconcileNotification(csr)
	if err != nil {
		// TODO: Update status with this...
		c.Logger.Infof("Failed to reconcile GCS Notification: %s", err)
		csr.Status.MarkGCSNotReady(fmt.Sprintf("Failed to create GCS notification: %s", err), "")
		return err
	}

	csr.Status.MarkGCSReady()
	c.Logger.Infof("Reconciled GCS notification: %+v", notification)
	csr.Status.NotificationID = notification.ID

	err = c.reconcileEventTypes(ctx, csr)
	if err != nil {
		c.Logger.Infof("Failed to reconcile GCS EventTypes: %s", err)
		csr.Status.MarkEventTypesNotProvided(fmt.Sprintf("Failed to reconcile GCS EventTypes: %s", err), "")
	}

	csr.Status.MarkEventTypesProvided()

	return nil
}

func (c *Reconciler) reconcilePubSub(csr *v1alpha1.GCSSource) (*pubsubsourcev1alpha1.GcpPubSubSource, error) {
	pubsubClient := c.pubsubClient.SourcesV1alpha1().GcpPubSubSources(csr.Namespace)
	existing, err := pubsubClient.Get(csr.Name, metav1.GetOptions{})
	if err == nil {
		// TODO: Handle any updates...
		c.Logger.Infof("Found existing pubsubsource: %+v", existing)
		return existing, nil
	}
	if errors.IsNotFound(err) {
		pubsub := resources.MakePubSub(csr, "testing")
		c.Logger.Infof("Creating service %+v", pubsub)
		return pubsubClient.Create(pubsub)
	}
	return nil, err
}

func (c *Reconciler) reconcileNotification(gcs *v1alpha1.GCSSource) (*storage.Notification, error) {
	ctx := context.Background()
	gcsClient, err := storage.NewClient(ctx)
	if err != nil {
		c.Logger.Infof("Failed to create storage client: %s", err)
		return nil, err
	}

	bucket := gcsClient.Bucket(gcs.Spec.Bucket)

	notifications, err := bucket.Notifications(ctx)
	if err != nil {
		c.Logger.Infof("Failed to fetch existing notifications: %s", err)
		return nil, err
	}

	if gcs.Status.NotificationID != "" {
		if existing, ok := notifications[gcs.Status.NotificationID]; ok {
			c.Logger.Infof("Found existing notification: %+v", existing)
			return existing, nil
		}
	}

	customAttributes := make(map[string]string)
	for k, v := range gcs.Spec.CustomAttributes {
		customAttributes[k] = v
	}

	// Add our own event type here...
	customAttributes["ce-type"] = "google.gcs"

	c.Logger.Infof("Creating a notification on bucket %s", gcs.Spec.Bucket)
	notification, err := bucket.AddNotification(ctx, &storage.Notification{
		TopicProjectID:   gcs.Spec.GoogleCloudProject,
		TopicID:          gcs.Status.Topic,
		PayloadFormat:    storage.JSONPayload,
		EventTypes:       c.getEventTypesAsString(gcs.Spec.EventTypes),
		ObjectNamePrefix: gcs.Spec.ObjectNamePrefix,
		CustomAttributes: customAttributes,
	})

	if err != nil {
		c.Logger.Infof("Failed to create Notification: %s", err)
		return nil, err
	}
	c.Logger.Infof("Created Notification %q", notification.ID)

	return notification, nil
}

func (c *Reconciler) getEventTypesAsString(gcsTypes *v1alpha1.GCSEventTypes) []string {
	eventTypes := make([]string, 0)
	if gcsTypes.Finalize != nil {
		eventTypes = append(eventTypes, v1alpha1.GCSEventTypesMapping[gcsTypes.Finalize.Type])
	}
	if gcsTypes.Archive != nil {
		eventTypes = append(eventTypes, v1alpha1.GCSEventTypesMapping[gcsTypes.Archive.Type])
	}
	if gcsTypes.Delete != nil {
		eventTypes = append(eventTypes, v1alpha1.GCSEventTypesMapping[gcsTypes.Delete.Type])
	}
	if gcsTypes.MetadataUpdate != nil {
		eventTypes = append(eventTypes, v1alpha1.GCSEventTypesMapping[gcsTypes.MetadataUpdate.Type])
	}
	return eventTypes
}

func (c *Reconciler) reconcileTopic(csr *v1alpha1.GCSSource) error {
	if csr.Status.Topic == "" {
		c.Logger.Infof("No topic found in status, creating a unique one")
		// Create a UUID for the topic. prefix with gcs- to make it conformant.
		csr.Status.Topic = fmt.Sprintf("gcs-%s", uuid.New().String())
	}

	ctx := context.Background()
	psc, err := pubsub.NewClient(ctx, csr.Spec.GoogleCloudProject)
	if err != nil {
		return err
	}
	topic := psc.Topic(csr.Status.Topic)
	exists, err := topic.Exists(ctx)
	if err != nil {
		c.Logger.Infof("Failed to check for topic %q existence : %s", csr.Status.Topic, err)
		return err
	}
	if exists {
		c.Logger.Infof("Topic %q exists already", csr.Status.Topic)
		return nil
	}

	c.Logger.Infof("Creating topic %q", csr.Status.Topic)
	newTopic, err := psc.CreateTopic(ctx, csr.Status.Topic)
	if err != nil {
		c.Logger.Infof("Failed to create topic %q : %s", csr.Status.Topic, err)
		return err
	}
	c.Logger.Infof("Created topic %q : %+v", csr.Status.Topic, newTopic)
	return nil
}

func (r *Reconciler) reconcileEventTypes(ctx context.Context, src *v1alpha1.GCSSource) error {
	current, err := r.getEventTypes(ctx, src)
	if err != nil {
		r.Logger.Errorf("Unable to get existing event types: %v", err)
		return err
	}

	expected, err := r.makeEventTypes(src)
	if err != nil {
		return err
	}

	toCreate, toDelete := r.computeDiff(current, expected)

	for _, eventType := range toDelete {
		if err = r.eventtypeclientset.EventingV1alpha1().EventTypes(src.Namespace).Delete(eventType.Name, &metav1.DeleteOptions{}); err != nil {
			r.Logger.Errorf("Error deleting eventType: %v", eventType)
			return err
		}
	}

	for _, eventType := range toCreate {
		if _, err = r.eventtypeclientset.EventingV1alpha1().EventTypes(src.Namespace).Create(&eventType); err != nil {
			r.Logger.Errorf("Error creating eventType: %v", eventType)
			return err
		}
	}

	return err
}

func (r *Reconciler) getEventTypes(ctx context.Context, src *v1alpha1.GCSSource) ([]eventingv1alpha1.EventType, error) {
	etl, err := r.eventtypeclientset.EventingV1alpha1().EventTypes(src.Namespace).List(metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(resources.Labels(src.Name)).String(),
	})

	if err != nil {
		r.Logger.Errorf("Unable to list event types: %v", err)
		return nil, err
	}
	eventTypes := make([]eventingv1alpha1.EventType, 0)
	for _, et := range etl.Items {
		if metav1.IsControlledBy(&et, src) {
			eventTypes = append(eventTypes, et)
		}
	}
	return eventTypes, nil
}

func (r *Reconciler) makeEventTypes(src *v1alpha1.GCSSource) ([]eventingv1alpha1.EventType, error) {
	// TODO in resources.
	eventTypes := make([]eventingv1alpha1.EventType, 0)

	// Only create EventTypes for Broker sinks.
	// We add this check here in case the GCSSource was changed from Broker to non-Broker sink.
	// If so, we need to delete the existing ones, thus we return empty expected.
	if src.Spec.Sink.Kind != "Broker" {
		return eventTypes, nil
	}

	ets := src.Spec.EventTypes
	specs := make([]eventingv1alpha1.EventTypeSpec, 0)
	templateSpec := eventingv1alpha1.EventTypeSpec{
		Source: v1alpha1.GCSEventSource(src.Spec.GoogleCloudProject, src.Spec.Bucket),
		Broker: src.Spec.Sink.Name,
	}
	if ets.Finalize != nil {
		spec := templateSpec.DeepCopy()
		spec.Type = v1alpha1.GCSFinalizeType
		spec.Schema = v1alpha1.GCSFinalizeSchema
		specs = append(specs, *spec)
	}
	if ets.Delete != nil {
		spec := templateSpec.DeepCopy()
		spec.Type = v1alpha1.GCSDeleteType
		spec.Schema = v1alpha1.GCSDeleteSchema
		specs = append(specs, *spec)
	}
	if ets.Archive != nil {
		spec := templateSpec.DeepCopy()
		spec.Type = v1alpha1.GCSArchiveType
		spec.Schema = v1alpha1.GCSArchiveType
		specs = append(specs, *spec)
	}
	if ets.MetadataUpdate != nil {
		spec := templateSpec.DeepCopy()
		spec.Type = v1alpha1.GCSMetaUpdateType
		spec.Schema = v1alpha1.GCSMetaUpdateSchema
		specs = append(specs, *spec)
	}

	return eventTypes, nil
}

func (r *Reconciler) computeDiff(current []eventingv1alpha1.EventType, expected []eventingv1alpha1.EventType) ([]eventingv1alpha1.EventType, []eventingv1alpha1.EventType) {
	toCreate := make([]eventingv1alpha1.EventType, 0)
	toDelete := make([]eventingv1alpha1.EventType, 0)
	currentMap := asMap(current, keyFromEventType)
	expectedMap := asMap(expected, keyFromEventType)

	// Iterate over the slices instead of the maps for predictable UT expectations.
	for _, e := range expected {
		if c, ok := currentMap[keyFromEventType(&e)]; !ok {
			toCreate = append(toCreate, e)
		} else {
			if !equality.Semantic.DeepEqual(e.Spec, c.Spec) {
				toDelete = append(toDelete, c)
				toCreate = append(toCreate, e)
			}
		}
	}
	// Need to check whether the current EventTypes are not in the expected map. If so, we have to delete them.
	// This could happen if the GCSSource CO changes its broker.
	for _, c := range current {
		if _, ok := expectedMap[keyFromEventType(&c)]; !ok {
			toDelete = append(toDelete, c)
		}
	}
	return toCreate, toDelete
}

func asMap(eventTypes []eventingv1alpha1.EventType, keyFunc func(*eventingv1alpha1.EventType) string) map[string]eventingv1alpha1.EventType {
	eventTypesAsMap := make(map[string]eventingv1alpha1.EventType, 0)
	for _, eventType := range eventTypes {
		key := keyFunc(&eventType)
		eventTypesAsMap[key] = eventType
	}
	return eventTypesAsMap
}

func keyFromEventType(eventType *eventingv1alpha1.EventType) string {
	return fmt.Sprintf("%s_%s_%s_%s", eventType.Spec.Type, eventType.Spec.Source, eventType.Spec.Schema, eventType.Spec.Broker)
}

func (c *Reconciler) deleteTopic(project string, topic string) error {
	ctx := context.Background()
	psc, err := pubsub.NewClient(ctx, project)
	if err != nil {
		return err
	}
	t := psc.Topic(topic)
	err = t.Delete(context.Background())
	if err == nil {
		c.Logger.Infof("Deleted topic %q", topic)
		return nil
	}

	if st, ok := gstatus.FromError(err); !ok {
		c.Logger.Infof("Unknown error from the pubsub client: %s", err)
		return err
	} else if st.Code() != codes.NotFound {
		return err
	}
	return nil
}

func (c *Reconciler) deleteNotification(gcs *v1alpha1.GCSSource) error {
	if gcs.Status.NotificationID == "" {
		return nil
	}
	ctx := context.Background()
	gcsClient, err := storage.NewClient(ctx)
	if err != nil {
		c.Logger.Infof("Failed to create storage client: %s", err)
		return err
	}

	bucket := gcsClient.Bucket(gcs.Spec.Bucket)
	c.Logger.Infof("Deleting notification as: %q", gcs.Status.NotificationID)
	err = bucket.DeleteNotification(ctx, gcs.Status.NotificationID)
	if err == nil {
		c.Logger.Infof("Deleted Notification: %q", gcs.Status.NotificationID)
		return nil
	}

	if st, ok := gstatus.FromError(err); !ok {
		c.Logger.Infof("Unknown error from the cloud storage client: %s", err)
		return err
	} else if st.Code() != codes.NotFound {
		return err
	}
	return nil
}

func (c *Reconciler) addFinalizer(csr *v1alpha1.GCSSource) {
	finalizers := sets.NewString(csr.Finalizers...)
	finalizers.Insert(finalizerName)
	csr.Finalizers = finalizers.List()
}

func (c *Reconciler) removeFinalizer(csr *v1alpha1.GCSSource) {
	finalizers := sets.NewString(csr.Finalizers...)
	finalizers.Delete(finalizerName)
	csr.Finalizers = finalizers.List()
}

func (c *Reconciler) update(desired *v1alpha1.GCSSource) (*v1alpha1.GCSSource, error) {
	csr, err := c.gcssourcesLister.GCSSources(desired.Namespace).Get(desired.Name)
	if err != nil {
		return nil, err
	}
	// Check if there is anything to update.
	if !reflect.DeepEqual(csr.Status, desired.Status) || !reflect.DeepEqual(csr.ObjectMeta, desired.ObjectMeta) {
		// Don't modify the informers copy
		existing := csr.DeepCopy()
		existing.Status = desired.Status
		existing.Finalizers = desired.Finalizers
		client := c.gcssourceclientset.SourcesV1alpha1().GCSSources(desired.Namespace)
		// TODO: for CRD there's no updatestatus, so use normal update.
		return client.Update(existing)
	}
	return csr, nil
}
