package engine

import (
	"context"
	"sort"
	"strconv"

	"github.com/lucasepe/kctx/internal/model"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// serviceEndpoints prefers EndpointSlices and falls back to legacy Endpoints
// when no slices exist for the Service.
func (e *TypedEngine) serviceEndpoints(ctx context.Context, service *corev1.Service, podByIP map[string]corev1.Pod) ([]model.EndpointSummary, error) {
	slices, err := e.kube.ListEndpointSlices(ctx, service.Namespace)
	if err != nil {
		return nil, err
	}

	var matched []discoveryv1.EndpointSlice
	for _, slice := range slices {
		if slice.Labels[serviceNameLabel] == service.Name {
			matched = append(matched, slice)
		}
	}
	if len(matched) > 0 {
		return endpointSummariesFromSlices(matched, podByIP), nil
	}

	endpoints, err := e.kube.GetEndpoints(ctx, service.Namespace, service.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return endpointSummariesFromEndpoints(endpoints, podByIP), nil
}

// endpointSummariesFromSlices flattens EndpointSlices into per-address,
// per-port summaries and correlates each endpoint to a Pod when possible.
func endpointSummariesFromSlices(slices []discoveryv1.EndpointSlice, podByIP map[string]corev1.Pod) []model.EndpointSummary {
	var out []model.EndpointSummary
	for _, slice := range slices {
		for _, endpoint := range slice.Endpoints {
			ready := endpointReady(endpoint.Conditions.Ready)
			podName := targetPodName(endpoint.TargetRef)
			for _, address := range endpoint.Addresses {
				if podName == "" {
					if pod, ok := podByIP[address]; ok {
						podName = pod.Name
					}
				}
				if len(slice.Ports) == 0 {
					out = append(out, endpointSummary(address, 0, "", "", ready, podName, endpointNode(endpoint), "EndpointSlice"))
					continue
				}
				for _, port := range slice.Ports {
					out = append(out, endpointSummary(address, endpointSlicePort(port), endpointSlicePortName(port), endpointSliceProtocol(port), ready, podName, endpointNode(endpoint), "EndpointSlice"))
				}
			}
		}
	}
	sortEndpoints(out)
	return out
}

// endpointSummariesFromEndpoints flattens legacy Endpoints subsets into the
// same normalized shape used for EndpointSlices.
func endpointSummariesFromEndpoints(endpoints *corev1.Endpoints, podByIP map[string]corev1.Pod) []model.EndpointSummary {
	var out []model.EndpointSummary
	for _, subset := range endpoints.Subsets {
		for _, address := range subset.Addresses {
			out = append(out, endpointSummariesFromEndpointAddress(address, subset.Ports, true, podByIP)...)
		}
		for _, address := range subset.NotReadyAddresses {
			out = append(out, endpointSummariesFromEndpointAddress(address, subset.Ports, false, podByIP)...)
		}
	}
	sortEndpoints(out)
	return out
}

// endpointSummariesFromEndpointAddress expands one legacy EndpointAddress
// across all declared endpoint ports.
func endpointSummariesFromEndpointAddress(address corev1.EndpointAddress, ports []corev1.EndpointPort, ready bool, podByIP map[string]corev1.Pod) []model.EndpointSummary {
	podName := targetPodName(address.TargetRef)
	if podName == "" {
		if pod, ok := podByIP[address.IP]; ok {
			podName = pod.Name
		}
	}
	node := ""
	if address.NodeName != nil {
		node = *address.NodeName
	}
	if len(ports) == 0 {
		return []model.EndpointSummary{endpointSummary(address.IP, 0, "", "", ready, podName, node, "Endpoints")}
	}
	out := make([]model.EndpointSummary, 0, len(ports))
	for _, port := range ports {
		out = append(out, endpointSummary(address.IP, port.Port, port.Name, string(port.Protocol), ready, podName, node, "Endpoints"))
	}
	return out
}

// endpointSummary constructs the deterministic endpoint ID used in traces and
// relations.
func endpointSummary(address string, port int32, portName, protocol string, ready bool, podName, node, source string) model.EndpointSummary {
	id := address
	if port > 0 {
		id = id + ":" + strconv.FormatInt(int64(port), 10)
	}
	return model.EndpointSummary{ID: id, Address: address, Port: port, PortName: portName, Protocol: protocol, Ready: ready, Pod: podName, Node: node, Source: source}
}

// endpointReady mirrors Kubernetes readiness semantics where nil means ready.
func endpointReady(ready *bool) bool {
	return ready == nil || *ready
}

// targetPodName returns the referenced Pod name when a targetRef points to a Pod.
func targetPodName(ref *corev1.ObjectReference) string {
	if ref == nil || ref.Kind != "Pod" {
		return ""
	}
	return ref.Name
}

// endpointNode returns the EndpointSlice node name when Kubernetes provided one.
func endpointNode(endpoint discoveryv1.Endpoint) string {
	if endpoint.NodeName == nil {
		return ""
	}
	return *endpoint.NodeName
}

// endpointSlicePort returns the numeric port value or zero when omitted.
func endpointSlicePort(port discoveryv1.EndpointPort) int32 {
	if port.Port == nil {
		return 0
	}
	return *port.Port
}

// endpointSlicePortName returns the EndpointSlice port name or an empty string.
func endpointSlicePortName(port discoveryv1.EndpointPort) string {
	if port.Name == nil {
		return ""
	}
	return *port.Name
}

// endpointSliceProtocol returns the EndpointSlice protocol or an empty string.
func endpointSliceProtocol(port discoveryv1.EndpointPort) string {
	if port.Protocol == nil {
		return ""
	}
	return string(*port.Protocol)
}

// sortEndpoints keeps endpoint summaries stable by address, port, and Pod.
func sortEndpoints(endpoints []model.EndpointSummary) {
	sort.Slice(endpoints, func(i, j int) bool {
		if endpoints[i].Address != endpoints[j].Address {
			return endpoints[i].Address < endpoints[j].Address
		}
		if endpoints[i].Port != endpoints[j].Port {
			return endpoints[i].Port < endpoints[j].Port
		}
		return endpoints[i].Pod < endpoints[j].Pod
	})
}
