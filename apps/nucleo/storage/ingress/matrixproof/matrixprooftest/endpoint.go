// SPDX-License-Identifier: Apache-2.0
package matrixprooftest

import (
	"errors"
	"net"
	"net/url"
	"strconv"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// DisposableHTTPOrigin accepts only explicitly selected ephemeral loopback
// servers. Fixtures cannot inherit a production URL or a credential in a URL.
func DisposableHTTPOrigin(raw string) (*url.URL, error) {
	endpoint, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	ip := net.ParseIP(endpoint.Hostname())
	port, err := strconv.Atoi(endpoint.Port())
	if err != nil || ip == nil || !ip.IsLoopback() || port < 1024 || port > 65535 || endpoint.Scheme != "http" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" || (endpoint.Path != "" && endpoint.Path != "/") {
		return nil, errors.New("fixture requires an ephemeral loopback HTTP origin")
	}
	endpoint.Path = ""
	return endpoint, nil
}

// IngressDeviceVector contains disposable routing only, never provider tokens,
// an administrator credential, recovery material or device private keys.
type IngressDeviceVector struct {
	Scope        storage.Scope `json:"scope"`
	Epoch        string        `json:"epoch"`
	UserID       string        `json:"user_id"`
	RoomID       string        `json:"room_id"`
	SynapseURL   string        `json:"synapse_url"`
	LegacyUserID string        `json:"legacy_user_id,omitempty"`
	LegacyRoomID string        `json:"legacy_room_id,omitempty"`
}
