// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Tests de los handlers HTTP JMAP/REST con un fake del store y del objectstore,
// sin PostgreSQL ni S3. Ejercitan rutas OK, validación de entrada, mapeo de
// errores a códigos HTTP, el sobre JSON canónico y la autenticación por claims.
package jmap

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/mail/internal/config"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
	"git.sytue.eu/sytue/one/services/mail/internal/store"
)

// --- fakes ---------------------------------------------------------------

type fakeStore struct {
	// compartidos es cuántos buzones compartidos tiene ya la organización.
	compartidos  int
	dominios     []string
	dominiosErr  error
	acc          domain.Account
	accErr       error
	bySubject    domain.Account
	bySubErr     error
	mailboxes    []domain.Mailbox
	mailboxErr   error
	createdMB    domain.Mailbox
	createErr    error
	renameErr    error
	delMBErr     error
	messages     []domain.Message
	msgErr       error
	message      domain.Message
	getMsgErr    error
	getMsgsRet   []domain.Message
	getMsgsErr   error
	searchRet    []domain.Message
	searchErr    error
	setFlagsErr  error
	moveErr      error
	delMsgErr    error
	byRole       domain.Mailbox
	byRoleErr    error
	byIDErr      error
	insertErr    error
	filters      []domain.Filter
	filtersErr   error
	createdFltr  domain.Filter
	createFErr   error
	updateFErr   error
	deleteFErr   error
	scheduled    []domain.OutboundMessage
	schedErr     error
	reschedErr   error
	cancelBlob   string
	cancelHold   string
	cancelErr    error
	labels       []domain.Label
	labelsErr    error
	createLErr   error
	updateLErr   error
	deleteLErr   error
	applyLErr    error
	labelMsgs    []domain.Message
	labelMsgsErr error
	aliases      []domain.Alias
	aliasesErr   error
	createdAlias domain.Alias
	createAErr   error
	deleteAErr   error
	aliasOwned   bool
	aliasOwnErr  error
	vacation     domain.Vacation
	getVacErr    error
	setVacErr    error
	signature    string
	getSigErr    error
	setSigErr    error
	// Hilo completo (cross-carpeta).
	threadMsgs   []domain.Message
	threadErr    error
	lastThreadID uuid.UUID

	// Lote + delta incremental.
	batchErr       error
	changedRet     []domain.Message
	destroyedRet   []uuid.UUID
	changesRetTS   time.Time
	changesRetID   uuid.UUID
	changesHasMore bool
	changesErr     error

	// Captura de argumentos para asserts.
	lastFlag       string
	lastFlagAdd    bool
	lastSearch     store.SearchFilter
	lastMoveTo     uuid.UUID
	lastInsert     *domain.Message
	lastRoleWant   domain.SpecialUse
	lastBefore     time.Time
	lastBeforeID   uuid.UUID
	lastFilter     domain.Filter
	lastFilterID   uuid.UUID
	lastRelease    time.Time
	lastCancelID   uuid.UUID
	lastLabelID    uuid.UUID
	lastLabelName  string
	lastLabelColor string
	lastApplyIDs   []uuid.UUID
	lastApplyAdd   bool
	lastAliasAddr  string
	lastAliasID    uuid.UUID
	lastVacation   domain.Vacation
	lastSignature  string
	lastBatchIDs   []uuid.UUID
	lastBatchDest  uuid.UUID
	lastBatchFlag  string
	lastBatchAdd   bool
	lastSince      time.Time
	lastSinceID    uuid.UUID

	// Buzones compartidos + delegación.
	accessible      []domain.SharedMailbox
	accessibleErr   error
	granteePerm     domain.Permission
	granteeErr      error
	grantedIdent    domain.GrantedIdentity
	grantedErr      error
	mbInAccount     bool
	mbInAccErr      error
	sharedAcc       domain.Account
	ensureShErr     error
	grants          []domain.MailboxGrant
	grantsErr       error
	grantErr        error
	revokeErr       error
	lastGranteeAcct uuid.UUID
	lastGrantedAddr string
	lastSharedAddr  string
	lastOwnerSubj   string
	lastGrantsAcct  uuid.UUID
	lastGrantAcct   uuid.UUID
	lastGrantee     string
	lastGrantPerm   domain.Permission
	lastRevokeAcct  uuid.UUID
	lastRevokee     string

	// Mudanza de la dirección principal (la organización conecta su dominio).
	mudadaA  string
	mudarErr error
}

func (f *fakeStore) EnsureAccountConCuota(ctx context.Context, s, a, d string, q int64, _ bool) (domain.Account, error) {
	return f.EnsureAccount(ctx, s, a, d, q)
}

func (f *fakeStore) EnsureAccount(context.Context, string, string, string, int64) (domain.Account, error) {
	return f.acc, f.accErr
}

func (f *fakeStore) MoveAccountAddress(_ context.Context, acc domain.Account, nueva string) (domain.Account, error) {
	f.mudadaA = nueva
	acc.Address = nueva
	return acc, f.mudarErr
}

func (f *fakeStore) AccountBySubject(context.Context, string) (domain.Account, error) {
	return f.bySubject, f.bySubErr
}

func (f *fakeStore) ListMailboxes(context.Context, uuid.UUID) ([]domain.Mailbox, error) {
	return f.mailboxes, f.mailboxErr
}

func (f *fakeStore) CreateMailbox(context.Context, uuid.UUID, *uuid.UUID, string) (domain.Mailbox, error) {
	return f.createdMB, f.createErr
}

func (f *fakeStore) RenameMailbox(context.Context, uuid.UUID, uuid.UUID, string) error {
	return f.renameErr
}
func (f *fakeStore) DeleteMailbox(context.Context, uuid.UUID, uuid.UUID) error { return f.delMBErr }
func (f *fakeStore) ListMessages(_ context.Context, _ uuid.UUID, _ int, before time.Time, beforeID uuid.UUID) ([]domain.Message, error) {
	f.lastBefore, f.lastBeforeID = before, beforeID
	return f.messages, f.msgErr
}

func (f *fakeStore) ListThreadMessages(_ context.Context, _, threadID uuid.UUID, _ int) ([]domain.Message, error) {
	f.lastThreadID = threadID
	return f.threadMsgs, f.threadErr
}

func (f *fakeStore) GetMessage(context.Context, uuid.UUID) (domain.Message, error) {
	return f.message, f.getMsgErr
}

func (f *fakeStore) GetMessages(context.Context, []uuid.UUID) ([]domain.Message, error) {
	return f.getMsgsRet, f.getMsgsErr
}

func (f *fakeStore) SearchMessages(_ context.Context, _ uuid.UUID, filter store.SearchFilter) ([]domain.Message, error) {
	f.lastSearch = filter
	return f.searchRet, f.searchErr
}

func (f *fakeStore) SetFlags(_ context.Context, _, _ uuid.UUID, flag string, add bool) error {
	f.lastFlag, f.lastFlagAdd = flag, add
	return f.setFlagsErr
}

func (f *fakeStore) MoveMessage(_ context.Context, _, _, dest uuid.UUID) error {
	f.lastMoveTo = dest
	return f.moveErr
}

func (f *fakeStore) DeleteMessage(context.Context, uuid.UUID, uuid.UUID) error { return f.delMsgErr }

func (f *fakeStore) SetFlagsBatch(_ context.Context, _ uuid.UUID, ids []uuid.UUID, flag string, add bool) (int, error) {
	f.lastBatchIDs, f.lastBatchFlag, f.lastBatchAdd = ids, flag, add
	if f.batchErr != nil {
		return 0, f.batchErr
	}
	return len(ids), nil
}

func (f *fakeStore) MoveMessages(_ context.Context, _ uuid.UUID, ids []uuid.UUID, dest uuid.UUID) (int, error) {
	f.lastBatchIDs, f.lastBatchDest = ids, dest
	if f.batchErr != nil {
		return 0, f.batchErr
	}
	return len(ids), nil
}

//nolint:gocritic // la firma la impone la interfaz del store (contrato JMAP Email/changes).
func (f *fakeStore) Changes(_ context.Context, _ uuid.UUID, since time.Time, sinceID uuid.UUID, _ int) ([]domain.Message, []uuid.UUID, time.Time, uuid.UUID, bool, error) {
	f.lastSince, f.lastSinceID = since, sinceID
	if f.changesErr != nil {
		return nil, nil, time.Time{}, uuid.Nil, false, f.changesErr
	}
	return f.changedRet, f.destroyedRet, f.changesRetTS, f.changesRetID, f.changesHasMore, nil
}

func (f *fakeStore) MailboxByRole(_ context.Context, _ uuid.UUID, role domain.SpecialUse) (domain.Mailbox, error) {
	f.lastRoleWant = role
	return f.byRole, f.byRoleErr
}

// MailboxByID sirve al import: devuelve el buzón pedido dentro de la cuenta.
func (f *fakeStore) MailboxByID(_ context.Context, _, id uuid.UUID) (domain.Mailbox, error) {
	if f.byIDErr != nil {
		return domain.Mailbox{}, f.byIDErr
	}
	mb := f.byRole
	mb.ID = id
	return mb, nil
}

func (f *fakeStore) ListFilters(context.Context, uuid.UUID) ([]domain.Filter, error) {
	return f.filters, f.filtersErr
}

func (f *fakeStore) CreateFilter(_ context.Context, _ uuid.UUID, flt domain.Filter) (domain.Filter, error) {
	f.lastFilter = flt
	if f.createFErr != nil {
		return domain.Filter{}, f.createFErr
	}
	if f.createdFltr.ID == uuid.Nil {
		f.createdFltr = flt
		f.createdFltr.ID = uuid.New()
	}
	return f.createdFltr, nil
}

func (f *fakeStore) UpdateFilter(_ context.Context, _, id uuid.UUID, flt domain.Filter) error {
	f.lastFilter, f.lastFilterID = flt, id
	return f.updateFErr
}

func (f *fakeStore) DeleteFilter(_ context.Context, _, id uuid.UUID) error {
	f.lastFilterID = id
	return f.deleteFErr
}

func (f *fakeStore) ReservarBlobCompartido(context.Context, string, string, int64) (bool, bool, error) {
	return false, false, nil
}

func (f *fakeStore) MarcarBlobListo(context.Context, string) error { return nil }

func (f *fakeStore) LiberarBlobCompartido(context.Context, string) error { return nil }

func (f *fakeStore) InsertMessage(_ context.Context, m *domain.Message) error {
	if f.insertErr != nil {
		return f.insertErr
	}
	if m.ID == uuid.Nil {
		m.ID = uuid.New() // simula el id asignado por Postgres al insertar
	}
	f.lastInsert = m
	return nil
}

func (f *fakeStore) ListScheduled(_ context.Context, _ uuid.UUID, _ int, before time.Time, beforeID uuid.UUID) ([]domain.OutboundMessage, error) {
	f.lastBefore, f.lastBeforeID = before, beforeID
	return f.scheduled, f.schedErr
}

