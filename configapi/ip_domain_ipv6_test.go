// SPDX-FileCopyrightText: 2026 Balázs Toldi
// SPDX-License-Identifier: Apache-2.0

package configapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/omec-project/webconsole/configmodels"
	"github.com/omec-project/webconsole/dbadapter"
)

const (
	testUeIpv6Pool   = "2a0e:8f02:f07b:500::/56"
	testDnsIpv6      = "2001:4860:4860::8888"
	testPcscfIpv6    = "2001:db8:5::10"
	testUeIpv4Pool   = "172.250.0.0/16"
	testIpv4Dns      = "8.8.8.8"
	testIpv4Dns2     = "1.1.1.1"
	keyDnsV6         = "dns-primary-v6"
	keyPcscfV6       = "pcscf-primary-v6"
	errNotIPv6Prefix = "not an IPv6 prefix"
)

func ipv6IpDomain(pool string, prefixLen int32, dns, pcscf string) configmodels.DeviceGroupsIpDomainExpanded {
	return configmodels.DeviceGroupsIpDomainExpanded{
		Dnn:             dnnInternet,
		UeIpPool:        testUeIpv4Pool,
		DnsPrimary:      testIpv4Dns2,
		UeIpv6Pool:      pool,
		UeIpv6PrefixLen: prefixLen,
		DnsPrimaryV6:    dns,
		PcscfPrimaryV6:  pcscf,
	}
}

func TestValidateIpDomainIPv6(t *testing.T) {
	tests := []struct {
		name     string
		ipDomain configmodels.DeviceGroupsIpDomainExpanded
		wantErr  string // "" for valid
	}{
		{"no IPv6 settings", ipv6IpDomain("", 0, "", ""), ""},
		{"pool, default prefix length", ipv6IpDomain(testUeIpv6Pool, 0, "", ""), ""},
		{"pool and all addresses", ipv6IpDomain(testUeIpv6Pool, 64, testDnsIpv6, testPcscfIpv6), ""},
		{"prefix length equal to the pool", ipv6IpDomain(testUeIpv6Pool, 56, "", ""), ""},
		{"/64 pool, one session", ipv6IpDomain("2001:db8::/64", 0, "", ""), ""},
		{"addresses without a pool", ipv6IpDomain("", 0, testDnsIpv6, testPcscfIpv6), ""},
		{"prefix length without a pool", ipv6IpDomain("", 64, "", ""), "without ue-ipv6-pool"},
		{"prefix length shorter than the pool", ipv6IpDomain(testUeIpv6Pool, 48, "", ""), "outside 56"},
		{"prefix length longer than 64", ipv6IpDomain(testUeIpv6Pool, 80, "", ""), "outside 56"},
		{"negative prefix length", ipv6IpDomain(testUeIpv6Pool, -1, "", ""), "outside"},
		{"pool longer than 64", ipv6IpDomain("2001:db8::/80", 0, "", ""), "outside 80"},
		{"IPv4 pool", ipv6IpDomain("10.0.0.0/8", 0, "", ""), errNotIPv6Prefix},
		{"IPv4-mapped pool", ipv6IpDomain("::ffff:10.0.0.0/104", 0, "", ""), errNotIPv6Prefix},
		{"address without a length", ipv6IpDomain("2001:db8::", 0, "", ""), errNotIPv6Prefix},
		{"garbage pool", ipv6IpDomain("pool", 0, "", ""), errNotIPv6Prefix},
		{"host bits set", ipv6IpDomain("2a0e:8f02:f07b:500::1/56", 0, "", ""), "the pool is 2a0e:8f02:f07b:500::/56"},
		{"IPv4 DNS", ipv6IpDomain(testUeIpv6Pool, 0, testIpv4Dns, ""), keyDnsV6},
		{"IPv4-mapped DNS", ipv6IpDomain(testUeIpv6Pool, 0, "::ffff:8.8.8.8", ""), keyDnsV6},
		{"DNS with a zone", ipv6IpDomain(testUeIpv6Pool, 0, "fe80::1%eth0", ""), keyDnsV6},
		{"DNS as a prefix", ipv6IpDomain(testUeIpv6Pool, 0, "2001:db8::1/128", ""), keyDnsV6},
		{"IPv4 P-CSCF", ipv6IpDomain(testUeIpv6Pool, 0, "", "10.0.0.1"), keyPcscfV6},
		{"IPv4-mapped P-CSCF", ipv6IpDomain(testUeIpv6Pool, 0, "", "::ffff:10.0.0.1"), keyPcscfV6},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateIpDomainIPv6(&tc.ipDomain)
			switch {
			case tc.wantErr == "" && err != nil:
				t.Errorf("unexpected error: %v", err)
			case tc.wantErr != "" && err == nil:
				t.Errorf("accepted, want an error containing %q", tc.wantErr)
			case tc.wantErr != "" && !strings.Contains(err.Error(), tc.wantErr):
				t.Errorf("error %q does not contain %q", err, tc.wantErr)
			}
		})
	}
}

