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

import (
	"context"
	"knative.dev/pkg/apis"
)

func (gcs *GCSSource) Validate(ctx context.Context) *apis.FieldError {
	return gcs.Spec.Validate(ctx).ViaField("spec")
}

func (s *GCSSourceSpec) Validate(ctx context.Context) *apis.FieldError {
	var errs *apis.FieldError

	if s.EventTypes == nil {
		errs = errs.Also(apis.ErrMissingField("eventTypes"))
		return errs
	}

	if s.EventTypes.Finalize != nil {
		if ce := isValidCloudEvent(s.EventTypes.Finalize.Type, s.EventTypes.Finalize.Schema, GCSFinalizeType, GCSFinalizeSchema); ce != nil {
			errs = errs.Also(ce.ViaField("eventTypes.finalize"))
		}
	}

	if s.EventTypes.Archive != nil {
		if ce := isValidCloudEvent(s.EventTypes.Archive.Type, s.EventTypes.Archive.Schema, GCSArchiveType, GCSArchiveSchema); ce != nil {
			errs = errs.Also(ce.ViaField("eventTypes.archive"))
		}
	}

	if s.EventTypes.Delete != nil {
		if ce := isValidCloudEvent(s.EventTypes.Delete.Type, s.EventTypes.Delete.Schema, GCSDeleteType, GCSDeleteSchema); ce != nil {
			errs = errs.Also(ce.ViaField("eventTypes.delete"))
		}
	}

	if s.EventTypes.MetadataUpdate != nil {
		if ce := isValidCloudEvent(s.EventTypes.MetadataUpdate.Type, s.EventTypes.MetadataUpdate.Schema, GCSMetaUpdateType, GCSMetaUpdateSchema); ce != nil {
			errs = errs.Also(ce.ViaField("eventTypes.metadataUpdate"))
		}
	}

	return errs
}

func (gcs *GCSSource) CheckImmutableFields(ctx context.Context, og apis.Immutable) *apis.FieldError {
	return nil
}

func isValidCloudEvent(ceType, ceSchema, defaultType, defaultSchema string) *apis.FieldError {
	var errs *apis.FieldError
	if ceType != defaultType {
		fe := apis.ErrInvalidValue(ceType, "ceType")
		errs = errs.Also(fe)
	}
	if ceSchema != defaultSchema {
		fe := apis.ErrInvalidValue(ceSchema, "ceSchema")
		errs = errs.Also(fe)
	}
	return errs
}
