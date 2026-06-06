package kube

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Client is the read-only Kubernetes access layer used by kctx.
type Client struct {
	clientset kubernetes.Interface
	dynamic   dynamic.Interface
	discovery discovery.DiscoveryInterface
}

// NewClient creates a typed Kubernetes client wrapper.
func NewClient(clientset kubernetes.Interface) *Client {
	return &Client{clientset: clientset, discovery: clientset.Discovery()}
}

// NewClientWithDynamic creates a typed and dynamic Kubernetes client wrapper.
func NewClientWithDynamic(clientset kubernetes.Interface, dynamicClient dynamic.Interface) *Client {
	return &Client{clientset: clientset, dynamic: dynamicClient, discovery: clientset.Discovery()}
}

// NewDefaultClient loads kubeconfig using Kubernetes' default loading rules.
func NewDefaultClient() (*Client, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	return NewClientFromLoadingRules(rules)
}

// NewInClusterClient loads Kubernetes credentials from the current Pod.
func NewInClusterClient() (*Client, error) {
	restConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	return NewClientFromRESTConfig(restConfig)
}

// NewClientFromKubeconfig loads kubeconfig from an explicit path.
func NewClientFromKubeconfig(path string) (*Client, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	rules.ExplicitPath = path
	return NewClientFromLoadingRules(rules)
}

func NewClientFromLoadingRules(rules *clientcmd.ClientConfigLoadingRules) (*Client, error) {
	config := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{})
	restConfig, err := config.ClientConfig()
	if err != nil {
		return nil, err
	}
	return NewClientFromRESTConfig(restConfig)
}

func NewClientFromRESTConfig(restConfig *rest.Config) (*Client, error) {
	config := rest.CopyConfig(restConfig)
	config.Wrap(requestIDTransport)

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return NewClientWithDynamic(clientset, dynamicClient), nil
}

func (c *Client) GetNamespace(ctx context.Context, name string) (*corev1.Namespace, error) {
	if err := takeKubeAPIBudget(ctx); err != nil {
		return nil, err
	}
	return c.clientset.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
}

func (c *Client) GetPod(ctx context.Context, namespace, name string) (*corev1.Pod, error) {
	if err := takeKubeAPIBudget(ctx); err != nil {
		return nil, err
	}
	return c.clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
}

func (c *Client) GetService(ctx context.Context, namespace, name string) (*corev1.Service, error) {
	if err := takeKubeAPIBudget(ctx); err != nil {
		return nil, err
	}
	return c.clientset.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
}

func (c *Client) GetNode(ctx context.Context, name string) (*corev1.Node, error) {
	if err := takeKubeAPIBudget(ctx); err != nil {
		return nil, err
	}
	return c.clientset.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
}

func (c *Client) GetReplicaSet(ctx context.Context, namespace, name string) (*appsv1.ReplicaSet, error) {
	if err := takeKubeAPIBudget(ctx); err != nil {
		return nil, err
	}
	return c.clientset.AppsV1().ReplicaSets(namespace).Get(ctx, name, metav1.GetOptions{})
}

func (c *Client) GetDeployment(ctx context.Context, namespace, name string) (*appsv1.Deployment, error) {
	if err := takeKubeAPIBudget(ctx); err != nil {
		return nil, err
	}
	return c.clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
}

func (c *Client) GetStatefulSet(ctx context.Context, namespace, name string) (*appsv1.StatefulSet, error) {
	if err := takeKubeAPIBudget(ctx); err != nil {
		return nil, err
	}
	return c.clientset.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
}

func (c *Client) GetDaemonSet(ctx context.Context, namespace, name string) (*appsv1.DaemonSet, error) {
	if err := takeKubeAPIBudget(ctx); err != nil {
		return nil, err
	}
	return c.clientset.AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
}

