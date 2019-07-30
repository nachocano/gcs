/*
Copyright 2019 The Knative Authors

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

import "context"

func (gcs *GCSSource) SetDefaults(ctx context.Context) {
	gcs.Spec.SetDefaults(ctx)
}

func (s *GCSSourceSpec) SetDefaults(ctx context.Context) {
	// If nil, we will subscribe to all EventTypes.
	if s.EventTypes == nil {
		s.EventTypes = &GCSEventTypes{}
		s.EventTypes.Finalize = &GCSObjectFinalize{}
		s.EventTypes.Delete = &GCSObjectDelete{}
		s.EventTypes.Archive = &GCSObjectArchive{}
		s.EventTypes.MetadataUpdate = &GCSMetadataUpdate{}
	}

	if s.EventTypes.Finalize != nil {
		s.EventTypes.Finalize.Type = GCSFinalizeType
		s.EventTypes.Finalize.Schema = GCSFinalizeSchema
	}
	if s.EventTypes.Archive != nil {
		s.EventTypes.Archive.Type = GCSArchiveType
		s.EventTypes.Archive.Schema = GCSArchiveSchema
	}
	if s.EventTypes.Delete != nil {
		s.EventTypes.Delete.Type = GCSDeleteType
		s.EventTypes.Delete.Schema = GCSDeleteSchema
	}
	if s.EventTypes.MetadataUpdate != nil {
		s.EventTypes.MetadataUpdate.Type = GCSMetaUpdateType
		s.EventTypes.MetadataUpdate.Schema = GCSMetaUpdateSchema
	}
}
