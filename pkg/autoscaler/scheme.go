package autoscaler

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"

	machinev1 "github.com/openshift/api/machine/v1beta1"
)

var (
	Scheme = runtime.NewScheme()
)

func init() {
	scheme.AddToScheme(Scheme)
	machinev1.AddToScheme(Scheme)
}