func (f *fakeStore) RescheduleHeld(_ context.Context, id, _ uuid.UUID, releaseAt time.Time) error {
	f.lastCancelID, f.lastRelease = id, releaseAt
	return f.reschedErr
}

func (f *fakeStore) CancelHeld(_ context.Context, id, _ uuid.UUID) (string, string, error) {
	f.lastCancelID = id
	return f.cancelBlob, f.cancelHold, f.cancelErr
}

func (f *fakeStore) ListLabels(context.Context, uuid.UUID) ([]domain.Label, error) {
	return f.labels, f.labelsErr
}

func (f *fakeStore) CreateLabel(_ context.Context, _ uuid.UUID, name, color string) (domain.Label, error) {
	f.lastLabelName, f.lastLabelColor = name, color
	if f.createLErr != nil {
		return domain.Label{}, f.createLErr
	}
	return domain.Label{ID: uuid.New(), Name: name, Color: color}, nil
}

func (f *fakeStore) UpdateLabel(_ context.Context, _, id uuid.UUID, name, color string) error {
	f.lastLabelID, f.lastLabelName, f.lastLabelColor = id, name, color
	return f.updateLErr
}

func (f *fakeStore) DeleteLabel(_ context.Context, _, id uuid.UUID) error {
	f.lastLabelID = id
	return f.deleteLErr
}

func (f *fakeStore) ApplyLabel(_ context.Context, _, id uuid.UUID, messageIDs []uuid.UUID, add bool) error {
	f.lastLabelID, f.lastApplyIDs, f.lastApplyAdd = id, messageIDs, add
	return f.applyLErr
}

func (f *fakeStore) ListMessagesByLabel(_ context.Context, _, id uuid.UUID, _ int, before time.Time, beforeID uuid.UUID) ([]domain.Message, error) {
	f.lastLabelID, f.lastBefore, f.lastBeforeID = id, before, beforeID
	return f.labelMsgs, f.labelMsgsErr
}

func (f *fakeStore) ListAliases(context.Context, uuid.UUID) ([]domain.Alias, error) {
	return f.aliases, f.aliasesErr
}

func (f *fakeStore) CreateAlias(_ context.Context, _ uuid.UUID, address string) (domain.Alias, error) {
	f.lastAliasAddr = address
	if f.createAErr != nil {
		return domain.Alias{}, f.createAErr
	}
	if f.createdAlias.ID == uuid.Nil {
		f.createdAlias = domain.Alias{ID: uuid.New(), Address: address}
	}
	return f.createdAlias, nil
}

func (f *fakeStore) DeleteAlias(_ context.Context, _, id uuid.UUID) error {
	f.lastAliasID = id
	return f.deleteAErr
}

func (f *fakeStore) AliasBelongsTo(_ context.Context, _ uuid.UUID, address string) (bool, error) {
	f.lastAliasAddr = address
	return f.aliasOwned, f.aliasOwnErr
}

func (f *fakeStore) GetVacation(context.Context, uuid.UUID) (domain.Vacation, error) {
	return f.vacation, f.getVacErr
}

func (f *fakeStore) SetVacation(_ context.Context, _ uuid.UUID, v domain.Vacation) error {
	f.lastVacation = v
	return f.setVacErr
}

func (f *fakeStore) GetSignature(context.Context, uuid.UUID) (string, error) {
	return f.signature, f.getSigErr
}

func (f *fakeStore) SetSignature(_ context.Context, _ uuid.UUID, sig string) error {
	f.lastSignature = sig
	return f.setSigErr
}

func (f *fakeStore) ListAccessibleMailboxes(context.Context, string) ([]domain.SharedMailbox, error) {
	return f.accessible, f.accessibleErr
}

func (f *fakeStore) GranteePermission(_ context.Context, _ string, accountID uuid.UUID) (domain.Permission, error) {
	f.lastGranteeAcct = accountID
	return f.granteePerm, f.granteeErr
}

func (f *fakeStore) GrantedIdentityByAddress(_ context.Context, _, address string) (domain.GrantedIdentity, error) {
	f.lastGrantedAddr = address
	return f.grantedIdent, f.grantedErr
}

func (f *fakeStore) MailboxInAccount(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return f.mbInAccount, f.mbInAccErr
}

func (f *fakeStore) ContarBuzonesCompartidos(context.Context) (int, error) {
	return f.compartidos, nil
}

func (f *fakeStore) EnsureSharedMailbox(_ context.Context, address, displayName string, _ int64, ownerSubject string) (domain.Account, error) {
	f.lastSharedAddr, f.lastOwnerSubj = address, ownerSubject
	if f.ensureShErr != nil {
		return domain.Account{}, f.ensureShErr
	}
	if f.sharedAcc.ID == uuid.Nil {
		f.sharedAcc = domain.Account{ID: uuid.New(), Address: address, DisplayName: displayName}
	}
	return f.sharedAcc, nil
}

func (f *fakeStore) ListGrants(_ context.Context, accountID uuid.UUID) ([]domain.MailboxGrant, error) {
	f.lastGrantsAcct = accountID
	return f.grants, f.grantsErr
}

func (f *fakeStore) GrantAccess(_ context.Context, accountID uuid.UUID, grantee string, perm domain.Permission) error {
	f.lastGrantAcct, f.lastGrantee, f.lastGrantPerm = accountID, grantee, perm
	return f.grantErr
}

func (f *fakeStore) RevokeAccess(_ context.Context, accountID uuid.UUID, grantee string) error {
	f.lastRevokeAcct, f.lastRevokee = accountID, grantee
	return f.revokeErr
}

func (f *fakeStore) Dominios(context.Context) ([]string, error) { return f.dominios, f.dominiosErr }

var _ dataStore = (*fakeStore)(nil)

type fakeBlobs struct {
	putErr     error
	getBody    string
	getErr     error
	putKey     string
	deletedKey string
}

// Delete registra el borrado: la importación limpia el blob temporal tras
// copiarlo a su clave definitiva.
func (b *fakeBlobs) Delete(_ context.Context, _ tenancy.Tenant, key string) error {
	b.deletedKey = key
	return nil
}

func (b *fakeBlobs) Put(_ context.Context, _ tenancy.Tenant, key string, r io.Reader, _ int64, _ objectstore.PutOptions) error {
	b.putKey = key
	_, _ = io.Copy(io.Discard, r)
	return b.putErr
}

func (b *fakeBlobs) Get(context.Context, tenancy.Tenant, string) (io.ReadCloser, error) {
	if b.getErr != nil {
		return nil, b.getErr
	}
	return io.NopCloser(strings.NewReader(b.getBody)), nil
}

var _ blobStore = (*fakeBlobs)(nil)

type fakeOutbound struct {
	err         error
	lastFrom    string
	lastRcpts   []string
	lastRawLen  int
	lastRaw     []byte
	scheduledID uuid.UUID
	schedErr    error
	lastRelease time.Time
	lastHold    string
}

func (o *fakeOutbound) SubmitRaw(_ context.Context, from string, rcpts []string, raw []byte) error {
	o.lastFrom, o.lastRcpts, o.lastRawLen, o.lastRaw = from, rcpts, len(raw), raw
	return o.err
}

func (o *fakeOutbound) Schedule(_ context.Context, from string, rcpts []string, raw []byte, releaseAt time.Time, hold string) (uuid.UUID, error) {
	o.lastFrom, o.lastRcpts, o.lastRawLen, o.lastRaw = from, rcpts, len(raw), raw
	o.lastRelease, o.lastHold = releaseAt, hold
	return o.scheduledID, o.schedErr
}

var _ outboundSubmitter = (*fakeOutbound)(nil)

// --- helpers -------------------------------------------------------------

func newAPI(st dataStore, blobs blobStore, out outboundSubmitter) *API {
	return &API{
		cfg:      config.Config{PublicURL: "https://mail.sytue.eu", MaxMessageBytes: 1024},
		store:    st,
		blobs:    blobs,
		outbound: out,
		logger:   slog.Default(),
	}
}

// authCtx inyecta claims (y opcionalmente tenant) como lo haría RequireAuth.
func authCtx(ctx context.Context, withTenant bool) context.Context {
	ctx = authn.Into(ctx, authn.Claims{Subject: "user-1", Email: "ada@sytue.eu", Name: "Ada", Tenant: "acme"})
	if withTenant {
		ctx = tenancy.Into(ctx, tenancy.Tenant{Slug: "acme"})
	}
	return ctx
}

// req construye una petición con claims/tenant y parámetros de ruta chi.
func req(method, target, body string, withTenant bool, params map[string]string) *http.Request {
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
	} else {
		r = httptest.NewRequest(method, target, http.NoBody)
	}
	r = r.WithContext(authCtx(r.Context(), withTenant))
	if len(params) > 0 {
		rctx := chi.NewRouteContext()
		for k, v := range params {
			rctx.URLParams.Add(k, v)
		}
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	}
	return r
}

// noAuthReq construye una petición sin claims en el contexto.
func noAuthReq(method, target, body string) *http.Request {
	if body != "" {
		return httptest.NewRequest(method, target, strings.NewReader(body))
	}
	return httptest.NewRequest(method, target, http.NoBody)
}

func decodeEnvelopeCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("cuerpo no es sobre de error JSON: %v (%s)", err, rec.Body.String())
	}
	return env.Error.Code
}

func assertJSONContentType(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q, quiero application/json", ct)
	}
}

// asMap asevera (comprobado) que v es un map[string]any de resultado JMAP.
func asMap(t *testing.T, v any) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("resultado no es map[string]any: %T", v)
	}
	return m
}

// decodeBody decodifica el cuerpo JSON del recorder a map[string]any.
func decodeBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("cuerpo no es JSON objeto: %v (%s)", err, rec.Body.String())
	}
	return m
}

// mapLen devuelve el número de claves de m[key] (un map anidado, comprobado).
func mapLen(t *testing.T, m map[string]any, key string) int {
	t.Helper()
	sub, ok := m[key].(map[string]any)
	if !ok {
		t.Fatalf("%q no es un objeto: %T", key, m[key])
	}
	return len(sub)
}

// strSliceLen devuelve la longitud de m[key] (un []string, comprobado).
func strSliceLen(t *testing.T, m map[string]any, key string) int {
	t.Helper()
	s, ok := m[key].([]string)
	if !ok {
		t.Fatalf("%q no es []string: %T", key, m[key])
	}
	return len(s)
}

// --- listMailboxes -------------------------------------------------------

