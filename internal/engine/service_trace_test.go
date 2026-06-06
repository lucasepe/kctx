package engine_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/lucasepe/kctx/internal/engine"
	"github.com/lucasepe/kctx/internal/render"
	"github.com/lucasepe/kctx/internal/testutil"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestTraceServiceSelectorMatchesPods(t *testing.T) {
	service := testService("payments-api", "payments", map[string]string{"app": "api"})
	pod := testPod("api-1", "payments")
	pod.Labels = map[string]string{"app": "api"}
	pod.Status.PodIP = "10.244.1.10"

	got := traceService(t, service, pod)

	if len(got.Pods) != 1 || got.Pods[0].Name != "api-1" {
		t.Fatalf("unexpected selected pods: %#v", got.Pods)
	}
	assertTraceRelation(t, got, "selects", "Service", "Pod")
}

func TestTraceServiceSelectorMatchesNoPods(t *testing.T) {
	service := testService("payments-api", "payments", map[string]string{"app": "api"})

	got := traceService(t, service)

	assertTraceSignal(t, got, "selector_matches_no_pods")
	assertTraceSignal(t, got, "no_endpoints_found")
	assertTraceSignal(t, got, "service_has_no_usable_backends")
}

func TestTraceServiceEndpointSliceReadyAndUnreadyEndpoints(t *testing.T) {
	service := testService("payments-api", "payments", map[string]string{"app": "api"})
	readyPod := backendPod("api-1", "payments", "10.244.1.10", true)
	unreadyPod := backendPod("api-2", "payments", "10.244.1.11", false)
	slice := endpointSlice("payments-api", "payments",
		sliceEndpoint("10.244.1.10", true, "api-1", "worker-01"),
		sliceEndpoint("10.244.1.11", false, "api-2", "worker-02"),
	)

	got := traceService(t, service, readyPod, unreadyPod, slice)

	if len(got.Endpoints) != 2 {
		t.Fatalf("expected two endpoints, got %#v", got.Endpoints)
	}
	assertTraceSignal(t, got, "endpoint_not_ready")
	assertTraceSignal(t, got, "selected_pod_not_ready")
}

func TestTraceServiceEndpointSliceTargetRefResolvesToPod(t *testing.T) {
	service := testService("payments-api", "payments", map[string]string{"app": "api"})
	pod := backendPod("api-1", "payments", "10.244.1.10", true)
	slice := endpointSlice("payments-api", "payments", sliceEndpoint("10.244.1.10", true, "api-1", "worker-01"))

	got := traceService(t, service, pod, slice)

	if len(got.Endpoints) != 1 || got.Endpoints[0].Pod != "api-1" {
		t.Fatalf("endpoint did not resolve to pod: %#v", got.Endpoints)
	}
	assertTraceRelation(t, got, "endpoint_targets", "Endpoint", "Pod")
}

func TestTraceServiceEndpointWithoutPodWarns(t *testing.T) {
	service := testService("payments-api", "payments", map[string]string{"app": "api"})
	slice := endpointSlice("payments-api", "payments", sliceEndpoint("10.244.1.99", true, "", "worker-01"))

	got := traceService(t, service, slice)

	assertTraceSignal(t, got, "endpoint_without_pod")
}

func TestTraceServiceSelectedPodMissingFromEndpointsWarns(t *testing.T) {
	service := testService("payments-api", "payments", map[string]string{"app": "api"})
	pod := backendPod("api-1", "payments", "10.244.1.10", true)
	slice := endpointSlice("payments-api", "payments", sliceEndpoint("10.244.1.99", true, "", "worker-01"))

	got := traceService(t, service, pod, slice)

	assertTraceSignal(t, got, "selected_pod_missing_from_endpoints")
}

func TestTraceServiceExternalNameGraceful(t *testing.T) {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "external-db", Namespace: "payments"},
		Spec: corev1.ServiceSpec{
			Type:         corev1.ServiceTypeExternalName,
			ExternalName: "db.example.test",
			Ports:        []corev1.ServicePort{{Name: "postgres", Protocol: corev1.ProtocolTCP, Port: 5432}},
		},
	}

	got := traceService(t, service)

	if got.Service.ExternalName != "db.example.test" {
		t.Fatalf("unexpected external name: %#v", got.Service)
	}
	if len(got.Pods) != 0 || len(got.Endpoints) != 0 {
		t.Fatalf("external name should not resolve pods/endpoints: pods=%#v endpoints=%#v", got.Pods, got.Endpoints)
	}
	assertTraceSignal(t, got, "external_name_service")
}

func TestTraceServiceWithoutSelectorWithEndpointsFallback(t *testing.T) {
	service := testService("manual-api", "payments", nil)
	endpoints := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{Name: "manual-api", Namespace: "payments"},
		Subsets: []corev1.EndpointSubset{{
			Addresses: []corev1.EndpointAddress{{IP: "10.244.1.20"}},
			Ports:     []corev1.EndpointPort{{Name: "http", Protocol: corev1.ProtocolTCP, Port: 8080}},
		}},
	}

	got := traceService(t, service, endpoints)

	if len(got.Endpoints) != 1 || got.Endpoints[0].Source != "Endpoints" {
		t.Fatalf("expected legacy endpoints fallback, got %#v", got.Endpoints)
	}
	assertTraceSignal(t, got, "service_has_no_selector")
}

