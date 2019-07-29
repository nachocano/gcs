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

package v1alpha1

import (
	"github.com/knative/pkg/apis/duck"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"knative.dev/pkg/apis"
	duckv1alpha1 "knative.dev/pkg/apis/duck/v1alpha1"
	duckv1beta1 "knative.dev/pkg/apis/duck/v1beta1"
	"knative.dev/pkg/kmeta"
	"knative.dev/pkg/webhook"
)

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// GCSSource is a specification for a GCSSource resource
type GCSSource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GCSSourceSpec   `json:"spec"`
	Status GCSSourceStatus `json:"status"`
}

// Check that GCSSource implements the Conditions duck type.
var (
	// Check that Channel can be validated, can be defaulted, and has immutable fields.
	_ apis.Validatable   = (*GCSSource)(nil)
	_ apis.Defaultable   = (*GCSSource)(nil)
	_ apis.Immutable     = (*GCSSource)(nil)
	_ runtime.Object     = (*GCSSource)(nil)
	_ webhook.GenericCRD = (*GCSSource)(nil)

	// Check that we can create OwnerReferences to a Channel.
	_ kmeta.OwnerRefable = (*GCSSource)(nil)
)

// GCSSourceSpec is the spec for a GCSSource resource
type GCSSourceSpec struct {
	// GCSCredsSecret is the credential to use to create the Notification on the GCS bucket.
	// The value of the secret entry must be a service account key in the JSON format (see
	// https://cloud.google.com/iam/docs/creating-managing-service-account-keys).
	GCSCredsSecret corev1.SecretKeySelector `json:"gcsCredsSecret"`

	// GcpCredsSecret is the credential to use to poll the GCP PubSub Subscription. It is not used
	// to create or delete the Subscription, only to poll it. The value of the secret entry must be
	// a service account key in the JSON format (see
	// https://cloud.google.com/iam/docs/creating-managing-service-account-keys).
	// If omitted, uses GCSCredsSecret from above
	// +optional
	GcpCredsSecret *corev1.SecretKeySelector `json:"gcpCredsSecret,omitempty"`

	// ServiceAccountName holds the name of the Kubernetes service account
	// as which the underlying K8s resources should be run. If unspecified
	// this will default to the "default" service account for the namespace
	// in which the GCSSource exists.
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`

	// GoogleCloudProject is the ID of the Google Cloud Project that the PubSub Topic exists in.
	GoogleCloudProject string `json:"googleCloudProject,omitempty"`

	// Bucket to subscribe to
	Bucket string `json:"bucket"`

	// EventTypes to subscribe to
	EventTypes *GCSEventTypes `json:"eventTypes,omitempty"`

	// ObjectNamePrefix limits the notifications to objects with this prefix
	// +optional
	ObjectNamePrefix string `json:"objectNamePrefix,omitempty"`

	// CustomAttributes is the optional list of additional attributes to attach to each Cloud PubSub
	// message published for this notification subscription.
	// +optional
	CustomAttributes map[string]string `json:"customAttributes,omitempty"`

	// PayloadFormat specifies the contents of the message payload.
	// See https://cloud.google.com/storage/docs/pubsub-notifications#payload.
	// +optional
	PayloadFormat string `json:"payloadFormat,omitempty"`

	// Sink is a reference to an object that will resolve to a domain name to use
	// as the sink.
	// +optional
	Sink *corev1.ObjectReference `json:"sink,omitempty"`
}

// GCSEventTypes contains the event types that can be configured by the GCSSource.
type GCSEventTypes struct {
	Finalize       *GCSObjectFinalize `json:"finalize,omitempty"`
	Delete         *GCSObjectDelete   `json:"delete,omitempty"`
	Archive        *GCSObjectArchive  `json:"archive,omitempty"`
	MetadataUpdate *GCSMetadataUpdate `json:"metadataUpdate,omitempty"`
}

type CloudEventProperties struct {
	Type   string `json:"ceType"`
	Schema string `json:"ceSchema"`
}

type GCSObjectFinalize struct {
	CloudEventProperties `json:",inline"`
}

type GCSObjectDelete struct {
	CloudEventProperties `json:",inline"`
}

type GCSObjectArchive struct {
	CloudEventProperties `json:",inline"`
}

type GCSMetadataUpdate struct {
	CloudEventProperties `json:",inline"`
}

const (
	// GCSConditionReady has status True when the GCSSource is ready to send events.
	GCSConditionReady = duckv1alpha1.ConditionReady

	// PubSubSourceReady has status True when the underlying GCP PubSub Source is ready
	PubSubSourceReady duckv1alpha1.ConditionType = "PubSubSourceReady"

	// PubSubTopicReady has status True when the underlying GCP PubSub topic is ready
	PubSubTopicReady duckv1alpha1.ConditionType = "PubSubTopicReady"

	// GCSReady has status True when GCS has been configured properly to send Notification events
	GCSReady duckv1alpha1.ConditionType = "GCSReady"
)

const (
	gCSFinalizeType   = "com.google.storage.finalize"
	gCSArchiveType    = "com.google.storage.archive"
	gCSDeleteType     = "com.google.storage.delete"
	gCSMetaUpdateType = "com.google.storage.metadataUpdate"

	gCSFinalizeSchema   = "finalize-schema"
	gCSArchiveSchema    = "archive-schema"
	gCSDeleteSchema     = "delete-schema"
	gCSMetaUpdateSchema = "metadataUpdate-schema"
)

var (
	GCSEventTypesMapping = map[string]string{
		gCSFinalizeType:   "OBJECT_FINALIZE",
		gCSArchiveType:    "OBJECT_ARCHIVE",
		gCSDeleteType:     "OBJECT_DELETE",
		gCSMetaUpdateType: "OBJECT_METADATA_UPDATE",
	}
)

var gcsSourceCondSet = duckv1alpha1.NewLivingConditionSet(
	PubSubSourceReady,
	PubSubTopicReady,
	GCSReady)

// GCSSourceStatus is the status for a GCSSource resource
type GCSSourceStatus struct {
	// inherits duck/v1beta1 Status, which currently provides:
	// * ObservedGeneration - the 'Generation' of the Service that was last processed by the controller.
	// * Conditions - the latest available observations of a resource's current state.
	duckv1beta1.Status `json:",inline"`

	// TODO: add conditions and other stuff here...
	// NotificationID is the ID that GCS identifies this notification as.
	// +optional
	NotificationID string `json:"notificationID,omitempty"`

	// Topic where the notifications are sent to.
	// +optional
	Topic string `json:"topic,omitempty"`

	// SinkURI is the current active sink URI that has been configured for the GCSSource.
	// +optional
	SinkURI string `json:"sinkUri,omitempty"`
}

// GetCondition returns the condition currently associated with the given type, or nil.
func (s *GCSSourceStatus) GetCondition(t duckv1alpha1.ConditionType) *duckv1alpha1.Condition {
	return gcsSourceCondSet.Manage(s).GetCondition(t)
}

// IsReady returns true if the resource is ready overall.
func (s *GCSSourceStatus) IsReady() bool {
	return gcsSourceCondSet.Manage(s).IsHappy()
}

// InitializeConditions sets relevant unset conditions to Unknown state.
func (s *GCSSourceStatus) InitializeConditions() {
	gcsSourceCondSet.Manage(s).InitializeConditions()
}

// MarkPubSubNotSourceReady sets the condition that the underlying PubSub source is not ready and why
func (s *GCSSourceStatus) MarkPubSubSourceNotReady(reason, messageFormat string, messageA ...interface{}) {
	gcsSourceCondSet.Manage(s).MarkFalse(PubSubSourceReady, reason, messageFormat, messageA...)
}

// MarkPubSubSourceReady sets the condition that the underlying PubSub source is ready
func (s *GCSSourceStatus) MarkPubSubSourceReady() {
	gcsSourceCondSet.Manage(s).MarkTrue(PubSubSourceReady)
}

// MarkPubSubTopicNotReady sets the condition that the PubSub topic was not created and why
func (s *GCSSourceStatus) MarkPubSubTopicNotReady(reason, messageFormat string, messageA ...interface{}) {
	gcsSourceCondSet.Manage(s).MarkFalse(PubSubTopicReady, reason, messageFormat, messageA...)
}

// MarkPubSubTopicReady sets the condition that the underlying PubSub topic was created successfully
func (s *GCSSourceStatus) MarkPubSubTopicReady() {
	gcsSourceCondSet.Manage(s).MarkTrue(PubSubTopicReady)
}

// MarkGCSNotReady sets the condition that the GCS has been configured to send Notifications
func (s *GCSSourceStatus) MarkGCSNotReady(reason, messageFormat string, messageA ...interface{}) {
	gcsSourceCondSet.Manage(s).MarkFalse(GCSReady, reason, messageFormat, messageA...)
}

func (s *GCSSourceStatus) MarkGCSReady() {
	gcsSourceCondSet.Manage(s).MarkTrue(GCSReady)
}

func (gcsSource *GCSSource) GetGroupVersionKind() schema.GroupVersionKind {
	return SchemeGroupVersion.WithKind("GCSSource")
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// GCSSourceList is a list of GCSSource resources
type GCSSourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []GCSSource `json:"items"`
}

// GetFullType implements duck.Implementable
func (s *GCSSource) GetFullType() duck.Populatable {
	return &GCSSource{}
}

// Populate implements duck.Populatable
func (c *GCSSource) Populate() {
	//
}