func TestListMailboxes(t *testing.T) {
	st := &fakeStore{
		acc:       domain.Account{ID: uuid.New()},
		mailboxes: []domain.Mailbox{{ID: uuid.New(), Name: "Inbox"}},
	}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.listMailboxes(rec, req(http.MethodGet, "/api/v1/mailboxes", "", true, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d", rec.Code)
	}
	assertJSONContentType(t, rec)
	var got struct {
		Mailboxes []domain.Mailbox `json:"mailboxes"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Mailboxes) != 1 || got.Mailboxes[0].Name != "Inbox" {
		t.Fatalf("mailboxes = %+v", got.Mailboxes)
	}
}

func TestListMailboxesNoAuth(t *testing.T) {
	a := newAPI(&fakeStore{}, nil, nil)
	rec := httptest.NewRecorder()
	a.listMailboxes(rec, noAuthReq(http.MethodGet, "/api/v1/mailboxes", ""))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("sin auth => %d, quiero 401", rec.Code)
	}
	if code := decodeEnvelopeCode(t, rec); code != "unauthorized" {
		t.Fatalf("code = %q", code)
	}
}

func TestListMailboxesStoreError(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}, mailboxErr: domain.ErrNotFound}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.listMailboxes(rec, req(http.MethodGet, "/api/v1/mailboxes", "", true, nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("código = %d, quiero 404", rec.Code)
	}
}

func TestListMailboxesEnsureAccountError(t *testing.T) {
	st := &fakeStore{accErr: domain.ErrQuotaExceeded}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.listMailboxes(rec, req(http.MethodGet, "/api/v1/mailboxes", "", true, nil))
	if rec.Code == http.StatusOK {
		t.Fatalf("error de EnsureAccount debe fallar, got %d", rec.Code)
	}
}

// --- createMailbox -------------------------------------------------------

func TestCreateMailboxOK(t *testing.T) {
	st := &fakeStore{
		acc:       domain.Account{ID: uuid.New()},
		createdMB: domain.Mailbox{ID: uuid.New(), Name: "Facturas"},
	}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.createMailbox(rec, req(http.MethodPost, "/api/v1/mailboxes", `{"name":"Facturas"}`, true, nil))
	if rec.Code != http.StatusCreated {
		t.Fatalf("código = %d, quiero 201", rec.Code)
	}
}

func TestCreateMailboxBadJSON(t *testing.T) {
	a := newAPI(&fakeStore{acc: domain.Account{ID: uuid.New()}}, nil, nil)
	rec := httptest.NewRecorder()
	a.createMailbox(rec, req(http.MethodPost, "/api/v1/mailboxes", `{not json`, true, nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("código = %d, quiero 400", rec.Code)
	}
}

func TestCreateMailboxBadParent(t *testing.T) {
	a := newAPI(&fakeStore{acc: domain.Account{ID: uuid.New()}}, nil, nil)
	rec := httptest.NewRecorder()
	a.createMailbox(rec, req(http.MethodPost, "/api/v1/mailboxes", `{"name":"x","parent_id":"nope"}`, true, nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("parent_id inválido => %d, quiero 400", rec.Code)
	}
}

func TestCreateMailboxConflict(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}, createErr: domain.ErrMailboxExists}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.createMailbox(rec, req(http.MethodPost, "/api/v1/mailboxes", `{"name":"Inbox"}`, true, nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("buzón existente => %d, quiero 409", rec.Code)
	}
}

func TestCreateMailboxWithValidParent(t *testing.T) {
	parent := uuid.New()
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}, createdMB: domain.Mailbox{ID: uuid.New()}}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.createMailbox(rec, req(http.MethodPost, "/api/v1/mailboxes", `{"name":"Sub","parent_id":"`+parent.String()+`"}`, true, nil))
	if rec.Code != http.StatusCreated {
		t.Fatalf("código = %d, quiero 201", rec.Code)
	}
}

// --- listMessages --------------------------------------------------------

func TestListMessagesOK(t *testing.T) {
	mbID := uuid.New()
	st := &fakeStore{mbInAccount: true, messages: []domain.Message{{ID: uuid.New(), Subject: "hola"}}}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.listMessages(rec, req(http.MethodGet, "/x?limit=10", "", true, map[string]string{"mailboxID": mbID.String()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d", rec.Code)
	}
	var got struct {
		Messages   []domain.Message `json:"messages"`
		Limit      int              `json:"limit"`
		NextCursor string           `json:"next_cursor"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Limit != 10 || len(got.Messages) != 1 {
		t.Fatalf("limit/messages = %d/%d", got.Limit, len(got.Messages))
	}
	// Página incompleta (1 < limit 10) => no hay cursor de siguiente página.
	if got.NextCursor != "" {
		t.Fatalf("next_cursor = %q, quiero vacío", got.NextCursor)
	}
	// Sin cursor => primera página (keyset en cero).
	if !st.lastBefore.IsZero() || st.lastBeforeID != uuid.Nil {
		t.Fatalf("cursor propagado = %v/%v, quiero cero", st.lastBefore, st.lastBeforeID)
	}
}

func TestListMessagesKeysetCursor(t *testing.T) {
	mbID := uuid.New()
	last := domain.Message{ID: uuid.New(), ReceivedAt: time.Date(2026, 7, 8, 10, 0, 0, 123456000, time.UTC)}
	st := &fakeStore{mbInAccount: true, messages: []domain.Message{{ID: uuid.New()}, last}} // página llena (limit=2)
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.listMessages(rec, req(http.MethodGet, "/x?limit=2", "", true, map[string]string{"mailboxID": mbID.String()}))
	var got struct {
		NextCursor string `json:"next_cursor"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.NextCursor == "" {
		t.Fatal("página llena debe traer next_cursor")
	}

	// El next_cursor devuelto hace round-trip: la siguiente petición pagina
	// desde el (received_at, id) del último mensaje.
	rec = httptest.NewRecorder()
	a.listMessages(rec, req(http.MethodGet, "/x?limit=2&cursor="+got.NextCursor, "", true, map[string]string{"mailboxID": mbID.String()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("código con cursor = %d", rec.Code)
	}
	if !st.lastBefore.Equal(last.ReceivedAt) || st.lastBeforeID != last.ID {
		t.Fatalf("cursor propagado = %v/%v, quiero %v/%v", st.lastBefore, st.lastBeforeID, last.ReceivedAt, last.ID)
	}
}

func TestListMessagesBadCursor(t *testing.T) {
	mbID := uuid.New()
	a := newAPI(&fakeStore{mbInAccount: true}, nil, nil)
	for _, c := range []string{"garbage", "2026-07-08T10:00:00Z", "2026-07-08T10:00:00Z,not-a-uuid", "no-fecha," + uuid.NewString()} {
		rec := httptest.NewRecorder()
		a.listMessages(rec, req(http.MethodGet, "/x?cursor="+c, "", true, map[string]string{"mailboxID": mbID.String()}))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("cursor %q => %d, quiero 400", c, rec.Code)
		}
	}
}

func TestListMessagesBadMailboxID(t *testing.T) {
	a := newAPI(&fakeStore{}, nil, nil)
	rec := httptest.NewRecorder()
	a.listMessages(rec, req(http.MethodGet, "/x", "", true, map[string]string{"mailboxID": "nope"}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("mailboxID inválido => %d, quiero 400", rec.Code)
	}
}

// --- getMessage ----------------------------------------------------------

func TestGetMessageOK(t *testing.T) {
	id := uuid.New()
	st := &fakeStore{message: domain.Message{ID: id, Subject: "asunto"}}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.getMessage(rec, req(http.MethodGet, "/x", "", true, map[string]string{"messageID": id.String()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d", rec.Code)
	}
}

func TestGetMessageNotFound(t *testing.T) {
	id := uuid.New()
	st := &fakeStore{getMsgErr: domain.ErrNotFound}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.getMessage(rec, req(http.MethodGet, "/x", "", true, map[string]string{"messageID": id.String()}))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("código = %d, quiero 404", rec.Code)
	}
}

func TestGetMessageBadID(t *testing.T) {
	a := newAPI(&fakeStore{}, nil, nil)
	rec := httptest.NewRecorder()
	a.getMessage(rec, req(http.MethodGet, "/x", "", true, map[string]string{"messageID": "bad"}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("código = %d, quiero 400", rec.Code)
	}
}

// --- getMessageRaw -------------------------------------------------------

func TestGetMessageRawOK(t *testing.T) {
	id := uuid.New()
	st := &fakeStore{message: domain.Message{ID: id, BlobKey: "messages/x/y"}}
	blobs := &fakeBlobs{getBody: "From: a\r\n\r\nhola"}
	a := newAPI(st, blobs, nil)
	rec := httptest.NewRecorder()
	a.getMessageRaw(rec, req(http.MethodGet, "/x", "", true, map[string]string{"messageID": id.String()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "message/rfc822" {
		t.Fatalf("Content-Type = %q", ct)
	}
	if !strings.Contains(rec.Body.String(), "hola") {
		t.Fatalf("cuerpo = %q", rec.Body.String())
	}
}

func TestGetMessageRawNoTenant(t *testing.T) {
	id := uuid.New()
	st := &fakeStore{message: domain.Message{ID: id}}
	a := newAPI(st, &fakeBlobs{}, nil)
	rec := httptest.NewRecorder()
	// withTenant=false: hay claims pero no tenant en el contexto.
	a.getMessageRaw(rec, req(http.MethodGet, "/x", "", false, map[string]string{"messageID": id.String()}))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("sin tenant => %d, quiero 401", rec.Code)
	}
}

func TestGetMessageRawMessageNotFound(t *testing.T) {
	id := uuid.New()
	st := &fakeStore{getMsgErr: domain.ErrNotFound}
	a := newAPI(st, &fakeBlobs{}, nil)
	rec := httptest.NewRecorder()
	a.getMessageRaw(rec, req(http.MethodGet, "/x", "", true, map[string]string{"messageID": id.String()}))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("código = %d, quiero 404", rec.Code)
	}
}

func TestGetMessageRawBlobError(t *testing.T) {
	id := uuid.New()
	st := &fakeStore{message: domain.Message{ID: id, BlobKey: "k"}}
	a := newAPI(st, &fakeBlobs{getErr: objectstore.ErrNotFound}, nil)
	rec := httptest.NewRecorder()
	a.getMessageRaw(rec, req(http.MethodGet, "/x", "", true, map[string]string{"messageID": id.String()}))
	if rec.Code == http.StatusOK {
		t.Fatalf("blob ausente no debe dar 200")
	}
}

// --- setFlags ------------------------------------------------------------

func TestSetFlagsOK(t *testing.T) {
	id := uuid.New()
	st := &fakeStore{}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.setFlags(rec, req(http.MethodPost, "/x", `{"flag":"\\Seen","add":true}`, true, map[string]string{"messageID": id.String()}))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("código = %d, quiero 204", rec.Code)
	}
	if st.lastFlag != "\\Seen" || !st.lastFlagAdd {
		t.Fatalf("flag capturado = %q add=%v", st.lastFlag, st.lastFlagAdd)
	}
}

func TestSetFlagsMissingFlag(t *testing.T) {
	id := uuid.New()
	a := newAPI(&fakeStore{}, nil, nil)
	rec := httptest.NewRecorder()
	a.setFlags(rec, req(http.MethodPost, "/x", `{"add":true}`, true, map[string]string{"messageID": id.String()}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("flag ausente => %d, quiero 400", rec.Code)
	}
}

func TestSetFlagsBadID(t *testing.T) {
	a := newAPI(&fakeStore{}, nil, nil)
	rec := httptest.NewRecorder()
	a.setFlags(rec, req(http.MethodPost, "/x", `{"flag":"x"}`, true, map[string]string{"messageID": "bad"}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("código = %d, quiero 400", rec.Code)
	}
}

func TestSetFlagsBadJSON(t *testing.T) {
	id := uuid.New()
	a := newAPI(&fakeStore{}, nil, nil)
	rec := httptest.NewRecorder()
	a.setFlags(rec, req(http.MethodPost, "/x", `{bad`, true, map[string]string{"messageID": id.String()}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("código = %d, quiero 400", rec.Code)
	}
}

func TestSetFlagsStoreNotFound(t *testing.T) {
	id := uuid.New()
	st := &fakeStore{setFlagsErr: domain.ErrNotFound}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.setFlags(rec, req(http.MethodPost, "/x", `{"flag":"\\Seen","add":false}`, true, map[string]string{"messageID": id.String()}))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("código = %d, quiero 404", rec.Code)
	}
}

// --- moveMessage ---------------------------------------------------------

func TestMoveMessageOK(t *testing.T) {
	id := uuid.New()
	dest := uuid.New()
	st := &fakeStore{}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.moveMessage(rec, req(http.MethodPost, "/x", `{"mailbox_id":"`+dest.String()+`"}`, true, map[string]string{"messageID": id.String()}))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("código = %d, quiero 204", rec.Code)
	}
	if st.lastMoveTo != dest {
		t.Fatalf("destino = %v, quiero %v", st.lastMoveTo, dest)
	}
}

func TestMoveMessageBadDest(t *testing.T) {
	id := uuid.New()
	a := newAPI(&fakeStore{}, nil, nil)
	rec := httptest.NewRecorder()
	a.moveMessage(rec, req(http.MethodPost, "/x", `{"mailbox_id":"nope"}`, true, map[string]string{"messageID": id.String()}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("mailbox_id inválido => %d, quiero 400", rec.Code)
	}
}

func TestMoveMessageBadID(t *testing.T) {
	a := newAPI(&fakeStore{}, nil, nil)
	rec := httptest.NewRecorder()
	a.moveMessage(rec, req(http.MethodPost, "/x", `{"mailbox_id":"`+uuid.NewString()+`"}`, true, map[string]string{"messageID": "bad"}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("código = %d, quiero 400", rec.Code)
	}
}

// --- deleteMessage -------------------------------------------------------

func TestDeleteMessageOK(t *testing.T) {
	id := uuid.New()
	a := newAPI(&fakeStore{}, nil, nil)
	rec := httptest.NewRecorder()
	a.deleteMessage(rec, req(http.MethodDelete, "/x", "", true, map[string]string{"messageID": id.String()}))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("código = %d, quiero 204", rec.Code)
	}
}

func TestDeleteMessageBadID(t *testing.T) {
	a := newAPI(&fakeStore{}, nil, nil)
	rec := httptest.NewRecorder()
	a.deleteMessage(rec, req(http.MethodDelete, "/x", "", true, map[string]string{"messageID": "bad"}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("código = %d, quiero 400", rec.Code)
	}
}

func TestDeleteMessageNotFound(t *testing.T) {
	id := uuid.New()
	a := newAPI(&fakeStore{delMsgErr: domain.ErrNotFound}, nil, nil)
	rec := httptest.NewRecorder()
	a.deleteMessage(rec, req(http.MethodDelete, "/x", "", true, map[string]string{"messageID": id.String()}))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("código = %d, quiero 404", rec.Code)
	}
}

// --- sendMessage ---------------------------------------------------------

func TestSendMessageOK(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New(), Address: "ada@sytue.eu"}}
	out := &fakeOutbound{}
	a := newAPI(st, nil, out)
	rec := httptest.NewRecorder()
	body := `{"recipients":["bob@x.com"],"raw":"From: ada\r\n\r\nhola"}`
	a.sendMessage(rec, req(http.MethodPost, "/api/v1/send", body, true, nil))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("código = %d, quiero 202", rec.Code)
	}
	// From vacío usa la dirección de la cuenta.
	if out.lastFrom != "ada@sytue.eu" {
		t.Fatalf("from = %q, quiero la dirección de la cuenta", out.lastFrom)
	}
	if len(out.lastRcpts) != 1 || out.lastRawLen == 0 {
		t.Fatalf("recipients/raw no propagados: %+v", out)
	}
}

func TestSendMessageExplicitFrom(t *testing.T) {
	// Enviar-como un alias VERIFICADO del usuario: el store confirma la propiedad.
	st := &fakeStore{acc: domain.Account{ID: uuid.New(), Address: "ada@sytue.eu"}, aliasOwned: true}
	out := &fakeOutbound{}
	a := newAPI(st, nil, out)
	rec := httptest.NewRecorder()
	body := `{"from":"alias@sytue.eu","recipients":["bob@x.com"],"raw":"x"}`
	a.sendMessage(rec, req(http.MethodPost, "/api/v1/send", body, true, nil))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("código = %d, quiero 202 (%s)", rec.Code, rec.Body.String())
	}
	if out.lastFrom != "alias@sytue.eu" {
		t.Fatalf("from = %q, quiero el explícito", out.lastFrom)
	}
	if st.lastAliasAddr != "alias@sytue.eu" {
		t.Fatalf("no se validó la propiedad del alias: %q", st.lastAliasAddr)
	}
}

// Enviar-como una dirección que NO es del usuario (ni primaria ni alias propio):
// prohibido (403), sin encolar nada. Cierra la suplantación de remitente.
func TestSendMessageForeignFromForbidden(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New(), Address: "ada@sytue.eu"}, aliasOwned: false}
	out := &fakeOutbound{}
	a := newAPI(st, nil, out)
	rec := httptest.NewRecorder()
	body := `{"from":"victima@otro.eu","recipients":["bob@x.com"],"raw":"x"}`
	a.sendMessage(rec, req(http.MethodPost, "/api/v1/send", body, true, nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("from ajeno => %d, quiero 403", rec.Code)
	}
	if out.lastFrom != "" {
		t.Fatalf("no debe encolar con from ajeno, encoló %q", out.lastFrom)
	}
}

// --- Alias (listAliases / createAlias / deleteAlias) ---------------------

func TestListAliasesOK(t *testing.T) {
	st := &fakeStore{
		acc:     domain.Account{ID: uuid.New(), Address: "ada@sytue.eu"},
		aliases: []domain.Alias{{ID: uuid.New(), Address: "ventas@sytue.eu"}},
	}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.listAliases(rec, req(http.MethodGet, "/api/v1/aliases", "", true, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d", rec.Code)
	}
	m := decodeBody(t, rec)
	if m["primary"] != "ada@sytue.eu" {
		t.Fatalf("primary = %v, quiero la dirección de la cuenta", m["primary"])
	}
	items, ok := m["aliases"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("aliases = %+v", m["aliases"])
	}
}

func TestCreateAliasOK(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New(), Address: "ada@sytue.eu"}}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.createAlias(rec, req(http.MethodPost, "/api/v1/aliases", `{"address":"ventas@sytue.eu"}`, true, nil))
	if rec.Code != http.StatusCreated {
		t.Fatalf("código = %d, quiero 201 (%s)", rec.Code, rec.Body.String())
	}
	if st.lastAliasAddr != "ventas@sytue.eu" {
		t.Fatalf("dirección no propagada: %q", st.lastAliasAddr)
	}
}

// Un alias en un dominio que NO servimos (ni el de la cuenta ni el principal) se
// rechaza en el borde (400), sin llegar al store.
func TestCreateAliasForeignDomain(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New(), Address: "ada@sytue.eu"}}
	a := newAPI(st, nil, nil)
	a.cfg.PrimaryDomain = "sytue.eu"
	rec := httptest.NewRecorder()
	a.createAlias(rec, req(http.MethodPost, "/api/v1/aliases", `{"address":"yo@gmail.com"}`, true, nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("dominio ajeno => %d, quiero 400", rec.Code)
	}
	if st.lastAliasAddr != "" {
		t.Fatalf("no debe llegar al store: %q", st.lastAliasAddr)
	}
}

func TestCreateAliasConflict(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New(), Address: "ada@sytue.eu"}, createAErr: domain.ErrAliasExists}
	a := newAPI(st, nil, nil)
	a.cfg.PrimaryDomain = "sytue.eu"
	rec := httptest.NewRecorder()
	a.createAlias(rec, req(http.MethodPost, "/api/v1/aliases", `{"address":"ada.dos@sytue.eu"}`, true, nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("dirección en uso => %d, quiero 409", rec.Code)
	}
}

func TestDeleteAliasOK(t *testing.T) {
	id := uuid.New()
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.deleteAlias(rec, req(http.MethodDelete, "/x", "", true, map[string]string{"aliasID": id.String()}))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("código = %d, quiero 204", rec.Code)
	}
	if st.lastAliasID != id {
		t.Fatalf("id borrado = %v, quiero %v", st.lastAliasID, id)
	}
}

func TestDeleteAliasNotFound(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}, deleteAErr: domain.ErrNotFound}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.deleteAlias(rec, req(http.MethodDelete, "/x", "", true, map[string]string{"aliasID": uuid.NewString()}))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("alias ausente => %d, quiero 404", rec.Code)
	}
}

// --- Vacaciones (getVacation / setVacation) ------------------------------

func TestGetVacationOK(t *testing.T) {
	st := &fakeStore{
		acc:      domain.Account{ID: uuid.New()},
		vacation: domain.Vacation{Enabled: true, Subject: "Fuera", Body: "Vuelvo el lunes"},
	}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.getVacation(rec, req(http.MethodGet, "/api/v1/vacation", "", true, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d", rec.Code)
	}
	m := decodeBody(t, rec)
	if m["enabled"] != true || m["subject"] != "Fuera" {
		t.Fatalf("respuesta = %+v", m)
	}
}

func TestSetVacationOK(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	body := `{"enabled":true,"subject":"OOO","body":"Fuera","starts_at":"2026-07-20T00:00:00Z","ends_at":"2026-07-27T00:00:00Z"}`
	a.setVacation(rec, req(http.MethodPut, "/api/v1/vacation", body, true, nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("código = %d, quiero 204 (%s)", rec.Code, rec.Body.String())
	}
	if !st.lastVacation.Enabled || st.lastVacation.Subject != "OOO" {
		t.Fatalf("config guardada = %+v", st.lastVacation)
	}
	if st.lastVacation.StartsAt == nil || st.lastVacation.EndsAt == nil {
		t.Fatalf("rango no parseado: %+v", st.lastVacation)
	}
}

func TestSetVacationBadDate(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.setVacation(rec, req(http.MethodPut, "/api/v1/vacation", `{"enabled":true,"starts_at":"ayer"}`, true, nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("fecha inválida => %d, quiero 400", rec.Code)
	}
}

// --- Cuota de almacenamiento (getQuota) ----------------------------------

func TestGetQuotaOK(t *testing.T) {
	// a.account resuelve la cuenta vía EnsureAccount (fake => f.acc): el handler
	// solo expone su used_bytes (contador agregado del store) y quota_bytes.
	st := &fakeStore{acc: domain.Account{ID: uuid.New(), UsedBytes: 2560, QuotaBytes: 10 << 30}}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.getQuota(rec, req(http.MethodGet, "/api/v1/quota", "", true, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	m := decodeBody(t, rec)
	// JSON desnumera a float64; comparamos como tal.
	if m["used_bytes"] != float64(2560) {
		t.Fatalf("used_bytes = %v, quiero 2560", m["used_bytes"])
	}
	if m["quota_bytes"] != float64(10<<30) {
		t.Fatalf("quota_bytes = %v, quiero %d", m["quota_bytes"], int64(10<<30))
	}
}

func TestGetQuotaNoAuth(t *testing.T) {
	// Sin claims en el contexto, a.account falla y el handler no filtra nada.
	a := newAPI(&fakeStore{}, nil, nil)
	rec := httptest.NewRecorder()
	a.getQuota(rec, noAuthReq(http.MethodGet, "/api/v1/quota", ""))
	if rec.Code == http.StatusOK {
		t.Fatalf("sin auth => %d, no debe ser 200", rec.Code)
	}
}

func TestSendMessageMissingFields(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
	a := newAPI(st, nil, &fakeOutbound{})
	rec := httptest.NewRecorder()
	a.sendMessage(rec, req(http.MethodPost, "/api/v1/send", `{"recipients":[]}`, true, nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("recipients/raw vacíos => %d, quiero 400", rec.Code)
	}
}

func TestSendMessageBadJSON(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
	a := newAPI(st, nil, &fakeOutbound{})
	rec := httptest.NewRecorder()
	a.sendMessage(rec, req(http.MethodPost, "/api/v1/send", `{bad`, true, nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("código = %d, quiero 400", rec.Code)
	}
}

func TestSendMessageNoAuth(t *testing.T) {
	a := newAPI(&fakeStore{}, nil, &fakeOutbound{})
	rec := httptest.NewRecorder()
	a.sendMessage(rec, noAuthReq(http.MethodPost, "/api/v1/send", `{"recipients":["x@y.z"],"raw":"x"}`))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("sin auth => %d, quiero 401", rec.Code)
	}
}

func TestSendMessageOutboundError(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New(), Address: "ada@sytue.eu"}}
	out := &fakeOutbound{err: domain.ErrQuotaExceeded}
	a := newAPI(st, nil, out)
	rec := httptest.NewRecorder()
	a.sendMessage(rec, req(http.MethodPost, "/api/v1/send", `{"recipients":["b@x.com"],"raw":"x"}`, true, nil))
	if rec.Code == http.StatusAccepted {
		t.Fatalf("error de encolado no debe dar 202")
	}
}

// --- Borradores (createDraft / updateDraft) ------------------------------

func TestCreateDraftOK(t *testing.T) {
	st := &fakeStore{
		acc:    domain.Account{ID: uuid.New(), Address: "ada@sytue.eu"},
		byRole: domain.Mailbox{ID: uuid.New(), Role: domain.RoleDrafts},
	}
	blobs := &fakeBlobs{}
	a := newAPI(st, blobs, nil)
	rec := httptest.NewRecorder()
	raw := "From: ada@sytue.eu\r\nTo: bob@x.com\r\nSubject: hola\r\n\r\ncuerpo"
	body := `{"recipients":["bob@x.com"],"raw":` + strconv.Quote(raw) + `}`
	a.createDraft(rec, req(http.MethodPost, "/api/v1/drafts", body, true, nil))

	if rec.Code != http.StatusCreated {
		t.Fatalf("código = %d, quiero 201 (%s)", rec.Code, rec.Body.String())
	}
	if st.lastRoleWant != domain.RoleDrafts {
		t.Fatalf("buzón destino = %q, quiero drafts", st.lastRoleWant)
	}
	if st.lastInsert == nil {
		t.Fatal("no se insertó el borrador")
	}
	if st.lastInsert.MailboxID != st.byRole.ID {
		t.Fatalf("insertado en %v, quiero la carpeta Drafts %v", st.lastInsert.MailboxID, st.byRole.ID)
	}
	// Debe marcarse como borrador (\Draft) y no enviarse (no hay outbound).
	if !st.lastInsert.HasFlag(domain.FlagDraft) {
		t.Fatalf("flags = %v, falta \\Draft", st.lastInsert.Flags)
	}
	if !strings.HasPrefix(blobs.putKey, "shared/mensajes/") {
		t.Fatalf("clave de blob = %q, quiero shared/mensajes/… (una clave por contenido, compartida)", blobs.putKey)
	}
	// La respuesta trae el id del borrador creado (para editar/enviar/descartar).
	var got struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.ID == "" {
		t.Fatalf("respuesta sin id: %s", rec.Body.String())
	}
}

func TestCreateDraftMissingRaw(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
	a := newAPI(st, &fakeBlobs{}, nil)
	rec := httptest.NewRecorder()
	a.createDraft(rec, req(http.MethodPost, "/api/v1/drafts", `{"recipients":["b@x.com"]}`, true, nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("raw vacío => %d, quiero 400", rec.Code)
	}
}

func TestUpdateDraftReplacesOld(t *testing.T) {
	old := uuid.New()
	st := &fakeStore{
		acc:    domain.Account{ID: uuid.New(), Address: "ada@sytue.eu"},
		byRole: domain.Mailbox{ID: uuid.New(), Role: domain.RoleDrafts},
	}
	a := newAPI(st, &fakeBlobs{}, nil)
	rec := httptest.NewRecorder()
	raw := "From: ada@sytue.eu\r\nSubject: v2\r\n\r\neditado"
	body := `{"recipients":["bob@x.com"],"raw":` + strconv.Quote(raw) + `}`
	a.updateDraft(rec, req(http.MethodPut, "/api/v1/drafts/"+old.String(), body, true, map[string]string{"messageID": old.String()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d, quiero 200 (%s)", rec.Code, rec.Body.String())
	}
	if st.lastInsert == nil {
		t.Fatal("no se insertó la versión nueva del borrador")
	}
}

// --- Reglas/filtros (listFilters / createFilter / updateFilter / deleteFilter) ---

func TestListFiltersOK(t *testing.T) {
	st := &fakeStore{
		acc:     domain.Account{ID: uuid.New()},
		filters: []domain.Filter{{ID: uuid.New(), Name: "Boletines", MatchFrom: "newsletter"}},
	}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.listFilters(rec, req(http.MethodGet, "/api/v1/filters", "", true, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d", rec.Code)
	}
	var got struct {
		Filters []domain.Filter `json:"filters"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Filters) != 1 || got.Filters[0].Name != "Boletines" {
		t.Fatalf("filters = %+v", got.Filters)
	}
}

func TestCreateFilterOK(t *testing.T) {
	dest := uuid.New()
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	body := `{"name":"Facturas","match_subject":"factura","move_to":"` + dest.String() + `","mark_read":true}`
	a.createFilter(rec, req(http.MethodPost, "/api/v1/filters", body, true, nil))
	if rec.Code != http.StatusCreated {
		t.Fatalf("código = %d, quiero 201 (%s)", rec.Code, rec.Body.String())
	}
	// El cuerpo se tradujo a domain.Filter con move_to parseado y enabled por defecto true.
	if st.lastFilter.MatchSubject != "factura" || st.lastFilter.MoveTo == nil || *st.lastFilter.MoveTo != dest {
		t.Fatalf("filtro creado = %+v", st.lastFilter)
	}
	if !st.lastFilter.Enabled {
		t.Error("enabled ausente debe ser true por defecto")
	}
	if !st.lastFilter.MarkRead {
		t.Error("mark_read no propagado")
	}
}

func TestCreateFilterBadMoveTo(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.createFilter(rec, req(http.MethodPost, "/api/v1/filters", `{"match_from":"x","move_to":"nope"}`, true, nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("move_to inválido => %d, quiero 400", rec.Code)
	}
}

func TestCreateFilterInvalidModel(t *testing.T) {
	// Sin condición ni acción: el store devuelve ErrInvalidFilter => 400.
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}, createFErr: domain.ErrInvalidFilter}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.createFilter(rec, req(http.MethodPost, "/api/v1/filters", `{"name":"vacía"}`, true, nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("regla inválida => %d, quiero 400", rec.Code)
	}
}

func TestUpdateFilterOK(t *testing.T) {
	id := uuid.New()
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	body := `{"name":"v2","match_from":"jefe@empresa.eu","mark_flagged":true,"enabled":false}`
	a.updateFilter(rec, req(http.MethodPut, "/api/v1/filters/"+id.String(), body, true, map[string]string{"filterID": id.String()}))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("código = %d, quiero 204 (%s)", rec.Code, rec.Body.String())
	}
	if st.lastFilterID != id {
		t.Fatalf("id actualizado = %v, quiero %v", st.lastFilterID, id)
	}
	if st.lastFilter.Enabled {
		t.Error("enabled:false explícito debe propagarse como false")
	}
}

func TestUpdateFilterBadID(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.updateFilter(rec, req(http.MethodPut, "/x", `{"match_from":"x","mark_read":true}`, true, map[string]string{"filterID": "bad"}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("filterID inválido => %d, quiero 400", rec.Code)
	}
}

func TestUpdateFilterNotFound(t *testing.T) {
	id := uuid.New()
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}, updateFErr: domain.ErrNotFound}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.updateFilter(rec, req(http.MethodPut, "/x", `{"match_from":"x","mark_read":true}`, true, map[string]string{"filterID": id.String()}))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("regla ausente => %d, quiero 404", rec.Code)
	}
}

func TestDeleteFilterOK(t *testing.T) {
	id := uuid.New()
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.deleteFilter(rec, req(http.MethodDelete, "/x", "", true, map[string]string{"filterID": id.String()}))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("código = %d, quiero 204", rec.Code)
	}
	if st.lastFilterID != id {
		t.Fatalf("id borrado = %v, quiero %v", st.lastFilterID, id)
	}
}

func TestDeleteFilterNotFound(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}, deleteFErr: domain.ErrNotFound}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.deleteFilter(rec, req(http.MethodDelete, "/x", "", true, map[string]string{"filterID": uuid.NewString()}))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("regla ausente => %d, quiero 404", rec.Code)
	}
}

