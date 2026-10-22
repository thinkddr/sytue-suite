// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package docssync lleva a Sytue Docs lo que le pasa en la papelera de Drive al
// fichero de un documento: tirarlo, sacarlo o purgarlo hace lo mismo con el
// documento (mig 0027, decidido por el dueño el 2026-09-28).
//
// El disparador de la migración deja en `docs_sync_pending` QUÉ fichero cambió;
// este worker lee su estado de AHORA y se lo entrega a Docs por su puerta interna
// (`POST /internal/drive/files`, con el secreto propio de Docs y Drive). Entregar el estado y
// no el cambio es lo que lo hace idempotente y sin carreras: si alguien tira y
// restaura antes de que se entregue, a Docs le llega «vivo» una sola vez, y
// repetir una entrega no cambia nada.
//
// Lo que no se entrega se queda en la cola y se reintenta en la vuelta siguiente:
// una caída de Docs retrasa la papelera, no la desincroniza.
package docssync

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

// RutaDocs es la puerta interna de Docs que recibe los estados.
const RutaDocs = "/internal/drive/files"

// Cola es lo que el worker necesita del store.
type Cola interface {
	PendientesDocs(ctx context.Context, limit int) ([]store.PendienteDocs, error)
	QuitarPendientesDocs(ctx context.Context, entregados []store.PendienteDocs) error
	AplazarPendientesDocs(ctx context.Context, fallidos []store.PendienteDocs) ([]store.PendienteDocs, error)
}

// Worker vacía la cola hacia Docs cada `intervalo`.
type Worker struct {
	cola      Cola
	docsURL   string
	secreto   string
	http      *http.Client
	intervalo time.Duration
}

// Nuevo construye el worker. Sin URL de Docs o sin el secreto de las dos devuelve
// nil: no hay a quién entregar, o Docs lo rechazaría todo.
func Nuevo(cola Cola, docsURL, secreto string, intervalo time.Duration) *Worker {
	docsURL = strings.TrimRight(strings.TrimSpace(docsURL), "/")
	if docsURL == "" || secreto == "" {
		return nil
	}
	if intervalo <= 0 {
		intervalo = 5 * time.Second
	}
	return &Worker{
		cola: cola, docsURL: docsURL, secreto: secreto,
		http: &http.Client{Timeout: 15 * time.Second}, intervalo: intervalo,
	}
}

// Run entrega hasta que el contexto se cancele.
func (w *Worker) Run(ctx context.Context) error {
	t := time.NewTicker(w.intervalo)
	defer t.Stop()
	for {
		if err := w.Vuelta(ctx); err != nil {
			log.From(ctx).Warn("drive: entregar la papelera a Docs", "error", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
		}
	}
}

// fichero es lo que Docs recibe de cada uno.
type fichero struct {
	ID       string `json:"id"`
	Estado   string `json:"state"`
	OwnerSub string `json:"owner_sub"`
	Name     string `json:"name"`
}

// Vuelta entrega un lote. Lo entregado a una organización se quita de la cola
// aunque otra falle: cada una es su propia petición. Lo que falla se aplaza, cada
// vez más, y tras store.MaxIntentosDocs se da por perdido y se registra.
func (w *Worker) Vuelta(ctx context.Context) error {
	pendientes, err := w.cola.PendientesDocs(ctx, 0)
	if err != nil || len(pendientes) == 0 {
		return err
	}
	porTenant := map[string][]store.PendienteDocs{}
	var orden []string
	for _, p := range pendientes {
		if _, ok := porTenant[p.Tenant]; !ok {
			orden = append(orden, p.Tenant)
		}
		porTenant[p.Tenant] = append(porTenant[p.Tenant], p)
	}
	var entregados, fallidos []store.PendienteDocs
	var primerFallo error
	for _, tenant := range orden {
		lote := porTenant[tenant]
		if err := w.entregar(ctx, tenant, lote); err != nil {
			if primerFallo == nil {
				primerFallo = fmt.Errorf("organización %s: %w", tenant, err)
			}
			fallidos = append(fallidos, lote...)
			continue
		}
		entregados = append(entregados, lote...)
	}
	if err := w.cola.QuitarPendientesDocs(ctx, entregados); err != nil {
		return err
	}
	perdidos, err := w.cola.AplazarPendientesDocs(ctx, fallidos)
	if err != nil {
		return err
	}
	for _, p := range perdidos {
		log.From(ctx).Error("drive: la papelera de este fichero no llegó a Docs",
			"tenant", p.Tenant, "file_id", p.FileID.String(), "intentos", store.MaxIntentosDocs)
	}
	return primerFallo
}

func (w *Worker) entregar(ctx context.Context, tenant string, lote []store.PendienteDocs) error {
	cuerpo := struct {
		Tenant string    `json:"tenant"`
		Files  []fichero `json:"files"`
	}{Tenant: tenant, Files: make([]fichero, len(lote))}
	for i, p := range lote {
		cuerpo.Files[i] = fichero{ID: p.FileID.String(), Estado: p.Estado, OwnerSub: p.OwnerSub, Name: p.Name}
	}
	raw, err := json.Marshal(cuerpo)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.docsURL+RutaDocs, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+w.secreto)
	req.Header.Set("Content-Type", "application/json")
	resp, err := w.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("docs respondió %d", resp.StatusCode)
	}
	return nil
}
