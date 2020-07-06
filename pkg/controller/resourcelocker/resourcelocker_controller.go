package resourcelocker

import (
	"context"
	"encoding/json"
	errs "errors"
	"os"
	"text/template"

	"github.com/redhat-cop/operator-utils/pkg/util"
	"github.com/redhat-cop/operator-utils/pkg/util/lockedresourcecontroller"
	"github.com/redhat-cop/operator-utils/pkg/util/lockedresourcecontroller/lockedpatch"
	"github.com/redhat-cop/operator-utils/pkg/util/lockedresourcecontroller/lockedresource"
	redhatcopv1alpha1 "github.com/redhat-cop/resource-locker-operator/pkg/apis/redhatcop/v1alpha1"
	strset "github.com/scylladb/go-set/strset"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"sigs.k8s.io/yaml"
)

const controllerName = "resourcelocker-controller"

var log = logf.Log.WithName(controllerName)

//var statusChange = make(chan event.GenericEvent)

type ReconcileResourceLocker struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	lockedresourcecontroller.EnforcingReconciler
}

// type ResourceLocker struct {
// 	LockedResourceReconcilers []*lockedresourcecontroller.LockedResourceReconciler
// 	LockedPatchReconcilers    []*patchlocker.LockedPatchReconciler
// 	Manager                   stoppablemanager.StoppableManager
// }

// func (r *ResourceLocker) getResources() []unstructured.Unstructured {
// 	resources := []unstructured.Unstructured{}
// 	for _, reconciler := range r.LockedResourceReconcilers {
// 		resources = append(resources, reconciler.Resource)
// 	}
// 	return resources
// }

