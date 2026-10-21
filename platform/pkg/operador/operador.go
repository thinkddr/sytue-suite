// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package operador abre a quien OPERA la plataforma lo que un servicio guarda de
// sus clientes —hoy, las colas de denuncias de Cast y de Rooms (ADR 0073 y
// 0074, «Cola del operador»)— y deja la traza de cada acceso.
//
// Quién es operador lo sabe Sytue ID, no el servicio: allowlist
// `platform_admins`, segundo factor y elevación viva, mirados en cada petición.
// Así que el servicio no lo deduce de un rol ni de un claim: se lo pregunta con
// el token de quien llama (`POST /api/v1/platform/moderation/access`), diciendo
// qué va a hacer. Sytue ID lo apunta en su registro de plataforma —y en el de la
// organización afectada— y contesta 204. Cualquier otra cosa es «no»: si Sytue ID
// no contesta, o no puede escribir el registro, no se enseña nada.
//
// Y antes de preguntar, el token tiene que ser de la consola de plataforma
// (`Audiencia`): un operador elevado que navega por la web de Cast no abre la
// cola de todas las organizaciones desde ahí, sino solo desde
// platform.<dominio>, que además solo se alcanza por el túnel administrativo.
package operador

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"
)

// Audiencia es el client OIDC de la consola de plataforma.
const Audiencia = "sytue-platform"

// ErrNoEsOperador: quien llama no opera la plataforma ahora mismo, o su token no
// es de la consola de plataforma.
var ErrNoEsOperador = errors.New("operador: no opera la plataforma")

// Acceso es lo que se va a hacer, tal como queda en la traza.
type Acceso struct {
	// Servicio: "cast" o "rooms". Accion: "reports.list" o "report.resolve".
	Servicio, Accion string
	// Org es la organización dueña de lo que se toca; vacía al listar.
	Org string
	// Objetivo es el identificador de lo que se toca (la denuncia).
	Objetivo string
	Detalle  map[string]string
}

// Autorizar pregunta y deja la traza. nil = sí, y la traza está escrita.
// ErrNoEsOperador = no. Otro error = no se pudo saber, que también es no.
type Autorizar func(ctx context.Context, token string, a Acceso) error

// DeLaConsola dice si un token es de la consola de plataforma, por su `aud`.
func DeLaConsola(audiencia []string) bool { return slices.Contains(audiencia, Audiencia) }

// SytueID es el Autorizar real contra la base INTERNA de Sytue ID. Con baseURL
// vacía no hay a quién preguntar y siempre deniega.
func SytueID(baseURL string) Autorizar {
	baseURL = strings.TrimRight(baseURL, "/")
	cliente := &http.Client{Timeout: 5 * time.Second}
	return func(ctx context.Context, token string, a Acceso) error {
		if baseURL == "" || token == "" {
			return ErrNoEsOperador
		}
		cuerpo, err := json.Marshal(map[string]any{
			"service": a.Servicio, "action": a.Accion, "org": a.Org, "target": a.Objetivo, "detail": a.Detalle,
		})
		if err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/v1/platform/moderation/access", bytes.NewReader(cuerpo))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		resp, err := cliente.Do(req)
		if err != nil {
			return fmt.Errorf("operador: sytue id: %w", err)
		}
		defer func() { _ = resp.Body.Close() }()
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<14))
		switch resp.StatusCode {
		case http.StatusNoContent:
			return nil
		case http.StatusUnauthorized, http.StatusForbidden:
			return ErrNoEsOperador
		default:
			return fmt.Errorf("operador: sytue id respondió %d", resp.StatusCode)
		}
	}
}
