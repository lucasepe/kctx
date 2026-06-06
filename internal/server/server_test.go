package server_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lucasepe/kctx/internal/engine"
	"github.com/lucasepe/kctx/internal/server"
	"github.com/lucasepe/kctx/internal/testutil"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestLivez(t *testing.T) {
	rec := get(t, testHandler(t), "/livez")

	assertStatus(t, rec, http.StatusOK)
	assertContentType(t, rec, "application/json")
	assertBodyContains(t, rec, `"status": "alive"`)
}

func TestReadyz(t *testing.T) {
	rec := get(t, testHandler(t), "/readyz")

	assertStatus(t, rec, http.StatusOK)
	assertContentType(t, rec, "application/json")
	assertBodyContains(t, rec, `"status": "ready"`)
}

func TestMetrics(t *testing.T) {
	handler := testHandler(t)
	_ = get(t, handler, "/livez")
	rec := get(t, handler, "/metrics")

	assertStatus(t, rec, http.StatusOK)
	assertContentType(t, rec, "application/json")
	assertBodyContains(t, rec, `"http_requests_total"`)
	assertBodyContains(t, rec, `"GET /livez 200"`)
}

func TestHealthzRemoved(t *testing.T) {
	rec := get(t, testHandler(t), "/healthz")

	assertStatus(t, rec, http.StatusNotFound)
}

func TestVersion(t *testing.T) {
	rec := get(t, testHandler(t), "/version")

	assertStatus(t, rec, http.StatusOK)
	assertContentType(t, rec, "application/json")
	assertBodyContains(t, rec, `"name": "kctx"`)
	assertBodyContains(t, rec, `"version": "dev"`)
}

func TestContextPod(t *testing.T) {
	rec := get(t, testHandler(t, fixtureObjects()...), "/context/pod/payments/api-1")

	assertStatus(t, rec, http.StatusOK)
	assertContentType(t, rec, "application/json")
	assertBodyContains(t, rec, `"kind": "Pod"`)
	assertBodyContains(t, rec, `"name": "api-1"`)
}

func TestContextPodRejectsEventLimitOverMaximum(t *testing.T) {
	rec := get(t, testHandler(t, fixtureObjects()...), "/context/pod/payments/api-1?eventLimit=501")

	assertStatus(t, rec, http.StatusBadRequest)
	assertContentType(t, rec, "application/json")
	assertBodyContains(t, rec, `"code":"bad_request"`)
	assertBodyContains(t, rec, `eventLimit must be less than or equal to 500`)
}

func TestGraphPod(t *testing.T) {
	rec := get(t, testHandler(t, fixtureObjects()...), "/graph/pod/payments/api-1")

	assertStatus(t, rec, http.StatusOK)
	assertContentType(t, rec, "application/json")
	assertBodyContains(t, rec, `"nodes"`)
	assertBodyContains(t, rec, `"edges"`)
}

func TestTraceService(t *testing.T) {
	rec := get(t, testHandler(t, fixtureObjects()...), "/trace/service/payments/payments-api")

	assertStatus(t, rec, http.StatusOK)
	assertContentType(t, rec, "application/json")
	assertBodyContains(t, rec, `"service"`)
	assertBodyContains(t, rec, `"endpoints"`)
	assertBodyContains(t, rec, `"payments-api"`)
}

func TestHealthNamespace(t *testing.T) {
	rec := get(t, testHandler(t, fixtureObjects()...), "/health/namespace/payments")

	assertStatus(t, rec, http.StatusOK)
	assertContentType(t, rec, "application/json")
	assertBodyContains(t, rec, `"namespace": "payments"`)
	assertBodyContains(t, rec, `"summary"`)
}

func TestDumpNamespace(t *testing.T) {
	rec := get(t, testHandler(t, fixtureObjects()...), "/dump/namespace/payments")

	assertStatus(t, rec, http.StatusOK)
	assertContentType(t, rec, "application/json")
	assertBodyContains(t, rec, `"entities"`)
	assertBodyContains(t, rec, `"relations"`)
	assertBodyContains(t, rec, `"signals"`)
}

func TestKubeAPIBudgetExceeded(t *testing.T) {
	handler := testHandlerWithOptions(t, []runtime.Object{testNamespace("payments")}, server.WithKubeAPIBudget(1))

	rec := get(t, handler, "/dump/namespace/payments")

	assertStatus(t, rec, http.StatusTooManyRequests)
	assertContentType(t, rec, "application/json")
	assertBodyContains(t, rec, `"code":"limit_exceeded"`)
}

func TestJSONErrorResponses(t *testing.T) {
	rec := get(t, testHandler(t, testNamespace("payments")), "/context/pod/payments/missing")

	assertStatus(t, rec, http.StatusNotFound)
	assertContentType(t, rec, "application/json")
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("error response is not JSON: %v\n%s", err, rec.Body.String())
	}
	if body.Error.Code != "not_found" {
		t.Fatalf("unexpected error code %q in %s", body.Error.Code, rec.Body.String())
	}
}

