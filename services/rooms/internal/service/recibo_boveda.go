// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/vaultproof"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// El recibo de un ACK del correo cuando el correo vive en la bóveda (ADR 0087).
//
// Con Matrix, Rooms comprueba que la publicación es un evento cifrado del
// dispositivo de la persona. En la bóveda la publicación es un registro de Drive:
// Rooms le pregunta a Drive si ese registro está en la bóveda actual de la persona
// y si el destino es el de su clave activa. Cualquier duda rechaza el ACK: el
// correo se queda en la cola y se vuelve a entregar, nunca se da por guardado.

// RecibosBoveda comprueba un recibo contra la bóveda. La de verdad es RecibosBovedaDrive.
type RecibosBoveda interface {
	Verificar(ctx context.Context, scope storage.Scope, target storage.PublicationTarget, receipt storage.Reference) error
}

// SetRecibosBoveda activa los recibos de bóveda. Sin él se rechazan.
func (s *Service) SetRecibosBoveda(r RecibosBoveda) { s.recibosBoveda = r }

// verificarRecibo elige el camino del recibo por la autoridad del ámbito: la de una
// bóveda va a Drive; cualquier otra (o ninguna) sigue por Matrix, como siempre.
func verificarRecibo(ctx context.Context, scope storage.Scope, autoridad matrixproof.Authority, errAutoridad error, target storage.PublicationTarget, receipt storage.Reference, recibos RecibosBoveda, matrix func() error) error {
	if errAutoridad != nil || !vaultproof.IsAuthority(autoridad) {
		return matrix()
	}
	if autoridad.RequireActive(scope, autoridad.Generation) != nil {
		return matrixproof.ErrUnverified
	}
	if !target.Valid() || !receipt.Valid() || receipt.RoomID != target.RoomID {
		return domain.ErrInvalidInput
	}
	if recibos == nil {
		return domain.ErrMatrixUnavailable
	}
	return recibos.Verificar(ctx, scope, target, receipt)
}

// RecibosBovedaDrive pregunta a Drive por `POST /internal/pq-vault/receipt`.
type RecibosBovedaDrive struct {
	url, secreto string
	http         *http.Client
}

// NuevosRecibosBovedaDrive devuelve nil sin secreto: entonces no hay recibos de bóveda.
func NuevosRecibosBovedaDrive(base, secreto string) *RecibosBovedaDrive {
	if secreto == "" || base == "" {
		return nil
	}
	return &RecibosBovedaDrive{
		url: strings.TrimRight(base, "/") + "/internal/pq-vault/receipt", secreto: secreto,
		http: &http.Client{Timeout: 5 * time.Second},
	}
}

// Verificar: 204 es que sí; 404, que no (ErrUnverified); lo demás, que no se sabe.
func (c *RecibosBovedaDrive) Verificar(ctx context.Context, scope storage.Scope, target storage.PublicationTarget, receipt storage.Reference) error {
	cuerpo, err := json.Marshal(map[string]any{"scope": scope, "target": target, "receipt": receipt})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(cuerpo))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.secreto)
	res, err := c.http.Do(req)
	if err != nil {
		return domain.ErrMatrixUnavailable
	}
	defer func() { _ = res.Body.Close() }()
	switch res.StatusCode {
	case http.StatusNoContent:
		return nil
	case http.StatusNotFound:
		return matrixproof.ErrUnverified
	default:
		return domain.ErrMatrixUnavailable
	}
}
