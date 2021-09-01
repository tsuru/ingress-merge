package ingress_merge

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestHasIngressChangedAnnotations(t *testing.T) {
	oldIngress := &networkingv1.Ingress{
		ObjectMeta: metaV1.ObjectMeta{
			Annotations: map[string]string{
				"external-managed-field-01": "t",
				"ingress-field-01":          "test",
			},
		},
	}

	newIngress01 := &networkingv1.Ingress{
		ObjectMeta: metaV1.ObjectMeta{
			Annotations: map[string]string{
				"ingress-field-01": "test",
			},
		},
	}
	newIngress02 := &networkingv1.Ingress{
		ObjectMeta: metaV1.ObjectMeta{
			Annotations: map[string]string{
				"ingress-field-01": "test-changed",
			},
		},
	}
	r := &IngressReconciler{
		Log: zap.New(zap.UseDevMode(true)),
	}
	result := r.hasIngressChanged(oldIngress, newIngress01)
	assert.False(t, result)

	result = r.hasIngressChanged(oldIngress, newIngress02)
	assert.True(t, result)
}

func TestReconcile(t *testing.T) {
	ctx := context.Background()

	instance1 := &networkingv1.Ingress{
		ObjectMeta: metaV1.ObjectMeta{
			Namespace: "my-namespace",
			Name:      "my-instance",
			Annotations: map[string]string{
				IngressClassAnnotation: "merge",
				ConfigAnnotation:       "kubernetes-shared-ingress",
			},
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{
					Host: "instance1.example.org",
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path: "/*",
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: "instance1",
											Port: networkingv1.ServiceBackendPort{
												Number: 8888,
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
	}

	instance2 := &networkingv1.Ingress{
		ObjectMeta: metaV1.ObjectMeta{
			Namespace: "my-namespace",
			Name:      "my-instance2",
			Annotations: map[string]string{
				IngressClassAnnotation: "merge",
				ConfigAnnotation:       "kubernetes-shared-ingress",
			},
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{
					Host: "instance2.example.org",
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path: "/*",
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: "instance2",
											Port: networkingv1.ServiceBackendPort{
												Number: 8888,
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
	}

	instance3 := &networkingv1.Ingress{
		ObjectMeta: metaV1.ObjectMeta{
			Namespace: "my-namespace",
			Name:      "my-instance3",
			Annotations: map[string]string{
				IngressClassAnnotation: "merge",
				ConfigAnnotation:       "kubernetes-shared-ingress",
			},
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{
					Host: "instance2.example.org",
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path: "/special-route",
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: "instance3",
											Port: networkingv1.ServiceBackendPort{
												Number: 8888,
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
	}

	configMap1 := &corev1.ConfigMap{
		ObjectMeta: metaV1.ObjectMeta{
			Namespace: "my-namespace",
			Name:      "kubernetes-shared-ingress",
		},
		Data: map[string]string{
			"labels":           `ingress-merge-label: "label01"`,
			"ingressClassName": "my-next-ingress",
			"annotations":      `ingress-merge-annotation: "annotation01"`,
		},
	}

	t.Run("config map not found", func(t *testing.T) {
		reconciler := newTestReconciler([]runtime.Object{
			instance1,
		})

		_, err := reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: "my-namespace",
				Name:      "my-instance",
			},
		})

		require.NoError(t, err)
		sharedIngress := networkingv1.Ingress{}
		err = reconciler.Client.Get(ctx, client.ObjectKey{Namespace: "my-namespace", Name: "kubernetes-shared-ingress"}, &sharedIngress)
		require.True(t, k8sErrors.IsNotFound(err))
	})

	t.Run("config map found", func(t *testing.T) {
		reconciler := newTestReconciler([]runtime.Object{
			instance1, configMap1,
		})

		_, err := reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: "my-namespace",
				Name:      "my-instance",
			},
		})

		require.NoError(t, err)
		sharedIngress := networkingv1.Ingress{}
		err = reconciler.Client.Get(ctx, client.ObjectKey{Namespace: "my-namespace", Name: "kubernetes-shared-ingress"}, &sharedIngress)
		require.NoError(t, err)

		ingressClassName := "my-next-ingress"
		assert.Equal(t, networkingv1.IngressSpec{
			IngressClassName: &ingressClassName,
			Rules: []networkingv1.IngressRule{
				{
					Host: "instance1.example.org",
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path: "/*",
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: "instance1",
											Port: networkingv1.ServiceBackendPort{
												Number: 8888,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}, sharedIngress.Spec)
	})

	t.Run("multiple instances and one ingress, not found shared ingress", func(t *testing.T) {
		reconciler := newTestReconciler([]runtime.Object{
			instance1, instance2, instance3,
			configMap1,
		})

		_, err := reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: "my-namespace",
				Name:      "my-instance",
			},
		})

		require.NoError(t, err)
		sharedIngress := networkingv1.Ingress{}
		err = reconciler.Client.Get(ctx, client.ObjectKey{Namespace: "my-namespace", Name: "kubernetes-shared-ingress"}, &sharedIngress)
		require.NoError(t, err)

		assert.Equal(t, map[string]string{
			"ingress-merge-annotation":           "annotation01",
			"merge.ingress.kubernetes.io/result": "true",
		}, sharedIngress.Annotations)

		assert.Equal(t, map[string]string{
			"ingress-merge-label": "label01",
		}, sharedIngress.Labels)

		ingressClassName := "my-next-ingress"
		assert.Equal(t, networkingv1.IngressSpec{
			IngressClassName: &ingressClassName,
			Rules: []networkingv1.IngressRule{
				{
					Host: "instance1.example.org",
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path: "/*",
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: "instance1",
											Port: networkingv1.ServiceBackendPort{
												Number: 8888,
											},
										},
									},
								},
							},
						},
					},
				},
				{
					Host: "instance2.example.org",
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path: "/*",
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: "instance2",
											Port: networkingv1.ServiceBackendPort{
												Number: 8888,
											},
										},
									},
								},
								{
									Path: "/special-route",
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: "instance3",
											Port: networkingv1.ServiceBackendPort{
												Number: 8888,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}, sharedIngress.Spec)
	})

	t.Run("multiple instances and one ingress, found shared ingress", func(t *testing.T) {
		sharedIngress := &networkingv1.Ingress{
			ObjectMeta: metaV1.ObjectMeta{
				Name:      "kubernetes-shared-ingress",
				Namespace: "my-namespace",
				Annotations: map[string]string{
					"merge.ingress.kubernetes.io/result": "true",
				},
			},
			Status: networkingv1.IngressStatus{
				LoadBalancer: corev1.LoadBalancerStatus{
					Ingress: []corev1.LoadBalancerIngress{
						{
							IP: "1.1.8.8",
						},
					},
				},
			},
		}
		reconciler := newTestReconciler([]runtime.Object{
			instance1, instance2, instance3, sharedIngress,
			configMap1,
		})

		_, err := reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: "my-namespace",
				Name:      "my-instance",
			},
		})

		require.NoError(t, err)
		err = reconciler.Client.Get(ctx, client.ObjectKey{Namespace: "my-namespace", Name: "kubernetes-shared-ingress"}, sharedIngress)
		require.NoError(t, err)

		require.Len(t, sharedIngress.Spec.Rules, 2)

		var instance networkingv1.Ingress
		err = reconciler.Client.Get(ctx, client.ObjectKey{Namespace: "my-namespace", Name: "my-instance"}, &instance)
		require.NoError(t, err)

		assert.Equal(t, networkingv1.IngressStatus{
			LoadBalancer: corev1.LoadBalancerStatus{
				Ingress: []corev1.LoadBalancerIngress{
					{
						IP: "1.1.8.8",
					},
				},
			},
		}, instance.Status)
	})
}

func newTestReconciler(objs []runtime.Object) *IngressReconciler {
	scheme := runtime.NewScheme()
	_ = networkingv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	reconciler := &IngressReconciler{
		Log:          zap.New(zap.UseDevMode(true)),
		IngressClass: "merge",
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithRuntimeObjects(objs...).
			Build(),
	}

	return reconciler
}