func TestMethodNotAllowed(t *testing.T) {
	handler := testHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/livez", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assertStatus(t, rec, http.StatusMethodNotAllowed)
	assertContentType(t, rec, "application/json")
	assertBodyContains(t, rec, `"code":"method_not_allowed"`)
}

func testHandler(t *testing.T, objects ...runtime.Object) http.Handler {
	t.Helper()
	return testHandlerWithOptions(t, objects)
}

func testHandlerWithOptions(t *testing.T, objects []runtime.Object, opts ...server.Option) http.Handler {
	t.Helper()
	reader := testutil.NewFakeReader(objects...)
	return server.New(engine.New(reader), slog.Default(), opts...).Handler()
}

func get(t *testing.T, handler http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func fixtureObjects() []runtime.Object {
	pod := backendPod("api-1", "payments", "10.244.1.10", false)
	pod.OwnerReferences = []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "ReplicaSet", Name: "api-7d9f8"}}
	pod.Spec.Volumes = []corev1.Volume{
		{Name: "config", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "api-config"}}}},
		{Name: "secret", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "db-credentials"}}},
		{Name: "data", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "api-data"}}},
	}
	replicas := int32(1)
	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "api-7d9f8",
			Namespace: "payments",
			OwnerReferences: []metav1.OwnerReference{
				{APIVersion: "apps/v1", Kind: "Deployment", Name: "api"},
			},
		},
		Spec:   appsv1.ReplicaSetSpec{Replicas: &replicas},
		Status: appsv1.ReplicaSetStatus{Replicas: 1, ReadyReplicas: 0},
	}
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "payments"},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		Status:     appsv1.DeploymentStatus{Replicas: 1, ReadyReplicas: 0, AvailableReplicas: 0},
	}
	service := testService("payments-api", "payments", map[string]string{"app": "api"})
	slice := endpointSlice("payments-api", "payments", sliceEndpoint("10.244.1.10", false, "api-1", "worker-01"))
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-01"}}
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "api-config", Namespace: "payments"}}
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "db-credentials", Namespace: "payments"}, Type: corev1.SecretTypeOpaque}
	pvc := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "api-data", Namespace: "payments"}, Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound}}
	return []runtime.Object{testNamespace("payments"), pod, rs, deploy, service, slice, node, cm, secret, pvc}
}

func testNamespace(name string) *corev1.Namespace {
	return &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}, Status: corev1.NamespaceStatus{Phase: corev1.NamespaceActive}}
}

func backendPod(name, namespace, ip string, ready bool) *corev1.Pod {
	status := corev1.ConditionFalse
	if ready {
		status = corev1.ConditionTrue
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, UID: types.UID(namespace + "-" + name + "-uid"), Labels: map[string]string{"app": "api"}},
		Spec:       corev1.PodSpec{NodeName: "worker-01", Containers: []corev1.Container{{Name: "api"}}},
		Status: corev1.PodStatus{
			Phase:      corev1.PodRunning,
			PodIP:      ip,
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: status}},
			ContainerStatuses: []corev1.ContainerStatus{{
				Name:         "api",
				Ready:        ready,
				RestartCount: 1,
				State:        corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
			}},
		},
	}
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

func endpointSlice(serviceName, namespace string, endpoints ...discoveryv1.Endpoint) *discoveryv1.EndpointSlice {
	protocol := corev1.ProtocolTCP
	portName := "http"
	port := int32(8080)
	return &discoveryv1.EndpointSlice{
		ObjectMeta:  metav1.ObjectMeta{Name: serviceName + "-slice", Namespace: namespace, Labels: map[string]string{"kubernetes.io/service-name": serviceName}},
		AddressType: discoveryv1.AddressTypeIPv4,
		Ports:       []discoveryv1.EndpointPort{{Name: &portName, Protocol: &protocol, Port: &port}},
		Endpoints:   endpoints,
	}
}

func sliceEndpoint(address string, ready bool, podName, nodeName string) discoveryv1.Endpoint {
	endpoint := discoveryv1.Endpoint{Addresses: []string{address}, Conditions: discoveryv1.EndpointConditions{Ready: &ready}, NodeName: &nodeName}
	if podName != "" {
		endpoint.TargetRef = &corev1.ObjectReference{Kind: "Pod", Namespace: "payments", Name: podName}
	}
	return endpoint
}

func assertStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status = %d, want %d\nbody:\n%s", rec.Code, want, rec.Body.String())
	}
}

func assertContentType(t *testing.T, rec *httptest.ResponseRecorder, wantPrefix string) {
	t.Helper()
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, wantPrefix) {
		t.Fatalf("Content-Type = %q, want prefix %q", got, wantPrefix)
	}
}

func assertBodyContains(t *testing.T, rec *httptest.ResponseRecorder, want string) {
	t.Helper()
	if !strings.Contains(rec.Body.String(), want) {
		t.Fatalf("body missing %q:\n%s", want, rec.Body.String())
	}
}
