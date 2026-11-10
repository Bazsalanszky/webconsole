// SPDX-FileCopyrightText: 2026 Balázs Toldi
//
// SPDX-License-Identifier: Apache-2.0

package webui_context

import (
	"bytes"
	"fmt"
	"net/url"
	"testing"

	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/webconsole/dbadapter"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	amfOam   = models.ServiceName("namf-oam")
	smfOam   = models.ServiceName("nsmf-oam")
	testIPv4 = "10.0.0.1"
	testIPv6 = "2001:db8::1"
	testPort = int32(29518)
	uriIPv6  = "http://[2001:db8::1]:29518"
)

func endPoint(ipv4, ipv6 string, port int32) models.IpEndPoint {
	ep := models.NewIpEndPoint()
	if ipv4 != "" {
		ep.SetIpv4Address(ipv4)
	}
	if ipv6 != "" {
		ep.SetIpv6Address(ipv6)
	}
	if port != 0 {
		ep.SetPort(port)
	}
	return *ep
}

func oamService(scheme models.UriScheme, endPoints ...models.IpEndPoint) models.NFService {
	return models.NFService{
		ServiceInstanceId: "0",
		ServiceName:       amfOam,
		Scheme:            scheme,
		NfServiceStatus:   models.NFSERVICESTATUS_REGISTERED,
		IpEndPoints:       endPoints,
	}
}

func TestGetNfOamUri(t *testing.T) {
	withFqdn := func(s models.NFService, fqdn string) models.NFService {
		s.SetFqdn(fqdn)
		return s
	}
	withApiPrefix := func(s models.NFService, apiPrefix string) models.NFService {
		s.SetApiPrefix(apiPrefix)
		return s
	}
	suspended := oamService(models.URISCHEME_HTTP, endPoint(testIPv4, "", testPort))
	suspended.NfServiceStatus = models.NFSERVICESTATUS_SUSPENDED
	otherName := oamService(models.URISCHEME_HTTP, endPoint(testIPv4, "", testPort))
	otherName.ServiceName = models.SERVICENAME_NAMF_COMM

	tests := []struct {
		name    string
		profile models.NFProfile
		want    string
	}{
		{
			// The chart default: the AMF registers fqdn "amf". This used to
			// return the bare "amf", and the UE context request failed with
			// "unsupported protocol scheme".
			name: "profile fqdn, as the chart-default AMF registers",
			profile: models.NFProfile{
				Fqdn:       new("amf"),
				NfServices: []models.NFService{withFqdn(oamService(models.URISCHEME_HTTP, endPoint("", "", testPort)), "amf")},
			},
			want: "http://amf:29518",
		},
		{
			name:    "service fqdn, https without a port",
			profile: models.NFProfile{NfServices: []models.NFService{withFqdn(oamService(models.URISCHEME_HTTPS), "amf.example.org")}},
			want:    "https://amf.example.org:443",
		},
		{
			name: "apiPrefix verbatim",
			profile: models.NFProfile{NfServices: []models.NFService{
				withApiPrefix(oamService(models.URISCHEME_HTTP, endPoint(testIPv4, "", testPort)), "http://[2001:db8::9]:8000"),
			}},
			want: "http://[2001:db8::9]:8000",
		},
		{
			name:    "IPv4 endpoint",
			profile: models.NFProfile{NfServices: []models.NFService{oamService(models.URISCHEME_HTTP, endPoint(testIPv4, "", testPort))}},
			want:    "http://10.0.0.1:29518",
		},
		{
			name:    "IPv6 endpoint is bracketed",
			profile: models.NFProfile{NfServices: []models.NFService{oamService(models.URISCHEME_HTTP, endPoint("", testIPv6, testPort))}},
			want:    uriIPv6,
		},
		{
			name: "dual-stack endpoints: IPv4",
			profile: models.NFProfile{NfServices: []models.NFService{
				oamService(models.URISCHEME_HTTP, endPoint("", testIPv6, testPort), endPoint(testIPv4, "", testPort)),
			}},
			want: "http://10.0.0.1:29518",
		},
		{
			name: "IPv6-only profile address with a port-only endpoint",
			profile: models.NFProfile{
				Ipv6Addresses: []string{testIPv6},
				NfServices:    []models.NFService{oamService(models.URISCHEME_HTTPS, endPoint("", "", testPort))},
			},
			want: "https://[2001:db8::1]:29518",
		},
		{
			name: "nfServiceList only",
			profile: models.NFProfile{NfServiceList: &map[string]models.NFService{
				"0": oamService(models.URISCHEME_HTTP, endPoint("", testIPv6, testPort)),
			}},
			want: uriIPv6,
		},
		{
			name:    "suspended service",
			profile: models.NFProfile{NfServices: []models.NFService{suspended}},
		},
		{
			name:    "other service",
			profile: models.NFProfile{NfServices: []models.NFService{otherName}},
		},
		{
			name:    "no address",
			profile: models.NFProfile{NfServices: []models.NFService{oamService(models.URISCHEME_HTTP)}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getNfOamUri(tt.profile, amfOam)
			if got != tt.want {
				t.Fatalf("getNfOamUri() = %q, want %q", got, tt.want)
			}
			if got == "" {
				return
			}
			if u, err := url.Parse(got); err != nil || u.Scheme == "" || u.Port() == "" {
				t.Errorf("url.Parse(%q) = %+v, %v: want a scheme and a port", got, u, err)
			}
		})
	}
}

