module github.com/redhat-cop/resource-locker-operator

require (
	github.com/go-openapi/spec v0.19.2
	github.com/operator-framework/operator-sdk v0.11.1-0.20191014155558-888dde512025
	github.com/pkg/errors v0.8.1
	github.com/redhat-cop/operator-utils v0.0.0-20191011134647-d53c2c139d07
	github.com/spf13/pflag v1.0.3
	golang.org/x/tools/gopls v0.1.7 // indirect
	k8s.io/api v0.0.0-20191031065753-b19d8caf39be
	k8s.io/apimachinery v0.0.0-20191030190112-bb31b70367b7
	k8s.io/client-go v11.0.1-0.20190409021438-1a26190bd76a+incompatible
	k8s.io/kube-openapi v0.0.0-20190816220812-743ec37842bf
	k8s.io/kubectl v0.0.0-20191031072635-2ba9448df4cc // indirect
	sigs.k8s.io/controller-runtime v0.2.0
	sigs.k8s.io/yaml v1.1.0
)

// Pinned to kubernetes-1.14.1
replace (
	k8s.io/api => k8s.io/api v0.0.0-20190409021203-6e4e0e4f393b
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.0.0-20190409022649-727a075fdec8
	k8s.io/apimachinery => k8s.io/apimachinery v0.0.0-20190404173353-6a84e37a896d
	k8s.io/client-go => k8s.io/client-go v11.0.1-0.20190409021438-1a26190bd76a+incompatible
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.0.0-20190409023720-1bc0c81fa51d
)

replace (
	// Indirect operator-sdk dependencies use git.apache.org, which is frequently
	// down. The github mirror should be used instead.
	// Locking to a specific version (from 'go mod graph'):
	git.apache.org/thrift.git => github.com/apache/thrift v0.0.0-20180902110319-2566ecd5d999
	github.com/coreos/prometheus-operator => github.com/coreos/prometheus-operator v0.31.1
	// Pinned to v2.10.0 (kubernetes-1.14.1) so https://proxy.golang.org can
	// resolve it correctly.
	github.com/prometheus/prometheus => github.com/prometheus/prometheus v0.0.0-20190525122359-d20e84d0fb64
)

replace github.com/operator-framework/operator-sdk => github.com/operator-framework/operator-sdk v0.11.0
