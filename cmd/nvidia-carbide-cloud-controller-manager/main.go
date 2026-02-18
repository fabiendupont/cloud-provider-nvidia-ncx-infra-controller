/*
Copyright 2026 Fabien Dupont.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"fmt"
	"os"

	"k8s.io/klog/v2"

	// Import NVIDIA Carbide cloud provider to register it
	_ "github.com/fabiendupont/cloud-provider-nvidia-carbide/pkg/cloudprovider"
)

const (
	// ComponentName is the name of the cloud controller manager component
	ComponentName = "nvidia-carbide-cloud-controller-manager"
)

func main() {
	klog.InitFlags(nil)

	fmt.Println("NVIDIA Carbide Cloud Controller Manager")
	fmt.Println("====================================")
	fmt.Println()
	fmt.Println("This is a cloud provider implementation for NVIDIA Carbide.")
	fmt.Println()
	fmt.Println("To build and run a full cloud controller manager, you need to:")
	fmt.Println("1. Ensure all k8s.io/* dependencies are aligned to the same version")
	fmt.Println("2. Use k8s.io/cloud-provider/app.NewCloudControllerManagerCommand()")
	fmt.Println("3. Integrate with your Kubernetes version's cloud controller framework")
	fmt.Println()
	fmt.Println("The cloud provider implementation is in pkg/cloudprovider/")
	fmt.Println()
	fmt.Println("For production use, integrate this with your Kubernetes distribution's")
	fmt.Println("cloud controller manager framework, ensuring version compatibility.")

	// TODO: Full implementation would use k8s.io/cloud-provider/app
	// command := app.NewCloudControllerManagerCommand()
	// if err := command.Execute(); err != nil {
	// 	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	// 	os.Exit(1)
	// }

	klog.Info("Cloud provider 'nvidia-carbide' is registered and available")
	klog.Info("Provider implements: InstancesV2, Zones")
	klog.Info("Provider does not implement: LoadBalancer, Routes")

	os.Exit(0)
}
