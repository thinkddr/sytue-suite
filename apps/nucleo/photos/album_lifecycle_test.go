// SPDX-License-Identifier: Apache-2.0
package photos

import "testing"

func TestAlbumProjectionsRetireOnDrainAndCloseAndRebuildOnlyAfterResume(t *testing.T) {
	plans, _, _, _ := archivePublishFixture(t)
	repository, err := New(Config{Context: t.Context(), Scope: plans.cfg.Scope, Crypto: plans.cfg.Crypto, Gateway: plans.cfg.Gateway, Journal: plans.cfg.Journal, Source: &PlannedSource{scope: plans.cfg.Scope, plans: plans}})
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	if err := repository.rememberMemberships(nil, nil); err != nil {
		t.Fatal(err)
	}
	repository.albumCheckpoints = map[string]albumCheckpointProjection{"owned": {}}
	if err := repository.Drain(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !repository.albumProjectionsRetired || repository.membershipCache != nil || repository.albumCheckpoints != nil {
		t.Fatal("drain retained album projections")
	}
	if err := repository.rememberMemberships(nil, nil); err != nil {
		t.Fatal(err)
	}
	if repository.membershipCache != nil {
		t.Fatal("a draining worker repopulated retired projections")
	}
	if err := repository.Resume(); err != nil {
		t.Fatal(err)
	}
	if err := repository.rememberMemberships(nil, nil); err != nil {
		t.Fatal(err)
	}
	if repository.albumProjectionsRetired || repository.membershipCache == nil {
		t.Fatal("resume did not permit a fresh authenticated projection")
	}
	repository.Close()
	if !repository.albumProjectionsRetired || repository.membershipCache != nil || repository.albumCheckpoints != nil {
		t.Fatal("close retained album projections")
	}
}
