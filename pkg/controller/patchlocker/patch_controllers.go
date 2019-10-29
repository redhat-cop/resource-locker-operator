package patchlocker

import (
	"github.com/redhat-cop/operator-utils/pkg/util"
	"github.com/redhat-cop/resource-locker-operator/pkg/apis/redhatcop/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var controllername = "controller_patchlocker"

var log = logf.Log.WithName(controllername)

type LockedPatchReconciler struct {
	util.ReconcilerBase
	v1alpha1.Patch
}

// NewReconciler returns a new reconcile.Reconciler
func NewLockedObjectReconciler(mgr manager.Manager, patch v1alpha1.Patch) (reconcile.Reconciler, error) {

	// TODO create the object is it does not exists

	reconciler := &LockedPatchReconciler{
		ReconcilerBase: util.NewReconcilerBase(mgr.GetClient(), mgr.GetScheme(), mgr.GetConfig(), mgr.GetEventRecorderFor(controllername+"_"+GetKeyFromPatch(&patch))),
		Patch:          patch,
	}

	controller, err := controller.New(controllername+"_"+GetKeyFromPatch(&patch), mgr, controller.Options{Reconciler: reconciler})
	if err != nil {
		return &LockedPatchReconciler{}, err
	}

	//create watcher for target
	gvk := getGVKfromReference(patch.TargetObjectRef)
	groupVersion := schema.GroupVersion{Group: gvk.Group, Version: gvk.Version}
	mgr.GetScheme().AddKnownTypes(groupVersion, &object)

	err = controller.Watch(&source.Kind{Type: &object}, &enqueueRequestForPatch{})
	if err != nil {
		return &LockedPatchReconciler{}, err
	}

	for _,source range patch.SourceObjectRefs {
		gvk := getGVKfromReference(patch.TargetObjectRef)
		groupVersion := schema.GroupVersion{Group: gvk.Group, Version: gvk.Version}
		mgr.GetScheme().AddKnownTypes(groupVersion, &object)
		err = controller.Watch(&source.Kind{Type: &object}, &enqueueRequestForPatch{})
		if err != nil {
			return &LockedPatchReconciler{}, err
		}
	}

	return reconciler, nil
}
