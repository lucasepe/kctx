package kube

import (
	"context"
	"errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var errDynamicClientUnavailable = errors.New("dynamic client is not configured")

type DynamicReader interface {
	Get(ctx context.Context, ref ResourceRef, namespace, name string) (*unstructured.Unstructured, error)
	List(ctx context.Context, ref ResourceRef, namespace string) ([]unstructured.Unstructured, error)
}

type ResourceRef struct {
	Group    string
	Version  string
	Resource string
}

func (r ResourceRef) GVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    r.Group,
		Version:  r.Version,
		Resource: r.Resource,
	}
}

func (c *Client) Get(ctx context.Context, ref ResourceRef, namespace, name string) (*unstructured.Unstructured, error) {
	if c.dynamic == nil {
		return nil, errDynamicClientUnavailable
	}
	return c.dynamic.Resource(ref.GVR()).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
}

func (c *Client) List(ctx context.Context, ref ResourceRef, namespace string) ([]unstructured.Unstructured, error) {
	if c.dynamic == nil {
		return nil, errDynamicClientUnavailable
	}
	list, err := c.dynamic.Resource(ref.GVR()).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func DecodeResource[T any](obj *unstructured.Unstructured) (T, error) {
	var out T
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &out)
	return out, err
}

func DecodeResourceList[T any](items []unstructured.Unstructured) ([]T, error) {
	out := make([]T, 0, len(items))
	for _, item := range items {
		decoded, err := DecodeResource[T](&item)
		if err != nil {
			return nil, err
		}
		out = append(out, decoded)
	}
	return out, nil
}
