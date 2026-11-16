// SPDX-License-Identifier: Apache-2.0
package matrixproof

import (
	"encoding/json"
	"testing"
)

func TestRequestDigestMatchesPublicWireWhenOptionalZeroIsOmitted(t *testing.T) {
	request := struct {
		Version  int   `json:"v"`
		Previous int64 `json:"previous_generation,omitempty"`
	}{Version: 1}
	wire, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if string(wire) != `{"v":1}` {
		t.Fatal("fixture does not exercise an omitted zero")
	}
	typed, err := RequestDigest(request)
	if err != nil {
		t.Fatal(err)
	}
	serialized, err := RequestDigest(json.RawMessage(wire))
	if err != nil || typed != serialized {
		t.Fatal("typed digest differs from the serialized public wire", err)
	}
}
