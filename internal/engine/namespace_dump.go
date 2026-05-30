package engine

import (
	"context"
	"time"

	"github.com/lucasepe/kctx/internal/model"
	corev1 "k8s.io/api/core/v1"
)

// DumpNamespaceRequest identifies the namespace to export as normalized JSON.
type DumpNamespaceRequest struct {
	Namespace string
}

// DumpNamespaceResponse is a compact operational snapshot made of entities,
// relations, signals, and recent warning events.
type DumpNamespaceResponse struct {
	GeneratedAt time.Time                `json:"generatedAt"`
	Namespace   string                   `json:"namespace"`
	Summary     model.DumpSummary        `json:"summary"`
	Entities    []model.DumpEntity       `json:"entities"`
	Relations   []model.DumpRelation     `json:"relations"`
	Signals     []model.DumpSignal       `json:"signals"`
	Events      []model.DumpEventSummary `json:"events"`
}

// DumpNamespace exports a deterministic, normalized namespace snapshot without
// raw manifests, Secret data, logs, or metrics.
func (e *TypedEngine) DumpNamespace(ctx context.Context, req DumpNamespaceRequest) (*DumpNamespaceResponse, error) {
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	ns, err := e.kube.GetNamespace(ctx, req.Namespace)
	if err != nil {
		return nil, err
	}
	pods, err := e.kube.ListPods(ctx, req.Namespace)
	if err != nil {
		return nil, err
	}
	deployments, err := e.kube.ListDeployments(ctx, req.Namespace)
	if err != nil {
		return nil, err
	}
	replicaSets, err := e.kube.ListReplicaSets(ctx, req.Namespace)
	if err != nil {
		return nil, err
	}
	statefulSets, err := e.kube.ListStatefulSets(ctx, req.Namespace)
	if err != nil {
		return nil, err
	}
	daemonSets, err := e.kube.ListDaemonSets(ctx, req.Namespace)
	if err != nil {
		return nil, err
	}
	jobs, err := e.kube.ListJobs(ctx, req.Namespace)
	if err != nil {
		return nil, err
	}
	cronJobs, err := e.kube.ListCronJobs(ctx, req.Namespace)
	if err != nil {
		return nil, err
	}
	services, err := e.kube.ListServices(ctx, req.Namespace)
	if err != nil {
		return nil, err
	}
	slices, err := e.kube.ListEndpointSlices(ctx, req.Namespace)
	if err != nil {
		return nil, err
	}
	configMaps, err := e.kube.ListConfigMaps(ctx, req.Namespace)
	if err != nil {
		return nil, err
	}
	secrets, err := e.kube.ListSecrets(ctx, req.Namespace)
	if err != nil {
		return nil, err
	}
	pvcs, err := e.kube.ListPersistentVolumeClaims(ctx, req.Namespace)
	if err != nil {
		return nil, err
	}
	events, err := e.kube.ListEvents(ctx, req.Namespace)
	if err != nil {
		return nil, err
	}

	builder := newDumpBuilder()
	builder.addEntity(namespaceDumpEntity(ns))

	podByName := map[string]corev1.Pod{}
	podByIP := map[string]corev1.Pod{}
	for _, pod := range pods {
		podByName[pod.Name] = pod
		if pod.Status.PodIP != "" {
			podByIP[pod.Status.PodIP] = pod
		}
		builder.addEntity(podDumpEntity(&pod))
	}
	for _, deploy := range deployments {
		builder.addEntity(workloadDumpEntity("Deployment", deploy.Namespace, deploy.Name, string(deploy.UID), deploy.Labels, readyMessage(deploy.Status.AvailableReplicas, replicasValue(deploy.Spec.Replicas))))
	}
	for _, rs := range replicaSets {
		builder.addEntity(workloadDumpEntity("ReplicaSet", rs.Namespace, rs.Name, string(rs.UID), rs.Labels, readyMessage(rs.Status.ReadyReplicas, replicasValue(rs.Spec.Replicas))))
	}
	for _, sts := range statefulSets {
		builder.addEntity(workloadDumpEntity("StatefulSet", sts.Namespace, sts.Name, string(sts.UID), sts.Labels, readyMessage(sts.Status.ReadyReplicas, replicasValue(sts.Spec.Replicas))))
	}
	for _, ds := range daemonSets {
		builder.addEntity(workloadDumpEntity("DaemonSet", ds.Namespace, ds.Name, string(ds.UID), ds.Labels, readyMessage(ds.Status.NumberAvailable, ds.Status.DesiredNumberScheduled)))
	}
	for _, job := range jobs {
		builder.addEntity(workloadDumpEntity("Job", job.Namespace, job.Name, string(job.UID), job.Labels, jobMessage(&job)))
	}
	for _, cronJob := range cronJobs {
		builder.addEntity(workloadDumpEntity("CronJob", cronJob.Namespace, cronJob.Name, string(cronJob.UID), cronJob.Labels, cronJobDumpStatus(&cronJob)))
	}
	for _, service := range services {
		builder.addEntity(serviceDumpEntity(&service))
	}
	for _, slice := range slices {
		builder.addEntity(endpointSliceDumpEntity(&slice))
	}
	for _, cm := range configMaps {
		builder.addEntity(metadataDumpEntity("ConfigMap", cm.Namespace, cm.Name, string(cm.UID), cm.Labels, ""))
	}
	for _, secret := range secrets {
		builder.addEntity(metadataDumpEntity("Secret", secret.Namespace, secret.Name, string(secret.UID), secret.Labels, string(secret.Type)))
	}
	for _, pvc := range pvcs {
		builder.addEntity(metadataDumpEntity("PersistentVolumeClaim", pvc.Namespace, pvc.Name, string(pvc.UID), pvc.Labels, string(pvc.Status.Phase)))
	}

	addDumpOwnership(builder, pods, replicaSets, jobs)
	addDumpServiceRelations(builder, services, pods)
	addDumpEndpointSliceRelations(builder, slices, podByName, podByIP)
	addDumpPodRelations(ctx, e, builder, pods)

	resp := &DumpNamespaceResponse{
		GeneratedAt: time.Now().UTC(),
		Namespace:   req.Namespace,
		Entities:    builder.entityList(),
		Relations:   builder.relationList(),
		Events:      dumpWarningEvents(events, 50),
	}
	resp.Signals = dumpSignals(resp.Entities, resp.Events, deployments, replicaSets, statefulSets, daemonSets, jobs, services, slices, pvcs)
	resp.Summary = model.DumpSummary{
		Pods:           len(pods),
		Deployments:    len(deployments),
		StatefulSets:   len(statefulSets),
		DaemonSets:     len(daemonSets),
		Jobs:           len(jobs),
		CronJobs:       len(cronJobs),
		Services:       len(services),
		EndpointSlices: len(slices),
		PVCs:           len(pvcs),
		Nodes:          countEntities(resp.Entities, "Node"),
		WarningEvents:  len(resp.Events),
		Signals:        len(resp.Signals),
	}
	sortDump(resp)
	return resp, nil
}