// legacyGetNfOamUri is the resolver this package used before, verbatim apart
// from the names.
func legacyGetNfOamUri(nfProfile models.NFProfile, serviceName models.ServiceName) (nfOamUri string) {
	for _, service := range nfProfile.NfServices {
		if service.ServiceName == serviceName && service.NfServiceStatus == models.NFSERVICESTATUS_REGISTERED {
			if nfProfile.GetFqdn() != "" {
				nfOamUri = nfProfile.GetFqdn()
			} else if service.GetFqdn() != "" {
				nfOamUri = service.GetFqdn()
			} else if service.GetApiPrefix() != "" {
				nfOamUri = service.GetApiPrefix()
			} else if len(service.IpEndPoints) > 0 {
				point := service.IpEndPoints[0]
				if point.GetIpv4Address() != "" {
					nfOamUri = legacyGetSbiUri(service.Scheme, point.GetIpv4Address(), point.GetPort())
				} else if len(nfProfile.Ipv4Addresses) != 0 {
					nfOamUri = legacyGetSbiUri(service.Scheme, nfProfile.Ipv4Addresses[0], point.GetPort())
				}
			}
		}
		if nfOamUri != "" {
			break
		}
	}
	return
}

func legacyGetSbiUri(scheme models.UriScheme, ipv4Address string, port int32) (uri string) {
	if port != 0 {
		uri = fmt.Sprintf("%s://%s:%d", scheme, ipv4Address, port)
	} else {
		switch scheme {
		case models.URISCHEME_HTTP:
			uri = fmt.Sprintf("%s://%s:80", scheme, ipv4Address)
		case models.URISCHEME_HTTPS:
			uri = fmt.Sprintf("%s://%s:443", scheme, ipv4Address)
		}
	}
	return
}

// For IPv4 profiles without an fqdn and with an http or https scheme (the
// cases where the old resolver returned a URI), the result is unchanged.
func TestGetNfOamUriKeepsLegacyResults(t *testing.T) {
	n := 0
	for _, scheme := range []models.UriScheme{models.URISCHEME_HTTP, models.URISCHEME_HTTPS} {
		for _, apiPrefix := range []string{"", "http://10.9.9.9:8000"} {
			for _, endPoints := range [][]models.IpEndPoint{
				nil,
				{endPoint("", "", 0)},
				{endPoint("", "", testPort)},
				{endPoint(testIPv4, "", 0)},
				{endPoint(testIPv4, "", testPort)},
			} {
				for _, profileIPv4 := range [][]string{nil, {"10.0.0.2"}} {
					for _, status := range []models.NFServiceStatus{models.NFSERVICESTATUS_REGISTERED, models.NFSERVICESTATUS_SUSPENDED} {
						service := oamService(scheme, endPoints...)
						service.NfServiceStatus = status
						if apiPrefix != "" {
							service.SetApiPrefix(apiPrefix)
						}
						profile := models.NFProfile{Ipv4Addresses: profileIPv4, NfServices: []models.NFService{service}}
						if got, want := getNfOamUri(profile, amfOam), legacyGetNfOamUri(profile, amfOam); got != want {
							t.Errorf("profile %+v: getNfOamUri() = %q, legacy %q", profile, got, want)
						}
						n++
					}
				}
			}
		}
	}
	if n != 80 {
		t.Fatalf("compared %d profile shapes, want 80", n)
	}
}

