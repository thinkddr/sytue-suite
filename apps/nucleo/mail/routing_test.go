// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/eml"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type routingFixture struct {
	value                   api.MailRoutingSnapshot
	reads, creates, deletes int
	readError               bool
	loseCreate, loseDelete  bool
}

func (m *routingFixture) RoutingMetadata(ctx context.Context, scope storage.Scope) (api.MailRoutingSnapshot, error) {
	if ctx.Err() != nil {
		return api.MailRoutingSnapshot{}, ctx.Err()
	}
	if scope != testScope {
		return api.MailRoutingSnapshot{}, ErrRecord
	}
	m.reads++
	if m.readError {
		return api.MailRoutingSnapshot{}, errors.New("synthetic unavailable metadata")
	}
	return api.MailRoutingSnapshot{Primary: m.value.Primary, Aliases: slices.Clone(m.value.Aliases)}, nil
}

func (m *routingFixture) CreateRoutingAlias(ctx context.Context, scope storage.Scope, address string) (api.MailAlias, error) {
	if scope != testScope || ctx.Err() != nil {
		return api.MailAlias{}, ErrRecord
	}
	m.creates++
	value := api.MailAlias{ID: "22222222-2222-4222-8222-222222222222", Address: address}
	m.value.Aliases = append(m.value.Aliases, value)
	if m.loseCreate {
		m.loseCreate = false
		return api.MailAlias{}, errors.New("synthetic lost accepted routing ACK")
	}
	return value, nil
}

func (m *routingFixture) DeleteRoutingAlias(ctx context.Context, scope storage.Scope, id string) error {
	if scope != testScope || ctx.Err() != nil {
		return ErrRecord
	}
	m.deletes++
	m.value.Aliases = slices.DeleteFunc(m.value.Aliases, func(value api.MailAlias) bool { return value.ID == id })
	if m.loseDelete {
		m.loseDelete = false
		return errors.New("synthetic lost accepted routing deletion ACK")
	}
	return nil
}

func (m *routingFixture) RoutingQuota(context.Context, storage.Scope) (api.MailQuota, error) {
	return api.MailQuota{UsedBytes: 9999, QuotaBytes: 10000}, nil
}

func newRoutingFixture() *routingFixture {
	return &routingFixture{value: api.MailRoutingSnapshot{Primary: "alice@example.test", Aliases: []api.MailAlias{{ID: "11111111-1111-4111-8111-111111111111", Address: "writing@example.test"}}}}
}

func TestPrivateRoutingProjectionRecoversAcceptedPublicationAndKeepsOfflineDraftIdentity(t *testing.T) {
	f := setupDrafts(t)
	m := newRoutingFixture()
	f.repo.cfg.Metadata = m
	f.cfg.Metadata = m
	f.crypto.loseIndex = true
	if _, err := f.repo.RefreshRouting(t.Context()); err == nil {
		t.Fatal("routing publication did not lose its accepted ACK")
	}
	owner := testScope.Tenant + "\x00" + testScope.Subject + "\x00mail"
	if n, err := f.repo.cfg.Journal.CountOwner(owner, writesNamespace); err != nil || n == 0 {
		t.Fatal("lost routing ACK discarded original WAL", n, err)
	}
	puts := f.crypto.puts
	if _, err := f.repo.RefreshRouting(t.Context()); err != nil || f.crypto.puts != puts {
		t.Fatal("routing retry duplicated its accepted private revision", err)
	}
	if n, err := f.repo.cfg.Journal.CountOwner(owner, writesNamespace); err != nil || n != 0 {
		t.Fatal("matching visible projection skipped exact WAL acknowledgement", n, err)
	}
	repo := f.reopen(t)
	if err := repo.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	m.readError = true
	reads := m.reads
	primary, aliases, err := repo.Aliases(t.Context())
	if err != nil || primary != m.value.Primary || !slices.Equal(aliases, m.value.Aliases) || m.reads != reads {
		t.Fatal("offline identity was replaced by a network fallback", err)
	}
	raw, err := eml.Componer(eml.Salida{De: aliases[0].Address, Asunto: "offline private sender", Texto: "private draft retained locally", Borrador: true}, time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	defer clear(raw)
	if _, err = repo.SaveDraft(t.Context(), "", aliases[0].Address, raw); err != nil || m.reads != reads {
		t.Fatal("ordinary offline draft unexpectedly required routing HTTP", err)
	}
}

func TestPrivateRoutingLostHTTPAcknowledgementsNeverDuplicateAliasOrEraseOriginalProjection(t *testing.T) {
	f := setupDrafts(t)
	m := newRoutingFixture()
	f.repo.cfg.Metadata = m
	if _, err := f.repo.RefreshRouting(t.Context()); err != nil {
		t.Fatal(err)
	}
	m.loseCreate = true
	if _, err := f.repo.CreateRoutingAlias(t.Context(), "new@example.test"); err == nil {
		t.Fatal("accepted public routing ACK was not lost")
	}
	created, err := f.repo.CreateRoutingAlias(t.Context(), "new@example.test")
	if err != nil || m.creates != 1 {
		t.Fatal("routing retry created another address", err, m.creates)
	}
	_, aliases, err := f.repo.Aliases(t.Context())
	if err != nil || len(aliases) != 2 {
		t.Fatal("new owned alias was not retained privately", err)
	}
	m.loseDelete = true
	if err = f.repo.DeleteRoutingAlias(t.Context(), created.ID); err == nil {
		t.Fatal("accepted public deletion ACK was not lost")
	}
	if err = f.repo.DeleteRoutingAlias(t.Context(), created.ID); err != nil || m.deletes != 1 {
		t.Fatal("routing retry replayed an already applied deletion", err, m.deletes)
	}
	_, aliases, err = f.repo.Aliases(t.Context())
	if err != nil || len(aliases) != 1 || aliases[0].Address != "writing@example.test" {
		t.Fatal("routing deletion changed unrelated retained senders", err)
	}
	before := f.crypto.puts
	if err = f.repo.DeleteRoutingAlias(t.Context(), "33333333-3333-4333-8333-333333333333"); !errors.Is(err, ErrNotFound) || f.crypto.puts != before {
		t.Fatal("an unknown alias was acknowledged as an owned deletion", err)
	}
	m.readError = true
	if _, err = f.repo.RefreshRouting(t.Context()); err == nil {
		t.Fatal("failed refresh became an empty routing projection")
	}
	_, retained, err := f.repo.Aliases(t.Context())
	if err != nil || !slices.Equal(retained, aliases) {
		t.Fatal("failed refresh erased the last authenticated projection", err)
	}
}

func TestPrivateRoutingInvalidOrForeignProjectionNeverFallsBackToHistoricalSenderData(t *testing.T) {
	f := setupDrafts(t)
	m := newRoutingFixture()
	f.repo.cfg.Metadata = m
	before := f.crypto.puts
	m.value.Aliases[0].ID = "not-an-alias-id"
	if _, err := f.repo.RefreshRouting(t.Context()); !errors.Is(err, ErrRecord) || f.crypto.puts != before {
		t.Fatal("unvalidated public routing entered the encrypted catalog", err)
	}
	wrong := testScope
	wrong.Subject = "other"
	f.crypto.seed(t, "routing", "aliases", routingProjection{Version: 1, Scope: wrong, Primary: "alice@example.test"})
	if _, _, err := f.repo.Aliases(t.Context()); !errors.Is(err, ErrRecord) {
		t.Fatal("foreign routing projection silently became the historical sender list", err)
	}
}
