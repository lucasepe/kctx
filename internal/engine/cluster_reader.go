package engine

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
)

// clusterReader is the typed read-only contract required by TypedEngine. It is
// composed from small interfaces so tests and future engines can satisfy only
// the capabilities they need before being assembled here.
type clusterReader interface {
	namespaceGetter
	podGetter
	serviceGetter
	nodeGetter
	replicaSetGetter
	deploymentGetter
	statefulSetGetter
	daemonSetGetter
	jobGetter
	cronJobGetter
	podLister
	deploymentLister
	replicaSetLister
	statefulSetLister
	daemonSetLister
	jobLister
	cronJobLister
	serviceLister
	endpointSliceLister
	configMapLister
	secretLister
	persistentVolumeClaimLister
	endpointsGetter
	eventLister
}

// namespaceGetter fetches a Namespace by name.
type namespaceGetter interface {
	GetNamespace(ctx context.Context, name string) (*corev1.Namespace, error)
}

// podGetter fetches a Pod by namespace/name.
type podGetter interface {
	GetPod(ctx context.Context, namespace, name string) (*corev1.Pod, error)
}

// serviceGetter fetches a Service by namespace/name.
type serviceGetter interface {
	GetService(ctx context.Context, namespace, name string) (*corev1.Service, error)
}

// nodeGetter fetches a Node by name.
type nodeGetter interface {
	GetNode(ctx context.Context, name string) (*corev1.Node, error)
}

// replicaSetGetter fetches a ReplicaSet by namespace/name.
type replicaSetGetter interface {
	GetReplicaSet(ctx context.Context, namespace, name string) (*appsv1.ReplicaSet, error)
}

// deploymentGetter fetches a Deployment by namespace/name.
type deploymentGetter interface {
	GetDeployment(ctx context.Context, namespace, name string) (*appsv1.Deployment, error)
}

// statefulSetGetter fetches a StatefulSet by namespace/name.
type statefulSetGetter interface {
	GetStatefulSet(ctx context.Context, namespace, name string) (*appsv1.StatefulSet, error)
}

// daemonSetGetter fetches a DaemonSet by namespace/name.
type daemonSetGetter interface {
	GetDaemonSet(ctx context.Context, namespace, name string) (*appsv1.DaemonSet, error)
}

// jobGetter fetches a Job by namespace/name.
type jobGetter interface {
	GetJob(ctx context.Context, namespace, name string) (*batchv1.Job, error)
}

// cronJobGetter fetches a CronJob by namespace/name.
type cronJobGetter interface {
	GetCronJob(ctx context.Context, namespace, name string) (*batchv1.CronJob, error)
}

// podLister lists Pods in one namespace.
type podLister interface {
	ListPods(ctx context.Context, namespace string) ([]corev1.Pod, error)
}

// deploymentLister lists Deployments in one namespace.
type deploymentLister interface {
	ListDeployments(ctx context.Context, namespace string) ([]appsv1.Deployment, error)
}

// replicaSetLister lists ReplicaSets in one namespace.
type replicaSetLister interface {
	ListReplicaSets(ctx context.Context, namespace string) ([]appsv1.ReplicaSet, error)
}

// statefulSetLister lists StatefulSets in one namespace.
type statefulSetLister interface {
	ListStatefulSets(ctx context.Context, namespace string) ([]appsv1.StatefulSet, error)
}

// daemonSetLister lists DaemonSets in one namespace.
type daemonSetLister interface {
	ListDaemonSets(ctx context.Context, namespace string) ([]appsv1.DaemonSet, error)
}

// jobLister lists Jobs in one namespace.
type jobLister interface {
	ListJobs(ctx context.Context, namespace string) ([]batchv1.Job, error)
}

// cronJobLister lists CronJobs in one namespace.
type cronJobLister interface {
	ListCronJobs(ctx context.Context, namespace string) ([]batchv1.CronJob, error)
}

// serviceLister lists Services in one namespace.
type serviceLister interface {
	ListServices(ctx context.Context, namespace string) ([]corev1.Service, error)
}

// endpointSliceLister lists EndpointSlices in one namespace.
type endpointSliceLister interface {
	ListEndpointSlices(ctx context.Context, namespace string) ([]discoveryv1.EndpointSlice, error)
}

// configMapLister lists ConfigMaps in one namespace.
type configMapLister interface {
	ListConfigMaps(ctx context.Context, namespace string) ([]corev1.ConfigMap, error)
}

// secretLister lists Secrets in one namespace.
type secretLister interface {
	ListSecrets(ctx context.Context, namespace string) ([]corev1.Secret, error)
}

// persistentVolumeClaimLister lists PersistentVolumeClaims in one namespace.
type persistentVolumeClaimLister interface {
	ListPersistentVolumeClaims(ctx context.Context, namespace string) ([]corev1.PersistentVolumeClaim, error)
}

// endpointsGetter fetches legacy Endpoints by namespace/name.
type endpointsGetter interface {
	GetEndpoints(ctx context.Context, namespace, name string) (*corev1.Endpoints, error)
}

// eventLister lists Events in one namespace.
type eventLister interface {
	ListEvents(ctx context.Context, namespace string) ([]corev1.Event, error)
}
