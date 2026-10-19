// Package v1alpha1 contains the API types of the maintenance.example.com group.
// +kubebuilder:object:generate=true
// +groupName=maintenance.example.com
package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
	// GroupVersion identifies the API group and version of this package.
	GroupVersion = schema.GroupVersion{Group: "maintenance.example.com", Version: "v1alpha1"}

	// SchemeBuilder registers the types of this package in a runtime.Scheme.
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}

	// AddToScheme adds the types of this package to a runtime.Scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)
