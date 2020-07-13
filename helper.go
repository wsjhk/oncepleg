package main

import "k8s.io/klog"

const (
	KubernetesPodNameLabel       = "io.kubernetes.pod.name"
	KubernetesPodNamespaceLabel  = "io.kubernetes.pod.namespace"
	KubernetesPodUIDLabel        = "io.kubernetes.pod.uid"
	KubernetesContainerNameLabel = "io.kubernetes.container.name"
)

type labeledContainerInfo struct {
	ContainerName string
	PodName       string
	PodNamespace  string
	PodUID        string
}

// getContainerInfoFromLabels gets labeledContainerInfo from labels.
func getContainerInfoFromLabels(labels map[string]string) *labeledContainerInfo {
	return &labeledContainerInfo{
		PodName:       getStringValueFromLabel(labels, KubernetesPodNameLabel),
		PodNamespace:  getStringValueFromLabel(labels, KubernetesPodNamespaceLabel),
		PodUID:        getStringValueFromLabel(labels, KubernetesPodUIDLabel),
		ContainerName: getStringValueFromLabel(labels, KubernetesContainerNameLabel),
	}
}

func getStringValueFromLabel(labels map[string]string, label string) string {
	if value, found := labels[label]; found {
		return value
	}
	// Do not report error, because there should be many old containers without label now.
	klog.Infof("Container doesn't have label %s, it may be an old or invalid container", label)
	// Return empty string "" for these containers, the caller will get value by other ways.
	return ""
}
