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

package controllers

import (
	"context"
	"encoding/json"
	errs "errors"
	"os"

	"github.com/go-logr/logr"
	"github.com/scylladb/go-set/strset"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"sigs.k8s.io/yaml"

	"github.com/redhat-cop/operator-utils/pkg/util"
	"github.com/redhat-cop/operator-utils/pkg/util/lockedresourcecontroller"
	"github.com/redhat-cop/operator-utils/pkg/util/lockedresourcecontroller/lockedpatch"
	"github.com/redhat-cop/operator-utils/pkg/util/lockedresourcecontroller/lockedresource"
	redhatcopv1alpha1 "github.com/redhat-cop/resource-locker-operator/api/v1alpha1"
)

const controllerName = "resourcelocker-controller"

// ResourceLockerReconciler reconciles a ResourceLocker object
type ResourceLockerReconciler struct {
	lockedresourcecontroller.EnforcingReconciler
	Log logr.Logger
}

// +kubebuilder:rbac:groups=redhatcop.redhat.io,resources=resourcelockers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=redhatcop.redhat.io,resources=resourcelockers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=serviceaccounts;secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=get,list,watch,create,patch
func (r *ResourceLockerReconciler) Reconcile(context context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("resourcelocker", req.NamespacedName)

	// Fetch the ResourceLocker instance
	instance := &redhatcopv1alpha1.ResourceLocker{}
	err := r.GetClient().Get(context, req.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}
	log.V(1).Info("reconciling", "instance", instance)
	if ok, err := r.IsValid(instance); !ok {
		return r.ManageError(context, instance, err)
	}

	if !r.IsInitialized(instance) {
		err := r.GetClient().Update(context, instance)
		if err != nil {
			log.Error(err, "unable to update instance", "instance", instance)
			return r.ManageError(context, instance, err)
		}
		return reconcile.Result{}, nil
	}

	if util.IsBeingDeleted(instance) {
		if !util.HasFinalizer(instance, controllerName) {
			return reconcile.Result{}, nil
		}
		err := r.manageCleanUpLogic(instance)
		if err != nil {
			log.Error(err, "unable to delete instance", "instance", instance)
			return r.ManageError(context, instance, err)
		}
		util.RemoveFinalizer(instance, controllerName)
		err = r.GetClient().Update(context, instance)
		if err != nil {
			log.Error(err, "unable to update instance", "instance", instance)
			return r.ManageError(context, instance, err)
		}
		return reconcile.Result{}, nil
	}

	// TODO manage lifecycle

	lockedResources, err := r.getLockedResources(instance)

	if err != nil {
		log.Error(err, "unable to get resources for", "instance", instance)
		return r.ManageError(context, instance, err)
	}

	config, err := r.getRestConfigFromInstance(context, instance)
	if err != nil {
		log.Error(err, "unable to get restconfig for", "instance", instance)
		return r.ManageError(context, instance, err)
	}

	lockedPatches, err := lockedpatch.GetLockedPatches(instance.Spec.Patches, config)
	if err != nil {
		log.Error(err, "unable to get patches for", "instance", instance)
		return r.ManageError(context, instance, err)
	}

	err = r.UpdateLockedResourcesWithRestConfig(context, instance, lockedResources, lockedPatches, config)
	if err != nil {
		log.Error(err, "unable to update locked resources")
		return r.ManageError(context, instance, err)
	}

	return r.ManageSuccess(context, instance)
}

// func getKeyFromInstance(instance *redhatcopv1alpha1.ResourceLocker) string {
// 	return types.NamespacedName{
// 		Name:      instance.GetName(),
// 		Namespace: instance.GetNamespace(),
// 	}.String()
// }

func (r *ResourceLockerReconciler) getLockedResource(raw *runtime.RawExtension) (*unstructured.Unstructured, error) {
	bb, err := yaml.YAMLToJSON(raw.Raw)
	if err != nil {
		r.Log.Error(err, "Error transforming yaml to json", "raw", raw.Raw)
		return &unstructured.Unstructured{}, err
	}
	obj := &unstructured.Unstructured{}
	err = json.Unmarshal(bb, obj)
	if err != nil {
		r.Log.Error(err, "Error unmarshalling json manifest", "manifest", string(bb))
		return &unstructured.Unstructured{}, err
	}
	return obj, nil
}

