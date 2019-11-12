# Resource Locker Operator

[![Build Status](https://travis-ci.org/redhat-cop/resource-locker-operator.svg?branch=master)](https://travis-ci.org/redhat-cop/resource-locker-operator) [![Docker Repository on Quay](https://quay.io/repository/redhat-cop/resource-locker-operator/status "Docker Repository on Quay")](https://quay.io/repository/redhat-cop/resource-locker-operator)

The resource locker operator allows you to specify a set of configurations that the operator will "keep in place" (lock) preventing any drifts.
Two types of configurations may be specified:

* Resources. This will instruct the operator to create and enforce the specified resource. In this case the operator "owns" the created resources.
* Patches to resources. This will instruct the operator to patch- and enforce the change on- a pre-existing resource. In this case the operator does not "own" the resource.

Locker resources are defined with the `ResourceLocker` CRD. Here is the high-level structure of this CRD:

```yaml
apiVersion: redhatcop.redhat.io/v1alpha1
kind: ResourceLocker
metadata:
  name: test-simple-resource
spec:
  resources:
  - apiVersion: v1
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

## Resources Locking

An example of a resource locking configuration is the following:

```yaml
apiVersion: redhatcop.redhat.io/v1alpha1
kind: ResourceLocker
metadata:
  name: test-simple-resource
spec:
  resources:
  - apiVersion: v1
    kind: ResourceQuota
    metadata:
      name: small-size
      namespace: resource-locker-test
    spec:
      hard:
        requests.cpu: "4"
        requests.memory: "2Gi"
  serviceAccountRef:
    name: default
```

I this example we lock in a ResourceQuota configuration. Resource must be fully specified (i.e. no templating is allowed and all the mandatory fields must be initialized).
Resources created this ways are allowed to drift from the initial created state only in the `metadata` and `status` section. If drift occurs for in the `spec`, the operator will immediately reset the resource.

Special handling is in place for legacy `v1` objects that do not comply with the `metadata/spec/status` conventional structure such as: `ServiceAccount`, `ConfigMap`, `Secret`, `Role`, `ClusterRole`. CRDs that do not comply with the `metadata/spec/status` structure will cause the operator to error out.

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

The following permissions are needed for on a locked resource type: `List`, `Get`, `Watch`, `Create`, `Update`.

The following permissions are needed on a source reference object type: `List`, `Get`, `Watch`.

The following permissions are needed on a target reference object type: `List`, `Get`, `Watch`, `Patch`.

## Deleting Resources

When a `ResourceLocker` is removed, `LockedResources` will be deleted by the finalizer. Patches will be left untouched because there is no clear way to know how to restore an object to the state before the application of a patch.

If a `ResourceLocker` is updated deleting a `LockedResource`, the operator will try its best to determine which resource was deleted by looking at its internal memory state and at the `kubectl.kubernetes.io/last-applied-configuration`. Both methods are not failsafe so there is a possibility that orphaned resources will be left unmanned. Again the operator does not try to undo patches when `LockedPatches` are removed from a `ResourceLocker` CR.

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
* Add ability to exclude section of locked objects (defaults to: status, metadata, replicas).
  * Improve event filter to consider exclusions.
  * Same logic for objects referenced in patches which use the fieldPath.
* Add ability to watch on specific objects, not just types (https://github.com/kubernetes-sigs/controller-runtime/issues/671).  
