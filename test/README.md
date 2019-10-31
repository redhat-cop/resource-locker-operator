# Tests

Execute the following:

```shell
oc new-project resource-locker-test
oc adm policy add-cluster-role-to-user cluster-admin -z default -n resource-locker-test
oc apply -f ./test/resource_test.yaml -n resource-locker-test
```

```shell
oc create sa test -n resource-locker-test
oc apply -f ./test/simple_patch.yaml -n resource-locker-test
```
