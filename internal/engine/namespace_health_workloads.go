package engine

import (
	"fmt"

	"github.com/lucasepe/kctx/internal/model"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
)

// namespaceWorkloadHealth summarizes supported workload availability in one
// normalized slice.
func namespaceWorkloadHealth(deployments []appsv1.Deployment, replicaSets []appsv1.ReplicaSet, statefulSets []appsv1.StatefulSet, daemonSets []appsv1.DaemonSet, jobs []batchv1.Job, cronJobs []batchv1.CronJob) []model.WorkloadHealth {
	var out []model.WorkloadHealth
	for _, deployment := range deployments {
		desired := replicasValue(deployment.Spec.Replicas)
		healthy := deployment.Status.AvailableReplicas >= desired
		out = append(out, model.WorkloadHealth{
			Kind:      "Deployment",
			Namespace: deployment.Namespace,
			Name:      deployment.Name,
			Desired:   desired,
			Ready:     deployment.Status.ReadyReplicas,
			Available: deployment.Status.AvailableReplicas,
			Healthy:   healthy,
			Message:   readyMessage(deployment.Status.ReadyReplicas, desired),
		})
	}
	for _, replicaSet := range replicaSets {
		desired := replicasValue(replicaSet.Spec.Replicas)
		out = append(out, model.WorkloadHealth{
			Kind:      "ReplicaSet",
			Namespace: replicaSet.Namespace,
			Name:      replicaSet.Name,
			Desired:   desired,
			Ready:     replicaSet.Status.ReadyReplicas,
			Available: replicaSet.Status.AvailableReplicas,
			Healthy:   replicaSet.Status.AvailableReplicas >= desired,
			Message:   readyMessage(replicaSet.Status.ReadyReplicas, desired),
		})
	}
	for _, statefulSet := range statefulSets {
		desired := replicasValue(statefulSet.Spec.Replicas)
		out = append(out, model.WorkloadHealth{
			Kind:      "StatefulSet",
			Namespace: statefulSet.Namespace,
			Name:      statefulSet.Name,
			Desired:   desired,
			Ready:     statefulSet.Status.ReadyReplicas,
			Healthy:   statefulSet.Status.ReadyReplicas >= desired,
			Message:   readyMessage(statefulSet.Status.ReadyReplicas, desired),
		})
	}
	for _, daemonSet := range daemonSets {
		desired := daemonSet.Status.DesiredNumberScheduled
		ready := daemonSet.Status.NumberReady
		out = append(out, model.WorkloadHealth{
			Kind:      "DaemonSet",
			Namespace: daemonSet.Namespace,
			Name:      daemonSet.Name,
			Desired:   desired,
			Ready:     ready,
			Available: daemonSet.Status.NumberAvailable,
			Healthy:   ready >= desired,
			Message:   readyMessage(ready, desired),
		})
	}
	for _, job := range jobs {
		desired := jobCompletions(&job)
		healthy := job.Status.Failed == 0 && job.Status.Succeeded >= desired
		out = append(out, model.WorkloadHealth{
			Kind:      "Job",
			Namespace: job.Namespace,
			Name:      job.Name,
			Desired:   desired,
			Succeeded: job.Status.Succeeded,
			Failed:    job.Status.Failed,
			Active:    job.Status.Active,
			Healthy:   healthy,
			Message:   jobMessage(&job),
		})
	}
	for _, cronJob := range cronJobs {
		suspended := cronJob.Spec.Suspend != nil && *cronJob.Spec.Suspend
		out = append(out, model.WorkloadHealth{
			Kind:      "CronJob",
			Namespace: cronJob.Namespace,
			Name:      cronJob.Name,
			Active:    int32(len(cronJob.Status.Active)),
			Healthy:   !suspended,
			Message:   cronJobDumpStatus(&cronJob),
		})
	}
	return out
}

// readyMessage renders desired-vs-ready counts for workload status fields.
func readyMessage(ready, desired int32) string {
	if ready >= desired {
		return fmt.Sprintf("ready %d/%d", ready, desired)
	}
	return fmt.Sprintf("ready %d/%d", ready, desired)
}

// jobMessage renders the compact Job execution counters used by health and dump.
func jobMessage(job *batchv1.Job) string {
	return fmt.Sprintf("succeeded %d failed %d active %d", job.Status.Succeeded, job.Status.Failed, job.Status.Active)
}

// replicasValue applies the Kubernetes default of one replica when omitted.
func replicasValue(replicas *int32) int32 {
	if replicas == nil {
		return 1
	}
	return *replicas
}
