package lockedresourcecontroller

import (
	"reflect"
	"strings"

	"encoding/json"

	"github.com/redhat-cop/operator-utils/pkg/util"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_lockedresource")

//workaround to weird secret behavior
var serviceAccountGVK = schema.GroupVersionKind{
	Version: "v1",
	Kind:    "ServiceAccount",
}
var secretGVK = schema.GroupVersionKind{
	Version: "v1",
	Kind:    "Secret",
}
var configmapGVK = schema.GroupVersionKind{
	Version: "v1",
	Kind:    "ConfigMap",
}
var roleGVK = schema.GroupVersionKind{
	Version: "v1",
	Group:   "rbac.authorization.k8s.io",
	Kind:    "Role",
}
var roleBindingGVK = schema.GroupVersionKind{
	Version: "v1",
	Group:   "rbac.authorization.k8s.io",
	Kind:    "RoleBinding",
}

type LockedResourceReconciler struct {
	Resource     unstructured.Unstructured
	ExcludePaths []string
	util.ReconcilerBase
	status       ResourceStatus
	statusChange chan<- event.GenericEvent
	parentObject metav1.Object
}

type ResourceStatus struct {
	// Type of deployment condition.
	Type ResourceConditionType `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status corev1.ConditionStatus `json:"status"`
	// The last time this condition was updated.
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`
	// A human readable message indicating details about the transition.
	Message string `json:"message,omitempty"`
}

type ResourceConditionType string

const (
	// Enforcing means that the patch has been succesfully reconciled and it's being enforced
	Enforcing ResourceConditionType = "Enforcing"
	// Erro means that the patch has not been successfully reconciled and we cannot guarntee that it's being enforced
	Failure ResourceConditionType = "Failure"
)

// NewReconciler returns a new reconcile.Reconciler
func NewLockedObjectReconciler(mgr manager.Manager, object unstructured.Unstructured, excludePaths []string, statusChange chan<- event.GenericEvent, parentObject metav1.Object) (*LockedResourceReconciler, error) {

	reconciler := &LockedResourceReconciler{
		ReconcilerBase: util.NewReconcilerBase(mgr.GetClient(), mgr.GetScheme(), mgr.GetConfig(), mgr.GetEventRecorderFor("controller_locked_object_"+GetKeyFromObject(&object))),
		Resource:       object,
		ExcludePaths:   excludePaths,
		statusChange:   statusChange,
		parentObject:   parentObject,
	}

	err := reconciler.CreateOrUpdateResource(nil, "", object.DeepCopy())
	if err != nil {
		log.Error(err, "unable to create or update", "resource", object)
		return &LockedResourceReconciler{}, err
	}

	controller, err := controller.New("controller_locked_object_"+GetKeyFromObject(&object), mgr, controller.Options{Reconciler: reconciler})
	if err != nil {
		log.Error(err, "unable to create new controller", "with reconciler", reconciler)
		return &LockedResourceReconciler{}, err
	}

	gvk := object.GetObjectKind().GroupVersionKind()
	groupVersion := schema.GroupVersion{Group: gvk.Group, Version: gvk.Version}

	mgr.GetScheme().AddKnownTypes(groupVersion, &object)

	err = controller.Watch(&source.Kind{Type: &object}, &handler.EnqueueRequestForObject{}, &resourceModifiedPredicate{
		name:      object.GetName(),
		namespace: object.GetNamespace(),
	})
	if err != nil {
		log.Error(err, "unable to create new wtach", "with source", object)
		return &LockedResourceReconciler{}, err
	}

	return reconciler, nil
}

func (lor *LockedResourceReconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	log.Info("reconcile called for", "object", GetKeyFromObject(&lor.Resource), "request", request)
	//err := lor.CreateOrUpdateResource(nil, "", &lor.Object)

	// Fetch the  instance
	//instance := &unstructured.Unstructured{}
	client, err := lor.GetDynamicClientOnUnstructured(lor.Resource)
	if err != nil {
		log.Error(err, "unable to get dynamicClient", "on object", lor.Resource)
		return lor.manageError(err)
	}
	instance, err := client.Get(lor.Resource.GetName(), v1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			// if not found we have to recreate it.
			err = lor.CreateOrUpdateResource(nil, "", lor.Resource.DeepCopy())
			if err != nil {
				log.Error(err, "unable to create or update", "object", lor.Resource)
				lor.manageError(err)
			}
			return lor.manageSuccess()
		}
		// Error reading the object - requeue the request.
		log.Error(err, "unable to lookup", "object", lor.Resource)
		return lor.manageError(err)
	}
	log.V(1).Info("determining if resources are equal", "desired", lor.Resource, "current", instance)
	equal, err := lor.isEqual(instance)
	if err != nil {
		log.Error(err, "unable to determine if", "object", lor.Resource, "is equal to object", instance)
		return lor.manageError(err)
	}
	if !equal {
		log.V(1).Info("determined that resources are NOT equal")
		patch, err := filterOutPaths(&lor.Resource, lor.ExcludePaths)
		if err != nil {
			log.Error(err, "unable to filter out ", "excluded paths", lor.ExcludePaths, "from object", lor.Resource)
			return lor.manageError(err)
		}
		patchBytes, err := json.Marshal(patch)
		if err != nil {
			log.Error(err, "unable to marshall ", "object", patch)
			return lor.manageError(err)
		}
		log.V(1).Info("executing", "patch", string(patchBytes), "on object", instance)
		_, err = client.Patch(instance.GetName(), types.MergePatchType, patchBytes, metav1.PatchOptions{})
		if err != nil {
			log.Error(err, "unable to patch ", "object", instance, "with patch", string(patchBytes))
			return lor.manageError(err)
		}
		return lor.manageSuccess()
	}
	log.V(1).Info("determined that resources are equal")
	return lor.manageSuccess()
}

