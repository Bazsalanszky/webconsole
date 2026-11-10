// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package webui_context

import (
	"reflect"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/omec-project/openapi/v2/models"
	openapiutils "github.com/omec-project/openapi/v2/utils"
	"github.com/omec-project/webconsole/backend/logger"
	"github.com/omec-project/webconsole/dbadapter"
)

var webuiContext = WEBUIContext{}

type WEBUIContext struct {
	NFProfiles     []models.NFProfile
	NFOamInstances []NfOamInstance
}

type NfOamInstance struct {
	NfId   string
	NfType models.NFType
	Uri    string
}

func init() {
}

func (context *WEBUIContext) UpdateNfProfiles() {
	nfProfilesRaw, errGetMany := dbadapter.CommonDBClient.RestfulAPIGetMany("NfProfile", nil)
	if errGetMany != nil {
		logger.DbLog.Warnln(errGetMany)
	}
	nfProfiles, err := decode(nfProfilesRaw, time.RFC3339)
	if err != nil {
		logger.ContextLog.Error(err)
		return
	}

	context.NFProfiles = nfProfiles

	for _, nfProfile := range context.NFProfiles {
		if len(openapiutils.NFProfileServices(&nfProfile)) == 0 || context.NfProfileAlreadyExists(nfProfile) {
			continue
		}

		var uri string
		switch nfProfile.NfType {
		case models.NFTYPE_AMF:
			uri = getNfOamUri(nfProfile, models.ServiceName("namf-oam"))
		case models.NFTYPE_SMF:
			uri = getNfOamUri(nfProfile, models.ServiceName("nsmf-oam"))
		}
		if uri != "" {
			context.NFOamInstances = append(context.NFOamInstances, NfOamInstance{
				NfId:   nfProfile.NfInstanceId,
				NfType: nfProfile.NfType,
				Uri:    uri,
			})
		}
	}
}

func (context *WEBUIContext) NfProfileAlreadyExists(nfProfile models.NFProfile) bool {
	for _, instance := range context.NFOamInstances {
		if instance.NfId == nfProfile.NfInstanceId {
			return true
		}
	}
	return false
}

// getNfOamUri returns the base URI (scheme://host:port, an IPv6 host in
// brackets) of the registered service serviceName of nfProfile, or "" when
// the profile has none. It reads nfServiceList, else nfServices, and takes
// the address from the profile or service fqdn, the apiPrefix, an
// ipEndPoint or the profile addresses, in that order (see
// openapiutils.NFServiceURI). Webconsole has no address family of its own,
// so IPv4 is used when the NF offers both.
func getNfOamUri(nfProfile models.NFProfile, serviceName models.ServiceName) string {
	return openapiutils.SearchNFServiceURI(&nfProfile, serviceName, models.NFSERVICESTATUS_REGISTERED, false)
}

func (context *WEBUIContext) GetOamUris(targetNfType models.NFType) (uris []string) {
	for _, oamInstance := range context.NFOamInstances {
		if oamInstance.NfType == targetNfType {
			uris = append(uris, oamInstance.Uri)
			break
		}
	}
	return
}

func WEBUI_Self() *WEBUIContext {
	return &webuiContext
}

func decode(source any, format string) ([]models.NFProfile, error) {
	var target []models.NFProfile

	// config mapstruct
	stringToDateTimeHook := func(
		f reflect.Type,
		t reflect.Type,
		data any,
	) (any, error) {
		if t == reflect.TypeOf(time.Time{}) && f == reflect.TypeOf("") {
			return time.Parse(format, data.(string))
		}
		return data, nil
	}

	config := mapstructure.DecoderConfig{
		DecodeHook: stringToDateTimeHook,
		Result:     &target,
	}

	decoder, err := mapstructure.NewDecoder(&config)
	if err != nil {
		return nil, err
	}

	// Decode result to NfProfile structure
	err = decoder.Decode(source)
	if err != nil {
		return nil, err
	}
	return target, nil
}