func (r *ResourceLockerReconciler) getLockedResources(instance *redhatcopv1alpha1.ResourceLocker) ([]lockedresource.LockedResource, error) {
	objs := []lockedresource.LockedResource{}
	for _, resource := range instance.Spec.Resources {
		bb, err := yaml.YAMLToJSON(resource.Object.Raw)
		if err != nil {
			r.Log.Error(err, "Error transforming yaml to json", "raw", resource.Object.Raw)
			return []lockedresource.LockedResource{}, err
		}
		obj := &lockedresource.LockedResource{}
		err = json.Unmarshal(bb, &obj.Object)
		if err != nil {
			r.Log.Error(err, "Error unmarshalling json manifest", "manifest", string(bb))
			return []lockedresource.LockedResource{}, err
		}
		obj.ExcludedPaths = resource.ExcludedPaths
		objs = append(objs, *obj)
	}
	return objs, nil
}

var defaultExcludedPaths = []string{".metadata", ".status", ".spec.replicas"}

// IsInitialized will accept a ResourceLocker object and return true when no further changes
// to the object have been taken  to make sure it looks as expected. Possible changes include setting default values
// for the serviceAccountRef, patchStrategy, or namespace values for objects/patches if they are
// empty. This is changed at the custom resource (by the operator) so that the user is made
// aware of how those values are being interpreted by the operator logic.
//
// This will return false in all cases where the object has undergone a modification, and will return
// true if no modification takes place.
func (r *ResourceLockerReconciler) IsInitialized(instance *redhatcopv1alpha1.ResourceLocker) bool {
	// assume nothing has changed until something causes a change.
	needsUpdate := false
	if instance.Spec.ServiceAccountRef.Name == "" {
		instance.Spec.ServiceAccountRef.Name = "default"
		needsUpdate = true
	}

	for i := range instance.Spec.Resources {
		defaultSet := strset.New(defaultExcludedPaths...)
		currentSet := strset.New(instance.Spec.Resources[i].ExcludedPaths...)
		if !currentSet.IsEqual(strset.Union(defaultSet, currentSet)) {
			instance.Spec.Resources[i].ExcludedPaths = strset.Union(defaultSet, currentSet).List()
			needsUpdate = true
		}

		// If the resource doesn't have a namespace specified and is a namespaced object,
		// we add the namespace of the ResourceLocker CR.
		lockedResource, err := r.getLockedResource(&instance.Spec.Resources[i].Object)
		if err != nil {
			// failed to get the unstructured object from the resources array. If we fail here
			// we cannot inject anything into the resource so we need the else clause below.
			r.Log.Error(err, "Unable to get locked resource for object: ", "object", instance.Spec.Resources[i].Object)
		} else {
			// check if the resource has the namespace field already set. If not, check if we need
			// to fix it.
			var isNamespaced bool
			if lockedResource.GetNamespace() == "" {
				// the namespace field is blank so we check to see if it's a namespaced or cluster-scoped resource.
				// only namespaced resources need a namespace injected.
				isNamespaced, err = r.isNamespaced(lockedResource)
				if err != nil {
					// we were unable to get info from the server telling us the resource was namespaced.
					r.Log.Error(err, "Unable to determine if locked resource is namespaced", "object", instance.Spec.Resources[i].Object)
				}
			}

			if isNamespaced {
				// the resource is namespaced and is missing a namespace value, so we need
				// to inject one.
				r.Log.V(1).Info("Locked object found without namespace. Injecting namespace for namespaced object.",
					"Object.Metadata.Name", lockedResource.GetName(),
					"NamespaceToSet", instance.Namespace)
				lockedResource.SetNamespace(instance.Namespace)
				bb, err := yaml.Marshal(lockedResource)
				if err != nil {
					// Somehow we've corrupted the yaml in translation.
					r.Log.Error(err, "Unable to marshal the locked resource to yaml", "lockedResource", lockedResource, "instance", instance)
				} else {
					jsonbb, err := yaml.YAMLToJSON(bb)
					if err != nil {
						r.Log.Error(err, "Error transforming yaml to json", "bytes", bb)
					} else {
						// If we got here, the update and the namespace injection was successful
						// so we update needsUpdate
						instance.Spec.Resources[i].Object.Raw = jsonbb
						needsUpdate = true
					}
				}
			}
		}
	}

	for i := range instance.Spec.Patches {
		if instance.Spec.Patches[i].PatchType == "" {
			instance.Spec.Patches[i].PatchType = types.StrategicMergePatchType
			needsUpdate = true
		}
	}
	if len(instance.Spec.Resources)+len(instance.Spec.Patches) > 0 && !util.HasFinalizer(instance, controllerName) {
		util.AddFinalizer(instance, controllerName)
		needsUpdate = true
	}
	if len(instance.Spec.Resources)+len(instance.Spec.Patches) == 0 && util.HasFinalizer(instance, controllerName) {
		util.RemoveFinalizer(instance, controllerName)
		needsUpdate = true
	}

	return !needsUpdate
}

