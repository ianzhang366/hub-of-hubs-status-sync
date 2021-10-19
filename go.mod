module github.com/open-cluster-management/hub-of-hubs-status-sync

go 1.16

require (
	github.com/go-logr/logr v0.4.0
	github.com/jackc/pgx/v4 v4.11.0
	github.com/open-cluster-management/governance-policy-propagator v0.0.0-20210520203318-a78632de1e26
	github.com/open-cluster-management/klusterlet-addon-controller v0.0.0-20211008064339-48c75d186cce
	github.com/openshift/hive/apis v0.0.0
	github.com/operator-framework/operator-sdk v0.19.4
	github.com/spf13/pflag v1.0.5
	golang.org/x/tools v0.1.7 // indirect
	k8s.io/apimachinery v0.21.2
	k8s.io/client-go v12.0.0+incompatible
	sigs.k8s.io/controller-runtime v0.9.3-0.20210709165254-650ea59f19cc
)

replace (
	github.com/kubevirt/terraform-provider-kubevirt => github.com/nirarg/terraform-provider-kubevirt v0.0.0-20201222125919-101cee051ed3
	github.com/metal3-io/baremetal-operator => github.com/openshift/baremetal-operator v0.0.0-20200715132148-0f91f62a41fe
	github.com/metal3-io/cluster-api-provider-baremetal => github.com/openshift/cluster-api-provider-baremetal v0.0.0-20190821174549-a2a477909c1d
	github.com/openshift/hive/apis => github.com/openshift/hive/apis v0.0.0-20210802140536-4d8d83dcd464
	github.com/terraform-providers/terraform-provider-aws => github.com/openshift/terraform-provider-aws v1.60.1-0.20200630224953-76d1fb4e5699
	github.com/terraform-providers/terraform-provider-azurerm => github.com/openshift/terraform-provider-azurerm v1.40.1-0.20200707062554-97ea089cc12a
	github.com/terraform-providers/terraform-provider-ignition/v2 => github.com/community-terraform-providers/terraform-provider-ignition/v2 v2.1.0
	// need this for klusterletaddonconfig
	k8s.io/api => k8s.io/api v0.20.4
	k8s.io/client-go => k8s.io/client-go v0.20.5

	kubevirt.io/client-go => kubevirt.io/client-go v0.29.0

	sigs.k8s.io/cluster-api-provider-aws => github.com/openshift/cluster-api-provider-aws v0.2.1-0.20201022175424-d30c7a274820
	sigs.k8s.io/cluster-api-provider-azure => github.com/openshift/cluster-api-provider-azure v0.1.0-alpha.3.0.20201016155852-4090a6970205
	sigs.k8s.io/cluster-api-provider-openstack => github.com/openshift/cluster-api-provider-openstack v0.0.0-20201116051540-155384b859c5
	sigs.k8s.io/controller-runtime => sigs.k8s.io/controller-runtime v0.6.2
)