// mockDB serves the NfProfile collection; every other method panics.
type mockDB struct {
	dbadapter.DBInterface
	profiles []map[string]any
}

func (m *mockDB) RestfulAPIGetMany(collName string, _ bson.M) ([]map[string]any, error) {
	if collName != "NfProfile" {
		return nil, fmt.Errorf("unexpected collection %s", collName)
	}
	return m.profiles, nil
}

// storedProfile returns profile as webconsole reads it from MongoDB: the NRF
// stores bson.Marshal(profile), and util/mongoapi decodes documents into maps.
func storedProfile(t *testing.T, profile models.NFProfile) map[string]any {
	t.Helper()
	raw, err := bson.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	dec := bson.NewDecoder(bson.NewDocumentReader(bytes.NewReader(raw)))
	dec.DefaultDocumentMap()
	doc := map[string]any{}
	if err := dec.Decode(&doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestUpdateNfProfilesOamUris(t *testing.T) {
	origDB, origCtx := dbadapter.CommonDBClient, webuiContext
	t.Cleanup(func() { dbadapter.CommonDBClient, webuiContext = origDB, origCtx })

	amfService := oamService(models.URISCHEME_HTTP, endPoint("", testIPv6, testPort))
	smfService := oamService(models.URISCHEME_HTTP, endPoint("10.0.0.2", "", 29502), endPoint("", "2001:db8::2", 29502))
	smfService.ServiceName = smfOam
	dbadapter.CommonDBClient = &mockDB{profiles: []map[string]any{
		// An IPv6-only AMF that registers only the Rel-16 nfServiceList.
		storedProfile(t, models.NFProfile{
			NfInstanceId:  "amf-1",
			NfType:        models.NFTYPE_AMF,
			Ipv6Addresses: []string{testIPv6},
			NfServiceList: &map[string]models.NFService{"0": amfService},
		}),
		// A dual-stack SMF with nfServices.
		storedProfile(t, models.NFProfile{
			NfInstanceId:  "smf-1",
			NfType:        models.NFTYPE_SMF,
			Ipv4Addresses: []string{"10.0.0.2"},
			Ipv6Addresses: []string{"2001:db8::2"},
			NfServices:    []models.NFService{smfService},
		}),
		// No services at all: skipped.
		storedProfile(t, models.NFProfile{NfInstanceId: "amf-2", NfType: models.NFTYPE_AMF}),
	}}
	webuiContext = WEBUIContext{}

	WEBUI_Self().UpdateNfProfiles()

	if got := len(WEBUI_Self().NFProfiles); got != 3 {
		t.Fatalf("decoded %d profiles, want 3", got)
	}
	want := []NfOamInstance{
		{NfId: "amf-1", NfType: models.NFTYPE_AMF, Uri: uriIPv6},
		{NfId: "smf-1", NfType: models.NFTYPE_SMF, Uri: "http://10.0.0.2:29502"},
	}
	got := WEBUI_Self().NFOamInstances
	if len(got) != len(want) {
		t.Fatalf("NFOamInstances = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("NFOamInstances[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
	if uris := WEBUI_Self().GetOamUris(models.NFTYPE_AMF); len(uris) != 1 || uris[0] != want[0].Uri {
		t.Errorf("GetOamUris(AMF) = %q, want [%q]", uris, want[0].Uri)
	}
}
