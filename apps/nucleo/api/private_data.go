// SPDX-License-Identifier: Apache-2.0
package api

import "net/http"

// privateDataClient retains only a service's base URL for existing typed
// helpers. The original session, cookies and transport are not inherited;
// operations without private implementations are rejected before network I/O.
func privateDataClient(original *Client, denied error) *Client {
	if original == nil || denied == nil {
		return nil
	}
	return New(original.base, nil, &http.Client{Transport: privateDataHTTP{denied: denied}})
}

type privateDataHTTP struct{ denied error }

func (p privateDataHTTP) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		_ = req.Body.Close()
	}
	return nil, p.denied
}
