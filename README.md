# Resource Locker Operator

[![Build Status](https://travis-ci.org/redhat-cop/resource-locker-operator.svg?branch=master)](https://travis-ci.org/redhat-cop/resource-locker-operator) [![Docker Repository on Quay](https://quay.io/repository/redhat-cop/resource-locker-operator/status "Docker Repository on Quay")](https://quay.io/repository/redhat-cop/resource-locker-operator)

The resource locker operator allows you to specify a set of configurations that the operator will "keep in place" (lock) preventing any drifts.
Two types of configurations may be specified:

* Resources. This will instruct the operator to create and enforce the specified resource. In this case the operator "owns" the created resources.
* Patches to resources. This will instruct the operator to patch- and enforce the change on- a pre-existing resource. In this case the operator does not "own" the resource.

Locked resources are defined with the `ResourceLocker` CRD. Here is the high-level structure of this CRD:

```yaml
apiVersion: redhatcop.redhat.io/v1alpha1
kind: ResourceLocker
metadata:
  name: test-simple-resource
spec:
  resources:
  - object:
      apiVersion: v1
  ...
  patches:
  - targetObjectRef:
    ...
    patchTemplate: |
      metadata:
        annotations:
          ciao: hello
  ...
  serviceAccountRef:
    name: default
```

It contains:

* `resources`: representing an array of resources
* `patches`: representing an array of patches
* `serviceAccountRef`: a reference to a service account define din the same namespace as the ResourceLocker CR, that will be used to create the resources and apply the patches. If not specified the service account will be defaulted to: `default`

For each ResourceLocker a manager is dynamically allocated. For each resource and patch a controller with the needed watches is created and associated with the previously created manager.

## Deploying the Operator

This is a cluster-level operator that you can deploy in any namespace, `resource-locker-operator` is recommended.

You can either deploy it using [`Helm`](https://helm.sh/) or creating the manifests directly.

### Deploying with Helm

Here are the instructions to install the latest release with Helm.

```shell
oc new-project resource-locker-operator

helm repo add resource-locker-operator https://redhat-cop.github.io/resource-locker-operator
helm repo update
export resource-locker-operator_chart_version=$(helm search resource-locker-operator/resource-locker-operator | grep resource-locker-operator/resource-locker-operator | awk '{print $2}')

helm fetch resource-locker-operator/resource-locker-operator --version ${resource-locker-operator}
helm template resource-locker-operator-${resource-locker-operator}.tgz --namespace resource-locker-operator | oc apply -f - -n resource-locker-operator

rm resource-locker-operator-${resource-locker-operator}.tgz
```

### Deploying directly with manifests

Here are the instructions to install the latest release creating the manifest directly in OCP.

```shell
git clone git@github.com:redhat-cop/resource-locker-operator.git; cd resource-locker-operator
oc apply -f deploy/crds/redhatcop.redhat.io_resourcelockers_crd.yamlredhatcop.redhat.io_resourcelockers_crd.yaml
oc new-project resource-locker-operator
oc -n resource-locker-operator apply -f deploy
```

## Resources Locking

An example of a resource locking configuration is the following:

```yaml
apiVersion: redhatcop.redhat.io/v1alpha1
kind: ResourceLocker
metadata:
  name: test-simple-resource
spec:
  resources:
    - excludedPaths:
        - .metadata
        - .status
        - .spec.replicas
      object:
        apiVersion: v1
        kind: ResourceQuota
        metadata:
          name: small-size
          namespace: resource-locker-test
        spec:
          hard:
            requests.cpu: '4'
            requests.memory: 4Gi
  serviceAccountRef:
    name: default
```

I this example we lock in a ResourceQuota configuration. Resources must be fully specified (i.e. no templating is allowed and all the mandatory fields must be initialized).
Resources created this way are allowed to drift from the desired state only in the `excludedPaths`, which are jsonPath expressions. If drift occurs in other section of the resource, the operator will immediately reset the resource. The following `excludedPaths` are always added:

* `.metadata`
* `.status`
* `.spec.replicas`

and in most cases should be the right choice.

## Resource Patch Locking

An example of patch is the following:

```yaml
apiVersion: redhatcop.redhat.io/v1alpha1
kind: ResourceLocker
metadata:
  name: test-complex-patch
spec:
  serviceAccountRef:
    name: default
  patches:
  - targetObjectRef:
      apiVersion: v1
      kind: ServiceAccount
      name: test
      namespace: resource-locker-test
    patchTemplate: |
      metadata:
        annotations:
          {{ (index . 0).metadata.name }}: {{ (index . 1).metadata.name }}
    patchType: application/strategic-merge-patch+json
    sourceObjectRefs:
    - apiVersion: v1
      kind: Namespace
      name: resource-locker-test
    - apiVersion: v1
      kind: ServiceAccount
      name: default
      namespace: resource-locker-test
```

A patch is defined by the following:

* `targetObjectRef`: representing the object to which the patch needs to be applied. Notice that this kind of object reference can refer to any object type located in any namespace.
* `sourceObjectRefs`: representing a set of objects that will be used as parameters for the patch template. If the `fieldPath` is not specified, the entire object will be passed as parameter. If the `fieldPath` field is specified, then only the portion of the object specified selected by it will be passed as parameter. The `fieldPath` field must be a valid `jsonPath` expression as defined [here](https://goessner.net/articles/JsonPath/index.html#e2). This [site](https://jsonpath.com/) can be sued to test jsonPath expressions. The jsonPath expression is processed using this [library](https://godoc.org/k8s.io/client-go/util/jsonpath), if more than one result is returned by the jsonPath expression only the first one will be considered.
* `patchTemplate`: a [go template](https://golang.org/pkg/text/template/) template representing the patch. The go template must resolved to a yaml structure representing a valid patch for the target object. The yaml structure will be converted to json. When processing this template, one parameter is passed structured as an array containing the sourceObjects (or their fields if the `fieldPath` is specified).
* `patchType`: the type of patch, must be one of the following:
  * `application/json-patch+json`
  * `application/merge-patch+json`
  * `application/strategic-merge-patch+json`
  * `application/apply-patch+yaml`
  
If not specified the patchType will be defaulted to: `application/strategic-merge-patch+json`

## Multitenancy

The referenced service account will be used to create the client used by the manager and the underlying controller that enforce the resources and the patches. So while it is theoretically possible to declare the intention to create any object and to patch any object reading values potentially eny objects (including secrets), in reality one needs to have been given permission to do so via permission granted to the service referenced account.
This allows for the following:

1. Run the operator with relatively restricted permissions.
2. Prevent privilege escalation by making sure that used permissions have actually been explicitly granted.

The following permissions are needed for on a locked resource type: `List`, `Get`, `Watch`, `Create`, `Update`, `Patch`.

The following permissions are needed on a source reference object type: `List`, `Get`, `Watch`.

The following permissions are needed on a target reference object type: `List`, `Get`, `Watch`, `Patch`.

## Deleting Resources

When a `ResourceLocker` is removed, `LockedResources` will be deleted by the finalizer. Patches will be left untouched because there is no clear way to know how to restore an object to the state before the application of a patch.

If a `ResourceLocker` is updated deleting a `LockedResource`, the operator will try its best to determine which resource was deleted by looking at the following:

* its internal memory state.
* the `kubectl.kubernetes.io/last-applied-configuration`.
* the status reported by the CR.

LockedResources reported from these three sources of information will b compared with the desired set of resources. Resources that does not appear in the desired set will be deleted. This method is not 100% failsafe. In particular is someone disable the operator and at the same time changes a ResourceLocker CR by deleting a locked resource, it is possible that a locker resource becomes orphaned and be left un-managed. Again the operator does not try to undo patches when `LockedPatches` are removed from a `ResourceLocker` CR.

## Local Development

Execute the following steps to develop the functionality locally. It is recommended that development be done using a cluster with `cluster-admin` permissions.

```shell
go mod download
```

optionally:

```shell
go mod vendor
```

Using the [operator-sdk](https://github.com/operator-framework/operator-sdk), run the operator locally:

```shell
oc apply -f deploy/crds/redhatcop.redhat.io_resourcelockers_crd.yaml
export KUBERNETES_SERVICE_HOST=<your kube host>
export KUBERNETES_SERVICE_PORT=<your kube port>
OPERATOR_NAME='resource-locker-operator' operator-sdk --verbose up local --namespace ""
```

## Release Process

To release execute the following:

```shell
git tag -a "<version>" -m "release <version>"
git push upstream <version>
```

use this version format: vM.m.z

## Roadmap

* <s>Add status and error management</s>
* <s>Initialization/Finalization</s>
  * <s>resource deletion should be based on the last recorded status on the status object, change the current approach</s>
* <s>Add ability to exclude section of locked objects (defaults to: status, metadata, replicas).</s>
* Add ability to watch on specific objects, not just types (https://github.com/kubernetes-sigs/controller-runtime/issues/671).  
