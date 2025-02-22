package ingress_merge

import (
	"context"
	"fmt"
	"sort"
	"strings"
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

	instanceIgnoreWildcard := &networkingv1.Ingress{
		ObjectMeta: metaV1.ObjectMeta{
			Namespace: "my-namespace",
			Name:      "my-instance-ignore-wildcard",
			Annotations: map[string]string{
				IngressClassAnnotation: "merge",
				ConfigAnnotation:       "kubernetes-shared-ingress",
			},
			Labels: map[string]string{
				"test-ignore": "true",
			},
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{
					Host: "instance-other.example.com",
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path: "/*",
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: "instance-other",
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

	configMap2 := &corev1.ConfigMap{
		ObjectMeta: metaV1.ObjectMeta{
			Namespace: "my-namespace",
			Name:      "kubernetes-shared-ingress",
		},
		Data: map[string]string{
			"labels":                `ingress-merge-label: "label01"`,
			"ingressClassName":      "my-next-ingress",
			"annotations":           `ingress-merge-annotation: "annotation01"`,
			UseWildcardTLSKey:       "true",
			UseWildcardTLSIgnoreKey: "test-ignore=true",
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

		sharedIngressList, err := getSharedIngresses(ctx, reconciler.Client, "my-namespace")
		require.NoError(t, err)
		require.Len(t, sharedIngressList, 1)
		sharedIngress := sharedIngressList[0]

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

	t.Run("use wildcard TLS enabled", func(t *testing.T) {
		reconciler := newTestReconciler([]runtime.Object{
			instance1, instanceIgnoreWildcard, configMap2,
		})

		_, err := reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: "my-namespace",
				Name:      "my-instance",
			},
		})

		require.NoError(t, err)

		sharedIngressList, err := getSharedIngresses(ctx, reconciler.Client, "my-namespace")
		require.NoError(t, err)
		require.Len(t, sharedIngressList, 1)
		sharedIngress := sharedIngressList[0]

		ingressClassName := "my-next-ingress"
		assert.Equal(t, networkingv1.IngressSpec{
			IngressClassName: &ingressClassName,
			TLS: []networkingv1.IngressTLS{
				{
					Hosts:      []string{"*.example.org"},
					SecretName: sharedIngress.Name + wildcardTLSSuffix,
				},
			},
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
					Host: "instance-other.example.com",
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path: "/*",
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: "instance-other",
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
		sharedIngresses, err := getSharedIngresses(ctx, reconciler.Client, "my-namespace")
		require.NoError(t, err)
		require.Len(t, sharedIngresses, 1)
		sharedIngress := sharedIngresses[0]

		assert.Equal(t, map[string]string{
			"ingress-merge-annotation":                "annotation01",
			"merge.ingress.kubernetes.io/result":      "true",
			"merge.ingress.kubernetes.io/from-config": "kubernetes-shared-ingress",
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
					"merge.ingress.kubernetes.io/result":      "true",
					"merge.ingress.kubernetes.io/from-config": "kubernetes-shared-ingress",
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
		for _, instanceName := range []string{"my-instance", "my-instance2", "my-instance3"} {
			err = reconciler.Client.Get(ctx, client.ObjectKey{Namespace: "my-namespace", Name: instanceName}, &instance)
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
			assert.Equal(t, instanceName, instance.Name)
		}
	})

	t.Run("multiple instances and many buckets ingresss", func(t *testing.T) {
		objects := []runtime.Object{
			configMap1,
		}

		for i := 0; i < 100; i++ {
			objects = append(objects, &networkingv1.Ingress{
				ObjectMeta: metaV1.ObjectMeta{
					Namespace: "my-namespace",
					Name:      fmt.Sprintf("my-instance-%d", i),
					Annotations: map[string]string{
						IngressClassAnnotation: "merge",
						ConfigAnnotation:       "kubernetes-shared-ingress",
					},
				},
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{
							Host: fmt.Sprintf("instance-%d.example.org", i),
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
			})
		}

		reconciler := newTestReconciler(objects)

		_, err := reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: "my-namespace",
				Name:      "my-instance-0",
			},
		})

		require.NoError(t, err)

		sharedIngresses, err := getSharedIngresses(ctx, reconciler.Client, "my-namespace")
		require.NoError(t, err)
		require.Len(t, sharedIngresses, 3)

		assert.Equal(t, 100, (len(sharedIngresses[0].Spec.Rules) +
			len(sharedIngresses[1].Spec.Rules) +
			len(sharedIngresses[2].Spec.Rules)))

		_, err = reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: "my-namespace",
				Name:      "my-instance-1",
			},
		})
		require.NoError(t, err)

		sharedIngresses2, err := getSharedIngresses(ctx, reconciler.Client, "my-namespace")
		require.NoError(t, err)
		require.Len(t, sharedIngresses2, 3)

		// should maintain intact the list of sharedIngresses
		assert.Equal(t, sharedIngresses, sharedIngresses2)

		// updated shared ingress status should update also all related ingresses
		sharedIngresses, err = setSharedIngressesLB(ctx, reconciler.Client, "my-namespace", map[string]string{"foo": "bar"})
		require.NoError(t, err)

		_, err = reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: "my-namespace",
				Name:      "my-instance-3",
			},
		})
		require.NoError(t, err)

		for _, sharedIngress := range sharedIngresses {
			for _, ownerReference := range sharedIngress.GetOwnerReferences() {
				if ownerReference.Kind != "Ingress" {
					continue
				}
				instance := networkingv1.Ingress{}
				err = reconciler.Client.Get(ctx, client.ObjectKey{Namespace: "my-namespace", Name: ownerReference.Name}, &instance)
				require.NoError(t, err)
				assert.Equal(t, sharedIngress.Status, instance.Status)
			}
		}

		sharedIngresses, err = setSharedIngressesLB(ctx, reconciler.Client, "my-namespace", nil)
		require.NoError(t, err)

		_, err = reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: "my-namespace",
				Name:      "my-instance-4",
			},
		})
		require.NoError(t, err)

		for _, sharedIngress := range sharedIngresses {
			for _, ownerReference := range sharedIngress.GetOwnerReferences() {
				if ownerReference.Kind != "Ingress" {
					continue
				}
				instance := networkingv1.Ingress{}
				err = reconciler.Client.Get(ctx, client.ObjectKey{Namespace: "my-namespace", Name: ownerReference.Name}, &instance)
				require.NoError(t, err)
				assert.Equal(t, sharedIngress.Status, instance.Status)
			}
		}
	})
}