// func (r *ResourceLocker) getPatches() []patchlocker.Patch {
// 	patches := []patchlocker.Patch{}
// 	for _, reconciler := range r.LockedPatchReconcilers {
// 		patches = append(patches, reconciler.Patch)
// 	}
// 	return patches
// }

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new ResourceLocker Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) *ReconcileResourceLocker {
	return &ReconcileResourceLocker{
		EnforcingReconciler: lockedresourcecontroller.NewEnforcingReconciler(mgr.GetClient(), mgr.GetScheme(), mgr.GetConfig(), mgr.GetEventRecorderFor(controllerName), false),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r *ReconcileResourceLocker) error {
	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource ResourceLocker
	err = c.Watch(&source.Kind{Type: &redhatcopv1alpha1.ResourceLocker{
		TypeMeta: metav1.TypeMeta{
			Kind: "ResourceLocker",
		}}}, &handler.EnqueueRequestForObject{}, util.ResourceGenerationOrFinalizerChangedPredicate{})
	if err != nil {
		return err
	}

	// watch for changes in status in the locked resources

	//if interested in updates from the managed resources
	// watch for changes in status in the locked resources
	err = c.Watch(
		&source.Channel{Source: r.GetStatusChangeChannel()},
		&handler.EnqueueRequestForObject{},
	)
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileResourceLocker implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileResourceLocker{}

// ReconcileResourceLocker reconciles a ResourceLocker object

// Reconcile reads that state of the cluster for a ResourceLocker object and makes changes based on the state read
// and what is in the ResourceLocker.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileResourceLocker) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling ResourceLocker")

	// Fetch the ResourceLocker instance
	instance := &redhatcopv1alpha1.ResourceLocker{}
	err := r.GetClient().Get(context.TODO(), request.NamespacedName, instance)
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

	if ok, err := r.IsValid(instance); !ok {
		return r.ManageError(instance, err)
	}

	if !r.IsInitialized(instance) {
		err := r.GetClient().Update(context.TODO(), instance)
		if err != nil {
			log.Error(err, "unable to update instance", "instance", instance)
			return r.ManageError(instance, err)
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
			return r.ManageError(instance, err)
		}
		util.RemoveFinalizer(instance, controllerName)
		err = r.GetClient().Update(context.TODO(), instance)
		if err != nil {
			log.Error(err, "unable to update instance", "instance", instance)
			return r.ManageError(instance, err)
		}
		return reconcile.Result{}, nil
	}

	// TODO manage lifecycle

	lockedResources, err := getLockedResources(instance)
	if err != nil {
		log.Error(err, "unable to get resources for", "instance", instance)
		return r.ManageError(instance, err)
	}

	lockedPatches, err := getPatches(instance)
	if err != nil {
		log.Error(err, "unable to get patches for", "instance", instance)
		return r.ManageError(instance, err)
	}

	config, err := r.getRestConfigFromInstance(instance)
	if err != nil {
		log.Error(err, "unable to get restconfig for", "instance", instance)
		return r.ManageError(instance, err)
	}

	err = r.UpdateLockedResourcesWithRestConfig(instance, lockedResources, lockedPatches, config)
	if err != nil {
		log.Error(err, "unable to update locked resources")
		return r.ManageError(instance, err)
	}

	return r.ManageSuccess(instance)
}

// func getKeyFromInstance(instance *redhatcopv1alpha1.ResourceLocker) string {
// 	return types.NamespacedName{
// 		Name:      instance.GetName(),
// 		Namespace: instance.GetNamespace(),
// 	}.String()
// }

func getLockedResource(raw *runtime.RawExtension) (*unstructured.Unstructured, error) {
	bb, err := yaml.YAMLToJSON(raw.Raw)
	if err != nil {
		log.Error(err, "Error transforming yaml to json", "raw", raw.Raw)
		return &unstructured.Unstructured{}, err
	}
	obj := &unstructured.Unstructured{}
	err = json.Unmarshal(bb, obj)
	if err != nil {
		log.Error(err, "Error unmarshalling json manifest", "manifest", string(bb))
		return &unstructured.Unstructured{}, err
	}
	return obj, nil
}

func getLockedResources(instance *redhatcopv1alpha1.ResourceLocker) ([]lockedresource.LockedResource, error) {
	objs := []lockedresource.LockedResource{}
	for _, resource := range instance.Spec.Resources {
		bb, err := yaml.YAMLToJSON(resource.Object.Raw)
		if err != nil {
			log.Error(err, "Error transforming yaml to json", "raw", resource.Object.Raw)
			return []lockedresource.LockedResource{}, err
		}
		obj := &lockedresource.LockedResource{}
		err = json.Unmarshal(bb, &obj.Object)
		if err != nil {
			log.Error(err, "Error unmarshalling json manifest", "manifest", string(bb))
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
func (r *ReconcileResourceLocker) IsInitialized(instance *redhatcopv1alpha1.ResourceLocker) bool {
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
		lockedResource, err := getLockedResource(&instance.Spec.Resources[i].Object)
		if err != nil {
			// failed to get the unstructured object from the resources array. If we fail here
			// we cannot inject anything into the resource so we need the else clause below.
			log.Error(err, "Unable to get locked resource for object: ", "object", instance.Spec.Resources[i].Object)
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
					log.Error(err, "Unable to determine if locked resource is namespaced", "object", instance.Spec.Resources[i].Object)
				}
			}

			if isNamespaced {
				// the resource is namespaced and is missing a namespace value, so we need
				// to inject one.
				log.V(1).Info("Locked object found without namespace. Injecting namespace for namespaced object.",
					"Object.Metadata.Name", lockedResource.GetName(),
					"NamespaceToSet", instance.Namespace)
				lockedResource.SetNamespace(instance.Namespace)
				bb, err := yaml.Marshal(lockedResource)
				if err != nil {
					// Somehow we've corrupted the yaml in translation.
					log.Error(err, "Unable to marshal the locked resource to yaml", "lockedResource", lockedResource, "instance", instance)
				} else {
					jsonbb, err := yaml.YAMLToJSON(bb)
					if err != nil {
						log.Error(err, "Error transforming yaml to json", "bytes", bb)
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
	if len(instance.Spec.Resources) > 0 && !util.HasFinalizer(instance, controllerName) {
		util.AddFinalizer(instance, controllerName)
		needsUpdate = true
	}
	if len(instance.Spec.Resources) == 0 && util.HasFinalizer(instance, controllerName) {
		util.RemoveFinalizer(instance, controllerName)
		needsUpdate = true
	}

	return !needsUpdate
}

// isNamespaced returns true if the provided resource is namespaced.
func (r *ReconcileResourceLocker) isNamespaced(object *unstructured.Unstructured) (bool, error) {
	var namespaced bool

	// establish the discovery client
	dc, err := r.GetDiscoveryClient()
	if err != nil {
		log.Error(err, "Unable to get discovery client while "+
			"trying to determine if object is namespaced.")
		return namespaced, err
	}

	// get gvk for the unstructured object
	gvk := object.GroupVersionKind()

	// get the resources for the relevant gv
	resourceList, err := dc.ServerResourcesForGroupVersion(gvk.GroupVersion().String())
	if err != nil {
		log.Error(err, "Unable to get server resources for group version while "+
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

// func (r *ReconcileResourceLocker) getResourceLockerFromInstance(instance *redhatcopv1alpha1.ResourceLocker) (*ResourceLocker, error) {
// 	config, err := r.getRestConfigFromInstance(instance)
// 	if err != nil {
// 		log.Error(err, "unable to get rest config", "for service account", instance, instance.Spec.ServiceAccountRef)
// 		return &ResourceLocker{}, err
// 	}
// 	stoppableManager, err := stoppablemanager.NewStoppableManager(config, manager.Options{
// 		MetricsBindAddress: "0",
// 		LeaderElection:     false,
// 	})
// 	if err != nil {
// 		log.Error(err, "unable to create stoppable manager")
// 		return &ResourceLocker{}, err
// 	}
// 	resourceReconcilers := []*lockedresourcecontroller.LockedResourceReconciler{}
// 	for _, resource := range instance.Spec.Resources {
// 		lockedResource, err := getLockedResource(&resource.Object)
// 		if err != nil {
// 			log.Error(err, "unable to create lockedResource", "for object", resource.Object)
// 			return &ResourceLocker{}, err
// 		}
// 		reconciler, err := lockedresourcecontroller.NewLockedObjectReconciler(stoppableManager.Manager, *lockedResource, resource.ExcludedPaths, r.statusChange, instance)
// 		if err != nil {
// 			log.Error(err, "unable to create reconciler", "for locked resource", lockedResource)
// 			return &ResourceLocker{}, err
// 		}
// 		resourceReconcilers = append(resourceReconcilers, reconciler)
// 	}
// 	patchReconcilers := []*patchlocker.LockedPatchReconciler{}
// 	patches, err := getPatches(instance)
// 	if err != nil {
// 		log.Error(err, "unable to get patches", "from instance", instance)
// 		return &ResourceLocker{}, err
// 	}
// 	for _, patch := range patches {
// 		reconciler, err := patchlocker.NewPatchLockerReconciler(stoppableManager.Manager, patch, r.statusChange, instance)
// 		if err != nil {
// 			log.Error(err, "unable to create reconciler", "for patch", patch)
// 			return &ResourceLocker{}, err
// 		}
// 		patchReconcilers = append(patchReconcilers, reconciler)
// 	}
// 	return &ResourceLocker{
// 		LockedResourceReconcilers: resourceReconcilers,
// 		LockedPatchReconcilers:    patchReconcilers,
// 		Manager:                   stoppableManager,
// 	}, nil
// }

func (r *ReconcileResourceLocker) getRestConfigFromInstance(instance *redhatcopv1alpha1.ResourceLocker) (*rest.Config, error) {
	sa := corev1.ServiceAccount{}
	err := r.GetClient().Get(context.TODO(), types.NamespacedName{Name: instance.Spec.ServiceAccountRef.Name, Namespace: instance.GetNamespace()}, &sa)
	if err != nil {
		log.Error(err, "unable to get the specified", "service account", types.NamespacedName{Name: instance.Spec.ServiceAccountRef.Name, Namespace: instance.GetNamespace()})
		return &rest.Config{}, err
	}
	var tokenSecret corev1.Secret
	for _, secretRef := range sa.Secrets {
		secret := corev1.Secret{}
		err := r.GetClient().Get(context.TODO(), types.NamespacedName{Name: secretRef.Name, Namespace: instance.GetNamespace()}, &secret)
		if err != nil {
			log.Error(err, "(ignoring) unable to get ", "ref secret", types.NamespacedName{Name: secretRef.Name, Namespace: instance.GetNamespace()})
			continue
		}
		if secret.Type == "kubernetes.io/service-account-token" {
			tokenSecret = secret
			break
		}
	}
	if tokenSecret.Data == nil {
		err = errs.New("unable to find secret of type kubernetes.io/service-account-token")
		log.Error(err, "unable to find secret of type kubernetes.io/service-account-token for", "service account", sa)
		return &rest.Config{}, err
	}
	//KUBERNETES_SERVICE_PORT=443
	//KUBERNETES_SERVICE_HOST=172.30.0.1
	kubehost, found := os.LookupEnv("KUBERNETES_SERVICE_HOST")
	if !found {
		err = errs.New("unable to lookup environment variable KUBERNETES_SERVICE_HOST")
		log.Error(err, "KUBERNETES_SERVICE_HOST not found")
		return &rest.Config{}, err
	}
	kubeport, found := os.LookupEnv("KUBERNETES_SERVICE_PORT")
	if !found {
		err = errs.New("unable to lookup environment variable KUBERNETES_SERVICE_PORT")
		log.Error(err, "KUBERNETES_SERVICE_PORT not found")
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

func getPatches(instance *redhatcopv1alpha1.ResourceLocker) ([]lockedpatch.LockedPatch, error) {
	patches := []lockedpatch.LockedPatch{}
	for _, patch := range instance.Spec.Patches {
		myPatch := lockedpatch.LockedPatch{
			SourceObjectRefs: patch.SourceObjectRefs,
			TargetObjectRef:  patch.TargetObjectRef,
			PatchType:        patch.PatchType,
			PatchTemplate:    patch.PatchTemplate,
			ID:               patch.ID,
		}

		template, err := template.New(patch.PatchTemplate).Parse(patch.PatchTemplate)
		if err != nil {
			log.Error(err, "unable to parse ", "template", patch.PatchTemplate)
			return []lockedpatch.LockedPatch{}, err
		}
		myPatch.Template = *template
		patches = append(patches, myPatch)
	}
	return patches, nil
}

// func getPatchName(instance *redhatcopv1alpha1.ResourceLocker, patch *patchlocker.Patch) string {
// 	return types.NamespacedName{
// 		Name:      instance.GetName(),
// 		Namespace: instance.GetNamespace(),
// 	}.String() + "-" + types.NamespacedName{
// 		Name:      patch.TargetObjectRef.Name,
// 		Namespace: patch.TargetObjectRef.Namespace,
// 	}.String()
// }

// func getResourceName(obj *unstructured.Unstructured) string {
// 	return schema.FromAPIVersionAndKind(obj.GetAPIVersion(), obj.GetKind()).String() + "-" + types.NamespacedName{
// 		Name:      obj.GetName(),
// 		Namespace: obj.GetNamespace(),
// 	}.String()
// }

// type resourceGenerationOrFinalizerChangedPredicate struct {
// 	predicate.Funcs
// }

// // Update implements default UpdateEvent filter for validating resource version change
// func (resourceGenerationOrFinalizerChangedPredicate) Update(e event.UpdateEvent) bool {
// 	if e.MetaOld == nil {
// 		log.Error(nil, "UpdateEvent has no old metadata", "event", e)
// 		return false
// 	}
// 	if e.ObjectOld == nil {
// 		log.Error(nil, "GenericEvent has no old runtime object to update", "event", e)
// 		return false
// 	}
// 	if e.ObjectNew == nil {
// 		log.Error(nil, "GenericEvent has no new runtime object for update", "event", e)
// 		return false
// 	}
// 	if e.MetaNew == nil {
// 		log.Error(nil, "UpdateEvent has no new metadata", "event", e)
// 		return false
// 	}
// 	if e.MetaNew.GetGeneration() == e.MetaOld.GetGeneration() && reflect.DeepEqual(e.MetaNew.GetFinalizers(), e.MetaOld.GetFinalizers()) {
// 		return false
// 	}
// 	return true
// }

// func (r *ReconcileResourceLocker) manageError(instance *redhatcopv1alpha1.ResourceLocker, err1 error) (reconcile.Result, error) {
// 	status := v1alpha1.ResourceLockerStatus{
// 		Type:           v1alpha1.Failure,
// 		Status:         corev1.ConditionTrue,
// 		LastUpdateTime: metav1.Now(),
// 		Message:        err1.Error(),
// 	}
// 	instance.Status = status
// 	r.addResourcesAndPacthesStatuses(instance)
// 	err := r.GetClient().Status().Update(context.Background(), instance)
// 	if err != nil {
// 		log.Error(err, "unable to update status")
// 		return reconcile.Result{
// 			RequeueAfter: time.Second,
// 			Requeue:      true,
// 		}, nil
// 	}
// 	return reconcile.Result{}, err1
// }

// func getObjectReferencesFromStatus(instance *redhatcopv1alpha1.ResourceLocker) []corev1.ObjectReference {
// 	var res = []corev1.ObjectReference{}
// 	for _, resStatus := range instance.Status.ResourceStatuses {
// 		res = append(res, resStatus.ObjectReference)
// 	}
// 	return res
// }

// delete removed resources. This is a best effort implelemtnation for now.
// we use the last "kubectl.kubernetes.io/last-applied-configuration" annotation to copare and see if resources were deleted
// this seems the best we can do without using an external storage
// we ignore patcher because we can't undo them
// func (r *ReconcileResourceLocker) deleteRemovedResources(instance *redhatcopv1alpha1.ResourceLocker) error {
// 	// we create a set of all the possible resources that might have been managed in the past
// 	lastAppliedObjs, err := getLastAppliedResources(instance)
// 	if err != nil {
// 		log.Error(err, "unable to get resources from last applied annotation", "instance", instance)
// 		lastAppliedObjs = []unstructured.Unstructured{}
// 	}
// 	var lastAppliedRes = []unstructured.Unstructured{}
// 	resourceLocker, ok := r.ResourceLockers[getKeyFromInstance(instance)]
// 	if ok {
// 		lastAppliedRes = resourceLocker.getResources()
// 	}
// 	// past objects represent the union of all the past locked resources from all the known sources
// 	pastObjectRefs := objectreferenceset.Union(objectreferenceset.New(getObjectReferences(lastAppliedObjs)...), objectreferenceset.New(getObjectReferences(lastAppliedRes)...), objectreferenceset.New(getObjectReferencesFromStatus(instance)...))

// 	desiredObjs, err := getLockedResources(instance)
// 	if err != nil {
// 		return err
// 	}
// 	desiredObjectRefs := objectreferenceset.New(getObjectReferences(desiredObjs)...)

// 	toBeDeletedRefs := objectreferenceset.Difference(pastObjectRefs, desiredObjectRefs)
// 	for _, ref := range toBeDeletedRefs.List() {
// 		obj := getUnstructuredFromRef(&ref)
// 		err := r.DeleteResourceIfExists(&obj)
// 		if err != nil {
// 			if !errors.IsNotFound(err) {
// 				return err
// 			}
// 		}
// 	}
// 	return nil
// }

// func getUnstructuredFromRef(ref *corev1.ObjectReference) unstructured.Unstructured {
// 	res := unstructured.Unstructured{}
// 	res.SetGroupVersionKind(ref.GroupVersionKind())
// 	res.SetName(ref.Name)
// 	res.SetNamespace(ref.Namespace)
// 	return res
// }

// const lastAppliedAnnotation = "kubectl.kubernetes.io/last-applied-configuration"

// func getLastAppliedResources(instance *redhatcopv1alpha1.ResourceLocker) ([]unstructured.Unstructured, error) {
// 	lastApplied, ok := instance.GetAnnotations()[lastAppliedAnnotation]
// 	if !ok {
// 		return []unstructured.Unstructured{}, nil
// 	}
// 	obj := redhatcopv1alpha1.ResourceLocker{}
// 	err := json.Unmarshal([]byte(lastApplied), &obj)
// 	if err != nil {
// 		return []unstructured.Unstructured{}, err
// 	}
// 	return getLockedResources(&obj)

// }

// func sameObj(left unstructured.Unstructured, right unstructured.Unstructured) bool {
// 	return left.GetName() == right.GetName() &&
// 		left.GetNamespace() == right.GetNamespace() &&
// 		left.GetObjectKind().GroupVersionKind().GroupKind() == right.GetObjectKind().GroupVersionKind().GroupKind()
// }

// func leftOuterJoin(left []unstructured.Unstructured, right []unstructured.Unstructured) []unstructured.Unstructured {
// 	res := []unstructured.Unstructured{}
// 	for _, leftObj := range left {
// 		for _, rightObj := range right {
// 			if sameObj(leftObj, rightObj) {
// 				res = append(res, leftObj)
// 			}
// 		}
// 	}
// 	return res
// }

// manageCleanupLogic delete resources. We don't touch pacthes because we cannot undo them.
func (r *ReconcileResourceLocker) manageCleanUpLogic(instance *redhatcopv1alpha1.ResourceLocker) error {
	err := r.Terminate(instance, true)
	if err != nil {
		log.Error(err, "unable to terminate enforcing reconciler for", "instance", instance)
		return err
	}
	return nil
}

// func (r *ReconcileResourceLocker) manageSuccess(instance *redhatcopv1alpha1.ResourceLocker) (reconcile.Result, error) {
// 	status := v1alpha1.ResourceLockerStatus{
// 		Type:           v1alpha1.Success,
// 		Status:         corev1.ConditionTrue,
// 		LastUpdateTime: metav1.Now(),
// 	}
// 	instance.Status = status
// 	r.addResourcesAndPacthesStatuses(instance)
// 	err := r.GetClient().Status().Update(context.Background(), instance)
// 	if err != nil {
// 		log.Error(err, "unable to update status", "for instance", instance)
// 		return reconcile.Result{
// 			RequeueAfter: time.Second,
// 			Requeue:      true,
// 		}, nil
// 	}
// 	return reconcile.Result{}, nil
// }

// func getObjectReference(obj *unstructured.Unstructured) corev1.ObjectReference {
// 	return corev1.ObjectReference{
// 		Kind:       obj.GetKind(),
// 		APIVersion: obj.GetAPIVersion(),
// 		Name:       obj.GetName(),
// 		Namespace:  obj.GetNamespace(),
// 	}
// }

// func getObjectReferences(objs []unstructured.Unstructured) []corev1.ObjectReference {
// 	var res []corev1.ObjectReference
// 	for _, obj := range objs {
// 		res = append(res, getObjectReference(&obj))
// 	}
// 	return res
// }

// func (r *ReconcileResourceLocker) addResourcesAndPacthesStatuses(instance *redhatcopv1alpha1.ResourceLocker) {
// 	resourceLocker, ok := r.ResourceLockers[getKeyFromInstance(instance)]
// 	if !ok {
// 		return
// 	}
// 	resourceStatuses := []v1alpha1.LockingStatus{}
// 	for _, reconciler := range resourceLocker.LockedResourceReconcilers {
// 		resourceStatuses = append(resourceStatuses, v1alpha1.LockingStatus{
// 			ObjectReference: getObjectReference(&reconciler.Resource),
// 			Type:            v1alpha1.ConditionType(reconciler.GetStatus().Type),
// 			Status:          getStatusOrUnknown(reconciler.GetStatus().Status),
// 			LastUpdateTime:  getTimeOrNow(reconciler.GetStatus().LastUpdateTime),
// 			Message:         reconciler.GetStatus().Message,
// 		})
// 	}
// 	instance.Status.ResourceStatuses = resourceStatuses
// 	patchStatuses := []v1alpha1.LockingStatus{}
// 	for _, reconciler := range resourceLocker.LockedPatchReconcilers {
// 		patchStatuses = append(patchStatuses, v1alpha1.LockingStatus{
// 			ObjectReference: reconciler.Patch.TargetObjectRef,
// 			Type:            v1alpha1.ConditionType(reconciler.GetStatus().Type),
// 			Status:          getStatusOrUnknown(reconciler.GetStatus().Status),
// 			LastUpdateTime:  getTimeOrNow(reconciler.GetStatus().LastUpdateTime),
// 			Message:         reconciler.GetStatus().Message,
// 		})
// 	}
// 	instance.Status.PatchStatuses = patchStatuses
// }

// func getStatusOrUnknown(status corev1.ConditionStatus) corev1.ConditionStatus {
// 	if status == "" {
// 		return corev1.ConditionUnknown
// 	}
// 	return status
// }

// func getTimeOrNow(time metav1.Time) metav1.Time {
// 	if time.IsZero() {
// 		return metav1.Now()
// 	}
// 	return time
// }

// type genericEventPredicateLogger struct {
// 	predicate.Funcs
// }

// func (gep genericEventPredicateLogger) Generic(e event.GenericEvent) bool {
// 	return true
// }
