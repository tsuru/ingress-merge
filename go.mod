module github.com/tsuru/ingress-merge

go 1.15

require (
	github.com/ghodss/yaml v1.0.0
	github.com/go-logr/logr v0.4.0
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/hashicorp/go-multierror v1.0.0
	github.com/spf13/cobra v1.1.3
	github.com/stretchr/testify v1.7.0
	k8s.io/api v0.21.3
	k8s.io/apimachinery v0.21.3
	k8s.io/client-go v0.21.3
	sigs.k8s.io/controller-runtime v0.9.6
)
