package ingress_merge

import (
	"context"
	"reflect"
	"sort"
	"strconv"

	"github.com/ghodss/yaml"
	"github.com/go-logr/logr"
	multierror "github.com/hashicorp/go-multierror"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	IngressClassAnnotation = "kubernetes.io/ingress.class"
	ConfigAnnotation       = "merge.ingress.kubernetes.io/config"
	PriorityAnnotation     = "merge.ingress.kubernetes.io/priority"
	ResultAnnotation       = "merge.ingress.kubernetes.io/result"
)

const (
	NameConfigKey        = "name"
	LabelsConfigKey      = "labels"
	AnnotationsConfigKey = "annotations"
	BackendConfigKey     = "backend"
)

var _ reconcile.Reconciler = &IngressReconciler{}

type IngressReconciler struct {
	client.Client
	Log logr.Logger

	IngressClass         string
	IngressSelector      string
	ConfigMapSelector    string
	IngressWatchIgnore   []string
	ConfigMapWatchIgnore []string
}

func (r *IngressReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ingress := &networkingv1.Ingress{}

	err := r.Get(ctx, client.ObjectKey{
		Namespace: req.Namespace,
		Name:      req.Name,
	}, ingress)
	if err != nil {
		r.Log.Error(err, "could not get ingress object")
		return ctrl.Result{}, err
	}

	ingressClass := ingress.Annotations[IngressClassAnnotation]

	if ingressClass != r.IngressClass {
		r.Log.Info("ingress does not match ingressClass, ignoring",
			"ingress", req.String(),
			"ingressClass", r.IngressClass)
		return ctrl.Result{}, nil
	}

	err = r.reconcileNamespace(ctx, req.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *IngressReconciler) reconcileNamespace(ctx context.Context, ns string) error {
	ingresses := &networkingv1.IngressList{}
	err := r.Client.List(ctx, ingresses, &client.ListOptions{
		Namespace: ns,
	})

	if err != nil {
		return err
	}

	var (
		mergeMap   = make(map[string][]networkingv1.Ingress)
		configMaps = make(map[string]corev1.ConfigMap)
	)

	for _, ingress := range ingresses.Items {
		ingressClass := ingress.Annotations[IngressClassAnnotation]
		if ingressClass != r.IngressClass {
			continue
		}

		if r.isIgnored(&ingress) {
			continue
		}

		if priorityString, exists := ingress.Annotations[PriorityAnnotation]; exists {
			if _, err := strconv.Atoi(priorityString); err != nil {
				r.Log.Error(err, "ingress annotation must be an integer",
					"ingress", ingress.Name,
					"namespace", ingress.Namespace,
					"annotation", PriorityAnnotation,
				)
				// TODO: emit error event on ingress that priority must be integer

				continue
			}
		}

		configMapName, exists := ingress.Annotations[ConfigAnnotation]
		if !exists {
			r.Log.Error(nil, "ingress is missing annotation",
				"ingress", ingress.Name,
				"namespace", ingress.Namespace,
				"annotation", ConfigAnnotation,
			)
			// TODO: emit error event on ingress that no config map name is set
			continue
		}

		configMap := corev1.ConfigMap{}
		configMap, exists = configMaps[configMapName]

		if !exists {
			err := r.Get(ctx, client.ObjectKey{
				Namespace: ingress.Namespace,
				Name:      configMapName,
			}, &configMap)

			if err != nil {
				if k8sErrors.IsNotFound(err) {
					r.Log.Error(err, "configMap is not found", "name", configMapName, "ns", ns)
					continue
				}

				return err
			}

			configMaps[configMapName] = configMap
		}

		mergeMap[configMapName] = append(mergeMap[configMapName], ingress)
	}

	var errors error

	for configMapName, ingresses := range mergeMap {
		configMap := configMaps[configMapName]
		err = r.reconcileConfigMap(ctx, ns, configMap, ingresses)

		if err != nil {
			multierror.Append(errors, err)
		}
	}

	return errors
}

func (r *IngressReconciler) reconcileConfigMap(ctx context.Context, ns string, configMap corev1.ConfigMap, ingresses []networkingv1.Ingress) error {

	sort.Slice(ingresses, func(i, j int) bool {
		var (
			a         = ingresses[i]
			b         = ingresses[j]
			priorityA = 0
			priorityB = 0
		)

		if priorityString, exits := a.Annotations[PriorityAnnotation]; exits {
			priorityA, _ = strconv.Atoi(priorityString)
		}

		if priorityString, exits := b.Annotations[PriorityAnnotation]; exits {
			priorityB, _ = strconv.Atoi(priorityString)
		}

		if priorityA > priorityB {
			return true
		} else if priorityA < priorityB {
			return false
		} else {
			return a.Name < b.Name
		}
	})

	var (
		ownerReferences []metaV1.OwnerReference
		tls             []networkingv1.IngressTLS
		rules           []networkingv1.IngressRule
	)

	for _, ingress := range ingresses {
		ownerReferences = append(ownerReferences, metaV1.OwnerReference{
			APIVersion: "extensions/v1beta1",
			Kind:       "Ingress",
			Name:       ingress.Name,
			UID:        ingress.UID,
		})

		// FIXME: merge by SecretName/Hosts?
		tls = append(tls, ingress.Spec.TLS...)

	rules:
		for _, r := range ingress.Spec.Rules {
			for _, s := range rules {
				if r.Host == s.Host {
					s.HTTP.Paths = append(s.HTTP.Paths, r.HTTP.Paths...)
					continue rules
				}
			}

			rules = append(rules, *r.DeepCopy())
		}
	}

	var (
		name        string
		labels      map[string]string
		annotations map[string]string
		backend     *networkingv1.IngressBackend
	)

	if dataName, exists := configMap.Data[NameConfigKey]; exists {
		name = dataName
	} else {
		name = configMap.Name
	}

	if dataLabels, exists := configMap.Data[LabelsConfigKey]; exists {
		if err := yaml.Unmarshal([]byte(dataLabels), &labels); err != nil {
			labels = nil
			r.Log.Error(err, "Could unmarshal labels from configmap",
				"namespace", configMap.Namespace,
				"configmap", configMap.Name,
			)
		}
	}

	if dataAnnotations, exists := configMap.Data[AnnotationsConfigKey]; exists {
		if err := yaml.Unmarshal([]byte(dataAnnotations), &annotations); err != nil {
			annotations = nil
			r.Log.Error(err, "Could unmarshal annotations from configmap",
				"namespace", configMap.Namespace,
				"configmap", configMap.Name,
			)
		}

		if annotations[IngressClassAnnotation] == r.IngressClass {
			r.Log.Error(nil, "trying to create merged ingress of merge ingress class, you have to change ingress class",
				"namespace", configMap.Namespace,
				"configmap", configMap.Name,
				"ingress_class", r.IngressClass,
			)
			return nil
		}
	}

	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[ResultAnnotation] = "true"

	if dataBackend, exists := configMap.Data[BackendConfigKey]; exists {
		if err := yaml.Unmarshal([]byte(dataBackend), &backend); err != nil {
			backend = nil
			r.Log.Error(err, "Could not unmarshal backend from config",
				"namespace", configMap.Namespace,
				"config_map", configMap.Name)
		}
	}

	mergedIngress := &networkingv1.Ingress{
		ObjectMeta: metaV1.ObjectMeta{
			Namespace:       configMap.Namespace,
			Name:            name,
			Labels:          labels,
			Annotations:     annotations,
			OwnerReferences: ownerReferences,
		},
		Spec: networkingv1.IngressSpec{
			DefaultBackend: backend,
			TLS:            tls,
			Rules:          rules,
		},
	}

	var existingMergedIngress networkingv1.Ingress

	err := r.Get(ctx, client.ObjectKey{
		Namespace: configMap.Namespace,
		Name:      configMap.Name,
	}, &existingMergedIngress)
	existingMergedIngressFound := !k8sErrors.IsNotFound(err)

	if err != nil && existingMergedIngressFound {
		return err
	}
	changed := false

	if existingMergedIngressFound {
		if r.hasIngressChanged(&existingMergedIngress, mergedIngress) {
			changed = true

			mergedIngress.ObjectMeta.ResourceVersion = existingMergedIngress.ObjectMeta.ResourceVersion
			err = r.Update(ctx, mergedIngress)

			if err != nil {
				r.Log.Error(err, "could not update ingress",
					"namespace", mergedIngress.Namespace,
					"name", mergedIngress.Name,
				)
				return err
			}

			r.Log.Info("Updated merged ingress",
				"namespace", mergedIngress.Namespace,
				"name", mergedIngress.Name)

			mergedIngress.Status = existingMergedIngress.Status
		} else {
			mergedIngress = &existingMergedIngress
		}

	} else {
		changed = true
		err = r.Create(ctx, mergedIngress)
		if err != nil {
			r.Log.Error(err, "could not create ingress", "ingress", mergedIngress.Name, "namespace", ns)
			return err
		}

		r.Log.Info("Created merged ingress",
			"namespace", mergedIngress.Namespace,
			"name", mergedIngress.Name)
	}

	for _, ingress := range ingresses {
		if reflect.DeepEqual(ingress.Status, mergedIngress.Status) {
			continue
		}

		mergedIngress.Status.DeepCopyInto(&ingress.Status)

		changed = true
		err = r.Status().Update(ctx, &ingress)
		if err != nil {
			r.Log.Error(
				err, "Could not update status of ingress",
				"namespace", ingress.Namespace,
				"ingress", ingress.Name,
			)
			continue
		}

		r.Log.Info("Propagated ingress status back",
			"namespace", mergedIngress.Name,
			"from_ingress", mergedIngress.Name,
			"to_ingress", ingress.Name,
		)
	}

	if !changed {
		r.Log.Info("Nothing changed")
	}

	return nil
}

func (r *IngressReconciler) isIgnored(obj *networkingv1.Ingress) bool {
	for _, val := range r.IngressWatchIgnore {
		if _, exists := obj.Annotations[val]; exists {
			return true
		}
	}

	return false
}

func (r *IngressReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1.Ingress{}).
		Complete(r)
}

func (r *IngressReconciler) hasIngressChanged(old, new *networkingv1.Ingress) bool {
	if new.Namespace != old.Namespace {
		return true
	}
	if new.Name != old.Name {
		return true
	}
	if !reflect.DeepEqual(new.Labels, old.Labels) {
		return true
	}

	for k := range new.Annotations {
		if new.Annotations[k] != old.Annotations[k] {
			r.Log.Info("Change of annotation will trigger a change",
				"annotation", k,
				"namespace", old.Namespace,
				"ingress", old.Name)
			return true
		}
	}

	if !reflect.DeepEqual(new.OwnerReferences, old.OwnerReferences) {
		return true
	}
	if !reflect.DeepEqual(new.Spec, old.Spec) {
		return true
	}

	return false
}
