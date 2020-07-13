package main

import (
	"flag"
	"k8s.io/klog"
	"os"
)

func main() {
	klogFlags := flag.NewFlagSet("klog", flag.ExitOnError)
	klog.InitFlags(klogFlags)
	klogFlags.Set("v", "2")
	klogFlags.Set("logtostderr", "true")
	klogFlags.Set("skip_headers", "true")
	klogFlags.Parse(os.Args[1:])

	defer klog.Flush()

	runtimeService, err := newRuntimeServiceClient(remoteRuntimeEndpoint, runtimeRequestTimeout)
	if err != nil {
		klog.Fatal(err)
	}

	pods, err := runtimeService.getPods()
	if err != nil {
		klog.Fatal(err)
	}

	for _, pod := range pods {
		err = runtimeService.getPodStatus(pod.ID, pod.Name, pod.Namespace)
		if err != nil {
			klog.Fatal(err)
		}
	}

	os.Exit(0)
}
