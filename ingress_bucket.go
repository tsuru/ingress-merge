package ingress_merge

import (
	"errors"
	"sort"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

type IngressBucket struct {
	key                string // for sort propourses
	FreeSlots          int
	LoadBalancerStatus corev1.LoadBalancerStatus
	Ingresses          []networkingv1.Ingress
}

var logger = zap.New(zap.UseDevMode(true))

func GenerateIngressBuckets(origins []networkingv1.Ingress, maxServices int) []*IngressBucket {
	// first step group by ingress that have status filled

	// we dont touch in ingress that received a status, is mean that they are working
	bucketsWithAddressMap := map[string]*IngressBucket{}
	bucketsWithAddress := []*IngressBucket{}
	bucketsWithoutAddress := []*IngressBucket{}

	ingressesWithoutAddress := []networkingv1.Ingress{}

	for _, origin := range origins {
		lbKey, err := loadBalancerKey(origin.Status.LoadBalancer)
		if err != nil {
			logger.Error(err, "", "ingress", origin.Name, "namespace", origin.Name)
			continue
		}
		if lbKey != "" {
			if bucketsWithAddressMap[lbKey] == nil {
				bucketsWithAddressMap[lbKey] = &IngressBucket{
					FreeSlots: maxServices,
				}
			}

			bucketsWithAddressMap[lbKey].key = lbKey
			bucketsWithAddressMap[lbKey].LoadBalancerStatus = origin.Status.LoadBalancer
			bucketsWithAddressMap[lbKey].Ingresses = append(bucketsWithAddressMap[lbKey].Ingresses, origin)
			bucketsWithAddressMap[lbKey].FreeSlots -= ingressSlots(&origin)
			continue
		}

		ingressesWithoutAddress = append(ingressesWithoutAddress, origin)
	}

	sort.Slice(ingressesWithoutAddress, func(i, j int) bool {
		return ingressesWithoutAddress[i].ObjectMeta.CreationTimestamp.After(ingressesWithoutAddress[j].ObjectMeta.CreationTimestamp.Time)
	})

	for _, bucket := range bucketsWithAddressMap {
		bucketsWithAddress = append(bucketsWithAddress, bucket)
	}

	sort.Slice(bucketsWithAddress, func(i, j int) bool {
		return bucketsWithAddress[i].key < bucketsWithAddress[j].key
	})

	var currentBucket *IngressBucket
	reuseBucketsWithAddrPos := -1

	nextBucket := func() {
		for reuseBucketsWithAddrPos < len(bucketsWithAddress)-1 {
			reuseBucketsWithAddrPos++
			currentBucket = bucketsWithAddress[reuseBucketsWithAddrPos]

			if currentBucket.FreeSlots > 0 {
				return
			}
		}
		currentBucket = &IngressBucket{
			FreeSlots: maxServices,
		}
		bucketsWithoutAddress = append(bucketsWithoutAddress, currentBucket)
	}

	if len(ingressesWithoutAddress) > 0 {
		nextBucket()
	}

	for _, ingress := range ingressesWithoutAddress {
		slots := ingressSlots(&ingress)

		if currentBucket.FreeSlots-slots < 0 {
			nextBucket()
		}

		currentBucket.Ingresses = append(currentBucket.Ingresses, ingress)
		currentBucket.FreeSlots -= slots
	}

	result := []*IngressBucket{}
	result = append(result, bucketsWithAddress...)
	result = append(result, bucketsWithoutAddress...)
	return result
}

func loadBalancerKey(status corev1.LoadBalancerStatus) (string, error) {
	if len(status.Ingress) == 0 {
		return "", nil
	}

	if status.Ingress[0].IP != "" {
		return "ip:" + status.Ingress[0].IP, nil
	}

	if status.Ingress[0].Hostname != "" {
		return "hostname:" + status.Ingress[0].Hostname, nil
	}

	return "", errors.New("not found a ip or hostname in ingress status")
}

func ingressSlots(ingress *networkingv1.Ingress) int {
	slots := 0

	for _, rule := range ingress.Spec.Rules {
		paths := len(rule.HTTP.Paths)
		if paths == 0 {
			paths = 1
		}
		slots += paths
	}

	return slots
}
