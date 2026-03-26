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
	"os"
	"os/signal"
	"syscall"

	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/cloud-provider/app"
	"k8s.io/cloud-provider/app/config"
	"k8s.io/cloud-provider/names"
	"k8s.io/cloud-provider/options"
	"k8s.io/component-base/cli"
	cliflag "k8s.io/component-base/cli/flag"
	_ "k8s.io/component-base/logs/json/register"
	_ "k8s.io/component-base/metrics/prometheus/clientgo"
	_ "k8s.io/component-base/metrics/prometheus/version"
	"k8s.io/klog/v2"

	// Import NICo cloud provider to register it
	_ "github.com/fabiendupont/cloud-provider-nvidia-ncx-infra-controller/pkg/cloudprovider"
)

func main() {
	ccmOptions, err := options.NewCloudControllerManagerOptions()
	if err != nil {
		klog.Fatalf("unable to initialize command options: %v", err)
	}

	// Set up signal-aware stop channel so SIGTERM/SIGINT triggers graceful shutdown.
	// The CCM framework uses this channel to release leader election and drain controllers.
	stopCh := make(chan struct{})
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		sig := <-sigCh
		klog.InfoS("Received signal, initiating graceful shutdown", "signal", sig)
		close(stopCh)
		// Second signal forces immediate exit
		sig = <-sigCh
		klog.InfoS("Received second signal, forcing exit", "signal", sig)
		os.Exit(1)
	}()

	fss := cliflag.NamedFlagSets{}
	command := app.NewCloudControllerManagerCommand(
		ccmOptions,
		cloudInitializer,
		app.DefaultInitFuncConstructors,
		names.CCMControllerAliases(),
		fss,
		stopCh,
	)
	code := cli.Run(command)
	klog.InfoS("NICo CCM exiting", "code", code)
	os.Exit(code)
}

func cloudInitializer(cfg *config.CompletedConfig) cloudprovider.Interface {
	cloudConfig := cfg.ComponentConfig.KubeCloudShared.CloudProvider

	cloud, err := cloudprovider.InitCloudProvider(cloudConfig.Name, cloudConfig.CloudConfigFile)
	if err != nil {
		klog.Fatalf("Cloud provider could not be initialized: %v", err)
	}
	if cloud == nil {
		klog.Fatalf("Cloud provider is nil")
	}

	if !cloud.HasClusterID() {
		if cfg.ComponentConfig.KubeCloudShared.AllowUntaggedCloud {
			klog.Warning("detected a cluster without a ClusterID")
		} else {
			klog.Fatalf("no ClusterID found, set --allow-untagged-cloud to bypass")
		}
	}

	return cloud
}
