//go:build integration && cgo

// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type bootstrapRecoveryFaultTransport struct {
	base http.RoundTripper
	deny atomic.Bool
}

func (t *bootstrapRecoveryFaultTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.deny.Load() && req.Method == http.MethodGet && strings.Contains(req.URL.Path, "/account_data/"+ssssVaultEventPrefix+"resource_bootstrap_") {
		body := "{\"errcode\":\"M_FORBIDDEN\",\"error\":\"artificial scoped recovery read interruption\"}"
		return &http.Response{StatusCode: http.StatusForbidden, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body)), Request: req}, nil
	}
	return t.base.RoundTrip(req)
}

func newBootstrapRecoveryTestVault(ctx context.Context, t *testing.T, canonical *Client, scope storage.Scope) (storageSynapseDevice, *bootstrapRecoveryFaultTransport) {
	t.Helper()
	transport := canonical.cfg.HTTPClient.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	fault := &bootstrapRecoveryFaultTransport{base: transport}
	client := &http.Client{Transport: fault, Timeout: canonical.cfg.HTTPClient.Timeout, CheckRedirect: canonical.cfg.HTTPClient.CheckRedirect, Jar: canonical.cfg.HTTPClient.Jar}
	return newStorageSynapseDeviceForScope(ctx, t, canonical.cfg.Bootstrap.HomeserverURL, client, scope, storage.IsolatedIdentity), fault
}