// An IPv6-only DNN has no IPv4 pool and no IPv4 DNS server: those keys are
// simply left out, as webconsole never required them.
func TestDeviceGroupPostIPv6IpDomain(t *testing.T) {
	dualStack := deviceGroup(testGroupName)
	dualStack.IpDomainsExpanded[0].UeIpv6Pool = testUeIpv6Pool
	dualStack.IpDomainsExpanded[0].DnsPrimaryV6 = testDnsIpv6
	dualStack.IpDomainsExpanded[0].PcscfPrimaryV6 = testPcscfIpv6

	ipv6Only := deviceGroup(testGroupName)
	ipv6Only.IpDomainsExpanded[0].UeIpPool = ""
	ipv6Only.IpDomainsExpanded[0].DnsPrimary = ""
	ipv6Only.IpDomainsExpanded[0].DnsSecondary = ""
	ipv6Only.IpDomainsExpanded[0].UeIpv6Pool = testUeIpv6Pool
	ipv6Only.IpDomainsExpanded[0].UeIpv6PrefixLen = 60
	ipv6Only.IpDomainsExpanded[0].DnsPrimaryV6 = testDnsIpv6

	invalid := deviceGroup(testGroupName)
	invalid.IpDomainsExpanded[0].UeIpv6Pool = testUeIpv6Pool
	invalid.IpDomainsExpanded[0].DnsPrimaryV6 = testIpv4Dns

	for _, tc := range []struct {
		name     string
		group    configmodels.DeviceGroups
		wantCode int
	}{
		{"dual stack", dualStack, http.StatusOK},
		{"IPv6 only", ipv6Only, http.StatusOK},
		{"IPv4 DNS server as dns-primary-v6", invalid, http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			originalDBClient := dbadapter.CommonDBClient
			defer func() { dbadapter.CommonDBClient = originalDBClient }()
			dbMock := &DeviceGroupMockDBClient{}
			dbadapter.CommonDBClient = dbMock

			w := postDeviceGroup(t, tc.group)
			if w.Code != tc.wantCode {
				t.Fatalf("expected %d, got %d: %s", tc.wantCode, w.Code, w.Body.String())
			}
			if tc.wantCode != http.StatusOK {
				if len(dbMock.postData) != 0 {
					t.Errorf("the device group was stored although the request was refused: %+v", dbMock.postData)
				}
				if !strings.Contains(w.Body.String(), keyDnsV6) {
					t.Errorf("the error does not name the field: %s", w.Body.String())
				}
				return
			}
			if len(dbMock.postData) == 0 {
				t.Fatal("expected the device group to be stored")
			}
			var stored configmodels.DeviceGroups
			if err := json.Unmarshal(configmodels.MapToByte(dbMock.postData[0][dataKey].(map[string]any)), &stored); err != nil {
				t.Fatalf("failed to unmarshal the stored device group: %v", err)
			}
			got, want := stored.IpDomainsExpanded[0], tc.group.IpDomainsExpanded[0]
			if got.UeIpv6Pool != want.UeIpv6Pool || got.UeIpv6PrefixLen != want.UeIpv6PrefixLen ||
				got.DnsPrimaryV6 != want.DnsPrimaryV6 || got.PcscfPrimaryV6 != want.PcscfPrimaryV6 ||
				got.UeIpPool != want.UeIpPool || got.DnsPrimary != want.DnsPrimary {
				t.Errorf("stored IP domain = %+v, want the posted IPv6 settings of %+v", got, want)
			}
		})
	}
}

// A device group without IPv6 settings stores and returns the same JSON as
// before: the new keys are omitted.
func TestDeviceGroupJSONWithoutIPv6IsUnchanged(t *testing.T) {
	encoded, err := json.Marshal(deviceGroup(testGroupName))
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"ue-ipv6-pool", "ue-ipv6-prefix-len", keyDnsV6, keyPcscfV6} {
		if strings.Contains(string(encoded), key) {
			t.Errorf("%s is in the JSON of an IPv4-only device group: %s", key, encoded)
		}
	}
}
