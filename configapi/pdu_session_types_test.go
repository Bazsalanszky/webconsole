// SPDX-FileCopyrightText: 2026 Balázs Toldi
// SPDX-License-Identifier: Apache-2.0

package configapi

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"

	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/webconsole/configmodels"
	"github.com/omec-project/webconsole/dbadapter"
)

const (
	testImsiIPv6     = "208930100007487"
	dnnIPv6Only      = "ims"
	dnnDual          = "dual"
	testMcc, testMnc = "208", "93"
)

func TestPduSessionTypesByDnn(t *testing.T) {
	all := []models.PduSessionType{models.PDUSESSIONTYPE_IPV4, models.PDUSESSIONTYPE_IPV6, models.PDUSESSIONTYPE_IPV4_V6}
	ipv6 := []models.PduSessionType{models.PDUSESSIONTYPE_IPV6}
	tests := []struct {
		name      string
		ipDomains []configmodels.DeviceGroupsIpDomainExpanded
		want      map[string]dnnPduSessionTypes
	}{
		{
			name:      "IPv4 pool only",
			ipDomains: []configmodels.DeviceGroupsIpDomainExpanded{{Dnn: dnnInternet, UeIpPool: testUeIpv4Pool}},
			want:      map[string]dnnPduSessionTypes{},
		},
		{
			name:      "no pool",
			ipDomains: []configmodels.DeviceGroupsIpDomainExpanded{{Dnn: dnnInternet}},
			want:      map[string]dnnPduSessionTypes{},
		},
		{
			name:      "both pools",
			ipDomains: []configmodels.DeviceGroupsIpDomainExpanded{{Dnn: dnnInternet, UeIpPool: testUeIpv4Pool, UeIpv6Pool: testUeIpv6Pool}},
			want:      map[string]dnnPduSessionTypes{dnnInternet: {models.PDUSESSIONTYPE_IPV4, all}},
		},
		{
			name:      "IPv6 pool only",
			ipDomains: []configmodels.DeviceGroupsIpDomainExpanded{{Dnn: dnnIPv6Only, UeIpv6Pool: testUeIpv6Pool}},
			want:      map[string]dnnPduSessionTypes{dnnIPv6Only: {models.PDUSESSIONTYPE_IPV6, ipv6}},
		},
		{
			name: "the two pools of a DNN in separate IP domains",
			ipDomains: []configmodels.DeviceGroupsIpDomainExpanded{
				{Dnn: dnnInternet, UeIpv6Pool: testUeIpv6Pool},
				{Dnn: dnnInternet, UeIpPool: testUeIpv4Pool},
			},
			want: map[string]dnnPduSessionTypes{dnnInternet: {models.PDUSESSIONTYPE_IPV4, all}},
		},
		{
			name: "one DNN of each kind",
			ipDomains: []configmodels.DeviceGroupsIpDomainExpanded{
				{Dnn: "v4", UeIpPool: testUeIpv4Pool},
				{Dnn: dnnDual, UeIpPool: testUeIpv4Pool, UeIpv6Pool: testUeIpv6Pool},
				{Dnn: "v6", UeIpv6Pool: "2001:db8::/48"},
			},
			want: map[string]dnnPduSessionTypes{
				dnnDual: {models.PDUSESSIONTYPE_IPV4, all},
				"v6":    {models.PDUSESSIONTYPE_IPV6, ipv6},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := pduSessionTypesByDnn(tc.ipDomains); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

// smDataAsUdrServesIt decodes a stored SM subscription data document the way
// the UDR does (data_repository.go QuerySmDataProcedure: JSON marshal, then
// unmarshal into models.SessionManagementSubscriptionData). The UDM passes the
// DNN configurations through to the SMF, which reads pduSessionTypes in
// isAllowedPDUSessionType.
func smDataAsUdrServesIt(t *testing.T, doc map[string]any) models.SessionManagementSubscriptionData {
	t.Helper()
	raw, err := json.Marshal([]map[string]any{doc})
	if err != nil {
		t.Fatal(err)
	}
	var typed []models.SessionManagementSubscriptionData
	if err := json.Unmarshal(raw, &typed); err != nil {
		t.Fatalf("the UDR could not decode the document: %v\n%s", err, raw)
	}
	return typed[0]
}

func TestUpdatePolicyAndProvisionedDataPduSessionTypes(t *testing.T) {
	trafficClass := &configmodels.TrafficClassInfo{Qci: 9}
	qos := configmodels.DeviceGroupsIpDomainExpandedUeDnnQos{DnnMbrUplink: 1000000, DnnMbrDownlink: 1000000, TrafficClass: trafficClass}
	ipDomains := []configmodels.DeviceGroupsIpDomainExpanded{
		{Dnn: dnnInternet, UeIpPool: testUeIpv4Pool, UeDnnQos: &qos},
		{Dnn: dnnDual, UeIpPool: testUeIpv4Pool, UeIpv6Pool: testUeIpv6Pool, UeDnnQos: &qos},
		{Dnn: dnnIPv6Only, UeIpv6Pool: "2001:db8::/48", UeDnnQos: &qos},
	}
	dnnMap := map[string][]configmodels.DeviceGroupsIpDomainExpandedUeDnnQos{}
	for _, d := range ipDomains {
		dnnMap[d.Dnn] = append(dnnMap[d.Dnn], *d.UeDnnQos)
	}

	originalDBClient := dbadapter.CommonDBClient
	defer func() { dbadapter.CommonDBClient = originalDBClient }()
	dbMock := &NetworkSliceMockDBClient{}
	dbadapter.CommonDBClient = dbMock

	snssai := models.NewSnssai(1)
	snssai.SetSd("010203")
	if err := updatePolicyAndProvisionedData(testImsiIPv6, "", snssai, dnnMap, pduSessionTypesByDnn(ipDomains),
		testMcc, testMnc, aggregateQoS([]configmodels.DeviceGroupsIpDomainExpandedUeDnnQos{qos})); err != nil {
		t.Fatal(err)
	}
	var smData map[string]any
	for _, put := range dbMock.putData {
		if put[collKey] == smDataColl {
			smData = put[dataKey].(map[string]any)
		}
	}
	if smData == nil {
		t.Fatalf("no SM subscription data written: %+v", dbMock.putData)
	}

	served := smDataAsUdrServesIt(t, smData)
	dnnConfigurations := served.GetDnnConfigurations()
	for _, tc := range []struct {
		dnn         string
		wantDefault models.PduSessionType
		wantAllowed []models.PduSessionType
	}{
		{dnnInternet, models.PDUSESSIONTYPE_IPV4, []models.PduSessionType{models.PDUSESSIONTYPE_IPV4}},
		{dnnDual, models.PDUSESSIONTYPE_IPV4, []models.PduSessionType{models.PDUSESSIONTYPE_IPV4, models.PDUSESSIONTYPE_IPV6, models.PDUSESSIONTYPE_IPV4_V6}},
		{dnnIPv6Only, models.PDUSESSIONTYPE_IPV6, []models.PduSessionType{models.PDUSESSIONTYPE_IPV6}},
	} {
		got := dnnConfigurations[tc.dnn].PduSessionTypes
		if got.GetDefaultSessionType() != tc.wantDefault || !slices.Equal(got.AllowedSessionTypes, tc.wantAllowed) {
			t.Errorf("DNN %s: pduSessionTypes = %s/%v, want %s/%v", tc.dnn,
				got.GetDefaultSessionType(), got.AllowedSessionTypes, tc.wantDefault, tc.wantAllowed)
		}
	}
}

// A device group without IPv6 pools gets the same SM subscription data
// document as before IPv6 pools existed (the nil sessionTypes that the
// existing tests pass).
func TestBuildSmProvisionedDataDocumentIPv4Unchanged(t *testing.T) {
	snssai := models.NewSnssai(1)
	snssai.SetSd("010203")
	qos := configmodels.DeviceGroupsIpDomainExpandedUeDnnQos{DnnMbrUplink: 2000000, DnnMbrDownlink: 5000000, TrafficClass: &configmodels.TrafficClassInfo{Qci: 9}}
	dnnMap := map[string][]configmodels.DeviceGroupsIpDomainExpandedUeDnnQos{dnnInternet: {qos}}
	ipDomains := []configmodels.DeviceGroupsIpDomainExpanded{{Dnn: dnnInternet, UeIpPool: testUeIpv4Pool, UeDnnQos: &qos}}

	legacy, err := buildSmProvisionedDataDocument(snssai, dnnMap, nil, testMcc, testMnc, testImsiIPv6)
	if err != nil {
		t.Fatal(err)
	}
	derived, err := buildSmProvisionedDataDocument(snssai, dnnMap, pduSessionTypesByDnn(ipDomains), testMcc, testMnc, testImsiIPv6)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(legacy, derived) {
		t.Errorf("IPv4-only document changed:\n%+v\n%+v", legacy, derived)
	}
	want := map[string]any{
		"defaultSessionType":  models.PDUSESSIONTYPE_IPV4,
		"allowedSessionTypes": []models.PduSessionType{models.PDUSESSIONTYPE_IPV4},
	}
	got := legacy["dnnconfigurations"].(map[string]any)[dnnInternet].(map[string]any)["pduSessionTypes"]
	if !reflect.DeepEqual(got, want) {
		t.Errorf("pduSessionTypes = %#v, want %#v", got, want)
	}
}
