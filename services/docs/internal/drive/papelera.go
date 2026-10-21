// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// LA PAPELERA DE UN DOCUMENTO ES LA DE SU FICHERO (decidido por el dueño,
// 2026-09-28). Tirar, restaurar o purgar un documento desde Docs hace lo mismo con
// su .odt/.ods/.odp en Drive; lo contrario lo trae la cola de Drive (su mig 0027).
//
// VA POR LA PUERTA INTERNA, con un secreto propio de Docs y Drive, y no con el
// token de quien pide, al revés que el resto
// de este paquete: un fichero en la papelera no existe para la API pública de
// Drive —no se puede preguntar de quién es— y la verja que importa la pone Drive
// igual: solo actúa sobre un fichero DEL DUEÑO del documento y en el formato de su
// tipo, y solo purga lo que ya está en la papelera y si esa persona no está bajo
// retención legal. La autorización de la persona (ser dueña del documento) la ha
// hecho Docs antes de llamar.

// SinFichero es el id con el que se pregunta por un documento que no tiene fichero:
// antes de purgarlo, Drive dice si hay una retención legal sobre su dueño.
const SinFichero = "00000000-0000-0000-0000-000000000000"

// PapeleraActiva dice si hay Drive y secreto para la papelera compartida. Sin él,
// la papelera de Docs va por su cuenta, como antes.
func (c *Cliente) PapeleraActiva() bool { return c != nil && c.secreto != "" }

// Acciones sobre el fichero de un documento.
const (
	AccionTirar     = "trash"
	AccionRestaurar = "restore"
	AccionPurgar    = "purge"
	// AccionComprobar dice qué haría AccionTirar, sin hacerlo.
	AccionComprobar = "check"
)

// Lo que contesta Drive (service.DocsAccion).
const (
	ResultadoHecho    = "done"
	ResultadoYaEstaba = "unchanged"
	ResultadoNoAplica = "not_applicable"
	// ResultadoVivo: al purgar, el fichero sigue vivo en la unidad y no se tocó.
	ResultadoVivo = "live"
	// ResultadoModificado: el fichero cambió después de borrarse el documento.
	ResultadoModificado = "modified"
)

// ErrSinPuerta es un 404 de Drive a una purga o una comprobación: un Drive sin esta
// puerta, o una URL que pasa por el ingress (que la esconde). Sin ella no se sabe si
// hay retención legal, así que NO vale como «no aplica».
var ErrSinPuerta = errors.New("drive: la puerta de la papelera no responde (404)")

var (
	// ErrNombreOcupado: al restaurar, ya hay otro fichero con ese nombre en su
	// carpeta. Lo tiene que resolver la persona en Drive.
	ErrNombreOcupado = errors.New("drive: ya hay un fichero con ese nombre en su carpeta")
	// ErrRetencion: una retención legal impide purgar el fichero.
	ErrRetencion = errors.New("drive: el fichero está bajo retención legal")
)

// AccionFichero aplica `accion` al fichero `id` del documento de tipo `kind` de
// `ownerSub` en `tenant`. Que no haya fichero, o que no sea de esa persona, no es un
// error: Drive contesta que no aplica y Docs sigue con lo suyo. Un 404 tampoco: es
// un Drive anterior a esta puerta, y no puede dejar a nadie sin poder borrar.
func (c *Cliente) AccionFichero(ctx context.Context, tenant, ownerSub, kind, id, accion string) error {
	_, err := c.ResultadoFichero(ctx, tenant, ownerSub, kind, id, accion, time.Time{})
	return err
}

// ResultadoFichero es AccionFichero diciendo qué pasó (Resultado*). `borradoEn`
// (opcional) es cuándo se borró el documento: con él, Drive no tira un fichero
// modificado después. Un 404 es ResultadoNoAplica al tirar o restaurar (un Drive
// anterior no puede dejar a nadie sin borrar) y ErrSinPuerta al purgar o comprobar.
func (c *Cliente) ResultadoFichero(ctx context.Context, tenant, ownerSub, kind, id, accion string, borradoEn time.Time) (string, error) {
	cuerpo := map[string]string{"tenant": tenant, "owner_sub": ownerSub, "kind": kind, "action": accion}
	if !borradoEn.IsZero() {
		cuerpo["deleted_at"] = borradoEn.UTC().Format(time.RFC3339Nano)
	}
	raw, err := json.Marshal(cuerpo)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/internal/docs/files/"+url.PathEscape(id), bytes.NewReader(raw))
	if err != nil {
		return "", fmt.Errorf("drive: construir petición: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.secreto)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("drive: solicitud: %w", err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		var out struct {
			Result string `json:"result"`
		}
		if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&out); err != nil {
			return "", fmt.Errorf("drive: decodificar: %w", err)
		}
		return out.Result, nil
	case http.StatusNotFound:
		if accion == AccionPurgar || accion == AccionComprobar {
			return "", ErrSinPuerta
		}
		return ResultadoNoAplica, nil
	case http.StatusConflict:
		return "", ErrNombreOcupado
	case http.StatusLocked:
		return "", ErrRetencion
	}
	return "", fmt.Errorf("drive: respuesta %d a %s", resp.StatusCode, accion)
}
