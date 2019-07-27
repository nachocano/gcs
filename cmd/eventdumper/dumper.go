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

package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"cloud.google.com/go/pubsub"
	"github.com/knative/pkg/cloudevents"
)

// This is just a subset of the fields for demonstration purposes.
// Full object notification spec is here:
// https://cloud.google.com/storage/docs/json_api/v1/objects#resource-representations
type GCSObjectNotification struct {
	Name   string `json:"name,omitempty"`
	Bucket string `json:"bucket,omitempty"`
	// This is listed as unsigned long but in practice seems to be a string??
	Size      string `json:"size,omitempty"`
	MediaLink string `json:"mediaLink,omitempty"`
}

func myFunc(ctx context.Context, msg *pubsub.Message) error {
	// Extract only the Cloud Context from the context because that's
	// all we care about for this example and the entire context is toooooo much...
	ec := cloudevents.FromContext(ctx)
	if ec != nil {
		log.Printf("Received Cloud Event Context as: %+v", ec)
	} else {
		log.Printf("No Cloud Event Context found")
	}
	if len(msg.Data) > 0 {
		obj := &GCSObjectNotification{}
		err := json.Unmarshal(msg.Data, obj)
		if err != nil {
			log.Printf("Failed to umarshal object notification data: %s\n data was %q", err, string(msg.Data))
			return err
		}
		log.Printf("object notification metadata is: %+v", obj)
	} else {
		log.Printf("Object Notification event data is empty")
	}

	return nil
}

func main() {
	m := cloudevents.NewMux()
	err := m.Handle("google.gcs", myFunc)
	if err != nil {
		log.Fatalf("Failed to create handler %s", err)
	}
	// Until this goes in to release, etc. register for
	// old pubsub event types.
	// https://github.com/knative/eventing-sources/pull/175
	err = m.Handle("google.pubsub.topic.publish", myFunc)
	if err != nil {
		log.Fatalf("Failed to create handler %s", err)
	}
	http.ListenAndServe(":8080", m)
}
