// SPDX-License-Identifier: Apache-2.0
package model

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestSubmissionImmutableFingerprintRetainsGenerationFenceValidation(t *testing.T) {
	input := Submission{Operation: uuid.NewString(), Epoch: uuid.NewString(), From: "sender@example.test", Recipients: []string{"recipient@example.test"}, Consent: SMTPConsent, Generation: 1, Control: strings.Repeat("a", 64)}
	first, err := input.Fingerprint([]byte("entire immutable original MIME"))
	if err != nil {
		t.Fatal(err)
	}
	next := input
	next.Generation = 2
	next.Control = strings.Repeat("b", 64)
	second, err := next.Fingerprint([]byte("entire immutable original MIME"))
	if err != nil || first != second {
		t.Fatal("rescheduling changed original message commitment", err)
	}
	receipt := SubmissionReceipt{Operation: input.Operation, Fingerprint: first, States: []string{"ready"}, Generation: 1, Control: input.Control}
	if err := receipt.Validate(next, first); err == nil {
		t.Fatal("stale receipt authorized a newer generation")
	}
	receipt.Generation = 2
	receipt.Control = next.Control
	if err := receipt.Validate(next, first); err != nil {
		t.Fatal(err)
	}
	for _, fault := range []string{"claimed-cancel", "hidden-cancel", "malformed-control"} {
		bad := receipt
		switch fault {
		case "claimed-cancel":
			bad.Canceled = true
		case "hidden-cancel":
			bad.States = []string{"canceled"}
		case "malformed-control":
			bad.Control = "not-a-commitment"
		}
		if err := bad.Validate(next, first); err == nil {
			t.Fatal("inconsistent external receipt was accepted", fault)
		}
	}
}
