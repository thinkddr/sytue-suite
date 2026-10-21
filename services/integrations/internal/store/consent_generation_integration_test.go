// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"encoding/json"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/store"
)

func TestConsentCommitPreservesNewerPendingAndCompletedChoice(t *testing.T) {
	st, _, ctx, id := connectionFixture(t)
	if err := st.MarcarConsentimiento(ctx, id, "choice-a"); err != nil {
		t.Fatal(err)
	}
	if ok, err := st.ConsumirConsentimiento(ctx, id, "choice-a"); err != nil || !ok {
		t.Fatalf("consume A: %v %v", ok, err)
	}
	if err := st.MarcarConsentimiento(ctx, id, "choice-b"); err != nil {
		t.Fatal(err)
	}
	// A stale config save, including a legacy refresh, cannot restore generation A
	// or erase the newer pending state. Only the consent helpers control them.
	if _, err := st.UpdateInstance(ctx, id, "old snapshot", json.RawMessage(`{"_oauth_pending":"choice-a","_oauth_generation":"choice-a"}`), domain.StatusActive, nil); err != nil {
		t.Fatal(err)
	}
	commit := func(challenge, name string) error {
		cfg, _ := json.Marshal(map[string]string{"server": name})
		_, err := st.CommitInstanceConsent(ctx, id, name, cfg, domain.StatusActive, nil, map[string]string{store.SecretKeyBearer: name}, challenge)
		return err
	}
	if err := commit("choice-a", "stale-a"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("older exchange replaced a newer pending consent: %v", err)
	}
	conn, err := st.LoadInstanceWithSecrets(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if conn.Config[store.ConfigKeyConsentimiento] != "choice-b" || conn.Config[store.ConfigKeyConsentGeneration] != "choice-b" || conn.Secrets[store.SecretKeyBearer] != "bearer-old" {
		t.Fatal("older exchange erased the newer pending choice or changed credentials")
	}
	if ok, err := st.ConsumirConsentimiento(ctx, id, "choice-b"); err != nil || !ok {
		t.Fatalf("consume B: %v %v", ok, err)
	}
	if err := commit("choice-b", "chosen-b"); err != nil {
		t.Fatal(err)
	}
	if err := commit("choice-a", "stale-a"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("older exchange replaced a completed newer consent: %v", err)
	}
	conn, err = st.LoadInstanceWithSecrets(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if conn.Config["server"] != "chosen-b" || conn.Secrets[store.SecretKeyBearer] != "chosen-b" || conn.Config[store.ConfigKeyConsentimiento] != "" || conn.Config[store.ConfigKeyConsentGeneration] != "choice-b" {
		t.Fatal("older exchange replaced the newer completed choice")
	}
	if err := commit("", "unbound"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("consent without a generation accepted: %v", err)
	}
}

func TestConsentCommitRequiresTheStateToHaveBeenConsumed(t *testing.T) {
	st, _, ctx, id := connectionFixture(t)
	if err := st.MarcarConsentimiento(ctx, id, "pending"); err != nil {
		t.Fatal(err)
	}
	_, err := st.CommitInstanceConsent(ctx, id, "unsafe", json.RawMessage(`{}`), domain.StatusActive, nil, map[string]string{store.SecretKeyBearer: "unsafe"}, "pending")
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("unconsumed state committed: %v", err)
	}
}

func TestConsentStartedBeforeGenerationDeploymentCanFinish(t *testing.T) {
	st, _, ctx, _ := connectionFixture(t)
	inst, err := st.CreateInstance(ctx, "atomic-mail-fixture", "legacy pending", json.RawMessage(`{"_oauth_pending":"legacy-challenge"}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := st.ConsumirConsentimiento(ctx, inst.ID, "legacy-challenge"); err != nil || !ok {
		t.Fatalf("legacy consent was not consumed: %v %v", ok, err)
	}
	if _, err := st.CommitInstanceConsent(ctx, inst.ID, "legacy connected", json.RawMessage(`{}`), domain.StatusActive, nil, map[string]string{store.SecretKeyBearer: "legacy-token"}, "legacy-challenge"); err != nil {
		t.Fatalf("legitimate in-flight consent was rejected: %v", err)
	}
}