// --- handleUpload --------------------------------------------------------

func TestHandleUploadOK(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
	blobs := &fakeBlobs{}
	a := newAPI(st, blobs, nil)
	rec := httptest.NewRecorder()
	r := req(http.MethodPost, "/jmap/upload/x", "hola mundo", true, nil)
	r.Header.Set("Content-Type", "text/plain")
	a.handleUpload(rec, r)
	if rec.Code != http.StatusCreated {
		t.Fatalf("código = %d, quiero 201 (%s)", rec.Code, rec.Body.String())
	}
	var got uploadResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Type != "text/plain" || got.Size != int64(len("hola mundo")) {
		t.Fatalf("respuesta = %+v", got)
	}
	if !strings.HasPrefix(blobs.putKey, "blobs/") {
		t.Fatalf("clave de blob = %q", blobs.putKey)
	}
}

func TestHandleUploadTooLarge(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
	a := newAPI(st, &fakeBlobs{}, nil)
	a.cfg.MaxMessageBytes = 4
	rec := httptest.NewRecorder()
	r := req(http.MethodPost, "/jmap/upload/x", "demasiado grande", true, nil)
	a.handleUpload(rec, r)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("blob grande => %d, quiero 400", rec.Code)
	}
}

func TestHandleUploadNoTenant(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
	a := newAPI(st, &fakeBlobs{}, nil)
	rec := httptest.NewRecorder()
	a.handleUpload(rec, req(http.MethodPost, "/jmap/upload/x", "x", false, nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("sin tenant => %d, quiero 401", rec.Code)
	}
}

func TestHandleUploadNoBlobStore(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
	a := newAPI(st, nil, nil) // blobs nil
	rec := httptest.NewRecorder()
	a.handleUpload(rec, req(http.MethodPost, "/jmap/upload/x", "x", true, nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("sin almacenamiento => %d, quiero 500", rec.Code)
	}
}

func TestHandleUploadPutError(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
	a := newAPI(st, &fakeBlobs{putErr: objectstore.ErrNotFound}, nil)
	rec := httptest.NewRecorder()
	a.handleUpload(rec, req(http.MethodPost, "/jmap/upload/x", "x", true, nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("error de Put => %d, quiero 500", rec.Code)
	}
}

// --- handleDownload ------------------------------------------------------

func TestHandleDownloadOK(t *testing.T) {
	acc := uuid.New()
	st := &fakeStore{acc: domain.Account{ID: acc}}
	blobs := &fakeBlobs{getBody: "contenido"}
	a := newAPI(st, blobs, nil)
	rec := httptest.NewRecorder()
	key := "blobs/" + acc.String() + "/" + uuid.NewString()
	r := req(http.MethodGet, "/x", "", true, map[string]string{"blobId": key, "name": "adjunto.pdf"})
	a.handleDownload(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "contenido" {
		t.Fatalf("cuerpo = %q", rec.Body.String())
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "adjunto.pdf") {
		t.Fatalf("Content-Disposition = %q", cd)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("falta X-Content-Type-Options")
	}
}

func TestHandleDownloadBadBlobID(t *testing.T) {
	acc := uuid.New()
	st := &fakeStore{acc: domain.Account{ID: acc}}
	a := newAPI(st, &fakeBlobs{}, nil)
	rec := httptest.NewRecorder()
	r := req(http.MethodGet, "/x", "", true, map[string]string{"blobId": "../etc/passwd"})
	a.handleDownload(rec, r)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("blobId inválido => %d, quiero 400", rec.Code)
	}
}

func TestHandleDownloadWrongAccount(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
	a := newAPI(st, &fakeBlobs{}, nil)
	rec := httptest.NewRecorder()
	// clave de otra cuenta => 404 (no pertenece al token).
	key := "blobs/" + uuid.NewString() + "/" + uuid.NewString()
	r := req(http.MethodGet, "/x", "", true, map[string]string{"blobId": key})
	a.handleDownload(rec, r)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cuenta ajena => %d, quiero 404", rec.Code)
	}
}

func TestHandleDownloadNoTenant(t *testing.T) {
	acc := uuid.New()
	st := &fakeStore{acc: domain.Account{ID: acc}}
	a := newAPI(st, &fakeBlobs{}, nil)
	rec := httptest.NewRecorder()
	key := "blobs/" + acc.String() + "/x"
	a.handleDownload(rec, req(http.MethodGet, "/x", "", false, map[string]string{"blobId": key}))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("sin tenant => %d, quiero 401", rec.Code)
	}
}

func TestHandleDownloadBlobMissing(t *testing.T) {
	acc := uuid.New()
	st := &fakeStore{acc: domain.Account{ID: acc}}
	a := newAPI(st, &fakeBlobs{getErr: objectstore.ErrNotFound}, nil)
	rec := httptest.NewRecorder()
	key := "blobs/" + acc.String() + "/x"
	r := req(http.MethodGet, "/x", "", true, map[string]string{"blobId": key})
	a.handleDownload(rec, r)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("blob ausente => %d, quiero 404", rec.Code)
	}
}

