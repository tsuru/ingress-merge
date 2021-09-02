package ingress_merge

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
)

func TestGenerateIngressBucketsWithoutAddress(t *testing.T) {
	origins := []networkingv1.Ingress{}
	destinations := []networkingv1.Ingress{}

	for i := 0; i < 50; i++ {
		origins = append(origins, networkingv1.Ingress{
			ObjectMeta: metaV1.ObjectMeta{
				Name: fmt.Sprintf("origin-backend-%d", i+1),
			},
			Spec: networkingv1.IngressSpec{
				Rules: []networkingv1.IngressRule{
					{
						Host: fmt.Sprintf("origin-%d.example.org", i+1),
						IngressRuleValue: networkingv1.IngressRuleValue{
							HTTP: &networkingv1.HTTPIngressRuleValue{
								Paths: []networkingv1.HTTPIngressPath{
									{
										Path: "/",
										Backend: networkingv1.IngressBackend{
											Service: &networkingv1.IngressServiceBackend{
												Name: fmt.Sprintf("service-%d", i+1),
												Port: networkingv1.ServiceBackendPort{
													Number: 80,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		})
	}

	ingressBuckets := GenerateIngressBuckets(origins, destinations, 50)
	assert.Len(t, ingressBuckets, 1)

	ingressBuckets = GenerateIngressBuckets(origins, destinations, 45)
	assert.Len(t, ingressBuckets, 2)

	ingressBuckets = GenerateIngressBuckets(origins, destinations, 25)
	assert.Len(t, ingressBuckets, 2)

	ingressBuckets = GenerateIngressBuckets(origins, destinations, 5)
	assert.Len(t, ingressBuckets, 10)
}

func TestGenerateIngressBucketsWithAddress(t *testing.T) {
	origins := []networkingv1.Ingress{}
	destinations := []networkingv1.Ingress{
		{
			ObjectMeta: metaV1.ObjectMeta{
				Name:            "shared-01",
				UID:             uuid.NewUUID(),
				OwnerReferences: []metaV1.OwnerReference{},
			},
		},
		{
			ObjectMeta: metaV1.ObjectMeta{
				Name:            "shared-02",
				UID:             uuid.NewUUID(),
				OwnerReferences: []metaV1.OwnerReference{},
			},
		},
	}

	for i := 0; i < 60; i++ {
		uuid := uuid.NewUUID()
		var ingressStatus []corev1.LoadBalancerIngress
		if i%3 == 0 {
			ingressStatus = []corev1.LoadBalancerIngress{
				{
					IP: "10.1.1.1",
				},
			}
			destinations[0].ObjectMeta.OwnerReferences = append(destinations[0].ObjectMeta.OwnerReferences, metaV1.OwnerReference{
				APIVersion: "networking.k8s.io/v1",
				Kind:       "Ingress",
				Name:       fmt.Sprintf("origin-backend-%d", i+1),
				UID:        uuid,
			})
		} else if i%3 == 1 {
			ingressStatus = []corev1.LoadBalancerIngress{
				{
					IP: "10.1.1.2",
				},
			}
			destinations[1].ObjectMeta.OwnerReferences = append(destinations[1].ObjectMeta.OwnerReferences, metaV1.OwnerReference{
				APIVersion: "networking.k8s.io/v1",
				Kind:       "Ingress",
				Name:       fmt.Sprintf("origin-backend-%d", i+1),
				UID:        uuid,
			})
		} else if i%3 == 2 {
			ingressStatus = []corev1.LoadBalancerIngress{}
		}

		origins = append(origins, networkingv1.Ingress{
			ObjectMeta: metaV1.ObjectMeta{
				Name: fmt.Sprintf("origin-backend-%d", i+1),
				UID:  uuid,
			},
			Spec: networkingv1.IngressSpec{
				Rules: []networkingv1.IngressRule{
					{
						Host: fmt.Sprintf("origin-%d.example.org", i+1),
						IngressRuleValue: networkingv1.IngressRuleValue{
							HTTP: &networkingv1.HTTPIngressRuleValue{
								Paths: []networkingv1.HTTPIngressPath{
									{
										Path: "/",
										Backend: networkingv1.IngressBackend{
											Service: &networkingv1.IngressServiceBackend{
												Name: fmt.Sprintf("service-%d", i+1),
												Port: networkingv1.ServiceBackendPort{
													Number: 80,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			Status: networkingv1.IngressStatus{
				LoadBalancer: corev1.LoadBalancerStatus{
					Ingress: ingressStatus,
				},
			},
		})
	}

	ingressBuckets := GenerateIngressBuckets(origins, destinations, 50)
	assert.Len(t, ingressBuckets, 2)
	assert.Equal(t, "shared-01", ingressBuckets[0].DestinationIngress.Name)

	assert.Len(t, ingressBuckets[0].Ingresses, 40)
	assert.Equal(t, 10, ingressBuckets[0].FreeSlots)

	assert.Equal(t, "shared-02", ingressBuckets[1].DestinationIngress.Name)
	assert.Len(t, ingressBuckets[1].Ingresses, 20)
	assert.Equal(t, 30, ingressBuckets[1].FreeSlots)

	ingressBuckets = GenerateIngressBuckets(origins, destinations, 35)
	assert.Len(t, ingressBuckets, 2)
	assert.Len(t, ingressBuckets[0].Ingresses, 35)

	assert.Len(t, ingressBuckets[1].Ingresses, 25)

	ingressBuckets = GenerateIngressBuckets(origins, destinations, 25)
	assert.Len(t, ingressBuckets, 3)
	assert.Len(t, ingressBuckets[0].Ingresses, 25)
	assert.Len(t, ingressBuckets[1].Ingresses, 25)
	assert.Len(t, ingressBuckets[2].Ingresses, 10)

	ingressBuckets = GenerateIngressBuckets(origins, destinations, 5)
	assert.Len(t, ingressBuckets, 6)
	assert.Len(t, ingressBuckets[0].Ingresses, 20) // should not split ingress that have same IP
	assert.Len(t, ingressBuckets[1].Ingresses, 20)
	assert.Len(t, ingressBuckets[2].Ingresses, 5)
	assert.Len(t, ingressBuckets[3].Ingresses, 5)
	assert.Len(t, ingressBuckets[4].Ingresses, 5)
	assert.Len(t, ingressBuckets[5].Ingresses, 5)
}
