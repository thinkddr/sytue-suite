// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package pqvaulttest imita en memoria el servidor de la bóveda PQ directa
// (services/drive) y el registro de claves de firma de Sytue ID, para probar
// los clientes nativos sin red.
package pqvaulttest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/recoverywrap"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/storagetest"
)

// Fake imita services/drive (`/api/v1/pq-vault/{product}/*`) y el registro de
// Sytue ID (`/api/v1/me/vault-keys/{product}`) en memoria, con las reglas que
// importan aquí: el vaciado borra registros y claves y estrena identificador, y
// solo si la bóveda sigue como la vio el cliente (versión); la publicación es
// idempotente por transacción y sube la versión; con una marca de copia activa
// solo publica y vacía quien la tiene; el historial va del más reciente al más
// antiguo; un registro se busca por su transacción; un vaciado de la clave de
// firma sin identificación reciente es 403, y un traspaso (`handover`) lo exige
// siempre, solo en `mail` y nunca colgado de una clave que ya estaba sin él.
type Fake struct {
	t     *testing.T
	scope storage.Scope

	mu        sync.Mutex
	txns      map[string]int
	nextVault int
	putKeys   []bool // reset de cada PUT de claves
	claims    []string

	// El estado se puede leer y preparar entre peticiones, nunca durante una.
	VaultID     string
	ActiveKeyID string
	Keys        []recoverywrap.PQVaultWrapper
	Records     []api.PQVaultRecord
	Seq         int64
	Version     int64
	// Copy es la marca de copia (su Active lo calcula el servidor con Now).
	Copy *api.PQVaultCopy
	// Now es el reloj de la marca (time.Now si nil).
	Now func() time.Time
	// BeforeReset, si no es nil, corre justo antes de comprobar un vaciado: la
	// otra mitad de una carrera (la web publicando su recibo).
	BeforeReset func(*Fake)
	// FailList hace fallar el listado de registros (¿está vacía?).
	FailList bool
	// FailAppend, si no es nil, decide antes de guardar cada registro nuevo (con
	// los que ya hay) si la publicación se corta: el servidor contesta 503 y no
	// guarda nada, como un cliente que se cierra a mitad de una importación.
	FailAppend func(records int) bool
	Signing    []api.VaultKey
	// RecentLogin: la sesión vale como identificación reciente para un vaciado.
	RecentLogin bool
	// Handovers cuenta los PUT de la clave de firma que pidieron traspaso.
	Handovers int
	// Blobs son los bloques cifrados de /api/v1/cipher/blobs.
	Blobs storagetest.CipherState
}

// New arranca el servidor y devuelve los dos clientes reales que hablan con él.
func New(t *testing.T, scope storage.Scope) (*Fake, *api.Drive, *api.Account) {
	t.Helper()
	f := &Fake{t: t, scope: scope, txns: map[string]int{}}
	server := httptest.NewServer(f)
	t.Cleanup(server.Close)
	session := tokenSession{}
	return f, api.NewDrive(server.URL, session, server.Client()), api.NewAccount(server.URL, session, server.Client())
}

type tokenSession struct{}

func (tokenSession) AccessToken(context.Context) (string, error) { return "artificial", nil }
func (tokenSession) Invalidate(string)                           {}

func (f *Fake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer artificial" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/v1/me/vault-keys/") {
		f.vaultKeys(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/v1/cipher/blobs/") {
		if got, err := storage.DecodeScopeHeader(r.Header.Get(storage.ScopeHeader)); err != nil || got != f.scope {
			http.Error(w, "scope", http.StatusConflict)
			return
		}
		storagetest.ServeCipherBlob(w, r, f.scope.Product, &f.Blobs)
		return
	}
	prefix := "/api/v1/pq-vault/" + f.scope.Product
	if !strings.HasPrefix(r.URL.Path, prefix) {
		http.NotFound(w, r)
		return
	}
	if got, err := storage.DecodeScopeHeader(r.Header.Get(storage.ScopeHeader)); err != nil || got != f.scope {
		http.Error(w, "scope", http.StatusConflict)
		return
	}
	f.route(w, r, strings.TrimPrefix(r.URL.Path, prefix))
}