// --- handleSession -------------------------------------------------------

func TestHandleSession(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New(), Address: "ada@sytue.eu"}}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.handleSession(rec, req(http.MethodGet, "/jmap/session", "", true, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d", rec.Code)
	}
	var res sessionResource
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode session: %v", err)
	}
	if res.Username != "ada@sytue.eu" {
		t.Fatalf("username = %q", res.Username)
	}
	if res.APIURL != "https://mail.sytue.eu/jmap" {
		t.Fatalf("apiUrl = %q", res.APIURL)
	}
	if _, ok := res.Capabilities[capCore]; !ok {
		t.Fatal("falta capacidad core")
	}
}

func TestHandleSessionEnsureError(t *testing.T) {
	st := &fakeStore{accErr: domain.ErrQuotaExceeded}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.handleSession(rec, req(http.MethodGet, "/jmap/session", "", true, nil))
	if rec.Code == http.StatusOK {
		t.Fatalf("error de EnsureAccount no debe dar 200")
	}
}

// --- handleJMAP (sobre por lotes) ----------------------------------------

func TestHandleJMAPDispatch(t *testing.T) {
	accID := uuid.New()
	st := &fakeStore{
		bySubject: domain.Account{ID: accID},
		mailboxes: []domain.Mailbox{{ID: uuid.New(), Name: "Inbox", TotalCount: 7, UnreadCount: 3}},
	}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	envelope := `{"using":["urn:ietf:params:jmap:mail"],"methodCalls":[["Mailbox/get",{},"c0"]]}`
	a.handleJMAP(rec, req(http.MethodPost, "/jmap", envelope, true, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	var resp jmapResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.MethodResponses) != 1 {
		t.Fatalf("respuestas = %d", len(resp.MethodResponses))
	}
	if name, _ := resp.MethodResponses[0][0].(string); name != "Mailbox/get" {
		t.Fatalf("método de respuesta = %q", name)
	}

	// LOS NOMBRES SON LOS DEL RFC 8621 §2.3, en camelCase.
	//
	// Se devolvía `domain.Mailbox` tal cual, con las etiquetas de nuestra API REST
	// (`total_emails`, `unread_emails`). Un cliente JMAP lee `totalEmails`, no lo
	// encuentra y —como el nuestro hace `?? 0`— enseña un CERO creíble en vez de un
	// hueco: la barra lateral entera decía 0 mensajes y ningún no leído con los
	// buzones llenos. Por eso se comprueban los nombres y no solo el valor: con
	// `total_emails` el contador también sería correcto y la interfaz seguiría
	// mintiendo.
	args, _ := resp.MethodResponses[0][1].(map[string]any)
	lista, _ := args["list"].([]any)
	if len(lista) != 1 {
		t.Fatalf("buzones devueltos = %d", len(lista))
	}
	box, _ := lista[0].(map[string]any)
	for _, clave := range []string{"totalEmails", "unreadEmails"} {
		if _, ok := box[clave]; !ok {
			t.Errorf("falta %q; el cliente JMAP lo lee de ahí. Claves: %v", clave, clavesDe(box))
		}
	}
	for _, viejo := range []string{"total_emails", "unread_emails"} {
		if _, ok := box[viejo]; ok {
			t.Errorf("%q es el nombre de la API REST, no el de JMAP", viejo)
		}
	}
	if v, _ := box["totalEmails"].(float64); v != 7 {
		t.Errorf("totalEmails = %v, quiero 7", box["totalEmails"])
	}
}

// clavesDe ordena las claves de un mapa para que el fallo diga lo que SÍ llegó.
func clavesDe(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestHandleJMAPUnsupportedMethod(t *testing.T) {
	st := &fakeStore{bySubject: domain.Account{ID: uuid.New()}}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	envelope := `{"methodCalls":[["Foo/bar",{},"c0"]]}`
	a.handleJMAP(rec, req(http.MethodPost, "/jmap", envelope, true, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d", rec.Code)
	}
	var resp jmapResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if name, _ := resp.MethodResponses[0][0].(string); name != "error" {
		t.Fatalf("método no soportado debe dar error, got %q", name)
	}
}

func TestHandleJMAPBadJSON(t *testing.T) {
	a := newAPI(&fakeStore{}, nil, nil)
	rec := httptest.NewRecorder()
	a.handleJMAP(rec, req(http.MethodPost, "/jmap", `{bad`, true, nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("código = %d, quiero 400", rec.Code)
	}
}

func TestHandleJMAPDispatchAccountError(t *testing.T) {
	st := &fakeStore{bySubErr: domain.ErrNotFound}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	envelope := `{"methodCalls":[["Mailbox/get",{},"c0"]]}`
	a.handleJMAP(rec, req(http.MethodPost, "/jmap", envelope, true, nil))
	var resp jmapResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if name, _ := resp.MethodResponses[0][0].(string); name != "error" {
		t.Fatalf("cuenta no encontrada debe dar error, got %q", name)
	}
}

// --- dispatch de métodos individuales ------------------------------------

func TestEmailGetDispatch(t *testing.T) {
	accID := uuid.New()
	found := uuid.New()
	missing := uuid.New()
	st := &fakeStore{
		bySubject:  domain.Account{ID: accID},
		getMsgsRet: []domain.Message{{ID: found, AccountID: accID}},
	}
	a := newAPI(st, nil, nil)
	raw := rawArgs(`{"ids":["` + found.String() + `","` + missing.String() + `","not-a-uuid"]}`)
	r := req(http.MethodPost, "/jmap", "", true, nil)
	res, err := a.emailGet(r, domain.Account{ID: accID}, raw)
	if err != nil {
		t.Fatalf("emailGet: %v", err)
	}
	m := asMap(t, res)
	if n := strSliceLen(t, m, "notFound"); n != 2 {
		t.Fatalf("notFound = %v, quiero 2 (uuid ausente + no-uuid)", m["notFound"])
	}
}

// TestMessageReadIsolation es el check del gate de lectura: un mensaje de OTRA
// cuenta del mismo tenant responde 404 sin ACL (anti-enumeración, no 403) tanto
// en GET /messages/{id} como en Email/get; con un grant `read` vigente (buzón
// compartido/delegado) la lectura SÍ pasa.
func TestMessageReadIsolation(t *testing.T) {
	callerAcc := domain.Account{ID: uuid.New()}
	foreign := domain.Message{ID: uuid.New(), AccountID: uuid.New(), Subject: "ajeno"}

	// Sin ACL (GranteePermission → ErrNotFound): 404.
	st := &fakeStore{acc: callerAcc, message: foreign, granteeErr: domain.ErrNotFound}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	a.getMessage(rec, req(http.MethodGet, "/x", "", true, map[string]string{"messageID": foreign.ID.String()}))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("mensaje ajeno sin grant => %d, quiero 404", rec.Code)
	}

	// Email/get lo omite y lo reporta como notFound.
	st.getMsgsRet = []domain.Message{foreign}
	raw := rawArgs(`{"ids":["` + foreign.ID.String() + `"]}`)
	res, err := a.emailGet(req(http.MethodPost, "/jmap", "", true, nil), callerAcc, raw)
	if err != nil {
		t.Fatalf("emailGet: %v", err)
	}
	if n := strSliceLen(t, asMap(t, res), "notFound"); n != 1 {
		t.Fatalf("Email/get ajeno sin grant: notFound = %d, quiero 1", n)
	}

	// Con grant `read` (delegación/compartido) la lectura sigue funcionando.
	st.granteeErr, st.granteePerm = nil, domain.PermRead
	rec = httptest.NewRecorder()
	a.getMessage(rec, req(http.MethodGet, "/x", "", true, map[string]string{"messageID": foreign.ID.String()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("mensaje delegado con grant read => %d, quiero 200", rec.Code)
	}
}

func TestEmailQueryDispatch(t *testing.T) {
	mbID := uuid.New()
	st := &fakeStore{searchRet: []domain.Message{{ID: uuid.New()}, {ID: uuid.New()}}}
	a := newAPI(st, nil, nil)
	raw := rawArgs(`{"filter":{"inMailbox":"` + mbID.String() + `","hasKeyword":"$seen"},"limit":20}`)
	r := req(http.MethodPost, "/jmap", "", true, nil)
	res, err := a.emailQuery(r, domain.Account{ID: uuid.New()}, raw)
	if err != nil {
		t.Fatalf("emailQuery: %v", err)
	}
	m := asMap(t, res)
	if n := strSliceLen(t, m, "ids"); n != 2 {
		t.Fatalf("ids = %v", m["ids"])
	}
	if st.lastSearch.MailboxID != mbID || !st.lastSearch.Seen || st.lastSearch.Limit != 20 {
		t.Fatalf("filtro pasado al store = %+v", st.lastSearch)
	}
}

func TestMailboxSetCreateUpdateDestroy(t *testing.T) {
	newID := uuid.New()
	updID := uuid.New()
	delID := uuid.New()
	st := &fakeStore{createdMB: domain.Mailbox{ID: newID}}
	a := newAPI(st, nil, nil)
	raw := rawArgs(`{
		"create":{"tmp":{"name":"Nuevo"}},
		"update":{"` + updID.String() + `":{"name":"Renombrado"},"bad-id":{"name":"x"}},
		"destroy":["` + delID.String() + `","not-a-uuid"]
	}`)
	r := req(http.MethodPost, "/jmap", "", true, nil)
	res, err := a.mailboxSet(r, domain.Account{ID: uuid.New()}, raw)
	if err != nil {
		t.Fatalf("mailboxSet: %v", err)
	}
	m := asMap(t, res)
	if mapLen(t, m, "created") != 1 {
		t.Fatalf("created = %v", m["created"])
	}
	if mapLen(t, m, "updated") != 1 {
		t.Fatalf("updated = %v", m["updated"])
	}
	if mapLen(t, m, "notUpdated") != 1 {
		t.Fatalf("notUpdated = %v", m["notUpdated"])
	}
	if strSliceLen(t, m, "destroyed") != 1 {
		t.Fatalf("destroyed = %v", m["destroyed"])
	}
	if mapLen(t, m, "notDestroyed") != 1 {
		t.Fatalf("notDestroyed = %v", m["notDestroyed"])
	}
}

func TestEmailSetPatch(t *testing.T) {
	id := uuid.New()
	dest := uuid.New()
	delID := uuid.New()
	st := &fakeStore{}
	a := newAPI(st, nil, nil)
	raw := rawArgs(`{
		"update":{"` + id.String() + `":{"keywords/$seen":true,"mailboxIds/` + dest.String() + `":true}},
		"destroy":["` + delID.String() + `"]
	}`)
	r := req(http.MethodPost, "/jmap", "", true, nil)
	res, err := a.emailSet(r, domain.Account{ID: uuid.New()}, raw)
	if err != nil {
		t.Fatalf("emailSet: %v", err)
	}
	m := asMap(t, res)
	if mapLen(t, m, "updated") != 1 {
		t.Fatalf("updated = %v", m["updated"])
	}
	if strSliceLen(t, m, "destroyed") != 1 {
		t.Fatalf("destroyed = %v", m["destroyed"])
	}
	if st.lastFlag != domain.FlagSeen || !st.lastFlagAdd {
		t.Fatalf("flag aplicado = %q add=%v", st.lastFlag, st.lastFlagAdd)
	}
	if st.lastMoveTo != dest {
		t.Fatalf("mover a = %v, quiero %v", st.lastMoveTo, dest)
	}
}

func TestApplyEmailPatchFullKeywordsAndMailboxIds(t *testing.T) {
	id := uuid.New()
	dest := uuid.New()
	st := &fakeStore{}
	a := newAPI(st, nil, nil)
	r := req(http.MethodPost, "/jmap", "", true, nil)
	patch := map[string]json.RawMessage{
		"keywords":   json.RawMessage(`{"$flagged":true}`),
		"mailboxIds": json.RawMessage(`{"` + dest.String() + `":true}`),
	}
	if err := a.applyEmailPatch(r, uuid.New(), id, patch); err != nil {
		t.Fatalf("applyEmailPatch: %v", err)
	}
	if st.lastFlag != domain.FlagFlagged || st.lastMoveTo != dest {
		t.Fatalf("flag=%q move=%v", st.lastFlag, st.lastMoveTo)
	}
}

func TestApplyEmailPatchBadMailboxID(t *testing.T) {
	st := &fakeStore{}
	a := newAPI(st, nil, nil)
	r := req(http.MethodPost, "/jmap", "", true, nil)
	patch := map[string]json.RawMessage{"mailboxIds/not-a-uuid": json.RawMessage(`true`)}
	if err := a.applyEmailPatch(r, uuid.New(), uuid.New(), patch); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("mailboxId inválido debe dar ErrNotFound, got %v", err)
	}
}

// --- Mount (smoke sobre un router chi real) ------------------------------

func TestMountWellKnownRedirect(t *testing.T) {
	a := newAPI(&fakeStore{}, nil, nil)
	r := chi.NewRouter()
	// Solo montamos el well-known (no requiere verifier).
	r.Get("/.well-known/jmap", a.handleSessionRedirect)
	srv := httptest.NewServer(r)
	defer srv.Close()

	rec := httptest.NewRecorder()
	a.handleSessionRedirect(rec, httptest.NewRequest(http.MethodGet, "/.well-known/jmap", http.NoBody))
	if rec.Code != http.StatusPermanentRedirect {
		t.Fatalf("código = %d", rec.Code)
	}
}

// Sanity: el cuerpo de subida se lee entero aunque no supere el límite.
func TestUploadReadsBody(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
	blobs := &fakeBlobs{}
	a := newAPI(st, blobs, nil)
	rec := httptest.NewRecorder()
	r := req(http.MethodPost, "/jmap/upload/x", "abc", true, nil)
	r.Body = io.NopCloser(bytes.NewBufferString("abc"))
	a.handleUpload(rec, r)
	if rec.Code != http.StatusCreated {
		t.Fatalf("código = %d", rec.Code)
	}
}

// --- envío diferido: programado + deshacer -------------------------------

func TestResolveRelease(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)

	// send_at futuro => programado.
	at, hold, err := resolveRelease(now.Add(time.Hour).Format(time.RFC3339), 0, now)
	if err != nil || hold != domain.HoldScheduled || !at.After(now) {
		t.Fatalf("programado: at=%v hold=%q err=%v", at, hold, err)
	}
	// send_at pasado => error de cliente.
	if _, _, err := resolveRelease(now.Add(-time.Hour).Format(time.RFC3339), 0, now); err == nil {
		t.Fatal("send_at en el pasado debería fallar")
	}
	// send_at mal formado => error.
	if _, _, err := resolveRelease("no-es-fecha", 0, now); err == nil {
		t.Fatal("send_at mal formado debería fallar")
	}
	// undo_seconds => ventana de deshacer, acotada a maxUndoSeconds.
	at, hold, err = resolveRelease("", 999, now)
	if err != nil || hold != domain.HoldUndo {
		t.Fatalf("undo: hold=%q err=%v", hold, err)
	}
	if want := now.Add(maxUndoSeconds * time.Second); !at.Equal(want) {
		t.Fatalf("undo clamp: at=%v, quiero %v", at, want)
	}
	// Sin nada => inmediato.
	if _, hold, _ := resolveRelease("", 0, now); hold != domain.HoldNone {
		t.Fatalf("sin diferir: hold=%q, quiero vacío", hold)
	}
}

func TestSendMessageScheduled(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New(), Address: "ada@sytue.eu"}}
	out := &fakeOutbound{scheduledID: uuid.New()}
	a := newAPI(st, nil, out)
	rec := httptest.NewRecorder()
	at := time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
	body := `{"recipients":["bob@x.com"],"raw":"x","send_at":"` + at + `"}`
	a.sendMessage(rec, req(http.MethodPost, "/api/v1/send", body, true, nil))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("código = %d, quiero 202 (%s)", rec.Code, rec.Body.String())
	}
	if out.lastHold != domain.HoldScheduled {
		t.Fatalf("hold = %q, quiero scheduled", out.lastHold)
	}
	m := decodeBody(t, rec)
	if m["status"] != "scheduled" || m["id"] == nil {
		t.Fatalf("respuesta = %+v, quiero status=scheduled + id", m)
	}
}