func TestTraceServicePodOwnerChainResolvesDeployment(t *testing.T) {
	service := testService("payments-api", "payments", map[string]string{"app": "api"})
	pod := backendPod("api-1", "payments", "10.244.1.10", true)
	pod.OwnerReferences = []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "ReplicaSet", Name: "api-7d9f8"}}
	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "api-7d9f8",
			Namespace: "payments",
			OwnerReferences: []metav1.OwnerReference{
				{APIVersion: "apps/v1", Kind: "Deployment", Name: "api"},
			},
		},
	}
	deploy := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "payments"}}
	slice := endpointSlice("payments-api", "payments", sliceEndpoint("10.244.1.10", true, "api-1", "worker-01"))

	got := traceService(t, service, pod, rs, deploy, slice)

	assertTraceOwner(t, got, "ReplicaSet", "api-7d9f8")
	assertTraceOwner(t, got, "Deployment", "api")
	assertTraceRelation(t, got, "owned_by", "Pod", "ReplicaSet")
	assertTraceRelation(t, got, "owned_by", "ReplicaSet", "Deployment")
}

func TestTraceServiceJSONOutputDeterministic(t *testing.T) {
	service := testService("payments-api", "payments", map[string]string{"app": "api"})
	pod := backendPod("api-1", "payments", "10.244.1.10", true)
	slice := endpointSlice("payments-api", "payments", sliceEndpoint("10.244.1.10", true, "api-1", "worker-01"))
	got := traceService(t, service, pod, slice)

	var first bytes.Buffer
	var second bytes.Buffer
	if err := render.JSON(&first, got); err != nil {
		t.Fatalf("render JSON first: %v", err)
	}
	if err := render.JSON(&second, got); err != nil {
		t.Fatalf("render JSON second: %v", err)
	}
	if first.String() != second.String() {
		t.Fatalf("JSON output is not deterministic:\nfirst:\n%s\nsecond:\n%s", first.String(), second.String())
	}
	for _, want := range []string{`"service"`, `"endpoints"`, `"pods"`, `"relations"`} {
		if !strings.Contains(first.String(), want) {
			t.Fatalf("missing %s in JSON:\n%s", want, first.String())
		}
	}
}

func traceService(t *testing.T, objects ...runtime.Object) *engine.TraceServiceResponse {
	t.Helper()
	var target *corev1.Service
	for _, object := range objects {
		if service, ok := object.(*corev1.Service); ok && target == nil {
			target = service
		}
	}
	if target == nil {
		t.Fatal("traceService helper requires a Service object")
	}
	reader := testutil.NewFakeReader(objects...)
	got, err := engine.New(reader).TraceService(context.Background(), engine.TraceServiceRequest{Namespace: target.Namespace, Name: target.Name})
	if err != nil {
		t.Fatalf("TraceService() error = %v", err)
	}
	return got
}

func testService(name, namespace string, selector map[string]string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, UID: types.UID(namespace + "-" + name + "-uid")},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeClusterIP,
			Selector:  selector,
			ClusterIP: "10.96.42.17",
			Ports: []corev1.ServicePort{{
				Name:       "http",
				Protocol:   corev1.ProtocolTCP,
				Port:       80,
				TargetPort: intstr.FromInt32(8080),
			}},
		},
	}
}

func backendPod(name, namespace, ip string, ready bool) *corev1.Pod {
	pod := testPod(name, namespace)
	pod.Labels = map[string]string{"app": "api"}
	pod.Spec.NodeName = "worker-01"
	pod.Status.PodIP = ip
	pod.Status.Phase = corev1.PodRunning
	status := corev1.ConditionFalse
	if ready {
		status = corev1.ConditionTrue
	}
	pod.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodReady, Status: status}}
	pod.Status.ContainerStatuses = []corev1.ContainerStatus{{Name: "api", Ready: ready, RestartCount: 2}}
	return pod
}

func endpointSlice(serviceName, namespace string, endpoints ...discoveryv1.Endpoint) *discoveryv1.EndpointSlice {
	name := serviceName + "-slice"
	protocol := corev1.ProtocolTCP
	portName := "http"
	port := int32(8080)
	return &discoveryv1.EndpointSlice{
		ObjectMeta:  metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: map[string]string{"kubernetes.io/service-name": serviceName}},
		AddressType: discoveryv1.AddressTypeIPv4,
		Ports: []discoveryv1.EndpointPort{{
			Name:     &portName,
			Protocol: &protocol,
			Port:     &port,
		}},
		Endpoints: endpoints,
	}
}

func sliceEndpoint(address string, ready bool, podName, nodeName string) discoveryv1.Endpoint {
	endpoint := discoveryv1.Endpoint{
		Addresses:  []string{address},
		Conditions: discoveryv1.EndpointConditions{Ready: &ready},
		NodeName:   &nodeName,
	}
	if podName != "" {
		endpoint.TargetRef = &corev1.ObjectReference{Kind: "Pod", Namespace: "payments", Name: podName}
	}
	return endpoint
}

func assertTraceSignal(t *testing.T, got *engine.TraceServiceResponse, reason string) {
	t.Helper()
	for _, signal := range got.Signals {
		if signal.Reason == reason {
			return
		}
	}
	t.Fatalf("missing signal %s in %#v", reason, got.Signals)
}

func assertTraceRelation(t *testing.T, got *engine.TraceServiceResponse, relationType, sourceKind, targetKind string) {
	t.Helper()
	for _, relation := range got.Relations {
		if relation.Type == relationType && relation.Source.Kind == sourceKind && relation.Target.Kind == targetKind {
			return
		}
	}
	t.Fatalf("missing relation %s %s->%s in %#v", relationType, sourceKind, targetKind, got.Relations)
}

func assertTraceOwner(t *testing.T, got *engine.TraceServiceResponse, kind, name string) {
	t.Helper()
	for _, owner := range got.Owners {
		if owner.Kind == kind && owner.Name == name {
			return
		}
	}
	t.Fatalf("missing owner %s/%s in %#v", kind, name, got.Owners)
}
