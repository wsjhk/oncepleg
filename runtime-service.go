package main

import (
	"context"
	"fmt"
	"google.golang.org/grpc"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
	"k8s.io/klog"
	"net"
	"net/url"
	"time"
)

const (
	unixProtocol = "unix"
	maxMsgSize   = 1024 * 1024 * 16
)

var (
	remoteRuntimeEndpoint = "unix:///var/run/dockershim.sock"
	runtimeRequestTimeout = 2 * time.Minute
)

type runtimeService struct {
	Client  runtimeapi.RuntimeServiceClient
	Timeout time.Duration
}

// Pod is a group of containers.
type Pod struct {
	// The ID of the pod, which can be used to retrieve a particular pod
	// from the pod list returned by GetPods().
	ID string
	// The name and namespace of the pod, which is readable by human.
	Name      string
	Namespace string
}

func newRuntimeServiceClient(endpoint string, connectionTimeout time.Duration) (*runtimeService, error) {
	klog.V(5).Infof("Connecting to runtime service %s", endpoint)
	addr, dailer, err := getAddressAndDialer(endpoint)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, addr, grpc.WithInsecure(), grpc.WithDialer(dailer), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(maxMsgSize)))
	if err != nil {
		klog.Errorf("Connect remote runtime %s failed: %v", addr, err)
		return nil, err
	}

	return &runtimeService{
		Client:  runtimeapi.NewRuntimeServiceClient(conn),
		Timeout: connectionTimeout,
	}, nil

}

func getAddressAndDialer(endpoint string) (string, func(addr string, timeout time.Duration) (net.Conn, error), error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", nil, err
	}
	if u.Scheme != unixProtocol {
		return "", nil, fmt.Errorf("only support unix socket endpoint")
	}

	return u.Path, dial, nil
}

func dial(addr string, timeout time.Duration) (net.Conn, error) {
	return net.DialTimeout(unixProtocol, addr, timeout)
}

func (rs *runtimeService) getPods() ([]*Pod, error) {
	now := time.Now()
	pods, err := rs._getPods()
	if err != nil {
		return nil, err
	}
	elapsed := time.Since(now)
	klog.V(2).Infof("List all Pods, Threshold: %v\n", elapsed)
	return pods, nil
}

func (rs *runtimeService) getPodStatus(uid, name, namespace string) error {
	now := time.Now()
	err := rs._getPodStatus(uid, name, namespace)
	if err != nil {
		return err
	}
	elapsed := time.Since(now)
	klog.V(2).Infof("List pod %s Status, Threshold: %v\n", fmt.Sprintf("%s/%s", name, namespace), elapsed)

	return nil
}

func (rs *runtimeService) _getPods() ([]*Pod, error) {
	pods := make(map[string]*Pod)
	sandboxes, err := rs.getKubeletSandboxs("", true)
	if err != nil {
		return nil, err
	}
	for i := range sandboxes {
		s := sandboxes[i]
		if s.Metadata == nil {
			klog.V(2).Infof("Sandbox does not have metadata: %+v", s)
			continue
		}
		podUID := s.Metadata.Uid
		if _, ok := pods[podUID]; !ok {
			pods[podUID] = &Pod{
				ID:        podUID,
				Name:      s.Metadata.Name,
				Namespace: s.Metadata.Namespace,
			}
		}
	}

	containers, err := rs.getKubeletContainers("", true)
	if err != nil {
		return nil, err
	}
	for i := range containers {
		c := containers[i]
		if c.Metadata == nil {
			klog.V(2).Infof("Container does not have metadata: %+v", c)
			continue
		}

		labelledInfo := getContainerInfoFromLabels(c.Labels)
		pod, found := pods[labelledInfo.PodUID]
		if !found {
			pod = &Pod{
				ID:        labelledInfo.PodUID,
				Name:      labelledInfo.PodName,
				Namespace: labelledInfo.PodNamespace,
			}
			pods[labelledInfo.PodUID] = pod
		}
	}

	// Convert map to list.
	var result []*Pod
	for _, pod := range pods {
		result = append(result, pod)
	}

	return result, nil
}