// GetDynamicClientOnUnstructured returns a dynamic client on an Unstructured type. This client can be further namespaced.
func (r *LockedResourceReconciler) GetDynamicClientOnUnstructured(obj unstructured.Unstructured) (dynamic.ResourceInterface, error) {
	apiRes, err := r.getAPIReourceForUnstructured(obj)
	if err != nil {
		log.Error(err, "unable to get apiresource from unstructured", "unstructured", obj)
		return nil, err
	}
	dc, err := r.GetDynamicClientOnAPIResource(apiRes)
	if err != nil {
		log.Error(err, "unable to get namespaceable dynamic client from ", "resource", apiRes)
		return nil, err
	}
	if apiRes.Namespaced {
		return dc.Namespace(obj.GetNamespace()), nil
	}
	return dc, nil
}

func (r *LockedResourceReconciler) getAPIReourceForUnstructured(obj unstructured.Unstructured) (metav1.APIResource, error) {
	gvk := obj.GetObjectKind().GroupVersionKind()
	res := metav1.APIResource{}
	discoveryClient, err := r.GetDiscoveryClient()
	if err != nil {
		log.Error(err, "unable to create discovery client")
		return res, err
	}
	resList, err := discoveryClient.ServerResourcesForGroupVersion(gvk.GroupVersion().String())
	if err != nil {
		log.Error(err, "unable to retrieve resouce list for:", "groupversion", gvk.GroupVersion().String())
		return res, err
	}
	for _, resource := range resList.APIResources {
		if resource.Kind == gvk.Kind && !strings.Contains(resource.Name, "/") {
			res = resource
			res.Group = gvk.Group
			res.Version = gvk.Version
			break
		}
	}
	return res, nil
}

func (lor *LockedResourceReconciler) isEqual(instance *unstructured.Unstructured) (bool, error) {
	left, err := filterOutPaths(&lor.Resource, lor.ExcludePaths)
	log.V(1).Info("resource", "desired", left)
	if err != nil {
		return false, err
	}
	right, err := filterOutPaths(instance, lor.ExcludePaths)
	if err != nil {
		return false, err
	}
	log.V(1).Info("resource", "current", right)
	return reflect.DeepEqual(left, right), nil
}

func GetKeyFromObject(object *unstructured.Unstructured) string {
	return object.GroupVersionKind().String() + "/" + object.GetNamespace() + "/" + object.GetName()
}

type resourceModifiedPredicate struct {
	name      string
	namespace string
	predicate.Funcs
}

// Update implements default UpdateEvent filter for validating resource version change
func (p *resourceModifiedPredicate) Update(e event.UpdateEvent) bool {
	if e.MetaNew.GetNamespace() == p.namespace && e.MetaNew.GetName() == p.name {
		return true
	}
	return false
}

func (p *resourceModifiedPredicate) Create(e event.CreateEvent) bool {
	if e.Meta.GetNamespace() == p.namespace && e.Meta.GetName() == p.name {
		return true
	}
	return false
}

func (p *resourceModifiedPredicate) Delete(e event.DeleteEvent) bool {
	if e.Meta.GetNamespace() == p.namespace && e.Meta.GetName() == p.name {
		return true
	}
	return false
}

