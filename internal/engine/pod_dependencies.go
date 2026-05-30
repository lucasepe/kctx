package engine

import (
	"sort"

	"github.com/lucasepe/kctx/internal/model"
	corev1 "k8s.io/api/core/v1"
)

// resolveVolumes extracts Pod dependencies declared through volumes and
// environment references, deduplicated and sorted for stable output.
func resolveVolumes(pod *corev1.Pod) []model.VolumeRef {
	seen := map[model.VolumeRef]bool{}
	for _, volume := range pod.Spec.Volumes {
		if volume.PersistentVolumeClaim != nil {
			seen[model.VolumeRef{Type: "PVC", Name: volume.PersistentVolumeClaim.ClaimName}] = true
		}
		if volume.ConfigMap != nil {
			seen[model.VolumeRef{Type: "ConfigMap", Name: volume.ConfigMap.Name}] = true
		}
		if volume.Secret != nil {
			seen[model.VolumeRef{Type: "Secret", Name: volume.Secret.SecretName}] = true
		}
	}
	for _, container := range append(pod.Spec.InitContainers, pod.Spec.Containers...) {
		for _, envFrom := range container.EnvFrom {
			if envFrom.ConfigMapRef != nil {
				seen[model.VolumeRef{Type: "ConfigMap", Name: envFrom.ConfigMapRef.Name}] = true
			}
			if envFrom.SecretRef != nil {
				seen[model.VolumeRef{Type: "Secret", Name: envFrom.SecretRef.Name}] = true
			}
		}
		for _, env := range container.Env {
			if env.ValueFrom == nil {
				continue
			}
			if env.ValueFrom.ConfigMapKeyRef != nil {
				seen[model.VolumeRef{Type: "ConfigMap", Name: env.ValueFrom.ConfigMapKeyRef.Name}] = true
			}
			if env.ValueFrom.SecretKeyRef != nil {
				seen[model.VolumeRef{Type: "Secret", Name: env.ValueFrom.SecretKeyRef.Name}] = true
			}
		}
	}

	volumes := make([]model.VolumeRef, 0, len(seen))
	for volume := range seen {
		volumes = append(volumes, volume)
	}
	sort.Slice(volumes, func(i, j int) bool {
		if volumes[i].Type == volumes[j].Type {
			return volumes[i].Name < volumes[j].Name
		}
		return volumes[i].Type < volumes[j].Type
	})
	return volumes
}

// volumeRelation maps a normalized dependency reference to the relation type
// used by context and graph outputs.
func volumeRelation(source model.Entity, namespace string, volume model.VolumeRef) model.Relation {
	target := model.Entity{Kind: volume.Type, Namespace: namespace, Name: volume.Name}
	relationType := "uses_configmap"
	switch volume.Type {
	case "PVC":
		relationType = "mounts_pvc"
	case "Secret":
		relationType = "uses_secret"
	}
	return model.Relation{Type: relationType, Source: source, Target: target}
}
