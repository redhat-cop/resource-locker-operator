# Usage Examples

We assume that you have already installed the resource-locker-operator in your cluster and the CustomResourceDefinitions are available.

## Locking a resource

In this example, we assume your operator is installed in the recommended namespace:  `resource-locker-operator`. We will create a `ConfigMap` in a second namespace, `resource-locker-test-ns`, locked by a `ResourceLocker` CustomResource.

**NOTE** If you are using OpenShift, or OKD, feel free to substitute your `oc` command in place of `kubectl` as you see fit.

### Create a namespace

```shell
kubectl create namespace resource-locker-test-ns
```

### Create a service account in this namespaces

We could use the standard "default" `ServiceAccount`, but it may be best to use a separate `ServiceAccount` for locked resources as they may have additional privileges to access the Kubernetes API that the default service account may not need.

```shell
kubectl create serviceaccount resource-locker-test-sa -n resource-locker-test-ns
```

### Create a role with the appropriate permissions for the locked resource

We need to assign permissions to our newly created `ServiceAccount`. In this example, we're going to create a `ConfigMap` managed/locked by a `ResourceLocker`, so we'll give access to `ConfigMaps`within this namespace.

```shell
kubectl apply -f - <<EOF
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: resourcelocker-lock-configmaps
  namespace: resource-locker-test-ns
rules:
- apiGroups:
  - ""
  resources:
  - configmaps
  verbs:
  - list
  - get
  - watch
  - create
  - update
  - patch
EOF
```

This role provisions all the necessary verbs for a locked resource managed by `ResourceLocker`.

### Associate the role to our ServiceAccount

Now we need to grant our `ServiceAccount` this role to give it the necessary privileges to interact with our locked resource `ConfigMaps`.

```shell
kubectl apply -f - <<EOF
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: resource-locker-test-sa-can-manage-configmaps
  namespace: resource-locker-test-ns
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: resourcelocker-lock-configmaps
subjects:
- kind: ServiceAccount
  name: resource-locker-test-sa
  namespace: resource-locker-test-ns
EOF
```

### Create the ResourceLocker

Now, create a `ResourceLocker` in the same namespace as your `ServiceAccount`.

```shell
kubectl apply -f - <<EOF
apiVersion: redhatcop.redhat.io/v1alpha1
kind: ResourceLocker
metadata:
  name: locked-configmap-foo-bar-configmap
  namespace: resource-locker-test-ns
spec:
  resources:
    - excludedPaths:
        - .metadata
        - .status
        - .spec.replicas
      object:
        apiVersion: v1
        kind: ConfigMap
        metadata:
          name: foo-bar-configmap
          namespace: resource-locker-test-ns
        data:
          foo: bar
  serviceAccountRef:
    name: resource-locker-test-sa
EOF
```

### Confirm the resource is being managed

`ResourceLocker` should create the resource that you've specified. Confirm that this is the case.

```shell
kubectl get configmap -n resource-locker-test-ns foo-bar-configmap
NAME                DATA      AGE
foo-bar-configmap   1         64m
```
