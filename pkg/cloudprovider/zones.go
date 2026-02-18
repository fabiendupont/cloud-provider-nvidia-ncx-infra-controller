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

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	cloudprovider "k8s.io/cloud-provider"
)

// GetZone returns the Zone containing the current zone and locality region that the program is running in
func (c *NvidiaCarbideCloud) GetZone(ctx context.Context) (cloudprovider.Zone, error) {
	zone := cloudprovider.Zone{
		FailureDomain: c.getZoneFromSiteID(c.siteID),
		Region:        c.getRegionFromSiteID(c.siteID),
	}

	return zone, nil
}

// GetZoneByProviderID returns the Zone containing the zone and region for a specific provider ID
func (c *NvidiaCarbideCloud) GetZoneByProviderID(ctx context.Context, providerID string) (cloudprovider.Zone, error) {
	// Parse provider ID to get site ID
	// For now, use the configured site ID
	zone := cloudprovider.Zone{
		FailureDomain: c.getZoneFromSiteID(c.siteID),
		Region:        c.getRegionFromSiteID(c.siteID),
	}

	return zone, nil
}

// GetZoneByNodeName returns the Zone containing the zone and region for a specific node
func (c *NvidiaCarbideCloud) GetZoneByNodeName(
	ctx context.Context, nodeName types.NodeName,
) (cloudprovider.Zone, error) {
	// All nodes in an NVIDIA Carbide cluster are in the same site/zone
	zone := cloudprovider.Zone{
		FailureDomain: c.getZoneFromSiteID(c.siteID),
		Region:        c.getRegionFromSiteID(c.siteID),
	}

	return zone, nil
}
