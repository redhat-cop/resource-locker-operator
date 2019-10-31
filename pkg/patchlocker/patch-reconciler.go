package patchlocker

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"text/template"

	"github.com/redhat-cop/operator-utils/pkg/util"
	"github.com/redhat-cop/resource-locker-operator/pkg/apis/redhatcop/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/util/jsonpath"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var controllername = "controller_patchlocker"

var log = logf.Log.WithName(controllername)

type Patch struct {
	v1alpha1.Patch
	template.Template
}

type LockedPatchReconciler struct {
	util.ReconcilerBase
	Patch
}

// NewReconciler returns a new reconcile.Reconciler
func NewPatchLockerReconciler(mgr manager.Manager, patch Patch) (reconcile.Reconciler, error) {

	// TODO create the object is it does not exists

	reconciler := &LockedPatchReconciler{
		ReconcilerBase: util.NewReconcilerBase(mgr.GetClient(), mgr.GetScheme(), mgr.GetConfig(), mgr.GetEventRecorderFor(controllername+"_"+getKeyFromPatch(patch))),
		Patch:          patch,
	}

	controller, err := controller.New(controllername+"_"+getKeyFromPatch(patch), mgr, controller.Options{Reconciler: reconciler})
	if err != nil {
		return &LockedPatchReconciler{}, err
	}

	//create watcher for target
	gvk := getGVKfromReference(&patch.TargetObjectRef)
	groupVersion := schema.GroupVersion{Group: gvk.Group, Version: gvk.Version}
	obj := objectRefToRuntimeType(&patch.TargetObjectRef)
	mgr.GetScheme().AddKnownTypes(groupVersion, obj)

	err = controller.Watch(&source.Kind{Type: obj}, &enqueueRequestForPatch{
		reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      patch.TargetObjectRef.Name,
				Namespace: patch.TargetObjectRef.Namespace,
			},
		},
	}, &referenceModifiedPredicate{
		ObjectReference: patch.TargetObjectRef,
	})
	if err != nil {
		return &LockedPatchReconciler{}, err
	}

	for _, sourceRef := range patch.SourceObjectRefs {
		gvk := getGVKfromReference(&patch.TargetObjectRef)
		groupVersion := schema.GroupVersion{Group: gvk.Group, Version: gvk.Version}
		obj := objectRefToRuntimeType(&patch.TargetObjectRef)
		mgr.GetScheme().AddKnownTypes(groupVersion, obj)
		err = controller.Watch(&source.Kind{Type: obj}, &enqueueRequestForPatch{
			reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      sourceRef.Name,
					Namespace: sourceRef.Namespace,
				},
			},
		}, &referenceModifiedPredicate{
			ObjectReference: sourceRef,
		})
		if err != nil {
			return &LockedPatchReconciler{}, err
		}
	}

	return reconciler, nil
}

func getKeyFromPatch(patch Patch) string {
	return patch.TargetObjectRef.String()
}

func getGVKfromReference(objref *corev1.ObjectReference) schema.GroupVersionKind {
	return schema.FromAPIVersionAndKind(objref.APIVersion, objref.Kind)
}

func objectRefToRuntimeType(objref *corev1.ObjectReference) runtime.Object {
	obj := &unstructured.Unstructured{}
	obj.SetKind(objref.Kind)
	obj.SetAPIVersion(objref.APIVersion)
	obj.SetNamespace(objref.Namespace)
	obj.SetName(objref.Name)
	return obj
}

type enqueueRequestForPatch struct {
	reconcile.Request
}

var enqueueLog = logf.Log.WithName("eventhandler").WithName("EnqueueRequestForObject")

func (e *enqueueRequestForPatch) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	//enqueueLog.Info("CreateEvent received ", "event", evt)
	q.Add(e.Request)
}

// Update implements EventHandler
func (e *enqueueRequestForPatch) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	//enqueueLog.Info("UpdateEvent received ", "event", evt)
	q.Add(e.Request)
}

// Delete implements EventHandler
func (e *enqueueRequestForPatch) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	//enqueueLog.Info("DeleteEvent received ", "event", evt)
	q.Add(e.Request)
}

// Generic implements EventHandler
func (e *enqueueRequestForPatch) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
	//enqueueLog.Info("GenericEvent received ", "event", evt)
	q.Add(e.Request)
}

type referenceModifiedPredicate struct {
	corev1.ObjectReference
	predicate.Funcs
}

var predicateLog = logf.Log.WithName("predicate").WithName("ReferenceModifiedPredicate")

// Update implements default UpdateEvent filter for validating resource version change
func (p *referenceModifiedPredicate) Update(e event.UpdateEvent) bool {
	//predicateLog.Info("UpdateEvent received ", "event", e)
	if e.MetaNew.GetName() == p.ObjectReference.Name && e.MetaNew.GetNamespace() == p.ObjectReference.Namespace {
		return true
	}
	return false
}

