package v1alpha1

import (
	"github.com/redhat-cop/operator-utils/pkg/util/apis"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// ResourceLockerSpec defines the desired state of ResourceLocker
// +k8s:openapi-gen=true
type ResourceLockerSpec struct {
	// Resources is a list of resource manifests that should be locked into the specified configuration
	// +kubebuilder:validation:Optional
	// +listType=atomic
	Resources []Resource `json:"resources,omitempty"`

	// Patches is a list of patches that should be enforced at runtime.
	// +kubebuilder:validation:Optional
	// +listType="map"
	// +listMapKey="id"
	Patches []apis.Patch `json:"patches,omitempty"`

	// ServiceAccountRef is the service account to be used to run the controllers associated with this configuration
	// +kubebuilder:validation:Optional
	// kubebuilder:default:="{Name: &#34;default&#34;}"
	ServiceAccountRef corev1.LocalObjectReference `json:"serviceAccountRef,omitempty"`
}

// Resource represent a resource to be enforced
// +k8s:openapi-gen=true
type Resource struct {
	// +kubebuilder:validation:Required
	// +kubebuilder:pruning:PreserveUnknownFields
	Object runtime.RawExtension `json:"object"`
	// +kubebuilder:validation:Optional
	// +listType=set
	ExcludedPaths []string `json:"excludedPaths,omitempty"`
}

// ResourceLockerStatus defines the observed state of ResourceLocker
// +k8s:openapi-gen=true
type ResourceLockerStatus struct {
	apis.EnforcingReconcileStatus `json:",inline,omitempty"`
}

func (m *ResourceLocker) GetEnforcingReconcileStatus() apis.EnforcingReconcileStatus {
	return m.Status.EnforcingReconcileStatus
}

func (m *ResourceLocker) SetEnforcingReconcileStatus(reconcileStatus apis.EnforcingReconcileStatus) {
	m.Status.EnforcingReconcileStatus = reconcileStatus
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ResourceLocker is the Schema for the resourcelockers API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=resourcelockers,scope=Namespaced
type ResourceLocker struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ResourceLockerSpec   `json:"spec,omitempty"`
	Status ResourceLockerStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ResourceLockerList contains a list of ResourceLocker
type ResourceLockerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ResourceLocker `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ResourceLocker{}, &ResourceLockerList{})
}
