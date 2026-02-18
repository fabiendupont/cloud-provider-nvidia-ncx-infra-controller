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

package cloudprovider

// LoadBalancer functionality is not currently supported by NVIDIA Carbide.
// LoadBalancer services will need to use an external load balancer solution
// such as MetalLB, kube-vip, or a hardware load balancer.

// The LoadBalancer() method in nvidia_carbide_cloud.go returns cloudprovider.NotImplemented
// to indicate that this functionality is not available.

// Future implementation could integrate with:
// - External load balancer hardware at the site
// - Software load balancers like MetalLB
// - NVIDIA Carbide-native load balancing if the platform adds support
