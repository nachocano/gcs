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

	if len(s.EventTypes) == 0 {
		errs = errs.Also(apis.ErrMissingField("eventTypes"))
		return errs
	}

	if s.EventTypesInternal != nil {
		errs = errs.Also(apis.ErrDisallowedFields("_eventTypes"))
	}

	return errs
}

func (gcs *GCSSource) CheckImmutableFields(ctx context.Context, og apis.Immutable) *apis.FieldError {
	return nil
}