func setSharedIngressesLB(ctx context.Context, cli client.Client, namespace string, labels map[string]string) ([]networkingv1.Ingress, error) {
	sharedIngresses, err := getSharedIngresses(ctx, cli, namespace)
	if err != nil {
		return nil, err
	}
	for i := range sharedIngresses {
		for k, v := range labels {
			sharedIngresses[i].Labels[k] = v
		}
		sharedIngresses[i].Status = networkingv1.IngressStatus{
			LoadBalancer: corev1.LoadBalancerStatus{
				Ingress: []corev1.LoadBalancerIngress{
					{
						IP: fmt.Sprintf("1.1.8.%d", i),
					},
				},
			},
		}
	}
	err = updateSharedIngresses(ctx, cli, sharedIngresses, namespace)
	if err != nil {
		return nil, err
	}
	return sharedIngresses, err
}

func getSharedIngresses(ctx context.Context, cli client.Client, namespace string) ([]networkingv1.Ingress, error) {
	sharedIngresses := []networkingv1.Ingress{}
	sharedIngressList := networkingv1.IngressList{}
	err := cli.List(ctx, &sharedIngressList, &client.ListOptions{
		Namespace: "my-namespace",
	})

	if err != nil {
		return nil, err
	}

	for _, ingress := range sharedIngressList.Items {
		if strings.HasPrefix(ingress.Name, "kubernetes-shared-ingress") {
			sharedIngresses = append(sharedIngresses, ingress)
		}
	}

	sort.Slice(sharedIngresses, func(i, j int) bool {
		return sharedIngresses[i].Name < sharedIngresses[j].Name
	})

	return sharedIngresses, nil
}

func updateSharedIngresses(ctx context.Context, cli client.Client, sharedIngresses []networkingv1.Ingress, namespace string) error {
	for _, sharedIngress := range sharedIngresses {
		err := cli.Update(ctx, &sharedIngress)
		if err != nil {
			return err
		}
	}
	return nil
}

func newTestReconciler(objs []runtime.Object) *IngressReconciler {
	scheme := runtime.NewScheme()
	_ = networkingv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	reconciler := &IngressReconciler{
		IngressMaxSlots: 45,
		Log:             zap.New(zap.UseDevMode(true)),
		IngressClass:    "merge",
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithRuntimeObjects(objs...).
			Build(),
	}

	return reconciler
}