// func (lor *LockedResourceReconciler) secretSpecialHandling(instance *unstructured.Unstructured) (reconcile.Result, error) {
// 	tobeupdated := false
// 	if !reflect.DeepEqual(instance.UnstructuredContent()["data"], lor.Resource.UnstructuredContent()["data"]) {
// 		instance.UnstructuredContent()["data"] = lor.Resource.UnstructuredContent()["data"]
// 		tobeupdated = true
// 	}
// 	if !reflect.DeepEqual(instance.UnstructuredContent()["type"], lor.Resource.UnstructuredContent()["type"]) {
// 		instance.UnstructuredContent()["type"] = lor.Resource.UnstructuredContent()["type"]
// 		tobeupdated = true
// 	}
// 	if tobeupdated {
// 		err := lor.CreateOrUpdateResource(nil, "", instance)
// 		if err != nil {
// 			return reconcile.Result{}, err
// 		}
// 	}
// 	return reconcile.Result{}, nil
// }

// func (lor *LockedResourceReconciler) configmapSpecialHandling(instance *unstructured.Unstructured) (reconcile.Result, error) {
// 	if !reflect.DeepEqual(instance.UnstructuredContent()["data"], lor.Resource.UnstructuredContent()["data"]) {
// 		instance.UnstructuredContent()["data"] = lor.Resource.UnstructuredContent()["data"]
// 		err := lor.CreateOrUpdateResource(nil, "", instance)
// 		if err != nil {
// 			return reconcile.Result{}, err
// 		}
// 	}
// 	return reconcile.Result{}, nil
// }

// func (lor *LockedResourceReconciler) roleSpecialHandling(instance *unstructured.Unstructured) (reconcile.Result, error) {
// 	tobeupdated := false
// 	if !reflect.DeepEqual(instance.UnstructuredContent()["rules"], lor.Resource.UnstructuredContent()["rules"]) {
// 		instance.UnstructuredContent()["rules"] = lor.Resource.UnstructuredContent()["rules"]
// 		tobeupdated = true
// 	}
// 	if !reflect.DeepEqual(instance.UnstructuredContent()["aggregationRule"], lor.Resource.UnstructuredContent()["aggregationRule"]) {
// 		instance.UnstructuredContent()["aggregationRule"] = lor.Resource.UnstructuredContent()["aggregationRule"]
// 		tobeupdated = true
// 	}
// 	if tobeupdated {
// 		err := lor.CreateOrUpdateResource(nil, "", instance)
// 		if err != nil {
// 			return reconcile.Result{}, err
// 		}
// 	}
// 	return reconcile.Result{}, nil
// }

// func (lor *LockedResourceReconciler) roleBindingSpecialHandling(instance *unstructured.Unstructured) (reconcile.Result, error) {
// 	tobeupdated := false
// 	// if reflect.DeepEqual(instance.UnstructuredContent()["roleref"], lor.Object.UnstructuredContent()["roleref"]) {
// 	// 	instance.UnstructuredContent()["roleref"] = lor.Object.UnstructuredContent()["roleref"]
// 	// 	tobeupdated = true
// 	// }
// 	if !reflect.DeepEqual(instance.UnstructuredContent()["subjects"], lor.Resource.UnstructuredContent()["subjects"]) {
// 		instance.UnstructuredContent()["subjects"] = lor.Resource.UnstructuredContent()["subjects"]
// 		tobeupdated = true
// 	}
// 	if tobeupdated {
// 		err := lor.CreateOrUpdateResource(nil, "", instance)
// 		if err != nil {
// 			return reconcile.Result{}, err
// 		}
// 	}
// 	return reconcile.Result{}, nil
// }

// func (lor *LockedResourceReconciler) serviceAccountSpecialHandling(instance *unstructured.Unstructured) (reconcile.Result, error) {
// 	//service accounts are essentially read only
// 	return reconcile.Result{}, nil
// }

func (lor *LockedResourceReconciler) manageError(err error) (reconcile.Result, error) {
	condition := ResourceStatus{
		Type:           Failure,
		Status:         corev1.ConditionTrue,
		LastUpdateTime: metav1.Now(),
		Message:        err.Error(),
	}
	lor.setStatus(condition)
	return reconcile.Result{}, err
}

func (lor *LockedResourceReconciler) manageSuccess() (reconcile.Result, error) {
	condition := ResourceStatus{
		Type:           Enforcing,
		Status:         corev1.ConditionTrue,
		LastUpdateTime: metav1.Now(),
	}
	lor.setStatus(condition)
	return reconcile.Result{}, nil
}

func (lor *LockedResourceReconciler) setStatus(status ResourceStatus) {
	lor.status = status
	if lor.statusChange != nil {
		lor.statusChange <- event.GenericEvent{
			Meta: lor.parentObject,
		}
	}
}

func (lor *LockedResourceReconciler) GetStatus() ResourceStatus {
	return lor.status
}
