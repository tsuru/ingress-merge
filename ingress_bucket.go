package ingress_merge

import (
	"sort"

	networkingv1 "k8s.io/api/networking/v1"
	types "k8s.io/apimachinery/pkg/types"
)

type IngressBucket struct {
	Name               string
	FreeSlots          int
	Ingresses          []networkingv1.Ingress
	DestinationIngress *networkingv1.Ingress
}

func GenerateIngressBuckets(origins, destinations []networkingv1.Ingress, maxServices int) []*IngressBucket {
	type dependencyKey struct {
		name string
		uid  types.UID
	}

	bucketsWithDestinationMap := map[dependencyKey]*IngressBucket{}
	bucketsWithDestination := []*IngressBucket{}
	bucketsWithoutDestination := []*IngressBucket{}
	ingressesWithoutDestination := []networkingv1.Ingress{}
	originToDestinationMap := map[dependencyKey]*networkingv1.Ingress{}

	for i := range destinations {
		k := dependencyKey{destinations[i].Name, destinations[i].UID}
		bucketsWithDestinationMap[k] = &IngressBucket{
			Name:               destinations[i].Name,
			FreeSlots:          maxServices,
			DestinationIngress: &destinations[i],
		}

		for _, ownerReference := range destinations[i].OwnerReferences {
			if ownerReference.Kind != "Ingress" {
				continue
			}

			k = dependencyKey{ownerReference.Name, ownerReference.UID}
			originToDestinationMap[k] = &destinations[i]
		}
	}

	for _, bucket := range bucketsWithDestinationMap {
		bucketsWithDestination = append(bucketsWithDestination, bucket)
	}

	for _, origin := range origins {
		k := dependencyKey{origin.Name, origin.UID}

		if originToDestinationMap[k] != nil {
			destination := originToDestinationMap[k]
			destinationKey := dependencyKey{destination.Name, destination.UID}
			bucketsWithDestinationMap[destinationKey].Ingresses = append(bucketsWithDestinationMap[destinationKey].Ingresses, origin)
			bucketsWithDestinationMap[destinationKey].FreeSlots -= ingressSlots(&origin)
			continue
		}

		ingressesWithoutDestination = append(ingressesWithoutDestination, origin)
	}

	sort.Slice(ingressesWithoutDestination, func(i, j int) bool {
		slotsI := ingressSlots(&ingressesWithoutDestination[i])
		slotsJ := ingressSlots(&ingressesWithoutDestination[j])

		if slotsI != slotsJ {
			return slotsI < slotsJ
		}

		return ingressesWithoutDestination[i].ObjectMeta.CreationTimestamp.After(ingressesWithoutDestination[j].ObjectMeta.CreationTimestamp.Time)
	})

	sort.Slice(bucketsWithDestination, func(i, j int) bool {
		return bucketsWithDestination[i].Name < bucketsWithDestination[j].Name
	})

	var currentBucket *IngressBucket
	reuseBucketsWithAddrPos := -1

	nextBucket := func() {
		for reuseBucketsWithAddrPos < len(bucketsWithDestination)-1 {
			reuseBucketsWithAddrPos++
			currentBucket = bucketsWithDestination[reuseBucketsWithAddrPos]

			if currentBucket.FreeSlots > 0 {
				return
			}
		}
		currentBucket = &IngressBucket{
			FreeSlots: maxServices,
		}
		bucketsWithoutDestination = append(bucketsWithoutDestination, currentBucket)
	}

	if len(ingressesWithoutDestination) > 0 {
		nextBucket()
	}

	for _, ingress := range ingressesWithoutDestination {
		slots := ingressSlots(&ingress)

		if currentBucket.FreeSlots-slots < 0 {
			nextBucket()
		}

		currentBucket.Ingresses = append(currentBucket.Ingresses, ingress)
		currentBucket.FreeSlots -= slots
	}

	result := []*IngressBucket{}
	result = append(result, bucketsWithDestination...)
	result = append(result, bucketsWithoutDestination...)
	return result
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

	if slots == 0 {
		return 1
	}

	return slots
}
