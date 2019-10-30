package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ResourceLockerSpec defines the desired state of ResourceLocker
// +k8s:openapi-gen=true
type ResourceLockerSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html
	// +kubebuilder:validation:Optional
	// +listType=map
	Resources []runtime.RawExtension `json:"resources,omitempty"`
	// +kubebuilder:validation:Optional
	ServiceAccountRef corev1.LocalObjectReference `json:"serviceAccountRef,omitempty"`
	// +kubebuilder:validation:Optional
	// +listType=map
	Patches []Patch `json:"patches,omitempty"`
}

// +k8s:openapi-gen=true
type Patch struct {
	// +kubebuilder:validation:Optional
	// +listType=map
	SourceObjectRefs []corev1.ObjectReference `json:"sourceObjectRefs,omitempty"`
	// +kubebuilder:validation:Required
	TargetObjectRef corev1.ObjectReference `json:"targetObjectRef"`
	//see the client.patch file.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum={"application/json-patch+json"'"application/merge-patch+json","application/strategic-merge-patch+json","application/apply-patch+yaml"}
	PatchType types.PatchType `json:"patchType,omitempty"`
	// +kubebuilder:validation:Required
	PatchTemplate string `json:"patchTemplate"`
}

// ResourceLockerStatus defines the observed state of ResourceLocker
// +k8s:openapi-gen=true
type ResourceLockerStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html
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
