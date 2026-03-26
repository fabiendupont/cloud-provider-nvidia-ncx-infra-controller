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

	"github.com/fabiendupont/cloud-provider-nvidia-ncx-infra-controller/pkg/providerid"
)

// GetZone returns the Zone containing the current zone and locality region
func (c *NicoCloud) GetZone(ctx context.Context) (cloudprovider.Zone, error) {
	zone, region := c.resolveZoneAndRegion(ctx, c.siteID)
	return cloudprovider.Zone{
		FailureDomain: zone,
		Region:        region,
	}, nil
}

// GetZoneByProviderID returns the Zone for a specific provider ID
func (c *NicoCloud) GetZoneByProviderID(ctx context.Context, providerID string) (cloudprovider.Zone, error) {
	parsed, err := providerid.ParseProviderID(providerID)
	if err != nil {
		return cloudprovider.Zone{}, err
	}

	siteID := parsed.SiteName
	if siteID == "" {
		siteID = c.siteID
	}

	zone, region := c.resolveZoneAndRegion(ctx, siteID)
	return cloudprovider.Zone{
		FailureDomain: zone,
		Region:        region,
	}, nil
}

// GetZoneByNodeName returns the Zone for a specific node
func (c *NicoCloud) GetZoneByNodeName(
	ctx context.Context, nodeName types.NodeName,
) (cloudprovider.Zone, error) {
	zone, region := c.resolveZoneAndRegion(ctx, c.siteID)
	return cloudprovider.Zone{
		FailureDomain: zone,
		Region:        region,
	}, nil
}
