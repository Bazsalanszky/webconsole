// SPDX-FileCopyrightText: 2026 Balázs Toldi
// SPDX-License-Identifier: Apache-2.0

package nfconfig

import (
	"encoding/json"
	"testing"

	"github.com/omec-project/openapi/v2/nfConfigApi"
	"github.com/omec-project/webconsole/configmodels"
)

const (
	ueIpv6PoolTest = "2a0e:8f02:f07b:500::/56"
	dnsIpv6Test    = "2001:4860:4860::8888"
	pcscfIpv6Test  = "2001:db8:5::10"
)

// TestExtractIpDomainsIPv6 checks the JSON of the session-management IP
// domains as the SMF receives it from /nfconfig/session-management.
func TestExtractIpDomainsIPv6(t *testing.T) {
	tests := []struct {
		name     string
		ipDomain configmodels.DeviceGroupsIpDomainExpanded
		wantJSON string
	}{
		{
			name: "IPv4 only is unchanged",
			ipDomain: configmodels.DeviceGroupsIpDomainExpanded{
				Dnn: dnnInternet, UeIpPool: ueIpPoolTest, DnsPrimary: dnsPrimaryTest, PcscfPrimary: "10.10.10.10", Mtu: 1500,
			},
			wantJSON: `{"dnnName":"internet","dnsIpv4":"8.8.8.8","mtu":1500,"pcscfIpv4":"10.10.10.10","ueSubnet":"10.1.1.0/24"}`,
		},
		{
			name: "dual stack, default prefix length sent explicitly",
			ipDomain: configmodels.DeviceGroupsIpDomainExpanded{
				Dnn: dnnInternet, UeIpPool: ueIpPoolTest, DnsPrimary: dnsPrimaryTest, Mtu: 1500,
				UeIpv6Pool: ueIpv6PoolTest, DnsPrimaryV6: dnsIpv6Test, PcscfPrimaryV6: pcscfIpv6Test,
			},
			wantJSON: `{"dnnName":"internet","dnsIpv4":"8.8.8.8","dnsIpv6":"2001:4860:4860::8888","mtu":1500,` +
				`"pcscfIpv6":"2001:db8:5::10","ueIpv6PrefixLength":64,"ueSubnet":"10.1.1.0/24","ueSubnetIpv6":"2a0e:8f02:f07b:500::/56"}`,
		},
		{
			name: "IPv6 only: the required IPv4 fields are empty strings",
			ipDomain: configmodels.DeviceGroupsIpDomainExpanded{
				Dnn: dnnInternet, Mtu: 1500, UeIpv6Pool: ueIpv6PoolTest, UeIpv6PrefixLen: 60, DnsPrimaryV6: dnsIpv6Test,
			},
			wantJSON: `{"dnnName":"internet","dnsIpv4":"","dnsIpv6":"2001:4860:4860::8888","mtu":1500,` +
				`"ueIpv6PrefixLength":60,"ueSubnet":"","ueSubnetIpv6":"2a0e:8f02:f07b:500::/56"}`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			deviceGroups := map[string]configmodels.DeviceGroups{
				deviceGroupNameDG1: {IpDomainsExpanded: []configmodels.DeviceGroupsIpDomainExpanded{tc.ipDomain}},
			}
			ipDomains := extractIpDomains([]string{deviceGroupNameDG1}, deviceGroups)
			if len(ipDomains) != 1 {
				t.Fatalf("got %d IP domains, want 1", len(ipDomains))
			}
			got, err := json.Marshal(ipDomains[0])
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.wantJSON {
				t.Errorf("JSON =\n%s\nwant\n%s", got, tc.wantJSON)
			}
			// The SMF's client decodes it with the required-property check.
			var decoded nfConfigApi.IpDomain
			if err := json.Unmarshal(got, &decoded); err != nil {
				t.Fatalf("the SMF cannot decode it: %v", err)
			}
			if len(decoded.AdditionalProperties) != 0 {
				t.Errorf("unknown properties: %v", decoded.AdditionalProperties)
			}
		})
	}
}
