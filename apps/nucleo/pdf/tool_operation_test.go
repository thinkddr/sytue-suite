// SPDX-License-Identifier: Apache-2.0
package pdf

import "testing"

func TestToolOperationRejectsInvalidLevelAndNonFiniteMargins(t *testing.T) {
	for _, raw := range []string{"1,2,3", "-1,0,0,0", "NaN,0,0,0", "0,Inf,0,0", "0,0,10001,0"} {
		if _, err := ParseMargins(raw); err == nil {
			t.Fatal("invalid margin accepted", raw)
		}
	}
	if _, err := ApplyToolOperation(docDePrueba(t, "unchanged"), map[string]string{"op": "comprimir", "nivel": "unknown"}); err == nil {
		t.Fatal("unknown compression level accepted")
	}
	if _, err := ApplyToolOperation(nil, nil); err == nil {
		t.Fatal("nil document accepted")
	}
	if _, err := ApplyToolOperation(docDePrueba(t, "unchanged"), map[string]string{"op": "unknown"}); err == nil {
		t.Fatal("unknown tool accepted")
	}
}