func TestSendMessageUndo(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New(), Address: "ada@sytue.eu"}}
	out := &fakeOutbound{scheduledID: uuid.New()}
	a := newAPI(st, nil, out)
	rec := httptest.NewRecorder()
	body := `{"recipients":["bob@x.com"],"raw":"x","undo_seconds":10}`
	a.sendMessage(rec, req(http.MethodPost, "/api/v1/send", body, true, nil))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("código = %d, quiero 202", rec.Code)
	}
	if out.lastHold != domain.HoldUndo {
		t.Fatalf("hold = %q, quiero undo", out.lastHold)
	}
	// La liberación cae dentro de la ventana pedida (10 s desde ahora).
	if d := time.Until(out.lastRelease); d <= 0 || d > 11*time.Second {
		t.Fatalf("release fuera de la ventana de deshacer: %v", d)
	}
}

func TestSendMessageScheduledPast(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New(), Address: "ada@sytue.eu"}}
	a := newAPI(st, nil, &fakeOutbound{})
	rec := httptest.NewRecorder()
	past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	body := `{"recipients":["bob@x.com"],"raw":"x","send_at":"` + past + `"}`
	a.sendMessage(rec, req(http.MethodPost, "/api/v1/send", body, true, nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("send_at pasado => %d, quiero 400", rec.Code)
	}
}