func (c *Client) GetJob(ctx context.Context, namespace, name string) (*batchv1.Job, error) {
	if err := takeKubeAPIBudget(ctx); err != nil {
		return nil, err
	}
	return c.clientset.BatchV1().Jobs(namespace).Get(ctx, name, metav1.GetOptions{})
}

func (c *Client) GetCronJob(ctx context.Context, namespace, name string) (*batchv1.CronJob, error) {
	if err := takeKubeAPIBudget(ctx); err != nil {
		return nil, err
	}
	return c.clientset.BatchV1().CronJobs(namespace).Get(ctx, name, metav1.GetOptions{})
}

func (c *Client) ListPods(ctx context.Context, namespace string) ([]corev1.Pod, error) {
	if err := takeKubeAPIBudget(ctx); err != nil {
		return nil, err
	}
	list, err := c.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) ListDeployments(ctx context.Context, namespace string) ([]appsv1.Deployment, error) {
	if err := takeKubeAPIBudget(ctx); err != nil {
		return nil, err
	}
	list, err := c.clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) ListReplicaSets(ctx context.Context, namespace string) ([]appsv1.ReplicaSet, error) {
	if err := takeKubeAPIBudget(ctx); err != nil {
		return nil, err
	}
	list, err := c.clientset.AppsV1().ReplicaSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) ListStatefulSets(ctx context.Context, namespace string) ([]appsv1.StatefulSet, error) {
	if err := takeKubeAPIBudget(ctx); err != nil {
		return nil, err
	}
	list, err := c.clientset.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) ListDaemonSets(ctx context.Context, namespace string) ([]appsv1.DaemonSet, error) {
	if err := takeKubeAPIBudget(ctx); err != nil {
		return nil, err
	}
	list, err := c.clientset.AppsV1().DaemonSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) ListJobs(ctx context.Context, namespace string) ([]batchv1.Job, error) {
	if err := takeKubeAPIBudget(ctx); err != nil {
		return nil, err
	}
	list, err := c.clientset.BatchV1().Jobs(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) ListCronJobs(ctx context.Context, namespace string) ([]batchv1.CronJob, error) {
	if err := takeKubeAPIBudget(ctx); err != nil {
		return nil, err
	}
	list, err := c.clientset.BatchV1().CronJobs(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) ListServices(ctx context.Context, namespace string) ([]corev1.Service, error) {
	if err := takeKubeAPIBudget(ctx); err != nil {
		return nil, err
	}
	list, err := c.clientset.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) ListEndpointSlices(ctx context.Context, namespace string) ([]discoveryv1.EndpointSlice, error) {
	if err := takeKubeAPIBudget(ctx); err != nil {
		return nil, err
	}
	list, err := c.clientset.DiscoveryV1().EndpointSlices(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) ListConfigMaps(ctx context.Context, namespace string) ([]corev1.ConfigMap, error) {
	if err := takeKubeAPIBudget(ctx); err != nil {
		return nil, err
	}
	list, err := c.clientset.CoreV1().ConfigMaps(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) ListSecrets(ctx context.Context, namespace string) ([]corev1.Secret, error) {
	if err := takeKubeAPIBudget(ctx); err != nil {
		return nil, err
	}
	list, err := c.clientset.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) ListPersistentVolumeClaims(ctx context.Context, namespace string) ([]corev1.PersistentVolumeClaim, error) {
	if err := takeKubeAPIBudget(ctx); err != nil {
		return nil, err
	}
	list, err := c.clientset.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) GetEndpoints(ctx context.Context, namespace, name string) (*corev1.Endpoints, error) {
	if err := takeKubeAPIBudget(ctx); err != nil {
		return nil, err
	}
	return c.clientset.CoreV1().Endpoints(namespace).Get(ctx, name, metav1.GetOptions{})
}

func (c *Client) ListEvents(ctx context.Context, namespace string) ([]corev1.Event, error) {
	if err := takeKubeAPIBudget(ctx); err != nil {
		return nil, err
	}
	list, err := c.clientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}
