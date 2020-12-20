/*


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
	"github.com/redhat-cop/operator-utils/pkg/util/apis"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ResourceLockerSpec defines the desired state of ResourceLocker
type ResourceLockerSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// +kubebuilder:validation:Optional
	// +listType=atomic
	Resources []Resource `json:"resources,omitempty"`

	// Patches is a list of patches that should be enforced at runtime.
	// +kubebuilder:validation:Optional
	// +patchMergeKey=id
	// +patchStrategy=merge
	// +listType="map"
	// +listMapKey="id"
	Patches []apis.Patch `json:"patches,omitempty" patchStrategy:"merge" patchMergeKey:"id"`

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

// ResourceLocker Represents the intetiont to create and neforce a set of resources and/or patches.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:JSONPath=".spec.serviceAccountRef.name",description="The service account used to create and enfoce these resources and patches, must have required permissions via RBAC",name="SERVICE ACCOUNT",type="string"
type ResourceLocker struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ResourceLockerSpec   `json:"spec,omitempty"`
	Status ResourceLockerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ResourceLockerList contains a list of ResourceLocker
type ResourceLockerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ResourceLocker `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ResourceLocker{}, &ResourceLockerList{})
}