func TestListScheduled(t *testing.T) {
	last := domain.OutboundMessage{ID: uuid.New(), Subject: "reunión", NextRetryAt: time.Now().Add(time.Hour)}
	st := &fakeStore{
		acc:       domain.Account{ID: uuid.New()},
		scheduled: []domain.OutboundMessage{{ID: uuid.New(), Subject: "a"}, last}, // página llena (limit por defecto? no)
	}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	// limit=2 fuerza página llena => next_cursor no vacío.
	a.listScheduled(rec, req(http.MethodGet, "/api/v1/scheduled?limit=2", "", true, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d, quiero 200 (%s)", rec.Code, rec.Body.String())
	}
	m := decodeBody(t, rec)
	if m["next_cursor"] == "" {
		t.Fatal("página llena debería traer next_cursor")
	}
	items, ok := m["scheduled"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("scheduled = %+v, quiero 2 items", m["scheduled"])
	}
}

func TestRescheduleScheduledOK(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	id := uuid.New()
	at := time.Now().Add(3 * time.Hour).UTC().Format(time.RFC3339)
	body := `{"send_at":"` + at + `"}`
	a.rescheduleScheduled(rec, req(http.MethodPost, "/x", body, true, map[string]string{"id": id.String()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d, quiero 200 (%s)", rec.Code, rec.Body.String())
	}
	if st.lastCancelID != id || st.lastRelease.IsZero() {
		t.Fatalf("no propagó id/hora: id=%v release=%v", st.lastCancelID, st.lastRelease)
	}
}

func TestRescheduleScheduledGone(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}, reschedErr: domain.ErrScheduleGone}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	at := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	a.rescheduleScheduled(rec, req(http.MethodPost, "/x", `{"send_at":"`+at+`"}`, true, map[string]string{"id": uuid.New().String()}))
	if rec.Code != http.StatusConflict {
		t.Fatalf("ya liberado => %d, quiero 409", rec.Code)
	}
}

func TestCancelOutboundOK(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}, cancelHold: domain.HoldUndo}
	a := newAPI(st, nil, nil)
	rec := httptest.NewRecorder()
	id := uuid.New()
	a.cancelOutbound(rec, req(http.MethodPost, "/x", "", true, map[string]string{"id": id.String()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d, quiero 200 (%s)", rec.Code, rec.Body.String())
	}
	m := decodeBody(t, rec)
	if m["canceled"] != true || m["kind"] != domain.HoldUndo {
		t.Fatalf("respuesta = %+v, quiero canceled=true + kind=undo", m)
	}
	if st.lastCancelID != id {
		t.Fatalf("no propagó el id a CancelHeld: %v", st.lastCancelID)
	}
}

func TestCancelOutboundGone(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New()}, cancelErr: domain.ErrScheduleGone}
	a := newAPI(st, &fakeBlobs{}, nil)
	rec := httptest.NewRecorder()
	a.cancelOutbound(rec, req(http.MethodPost, "/x", "", true, map[string]string{"id": uuid.New().String()}))
	if rec.Code != http.StatusConflict {
		t.Fatalf("ya liberado => %d, quiero 409", rec.Code)
	}
}