// Uses REAL canonical SDK and product-isolated SDK2 SSSS, with the original
// native resource fixture's explicitly synthetic Registry/source approval.
// Canonical A is current during capture/backup. The final fresh-B branch later
// revokes it in Synapse and restores only these public bytes plus actual SDK2.
// This is not a real SourceSQL/Registry/Origin or physical OS composition proof.
func exerciseNativeBootstrapRecovery(ctx context.Context, t *testing.T, canonical *Client, current resource.VerifiedSession, plan resource.VerifiedBootstrapPlan, roster resource.CapturedBootstrapRosterEvidence, baseline resource.VerifiedBootstrapAcceptance, checkpoint *PreparedBootstrapPublication, vault storageSynapseDevice, fault *bootstrapRecoveryFaultTransport, lease offline.WorkLease, journal *offline.Store) (*Client, *nativeBootstrapPolicyWitness) {
	t.Helper()
	candidate, err := checkpoint.Candidate()
	if err != nil {
		t.Fatal(err)
	}
	verified, err := resource.VerifyBootstrapCandidate(ctx, candidate, plan, &baseline)
	if err != nil {
		t.Fatal(err)
	}
	sent, err := canonical.cli.SendMessageEvent(ctx, id.RoomID(plan.Value().RoomID), event.EventEncrypted, json.RawMessage(candidate.EncryptedEvent), mautrix.ReqSendEvent{TransactionID: candidate.Proof.TransactionID})
	if err != nil {
		t.Fatal(err)
	}
	ref := storage.Reference{RoomID: plan.Value().RoomID, EventID: sent.EventID.String()}
	cpAccepted, err := resource.ConfirmBootstrapAcceptance(ctx, verified, ref, nativeBootstrapAcceptance{client: canonical.cli})
	if err != nil {
		t.Fatal(err)
	}
	b, err := baseline.Baseline()
	if err != nil {
		t.Fatal(err)
	}
	cp, err := cpAccepted.Checkpoint()
	if err != nil {
		t.Fatal(err)
	}
	p := plan.Value()
	epoch := resource.SignedEpoch{Epoch: resource.Epoch{Version: 1, Domain: "org.sytue.resource.epoch.v1", Resource: p.Source.Resource, RoomID: p.RoomID, ID: p.Epoch, Generation: "1", Recipient: p.Recipient, MembershipSHA256: p.MembershipSHA256, History: p.History, Cutover: b.Position, Baseline: b, Checkpoint: cp, Validation: p.Validation, Issuer: p.Creator}}
	epoch.Signatures, err = canonical.signResourceJSON(ctx, epoch)
	if err != nil {
		t.Fatal(err)
	}
	activation, err := resource.VerifyBootstrapActivation(ctx, epoch, plan, baseline, cpAccepted)
	if err != nil {
		t.Fatal(err)
	}
	policyWitness := captureNativeBootstrapPolicyWitness(ctx, t, plan, roster, baseline, cpAccepted, activation, journal)
	anchors, err := canonical.ResourceBootstrapPolicyAnchors(ctx, current, vault.Client, lease)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := anchors.BootstrapPolicyAnchor(ctx, p.Source.Resource, p.ID); err == nil {
		t.Fatal("missing final private policy pin fell back to source")
	}
	fault.deny.Store(true)
	_, _, interruptedAnchor := canonical.PrepareResourceBootstrapRecovery(ctx, current, activation, policyWitness.ownerIntent, vault.Client, lease, journal)
	fault.deny.Store(false)
	if interruptedAnchor == nil {
		t.Fatal("fixture did not interrupt authenticated policy anchor readback")
	}
	ownerTuple, err := current.Scope().JournalOwner()
	if err != nil {
		t.Fatal(err)
	}
	if count, err := journal.CountOwner(ownerTuple, storage.ResourceBootstrapRecoveryNamespace); err != nil || count != 1 {
		t.Fatal("failed final policy pin backup was not retained pending", count, err)
	}
	guard, requirement, err := canonical.PrepareResourceBootstrapRecovery(ctx, current, activation, policyWitness.ownerIntent, vault.Client, lease, journal)
	if err != nil {
		t.Fatal("actual final policy SSSS pin readback", err)
	}
	if err := guard.CheckBootstrapRecoveryReady(ctx, requirement); err != nil {
		t.Fatal("signed owner capability prerequisite did not re-read authenticated vault", err)
	}
	changedRequirement := requirement
	changedRequirement.OwnerCapabilitySHA256 = requirement.PolicySHA256
	if err := guard.CheckBootstrapRecoveryReady(ctx, changedRequirement); err == nil {
		t.Fatal("bootstrap prerequisite accepted a substituted original owner capability hash")
	}
	independentAnchor, err := anchors.BootstrapPolicyAnchor(ctx, p.Source.Resource, p.ID)
	if err != nil || independentAnchor != activation.Anchor() {
		t.Fatal("private authenticated policy pin changed original epoch", err)
	}
	window, err := resource.CurrentEpoch(activation.Epoch())
	if err != nil {
		t.Fatal(err)
	}
	name, err := resource.BootstrapRecoverySecretID(plan.SHA256())
	if err != nil {
		t.Fatal(err)
	}
	owner := current.Scope().Tenant + "\x00" + current.Scope().Subject + "\x00" + current.Scope().Product
	baselinePending, err := journal.CountOwner(owner, storage.PrivateJournalNamespaces(current.Scope().Product)...)
	if err != nil || baselinePending < 2 {
		t.Fatal("suite inventory omitted prepared bootstrap publications", baselinePending, err)
	}
	if count, err := journal.CountOwner(owner, storage.ResourceBootstrapPublicationNamespace); err != nil || count != 2 {
		t.Fatal("both bootstrap descriptors must remain pending until original activation receipt", count, err)
	}
	if count, err := journal.CountOwner(owner, storage.ResourceBootstrapRecoveryNamespace); err != nil || count != 0 {
		t.Fatal("acknowledged resource backup retained unresolved work", count, err)
	}
	// An interrupted authenticated read must remain visible to the SAME complete
	// owner tuple and namespace inventory used by suite teardown, not Subject only.
	fault.deny.Store(true)
	interrupted := canonical.BackupResourceBootstrapKey(ctx, current, plan, vault.Client, lease, journal)
	fault.deny.Store(false)
	if interrupted == nil {
		t.Fatal("fixture did not interrupt the real scoped SSSS read")
	}
	if count, err := journal.CountOwner(owner, storage.PrivateJournalNamespaces(current.Scope().Product)...); err != nil || count != baselinePending+1 {
		t.Fatal("suite inventory lost the interrupted resource backup", count, err)
	}
	if count, err := journal.CountOwner(current.Scope().Subject, storage.ResourceBootstrapRecoveryNamespace); err != nil || count != 0 {
		t.Fatal("resource backup was stored under incomplete owner", count, err)
	}
	if err := canonical.BackupResourceBootstrapKey(ctx, current, plan, vault.Client, lease, journal); err != nil {
		t.Fatal("same accepted SSSS backup did not reconcile after read interruption", err)
	}
	if count, err := journal.CountOwner(owner, storage.PrivateJournalNamespaces(current.Scope().Product)...); err != nil || count != baselinePending {
		t.Fatal("reconciled resource backup remains pending", count, err)
	}
	private, rev, found, err := vault.Client.ReadSecret(ctx, name, 1)
	if err != nil || !found || rev != 1 {
		t.Fatal("original product vault has no exact bootstrap key", err)
	}
	defer clear(private)
	raw, err := resource.EncodeBootstrapRecovery(ctx, plan, checkpoint.room.key.Load())
	if err != nil || !bytes.Equal(private, raw) {
		clear(raw)
		t.Fatal("SSSS did not retain the exact signed plan/key", err)
	}
	clear(raw)
	login, err := vault.Matrix.Login(ctx, &mautrix.ReqLogin{Type: mautrix.AuthTypePassword, Identifier: mautrix.UserIdentifier{Type: mautrix.IdentifierTypeUser, User: vault.Matrix.UserID.String()}, Password: vault.Password, DeviceID: "RESOURCE_RECOVERY_FRESH_STORAGE"})
	if err != nil {
		t.Fatal(err)
	}
	cfg := vault.Config
	cfg.StoreDir = t.TempDir()
	cfg.Secrets = &memorySecrets{values: map[string][]byte{}}
	fresh := storageSynapseSession(ctx, t, cfg, credentials{Homeserver: cfg.Bootstrap.HomeserverURL, UserID: login.UserID, DeviceID: login.DeviceID, AccessToken: login.AccessToken, Subject: cfg.Subject, Tenant: cfg.Tenant})
	go func() { _ = fresh.Sync(ctx, func(DecryptedEvent, error) {}) }()
	if err := fresh.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	if h, err := canonical.RecoverResourceBootstrapEpoch(ctx, current, plan.SHA256(), window, fresh, lease); err == nil {
		h.Close()
		t.Fatal("token-only private device recovered the resource epoch")
	}
	if err := vault.Client.Close(); err != nil {
		t.Fatal(err)
	}
	if err := fresh.Recover(ctx, vault.Recovery); err != nil {
		t.Fatal("fresh product-isolated SSSS recovery", err)
	}
	freshAnchors, err := canonical.ResourceBootstrapPolicyAnchors(ctx, current, fresh, lease)
	if err != nil {
		t.Fatal("recovered product vault could not authenticate final policy pin", err)
	}
	if actual, err := freshAnchors.BootstrapPolicyAnchor(ctx, p.Source.Resource, p.ID); err != nil || actual != independentAnchor {
		t.Fatal("fresh SDK2 lost independent final policy pin", err)
	}
	handle, err := canonical.RecoverResourceBootstrapEpoch(ctx, current, plan.SHA256(), window, fresh, lease)
	if err != nil || handle == nil {
		t.Fatal("fresh SDK2 could not restore the original epoch", err)
	}
	handle.Close()
	// Reopening the canonical bootstrap uses the SAME ciphertext and a recovered
	// vault device; it may neither change the epoch key nor overwrite its backup.
	if err := canonical.BackupResourceBootstrapKey(ctx, current, plan, fresh, lease, journal); err != nil {
		t.Fatal("recovered vault rejected idempotent exact bootstrap backup", err)
	}
	if h, err := canonical.RecoverResourceBootstrapEpoch(ctx, current, plan.SHA256(), window, canonical, lease); err == nil {
		h.Close()
		t.Fatal("canonical global SSSS replaced the product-isolated vault")
	}
	t.Log("RESOURCE_BOOTSTRAP_ISOLATED_SSSS_EXACT_FRESH_DEVICE_GREEN")
	return fresh, policyWitness
}