// isNamespaced returns true if the provided resource is namespaced.
func (r *ResourceLockerReconciler) isNamespaced(object *unstructured.Unstructured) (bool, error) {
	var namespaced bool

	// establish the discovery client
	dc, err := r.GetDiscoveryClient()
	if err != nil {
		r.Log.Error(err, "Unable to get discovery client while "+
			"trying to determine if object is namespaced.")
		return namespaced, err
	}

	// get gvk for the unstructured object
	gvk := object.GroupVersionKind()

	// get the resources for the relevant gv
	resourceList, err := dc.ServerResourcesForGroupVersion(gvk.GroupVersion().String())
	if err != nil {
		r.Log.Error(err, "Unable to get server resources for group version while "+
			"trying to determine if object is namespaced",
			"GroupVersion", gvk.GroupVersion().String(),
			"Object", object.GetName())
		return namespaced, err
	}

	// for the returned resources, check to see if they're namespaced
	for _, apiResource := range resourceList.APIResources {
		if apiResource.Kind == gvk.Kind {
			namespaced = apiResource.Namespaced
			return namespaced, nil
		}
	}

	return namespaced, errs.New("unable to find type: " + gvk.String() + " in server")
}

func (r *ResourceLockerReconciler) getRestConfigFromInstance(context context.Context, instance *redhatcopv1alpha1.ResourceLocker) (*rest.Config, error) {
	sa := corev1.ServiceAccount{}
	err := r.GetClient().Get(context, types.NamespacedName{Name: instance.Spec.ServiceAccountRef.Name, Namespace: instance.GetNamespace()}, &sa)
	if err != nil {
		r.Log.Error(err, "unable to get the specified", "service account", types.NamespacedName{Name: instance.Spec.ServiceAccountRef.Name, Namespace: instance.GetNamespace()})
		return &rest.Config{}, err
	}
	var tokenSecret corev1.Secret
	for _, secretRef := range sa.Secrets {
		secret := corev1.Secret{}
		err := r.GetClient().Get(context, types.NamespacedName{Name: secretRef.Name, Namespace: instance.GetNamespace()}, &secret)
		if err != nil {
			r.Log.Error(err, "(ignoring) unable to get ", "ref secret", types.NamespacedName{Name: secretRef.Name, Namespace: instance.GetNamespace()})
			continue
		}
		if secret.Type == "kubernetes.io/service-account-token" {
			tokenSecret = secret
			break
		}
	}
	if tokenSecret.Data == nil {
		err = errs.New("unable to find secret of type kubernetes.io/service-account-token")
		r.Log.Error(err, "unable to find secret of type kubernetes.io/service-account-token for", "service account", sa)
		return &rest.Config{}, err
	}
	//KUBERNETES_SERVICE_PORT=443
	//KUBERNETES_SERVICE_HOST=172.30.0.1
	kubehost, found := os.LookupEnv("KUBERNETES_SERVICE_HOST")
	if !found {
		err = errs.New("unable to lookup environment variable KUBERNETES_SERVICE_HOST")
		r.Log.Error(err, "KUBERNETES_SERVICE_HOST not found")
		return &rest.Config{}, err
	}
	kubeport, found := os.LookupEnv("KUBERNETES_SERVICE_PORT")
	if !found {
		err = errs.New("unable to lookup environment variable KUBERNETES_SERVICE_PORT")
		r.Log.Error(err, "KUBERNETES_SERVICE_PORT not found")
		return &rest.Config{}, err
	}
	config := rest.Config{
		Host:        "https://" + kubehost + ":" + kubeport,
		BearerToken: string(tokenSecret.Data["token"]),
		TLSClientConfig: rest.TLSClientConfig{
			CAData: tokenSecret.Data["ca.crt"],
		},
	}
	return &config, nil
}

// manageCleanupLogic delete resources. We don't touch pacthes because we cannot undo them.
func (r *ResourceLockerReconciler) manageCleanUpLogic(instance *redhatcopv1alpha1.ResourceLocker) error {
	r.Log.V(1).Info("terminating resource locker", "instance", instance)
	err := r.Terminate(instance, true)
	if err != nil {
		r.Log.Error(err, "unable to terminate enforcing reconciler for", "instance", instance)
		return err
	}
	return nil
}

func (r *ResourceLockerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&redhatcopv1alpha1.ResourceLocker{}, builder.WithPredicates(util.ResourceGenerationOrFinalizerChangedPredicate{})).
		Watches(&source.Channel{Source: r.GetStatusChangeChannel()}, &handler.EnqueueRequestForObject{}).
		Complete(r)
}
