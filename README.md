# Resource Locker Operator

![build status](https://github.com/redhat-cop/resource-locker-operator/workflows/push/badge.svg)
[![Go Report Card](https://goreportcard.com/badge/github.com/redhat-cop/resource-locker-operator)](https://goreportcard.com/report/github.com/redhat-cop/resource-locker-operator)
![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/redhat-cop/resource-locker-operator)

The resource locker operator allows you to specify a set of configurations that the operator will "keep in place" (lock) preventing any drifts.
Two types of configurations may be specified:

* Resources. This will instruct the operator to create and enforce the specified resource. In this case, the operator "owns" the created resources.
* Patches to resources. This will instruct the operator to patch- and enforce the change on- a pre-existing resource. In this case, the operator does not "own" the resource.

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
* `serviceAccountRef`: a reference to a service account defined in the same namespace as the ResourceLocker CR, that will be used to create the resources and apply the patches. If not specified the service account will be defaulted to: `default`

For each ResourceLocker, a manager is dynamically allocated. For each resource and patch, a controller with the needed watches is created and associated with the previously created manager.

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

In this example, we lock in a ResourceQuota configuration. Resources must be fully specified (i.e. no templating is allowed and all the mandatory fields must be initialized).
Resources created this way are allowed to drift from the desired state only in the `excludedPaths`, which are jsonPath expressions. If drift occurs in other sections of the resource, the operator will immediately reset the resource. The following `excludedPaths` are always added:

* `.metadata`
* `.status`
* `.spec.replicas`

and in most cases should be the right choice.

For a concrete example of how to lock a resource, see [EXAMPLES.md](EXAMPLES.md).

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
    id: sa-annotation  
```

A patch is defined by the following:

* `targetObjectRef`: representing the object to which the patch needs to be applied. Notice that this kind of object reference can refer to any object type located in any namespace.
* `sourceObjectRefs`: representing a set of objects that will be used as parameters for the patch template. If the `fieldPath` is not specified, the entire object will be passed as the parameter. If the `fieldPath` field is specified, then only the portion of the object specified selected by it will be passed as the parameter. The `fieldPath` field must be a valid `jsonPath` expression as defined [here](https://goessner.net/articles/JsonPath/index.html#e2). This [site](https://jsonpath.com/) can be used to test jsonPath expressions. The jsonPath expression is processed using this [library](https://godoc.org/k8s.io/client-go/util/jsonpath) if more than one result is returned by the jsonPath expression only the first one will be considered.
* `patchTemplate`: a [go template](https://golang.org/pkg/text/template/) template representing the patch. The go template must be resolved to a yaml structure representing a valid patch for the target object. The yaml structure will be converted to json. When processing this template, one parameter is passed structured as an array containing the sourceObjects (or their fields if the `fieldPath` is specified).
* `patchType`: the type of patch, must be one of the following:
  * `application/json-patch+json`
  * `application/merge-patch+json`
  * `application/strategic-merge-patch+json`
  * `application/apply-patch+yaml`
* an `id` which must be unique in the array of patches.  
  
If not specified the patchType will be defaulted to: `application/strategic-merge-patch+json`

## Multitenancy

The referenced service account will be used to create the client used by the manager and the underlying controller that enforce the resources and the patches. So while it is theoretically possible to declare the intention to create any object and to patch any object reading values potentially any objects (including secrets), in reality, one needs to have been given permission to do so via permissions granted to the referenced service account.
This allows for the following:

1. Running the operator with relatively restricted permissions.
2. Preventing privilege escalation by making sure that used permissions have actually been explicitly granted.

The following permissions are needed on a locked resource object type: `List`, `Get`, `Watch`, `Create`, `Update`, `Patch`.

The following permissions are needed on a source reference object type: `List`, `Get`, `Watch`.

The following permissions are needed on a target reference object type: `List`, `Get`, `Watch`, `Patch`.

## Deleting Resources

When a `ResourceLocker` is removed, `LockedResources` will be deleted by the finalizer. Patches will be left untouched because there is no clear way to know how to restore an object to the state before the application of a patch.

## Deploying the Operator

This is a cluster-level operator that you can deploy in any namespace, `resource-locker-operator` is recommended.

It is recommended to deploy this operator via [`OperatorHub`](https://operatorhub.io/), but you can also deploy it using [`Helm`](https://helm.sh/).

### Multiarch Support

| Arch  | Support  |
|:-:|:-:|
| amd64  | ✅ |
| arm64  | ✅  |
| ppc64le  | ✅  |
| s390x  | ❌  |

### Deploying from OperatorHub

> **Note**: This operator supports being installed disconnected environments
If you want to utilize the Operator Lifecycle Manager (OLM) to install this operator, you can do so in two ways: from the UI or the CLI.

#### Deploying from OperatorHub UI

* If you would like to launch this operator from the UI, you'll need to navigate to the OperatorHub tab in the console. Before starting, make sure you've created the namespace that you want to install this operator to with the following:

```shell
oc new-project resource-locker-operator
```

* Once there, you can search for this operator by name: `resource locker operator`. This will then return an item for our operator and you can select it to get started. Once you've arrived here, you'll be presented with an option to install, which will begin the process.
* After clicking the install button, you can then select the namespace that you would like to install this to as well as the installation strategy you would like to proceed with (`Automatic` or `Manual`).
* Once you've made your selection, you can select `Subscribe` and the installation will begin. After a few moments, you can go ahead and check your namespace and you should see the operator running.

![Resource Locker Operator](./media/resource-locker-operator.png)

#### Deploying from OperatorHub using CLI

If you'd like to launch this operator from the command line, you can use the manifests contained in this repository by running the following:

oc new-project resource-locker-operator

```shell
oc apply -f config/operatorhub -n resource-locker-operator
```

This will create the appropriate OperatorGroup and Subscription and will trigger OLM to launch the operator in the specified namespace.

### Deploying with Helm

Here are the instructions to install the latest release with Helm.

```shell
oc new-project resource-locker-operator
helm repo add resource-locker-operator https://redhat-cop.github.io/resource-locker-operator
helm repo update
helm install resource-locker-operator resource-locker-operator/resource-locker-operator
```

This can later be updated with the following commands:

```shell
helm repo update
helm upgrade resource-locker-operator resource-locker-operator/resource-locker-operator
```

## Metrics

Prometheus compatible metrics are exposed by the Operator and can be integrated into OpenShift's default cluster monitoring. To enable OpenShift cluster monitoring, label the namespace the operator is deployed in with the label `openshift.io/cluster-monitoring="true"`.

```shell
oc label namespace <namespace> openshift.io/cluster-monitoring="true"
```

### Testing metrics

```sh
export operatorNamespace=resource-locker-operator-local # or resource-locker-operator
oc label namespace ${operatorNamespace} openshift.io/cluster-monitoring="true"
oc rsh -n openshift-monitoring -c prometheus prometheus-k8s-0 /bin/bash
export operatorNamespace=resource-locker-operator-local # or resource-locker-operator
curl -v -s -k -H "Authorization: Bearer $(cat /var/run/secrets/kubernetes.io/serviceaccount/token)" https://resource-locker-operator-controller-manager-metrics.${operatorNamespace}.svc.cluster.local:8443/metrics
exit
```

## Development

## Running the operator locally

```shell
make install
oc new-project resource-locker-operator-local
kustomize build ./config/local-development | oc apply -f - -n resource-locker-operator-local
export token=$(oc serviceaccounts get-token 'resource-locker-controller-manager' -n resource-locker-operator-local)
oc login --token ${token}
export KUBERNETES_SERVICE_HOST=<your kube host>
export KUBERNETES_SERVICE_PORT=6443
make run ENABLE_WEBHOOKS=false
```

### Test helm chart locally

Define an image and tag. For example...

```shell
export imageRepository="quay.io/redhat-cop/resource-locker-operator"
export imageTag="$(git -c 'versionsort.suffix=-' ls-remote --exit-code --refs --sort='version:refname' --tags https://github.com/redhat-cop/resource-locker-operator.git '*.*.*' | tail --lines=1 | cut --delimiter='/' --fields=3)"
```

Deploy chart...

```shell
make helmchart IMG=${imageRepository} VERSION=${imageTag}
helm upgrade -i resource-locker-operator-local charts/resource-locker-operator -n resource-locker-operator-local --create-namespace
```

Delete...

```shell
helm delete resource-locker-operator-local -n resource-locker-operator-local
kubectl delete -f charts/resource-locker-operator/crds/crds.yaml
```

## Building/Pushing the operator image

```shell
export repo=raffaelespazzoli #replace with yours
docker login quay.io/$repo
make docker-build IMG=quay.io/$repo/resource-locker-operator:latest
make docker-push IMG=quay.io/$repo/resource-locker-operator:latest
```

## Deploy to OLM via bundle

```shell
make manifests
make bundle IMG=quay.io/$repo/resource-locker-operator:latest
operator-sdk bundle validate ./bundle --select-optional name=operatorhub
make bundle-build BUNDLE_IMG=quay.io/$repo/resource-locker-operator-bundle:latest
docker push quay.io/$repo/resource-locker-operator-bundle:latest
operator-sdk bundle validate quay.io/$repo/resource-locker-operator-bundle:latest --select-optional name=operatorhub
oc new-project resource-locker-operator
oc label namespace resource-locker-operator openshift.io/cluster-monitoring="true"
operator-sdk cleanup resource-locker-operator -n resource-locker-operator
operator-sdk run bundle --install-mode AllNamespaces -n resource-locker-operator quay.io/$repo/resource-locker-operator-bundle:latest
```

## Releasing

```shell
git tag -a "<tagname>" -m "<commit message>"
git push upstream <tagname>
```

If you need to remove a release:

```shell
git tag -d <tagname>
git push upstream --delete <tagname>
```

If you need to "move" a release to the current main

```shell
git tag -f <tagname>
git push upstream -f <tagname>
```

### Cleaning up

```shell
operator-sdk cleanup resource-locker-operator -n resource-locker-operator
oc delete operatorgroup operator-sdk-og
oc delete catalogsource resource-locker-operator-catalog
```