func (p *referenceModifiedPredicate) Create(e event.CreateEvent) bool {
	//predicateLog.Info("CreateEvent received ", "event", e)
	if e.Meta.GetName() == p.ObjectReference.Name && e.Meta.GetNamespace() == p.ObjectReference.Namespace {
		return true
	}
	return false
}

func (p *referenceModifiedPredicate) Delete(e event.DeleteEvent) bool {
	//predicateLog.Info("DeleteEvent received ", "event", e)
	// we ignore Delete events because if we loosing references there is no point in trying to recompute the patch
	return false
}

func (p *referenceModifiedPredicate) Generic(e event.GenericEvent) bool {
	//predicateLog.Info("GenericEvent received ", "event", e)
	// we ignore Generic events
	return false
}

func (lpr *LockedPatchReconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	//gather all needed the objects
	targetObj, err := lpr.getReferecedObject(&lpr.TargetObjectRef)
	if err != nil {
		log.Error(err, "unable to retrieve", "target", lpr.TargetObjectRef)
		return reconcile.Result{}, err
	}
	sourceMaps := []map[string]interface{}{}
	for _, objref := range lpr.SourceObjectRefs {
		sourceObj, err := lpr.getReferecedObject(&objref)
		if err != nil {
			log.Error(err, "unable to retrieve", "source", sourceObj)
			return reconcile.Result{}, err
		}
		sourceMap, err := getSubMapFromObject(sourceObj, objref.FieldPath)
		if err != nil {
			log.Error(err, "unable to retrieve", "field", objref.FieldPath, "from object", sourceObj)
			return reconcile.Result{}, err
		}
		sourceMaps = append(sourceMaps, sourceMap)
	}

	//compute the template
	var b bytes.Buffer
	err = lpr.Template.Execute(&b, sourceMaps)
	if err != nil {
		log.Error(err, "unable to process ", "template", lpr.Template, "parameters", sourceMaps)
		return reconcile.Result{}, err
	}
	log.Info("processed", "template", b.String())
	//apply the patch

	patch := client.ConstantPatch(lpr.PatchType, b.Bytes())

	err = lpr.GetClient().Patch(context.TODO(), targetObj, patch)

	if err != nil {
		log.Error(err, "unable to apply ", "patch", patch, "on target", targetObj)
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (lpr *LockedPatchReconciler) getReferecedObject(objref *corev1.ObjectReference) (*unstructured.Unstructured, error) {
	var ri dynamic.ResourceInterface
	res, err := lpr.getAPIReourceForGVK(schema.FromAPIVersionAndKind(objref.APIVersion, objref.Kind))
	if err != nil {
		log.Error(err, "unable to get resourceAPI ", "objectref", objref)
		return &unstructured.Unstructured{}, err
	}
	nri, err := lpr.GetDynamicClientOnAPIResource(res)
	if err != nil {
		log.Error(err, "unable to get dynamicClient on ", "resourceAPI", res)
		return &unstructured.Unstructured{}, err
	}
	if res.Namespaced {
		ri = nri.Namespace(objref.Namespace)
	} else {
		ri = nri
	}
	obj, err := ri.Get(objref.Name, metav1.GetOptions{})
	if err != nil {
		log.Error(err, "unable to get referenced ", "object", objref)
		return &unstructured.Unstructured{}, err
	}
	return obj, nil
}

func getSubMapFromObject(obj *unstructured.Unstructured, fieldPath string) (map[string]interface{}, error) {
	// look into this: k8s.io/client-go/util/jsonpath
	jp := jsonpath.New("fieldPath")
	err := jp.Parse(fieldPath)
	if err != nil {
		log.Error(err, "unable to parse ", "fieldPath", fieldPath)
		return map[string]interface{}{}, err
	}
	var buf bytes.Buffer
	jp.Execute(&buf, obj)
	log.Info("result ", buf.String())
	values, err := jp.FindResults(obj)
	if err != nil {
		log.Error(err, "unable to apply ", "jsonpath", jp, " to obj ", obj)
		return map[string]interface{}{}, err
	}
	log.Info("results ", values)
	return map[string]interface{}{}, errors.New("not implemented")
}

func (lpr *LockedPatchReconciler) getAPIReourceForGVK(gvk schema.GroupVersionKind) (metav1.APIResource, error) {
	res := metav1.APIResource{}
	discoveryClient, err := lpr.GetDiscoveryClient()
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
			res.Namespaced = resource.Namespaced
			res.Group = gvk.Group
			res.Version = gvk.Version
			break
		}
	}
	return res, nil
}
