package engine

import (
	"github.com/lucasepe/kctx/internal/model"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
)

// namespaceServiceHealth summarizes Service endpoint availability from
// EndpointSlices.
func namespaceServiceHealth(services []corev1.Service, slices []discoveryv1.EndpointSlice) []model.ServiceHealth {
	out := make([]model.ServiceHealth, 0, len(services))
	for _, service := range services {
		ready, total := readyEndpointsForService(service.Name, slices)
		out = append(out, model.ServiceHealth{
			Namespace:      service.Namespace,
			Name:           service.Name,
			Type:           string(service.Spec.Type),
			ReadyEndpoints: ready,
			TotalEndpoints: total,
		})
	}
	return out
}

// readyEndpointsForService counts total and ready endpoints in EndpointSlices
// that belong to a Service.
func readyEndpointsForService(serviceName string, slices []discoveryv1.EndpointSlice) (int, int) {
	ready := 0
	total := 0
	for _, slice := range slices {
		if slice.Labels[serviceNameLabel] != serviceName {
			continue
		}
		for _, endpoint := range slice.Endpoints {
			total++
			if endpointReady(endpoint.Conditions.Ready) {
				ready++
			}
		}
	}
	return ready, total
}

// namespacePVCHealth summarizes PersistentVolumeClaim phases.
func namespacePVCHealth(pvcs []corev1.PersistentVolumeClaim) []model.PVCHealth {
	out := make([]model.PVCHealth, 0, len(pvcs))
	for _, pvc := range pvcs {
		out = append(out, model.PVCHealth{
			Namespace: pvc.Namespace,
			Name:      pvc.Name,
			Phase:     string(pvc.Status.Phase),
		})
	}
	return out
}