func (f *Fake) route(w http.ResponseWriter, r *http.Request, tail string) {
	switch {
	case tail == "/keys" && r.Method == http.MethodGet:
		f.json(w, http.StatusOK, f.state())
	case tail == "/keys" && r.Method == http.MethodPut:
		f.putKey(w, r)
	case tail == "/copy" && r.Method == http.MethodPut:
		f.claim(w, r)
	case tail == "/copy" && r.Method == http.MethodDelete:
		f.release(w, r)
	case tail == "/records" && r.Method == http.MethodPost:
		f.append(w, r)
	case tail == "/records" && r.Method == http.MethodGet:
		f.list(w, r)
	case strings.HasPrefix(tail, "/records/") && r.Method == http.MethodGet:
		f.get(w, r, strings.TrimPrefix(tail, "/records/"))
	default:
		http.NotFound(w, r)
	}
}

func (f *Fake) json(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (f *Fake) now() time.Time {
	if f.Now != nil {
		return f.Now()
	}
	return time.Now()
}

func (f *Fake) state() api.PQVaultState {
	out := api.PQVaultState{VaultID: f.VaultID, ActiveKeyID: f.ActiveKeyID, Keys: slices.Clone(f.Keys), Version: f.Version}
	if f.Copy != nil {
		c := *f.Copy
		c.Active = c.ExpiresAt.After(f.now())
		out.Copy = &c
	}
	return out
}

// copyActive: la marca puesta y en vigor.
func (f *Fake) copyActive() bool { return f.Copy != nil && f.Copy.ExpiresAt.After(f.now()) }

func (f *Fake) conflict(w http.ResponseWriter, code string) {
	f.json(w, http.StatusConflict, map[string]any{"error": map[string]string{"code": code, "message": code}})
}

// copyAllows es la regla del servidor (store.pqVaultCopyAllows).
func (f *Fake) copyAllows(mine string) bool {
	switch {
	case f.Copy == nil:
		return mine == ""
	case f.Copy.ID == mine:
		return true
	default:
		return mine == "" && !f.copyActive()
	}
}

func (f *Fake) claim(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID         string `json:"id"`
		Holder     string `json:"holder"`
		Kind       string `json:"kind"`
		Epoch      string `json:"epoch"`
		TTLSeconds int64  `json:"ttl_seconds"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.ID == "" || req.Holder == "" || req.Kind != api.PQVaultCopyKind || req.Epoch == "" || req.TTLSeconds <= 0 {
		http.Error(w, "bad", http.StatusBadRequest)
		return
	}
	if f.VaultID == "" {
		http.NotFound(w, r)
		return
	}
	if f.copyActive() && f.Copy.ID != req.ID {
		f.conflict(w, api.CodePQVaultCopyInProgress)
		return
	}
	started := f.now()
	if f.Copy != nil && f.Copy.ID == req.ID {
		started = f.Copy.StartedAt
	}
	f.claims = append(f.claims, req.ID)
	f.Copy = &api.PQVaultCopy{ID: req.ID, Holder: req.Holder, Kind: req.Kind, Epoch: req.Epoch, StartedAt: started, ExpiresAt: f.now().Add(time.Duration(req.TTLSeconds) * time.Second)}
	f.json(w, http.StatusOK, f.state())
}

func (f *Fake) release(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "bad", http.StatusBadRequest)
		return
	}
	if f.Copy != nil && f.Copy.ID != id {
		f.conflict(w, api.CodePQVaultChanged)
		return
	}
	f.Copy = nil
	f.json(w, http.StatusOK, f.state())
}

func (f *Fake) putKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Wrapper   recoverywrap.PQVaultWrapper `json:"wrapper"`
		SetActive bool                        `json:"set_active"`
		Reset     bool                        `json:"reset"`
		Expect    *api.PQVaultExpect          `json:"expect"`
		CopyID    string                      `json:"copy_id"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.Wrapper.Validate() != nil || (!req.Reset && req.Expect != nil) {
		http.Error(w, "bad", http.StatusBadRequest)
		return
	}
	if req.Reset {
		if f.BeforeReset != nil {
			before := f.BeforeReset
			f.BeforeReset = nil
			before(f)
		}
		// Sin expect (nativos v3.22 y anteriores) se vacía sin comparar, como antes.
		if req.Expect != nil && *req.Expect != (api.PQVaultExpect{VaultID: f.VaultID, Version: f.Version}) {
			f.conflict(w, api.CodePQVaultChanged)
			return
		}
		if f.copyActive() && f.Copy.ID != req.CopyID {
			f.conflict(w, api.CodePQVaultCopyInProgress)
			return
		}
	}
	f.putKeys = append(f.putKeys, req.Reset)
	if req.Reset {
		f.Records, f.Keys, f.txns = nil, nil, map[string]int{}
		f.Version++
		if f.Copy != nil && f.Copy.ID != req.CopyID {
			f.Copy = nil
		}
	}
	if f.VaultID == "" || req.Reset {
		f.nextVault++
		f.VaultID = "!pqv_" + strconv.Itoa(f.nextVault)
		f.ActiveKeyID = req.Wrapper.KeyID
	} else if req.SetActive {
		f.ActiveKeyID = req.Wrapper.KeyID
	}
	f.Keys = slices.DeleteFunc(f.Keys, func(k recoverywrap.PQVaultWrapper) bool { return k.KeyID == req.Wrapper.KeyID })
	f.Keys = append([]recoverywrap.PQVaultWrapper{req.Wrapper}, f.Keys...)
	f.json(w, http.StatusOK, f.state())
}

func (f *Fake) append(w http.ResponseWriter, r *http.Request) {
	var req struct {
		VaultID       string          `json:"vault_id"`
		TransactionID string          `json:"transaction_id"`
		Payload       json.RawMessage `json:"payload"`
		CopyID        string          `json:"copy_id"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		http.Error(w, "bad", http.StatusBadRequest)
		return
	}
	if _, found, err := pqc.DecodeStorageMessage(req.Payload); err != nil || !found {
		http.Error(w, "not storage_pq", http.StatusBadRequest)
		return
	}
	if req.VaultID != f.VaultID {
		http.Error(w, "vault", http.StatusConflict)
		return
	}
	if !f.copyAllows(req.CopyID) {
		f.conflict(w, api.CodePQVaultCopyInProgress)
		return
	}
	if i, ok := f.txns[req.TransactionID]; ok {
		f.json(w, http.StatusCreated, f.Records[i])
		return
	}
	if f.FailAppend != nil && f.FailAppend(len(f.Records)) {
		http.Error(w, "down", http.StatusServiceUnavailable)
		return
	}
	f.Seq++
	f.Version++
	rec := api.PQVaultRecord{EventID: "$pqe_" + strconv.FormatInt(f.Seq, 10), VaultID: f.VaultID, Seq: f.Seq, Payload: req.Payload}
	f.txns[req.TransactionID] = len(f.Records)
	f.Records = append(f.Records, rec)
	f.json(w, http.StatusCreated, rec)
}

func (f *Fake) list(w http.ResponseWriter, r *http.Request) {
	if f.FailList {
		http.Error(w, "down", http.StatusServiceUnavailable)
		return
	}
	q := r.URL.Query()
	if q.Get("vault_id") != f.VaultID {
		f.json(w, http.StatusOK, api.PQVaultRecordPage{Entries: []api.PQVaultRecord{}})
		return
	}
	if q.Has("transaction_id") {
		// La búsqueda por transacción: una página de cero o un registro.
		page := api.PQVaultRecordPage{Entries: []api.PQVaultRecord{}}
		if i, ok := f.txns[q.Get("transaction_id")]; ok {
			page.Entries = append(page.Entries, f.Records[i])
		}
		f.json(w, http.StatusOK, page)
		return
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	before := int64(1 << 62)
	if c := q.Get("cursor"); c != "" {
		before, _ = strconv.ParseInt(c, 10, 64)
	}
	page := api.PQVaultRecordPage{Entries: []api.PQVaultRecord{}}
	for i := len(f.Records) - 1; i >= 0; i-- {
		if f.Records[i].Seq >= before {
			continue
		}
		if len(page.Entries) == limit {
			page.NextCursor = strconv.FormatInt(page.Entries[len(page.Entries)-1].Seq, 10)
			break
		}
		page.Entries = append(page.Entries, f.Records[i])
	}
	f.json(w, http.StatusOK, page)
}

func (f *Fake) get(w http.ResponseWriter, r *http.Request, event string) {
	for _, rec := range f.Records {
		if rec.EventID == event && rec.VaultID == r.URL.Query().Get("vault_id") {
			f.json(w, http.StatusOK, rec)
			return
		}
	}
	http.NotFound(w, r)
}

func (f *Fake) vaultKeys(w http.ResponseWriter, r *http.Request) {
	if strings.TrimPrefix(r.URL.Path, "/api/v1/me/vault-keys/") != f.scope.Product {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodGet {
		f.json(w, http.StatusOK, map[string]any{"keys": f.Signing})
		return
	}
	var req api.VaultKeyRegistration
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		http.Error(w, "bad", http.StatusBadRequest)
		return
	}
	keyID, err := req.PublicKey.ID()
	if err != nil {
		http.Error(w, "bad", http.StatusBadRequest)
		return
	}
	if f.beforeHandover(w, req, keyID) {
		return
	}
	generation, previous := int64(1), ""
	if len(f.Signing) > 0 {
		generation, previous = f.Signing[0].Generation+1, f.Signing[0].KeyID
	}
	statement, err := recoverywrap.PQVaultKeyStatement(f.scope.Subject, f.scope.Tenant, f.scope.Product, generation, keyID, previous)
	if err != nil || req.PreviousKeyID != previous || req.PublicKey.Verify(recoverywrap.VaultKeyEnrollContext, statement, req.Proof) != nil {
		http.Error(w, "proof", http.StatusBadRequest)
		return
	}
	if previous != "" && !f.RecentLogin {
		f.json(w, http.StatusForbidden, map[string]string{"code": api.CodeVaultKeyReauth, "message": "reauth"})
		return
	}
	reason := "alta"
	switch {
	case req.Handover:
		reason = "traspaso"
	case previous != "":
		reason = "vaciado"
	}
	public, err := json.Marshal(req.PublicKey)
	if err != nil {
		http.Error(w, "bad", http.StatusBadRequest)
		return
	}
	f.Signing = append([]api.VaultKey{{Generation: generation, KeyID: keyID, PreviousKeyID: previous, PublicKey: public, Reason: reason}}, f.Signing...)
	f.json(w, http.StatusOK, f.Signing[0])
}

// beforeHandover es lo que Sytue ID resuelve antes de verificar firmas
// (antesDelTraspaso): un traspaso solo en `mail` y sin aprobación de la anterior;
// la misma clave otra vez no cambia nada (repetir un traspaso devuelve el hecho,
// pero no se cuelga de una clave que ya estaba sin él); y un traspaso exige
// siempre identificación reciente. Dice si ya contestó.
func (f *Fake) beforeHandover(w http.ResponseWriter, req api.VaultKeyRegistration, keyID string) bool {
	if req.Handover {
		f.Handovers++
	}
	if req.Handover && (f.scope.Product != "mail" || len(req.Continuity) > 0) {
		f.json(w, http.StatusBadRequest, map[string]string{"code": "vault_key_handover", "message": "handover"})
		return true
	}
	if len(f.Signing) > 0 && f.Signing[0].KeyID == keyID {
		if req.Handover && f.Signing[0].Reason != "traspaso" {
			f.json(w, http.StatusBadRequest, map[string]string{"code": "vault_key_handover", "message": "handover"})
			return true
		}
		f.json(w, http.StatusOK, f.Signing[0])
		return true
	}
	if req.Handover && !f.RecentLogin {
		f.json(w, http.StatusForbidden, map[string]string{"code": api.CodeVaultKeyReauth, "message": "reauth"})
		return true
	}
	return false
}

// RecordCount cuenta los registros guardados.
func (f *Fake) RecordCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.Records)
}

// AppendOther publica un registro como lo haría otro cliente (la web), sin
// marca de copia: la otra mitad de una carrera. Respeta las reglas del servidor
// y dice si se publicó.
func (f *Fake) AppendOther(transaction string, payload json.RawMessage) bool {
	if f.VaultID == "" || !f.copyAllows("") {
		return false
	}
	if _, ok := f.txns[transaction]; ok {
		return true
	}
	f.Seq++
	f.Version++
	f.txns[transaction] = len(f.Records)
	f.Records = append(f.Records, api.PQVaultRecord{EventID: "$pqe_" + strconv.FormatInt(f.Seq, 10), VaultID: f.VaultID, Seq: f.Seq, Payload: payload})
	return true
}

// Claims son los identificadores de cada marca puesta o renovada.
func (f *Fake) Claims() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.claims)
}

// CopyMarker es la marca de copia tal como la ve un cliente ahora.
func (f *Fake) CopyMarker() *api.PQVaultCopy {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state().Copy
}

// Resets dice, por cada PUT de claves, si pidió vaciar.
func (f *Fake) Resets() []bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.putKeys)
}
