// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

// IngressKind es el tipo de entrega de la entrada privada del Hub (ADR 0086).
const IngressKind = "hub.receive"

// incoming es el aviso tal como lo cifra el Hub (store.AvisoCifrado en
// services/hub): va entero en los metadatos de la entrega, con el cuerpo vacío.
type incoming struct {
	Version   int               `json:"v"`
	Tenant    string            `json:"tenant"`
	ID        string            `json:"id"`
	Source    string            `json:"source"`
	Type      string            `json:"type"`
	Level     string            `json:"level"`
	Title     string            `json:"title"`
	Body      string            `json:"body,omitempty"`
	URL       string            `json:"url,omitempty"`
	Params    map[string]string `json:"params,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
}

// IngressExecutor guarda en el almacén privado los avisos que el servidor dejó en la
// entrada privada mientras la fuente de One no es `legacy`. Es el ejecutor de dominio
// del trabajador compartido (apps/nucleo/ingress): recibe la entrega ya descifrada y
// el destino fijado por el reclamo, y devuelve la referencia del commit que el Hub
// verifica antes de dar el recibo. El id del aviso es el de la entrega: reintentar
// nunca crea dos avisos ni reinicia uno ya leído (Put).
type IngressExecutor struct{ Repository *Repository }

func (e IngressExecutor) notice(task ingress.Task, metadata []byte, size int64) (Notice, error) {
	if e.Repository == nil || task.Kind != IngressKind || task.Scope != e.Repository.scope || task.ValidateBinding(e.Repository.scope, task.Epoch) != nil || size != 0 {
		return Notice{}, ingress.ErrContract
	}
	var in incoming
	decoder := json.NewDecoder(bytes.NewReader(metadata))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&in) != nil || decoder.More() || in.Version != 1 || in.Tenant != e.Repository.scope.Tenant || in.ID != task.ID {
		return Notice{}, ingress.ErrContract
	}
	n := Notice{
		Version: 1, Scope: e.Repository.scope, ID: in.ID, Source: in.Source, Type: in.Type, Level: in.Level,
		Title: in.Title, Body: in.Body, URL: in.URL, Params: maps.Clone(in.Params), CreatedAt: in.CreatedAt.UTC(),
	}
	if len(n.Params) == 0 {
		n.Params = nil
	}
	if n.validate(e.Repository.scope, n.ID) != nil {
		return Notice{}, ErrNotice
	}
	return n, nil
}

// Reconcile busca el aviso ya guardado: nada (nil), el mismo aviso (su commit) o uno
// distinto con el mismo id (conflicto; nunca elige).
func (e IngressExecutor) Reconcile(ctx context.Context, claim ingress.Claim, metadata []byte, _ io.ReaderAt, size int64) (*storage.Reference, error) {
	if e.Repository == nil || claim.Validate(e.Repository.scope, claim.Task.Epoch) != nil {
		return nil, ingress.ErrContract
	}
	n, err := e.notice(claim.Task, metadata, size)
	if err != nil {
		return nil, err
	}
	return e.reconcile(ctx, claim.Target, n)
}

// ReconcileHandoff es Reconcile para el relevo de un aparato anterior: lo guardado
// tiene que estar en la sala de su asignación.
func (e IngressExecutor) ReconcileHandoff(ctx context.Context, task ingress.Task, previous ingress.Assignment, metadata []byte, _ io.ReaderAt, size int64) (*storage.Reference, error) {
	if !previous.Valid() {
		return nil, ingress.ErrClaim
	}
	n, err := e.notice(task, metadata, size)
	if err != nil {
		return nil, err
	}
	return e.reconcile(ctx, previous.Target, n)
}

func (e IngressExecutor) reconcile(ctx context.Context, target storage.PublicationTarget, n Notice) (*storage.Reference, error) {
	version, err := e.Repository.core.ReadyVersion(noticeKind, n.ID)
	if errors.Is(err, records.ErrNotFound) {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, err
	}
	current, err := e.Repository.current(ctx, n.ID)
	if err != nil {
		return nil, err
	}
	current.Read, current.ReadAt = false, nil
	a, _ := json.Marshal(current)
	b, _ := json.Marshal(n)
	defer clear(a)
	defer clear(b)
	if !bytes.Equal(a, b) {
		return nil, records.ErrConflict
	}
	if version.Reference.RoomID != target.RoomID {
		return nil, ingress.ErrReceipt
	}
	ref := version.Reference
	return &ref, ctx.Err()
}

// Execute guarda el aviso (si no estaba) y relee su commit autenticado.
func (e IngressExecutor) Execute(ctx context.Context, claim ingress.Claim, metadata []byte, source io.ReaderAt, size int64) (storage.Reference, error) {
	accepted, err := e.Reconcile(ctx, claim, metadata, source, size)
	if err != nil {
		return storage.Reference{}, err
	}
	if accepted == nil {
		n, err := e.notice(claim.Task, metadata, size)
		if err != nil {
			return storage.Reference{}, err
		}
		if err := e.Repository.Put(ctx, n); err != nil {
			return storage.Reference{}, err
		}
		if accepted, err = e.Reconcile(ctx, claim, metadata, source, size); err != nil {
			return storage.Reference{}, err
		}
	}
	if accepted == nil {
		return storage.Reference{}, ingress.ErrReceipt
	}
	return *accepted, nil
}