func (rs *runtimeService) _getPodStatus(uid, name, namespace string) error {
	klog.V(2).Infof("Pod ID: %s, Name: %s, Namespace: %s\n", uid, name, namespace)
	// get sandbox by uid
	sandboxes, err := rs.getKubeletContainers(uid, true)
	if err != nil {
		return err
	}
	if len(sandboxes) != 0 {
		for _, sandbox := range sandboxes {
			klog.V(2).Infof("Sandbox ID: %s", sandbox.Id)
			err := rs.getPodSandboxStatus(sandbox.Id)
			if err != nil {
				klog.Errorf("PodSandboxStatus of sandbox %q for pod %q error: %v", sandbox.Id, name, err)
				continue
			}
		}
	}

	// get container by uid
	containers, err := rs.getKubeletContainers(uid, true)
	if err != nil {
		return err
	}
	if len(containers) != 0 {
		for _, c := range containers {
			klog.V(2).Infof("Container ID: %s", c.Id)
			err := rs.getContainerStatus(c.Id)
			if err != nil {
				klog.Errorf("ContainerStatus for %s error: %v", c.Id, err)
				continue
			}
		}
	}

	return nil
}

func (rs *runtimeService) getContainerStatus(containerID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), rs.Timeout)
	defer cancel()

	resp, err := rs.Client.ContainerStatus(ctx, &runtimeapi.ContainerStatusRequest{
		ContainerId: containerID,
	})
	if err != nil {
		return err
	}
	status := resp.Status
	klog.V(2).Infof("Container ID: %s, Status: %s, Message: %s, Reason: %s\n", status.Id, status.State.String(), status.Message, status.Reason)
	klog.V(4).Infof("More Detail: %s\n", status.String())

	return nil
}

func (rs *runtimeService) getPodSandboxStatus(sandboxID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), rs.Timeout)
	defer cancel()

	resp, err := rs.Client.PodSandboxStatus(ctx, &runtimeapi.PodSandboxStatusRequest{
		PodSandboxId: sandboxID,
	})
	if err != nil {
		return err
	}

	status := resp.Status
	klog.V(2).Infof("Sandbox ID: %s, Status: %s\n", status.Id, status.State.String())
	klog.V(4).Infof("More Detail: %s\n", status.String())

	return nil
}

func (rs *runtimeService) getKubeletSandboxs(podUID string, all bool) ([]*runtimeapi.PodSandbox, error) {
	var filter = &runtimeapi.PodSandboxFilter{}
	if podUID != "" {
		filter.LabelSelector = map[string]string{KubernetesPodUIDLabel: podUID}
	}

	if !all {
		readyState := runtimeapi.PodSandboxState_SANDBOX_READY
		filter.State = &runtimeapi.PodSandboxStateValue{
			State: readyState,
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), rs.Timeout)
	defer cancel()

	resp, err := rs.Client.ListPodSandbox(ctx, &runtimeapi.ListPodSandboxRequest{
		Filter: filter,
	})
	if err != nil {
		klog.Errorf("ListPodSandbox with filter %+v from runtime service failed: %v", filter, err)
		return nil, err
	}

	return resp.Items, nil
}

func (rs *runtimeService) getKubeletContainers(podUID string, all bool) ([]*runtimeapi.Container, error) {
	var filter = &runtimeapi.ContainerFilter{}

	if podUID != "" {
		filter.LabelSelector = map[string]string{KubernetesPodUIDLabel: podUID}
	}
	if !all {
		filter.State = &runtimeapi.ContainerStateValue{
			State: runtimeapi.ContainerState_CONTAINER_RUNNING,
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), rs.Timeout)
	defer cancel()

	resp, err := rs.Client.ListContainers(ctx, &runtimeapi.ListContainersRequest{
		Filter: filter,
	})
	if err != nil {
		klog.Errorf("ListContainers with filter %+v from runtime service failed: %v", filter, err)
		return nil, err
	}

	return resp.Containers, nil
}
