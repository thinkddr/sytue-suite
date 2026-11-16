// SPDX-License-Identifier: Apache-2.0
package resource_test

import (
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
)

func TestBootstrapPlanRequiresCurrentRosterCapture(t *testing.T) {
	historical := prepareBootstrapPlanFixture(t)
	if err := historical.verified.Revalidate(t.Context()); err != nil {
		t.Fatal("legacy signed proof unexpectedly invalid", err)
	}
	if err := historical.verified.RequireCurrentRosterCapture(t.Context()); err == nil {
		t.Fatal("owner-only proof became complete current capture")
	}
	c := prepareCaptureCase(t)
	current, err := resource.CompleteBootstrapRosterCapture(t.Context(), c.pending, capturePages{inputs: c.inputs}, c.provider)
	if err != nil {
		t.Fatal(err)
	}
	if err := current.RequireCurrentRosterCapture(t.Context()); err != nil {
		t.Fatal("complete current capture rejected", err)
	}
	c.readerSource.Retire()
	if err := current.RequireCurrentRosterCapture(t.Context()); err == nil {
		t.Fatal("current capture gate forgot withdrawn non-owner")
	}
}
