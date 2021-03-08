package ingress_merge

import (
	"testing"

	"github.com/likexian/gokit/assert"
	extensionsV1beta1 "k8s.io/api/extensions/v1beta1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestHasIngressChangedAnnotations(t *testing.T) {
	oldIngress := &extensionsV1beta1.Ingress{
		ObjectMeta: metaV1.ObjectMeta{
			Annotations: map[string]string{
				"external-managed-field-01": "t",
				"ingress-field-01":          "test",
			},
		},
	}

	newIngress01 := &extensionsV1beta1.Ingress{
		ObjectMeta: metaV1.ObjectMeta{
			Annotations: map[string]string{
				"ingress-field-01": "test",
			},
		},
	}
	newIngress02 := &extensionsV1beta1.Ingress{
		ObjectMeta: metaV1.ObjectMeta{
			Annotations: map[string]string{
				"ingress-field-01": "test-changed",
			},
		},
	}
	result := hasIngressChanged(oldIngress, newIngress01)
	assert.False(t, result)

	result = hasIngressChanged(oldIngress, newIngress02)
	assert.True(t, result)
}