// La cuenta sigue a la dirección que asigna Sytue ID, no se queda en la del alta.
//
// El día que una organización conecta su dominio propio, Sytue ID muda a toda su
// plantilla y el reconciliador de buzones abre esta misma sesión JMAP para que la
// dirección nueva exista también en Sytue Mail. Abrir la sesión solo CREA la
// cuenta si no hay ninguna, así que la de quien ya la tenía se quedaba con la
// dirección vieja y la nueva no existía aquí ni como primaria ni como alias: el
// MX contestaba 550 al correo del dominio recién conectado y nada fallaba a la
// vista — Sytue ID daba la dirección por asignada y la fila por creada.
//
// Y la mudanza la manda SOLO el claim `mailbox`, el que firma Sytue ID: si la
// disparara `MailAddress()`, que cae al correo de registro cuando no hay buzón,
// una cuenta sin buzón asignado se llevaría el correo personal de esa persona a
// nuestro espacio de direcciones.
func TestSesionMudaLaDireccionQueAsignaSytueID(t *testing.T) {
	casos := []struct {
		nombre  string
		mailbox string
		cuenta  string
		quiere  string // dirección a la que se pide mudar; vacío = no se muda
	}{
		{"la organización conectó su dominio", "ada@acme.eu", "ada@acme.sytue.eu", "ada@acme.eu"},
		{"la misma de siempre", "ada@acme.sytue.eu", "ada@acme.sytue.eu", ""},
		{"mayúsculas no son otra dirección", "Ada@ACME.sytue.eu", "ada@acme.sytue.eu", ""},
		{"sin buzón asignado no se toca la cuenta", "", "ada@acme.sytue.eu", ""},
	}
	for _, c := range casos {
		f := &fakeStore{acc: domain.Account{ID: uuid.New(), Address: c.cuenta}}
		a := newAPI(f, nil, nil)
		r := httptest.NewRequest(http.MethodGet, "/jmap/session", http.NoBody)
		r = r.WithContext(authn.Into(r.Context(), authn.Claims{
			Subject: "user-1", Name: "Ada", Tenant: "acme",
			// El correo de registro, en un dominio que no servimos: es justo el que
			// no puede acabar siendo la dirección de la cuenta.
			Email: "ada@correo-personal.example", Mailbox: c.mailbox,
		}))
		rec := httptest.NewRecorder()
		a.handleSession(rec, r)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: sesión = %d, quiero 200", c.nombre, rec.Code)
		}
		if f.mudadaA != c.quiere {
			t.Errorf("%s: mudanza a %q, quiero %q", c.nombre, f.mudadaA, c.quiere)
		}
	}
}

// conClaims sustituye las claims de la petición: los tests de alias necesitan
// decir de qué clase de cuenta viene (personal, miembro, admin).
func conClaims(r *http.Request, c authn.Claims) *http.Request {
	return r.WithContext(authn.Into(r.Context(), c))
}

// Un alias que suplanta a la plataforma en el dominio raíz se rechaza en el
// borde con un código que el cliente puede traducir, sin llegar al store.
func TestCreateAliasReservadoEnElRaiz(t *testing.T) {
	st := &fakeStore{acc: domain.Account{ID: uuid.New(), Address: "ada@sytue.eu", Tenant: "sytue"}}
	a := newAPI(st, nil, nil)
	a.cfg.PrimaryDomain = "sytue.eu"
	a.cfg.RootTenant = "sytue"
	for _, addr := range []string{"admin@sytue.eu", "no-reply@sytue.eu", "sytue-id@sytue.eu"} {
		rec := httptest.NewRecorder()
		r := conClaims(req(http.MethodPost, "/api/v1/aliases", `{"address":"`+addr+`"}`, true, nil),
			authn.Claims{Subject: "user-1", Email: "ada@sytue.eu", Tenant: "sytue", Roles: []string{"owner"}})
		a.createAlias(rec, r)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "alias_reserved") {
			t.Fatalf("%s => %d %s, quiero 400 alias_reserved", addr, rec.Code, rec.Body.String())
		}
	}
	if st.lastAliasAddr != "" {
		t.Fatalf("no debía llegar al store: %q", st.lastAliasAddr)
	}
	// La misma palabra en el subdominio de una organización es asunto suyo.
	st2 := &fakeStore{acc: domain.Account{ID: uuid.New(), Address: "ada@acme.sytue.eu", Tenant: "acme"}}
	a2 := newAPI(st2, nil, nil)
	a2.cfg.PrimaryDomain = "sytue.eu"
	rec := httptest.NewRecorder()
	a2.createAlias(rec, conClaims(req(http.MethodPost, "/api/v1/aliases", `{"address":"admin@acme.sytue.eu"}`, true, nil),
		authn.Claims{Subject: "user-1", Tenant: "acme", Roles: []string{"owner"}}))
	if rec.Code != http.StatusCreated {
		t.Fatalf("admin@acme.sytue.eu => %d, quiero 201 (%s)", rec.Code, rec.Body.String())
	}
}

// Una cuenta común (personal, o miembro sin rol de administración) tiene un
// alias; el segundo devuelve 409 alias_limit. Un admin no tiene techo aquí.
func TestCreateAliasLimiteParaCuentasComunes(t *testing.T) {
	ya := []domain.Alias{{ID: uuid.New(), Address: "ada.p@sytue.eu"}}
	casos := []struct {
		nombre string
		claims authn.Claims
		quiere int
	}{
		{"personal", authn.Claims{Subject: "u", Tenant: "sytue", OrgKind: "personal", Roles: []string{"owner"}}, http.StatusConflict},
		{"miembro sin rol", authn.Claims{Subject: "u", Tenant: "acme", Roles: []string{"member"}}, http.StatusConflict},
		{"admin de organización", authn.Claims{Subject: "u", Tenant: "acme", Roles: []string{"admin"}}, http.StatusCreated},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			st := &fakeStore{acc: domain.Account{ID: uuid.New(), Address: "ada@acme.sytue.eu", Tenant: "acme"}, aliases: ya}
			a := newAPI(st, nil, nil)
			a.cfg.PrimaryDomain = "sytue.eu"
			rec := httptest.NewRecorder()
			a.createAlias(rec, conClaims(req(http.MethodPost, "/api/v1/aliases", `{"address":"segundo@acme.sytue.eu"}`, true, nil), c.claims))
			if rec.Code != c.quiere {
				t.Fatalf("código = %d, quiero %d (%s)", rec.Code, c.quiere, rec.Body.String())
			}
			if c.quiere == http.StatusConflict && !strings.Contains(rec.Body.String(), "alias_limit") {
				t.Fatalf("falta el código alias_limit: %s", rec.Body.String())
			}
		})
	}
	// Sin alias previo, la cuenta común crea el primero.
	st := &fakeStore{acc: domain.Account{ID: uuid.New(), Address: "ada@acme.sytue.eu", Tenant: "acme"}}
	a := newAPI(st, nil, nil)
	a.cfg.PrimaryDomain = "sytue.eu"
	rec := httptest.NewRecorder()
	a.createAlias(rec, conClaims(req(http.MethodPost, "/api/v1/aliases", `{"address":"primero@acme.sytue.eu"}`, true, nil),
		authn.Claims{Subject: "u", Tenant: "acme", OrgKind: "personal"}))
	if rec.Code != http.StatusCreated {
		t.Fatalf("primer alias de una cuenta personal => %d, quiero 201 (%s)", rec.Code, rec.Body.String())
	}
}
