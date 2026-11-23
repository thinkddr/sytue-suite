// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/recoverywrap"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/vaultproof"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// Las acciones de entrada firmadas por una bóveda post-cuántica (ADR 0087). La
// autoridad ya no es la firma cruzada de Matrix sino la clave de firma que Sytue
// ID tiene registrada como activa para esa persona, organización y producto.

// ClavesBoveda da la clave activa de una bóveda. La implementación de verdad
// pregunta a Sytue ID (ClavesBovedaID); las pruebas ponen la suya.
type ClavesBoveda interface {
	Activa(ctx context.Context, scope storage.Scope) (ClaveBoveda, error)
}

// ClaveBoveda es la clave activa tal como la registró Sytue ID: lo que verifica
// (Registered) y por qué y cuándo entró, que es lo que decide un traspaso.
type ClaveBoveda struct {
	vaultproof.Registered
	Motivo string
	Creada time.Time
}

// ErrSinClaveDeBoveda: Sytue ID no tiene ninguna clave registrada para esa bóveda.
var ErrSinClaveDeBoveda = errors.New("store: la bóveda no tiene clave registrada en Sytue ID")

// SetClavesBoveda activa la verificación de acciones firmadas por bóvedas. Sin
// ella, una prueba de bóveda se rechaza como no verificada.
func (s *Store) SetClavesBoveda(c ClavesBoveda) { s.clavesBoveda = c }

// verifyVaultStorageAction verifica una prueba de bóveda. Solo vale en un ámbito
// sin autoridad de Matrix o con el traspaso hecho (baseDeBoveda), y con la
// generación contada encima de la de Matrix que había al traspasar.
func (s *Store) verifyVaultStorageAction(ctx context.Context, scope storage.Scope, expected matrixproof.Action, proof matrixproof.SignedAction) (matrixproof.Authorization, error) {
	if s.clavesBoveda == nil || expected.Scope != scope || proof.Action != expected {
		return matrixproof.Authorization{}, domain.ErrForbidden
	}
	base, err := s.baseDeBoveda(ctx, scope.Subject, scope.Product)
	if err != nil {
		return matrixproof.Authorization{}, err
	}
	clave, err := s.clavesBoveda.Activa(ctx, scope)
	if errors.Is(err, ErrSinClaveDeBoveda) {
		return matrixproof.Authorization{}, matrixproof.ErrUnverified
	}
	if err != nil {
		return matrixproof.Authorization{}, domain.ErrMatrixUnavailable
	}
	reg, err := registroSobre(clave, base)
	if err != nil {
		return matrixproof.Authorization{}, err
	}
	return vaultproof.Verify(expected, proof, reg, time.Now())
}

// ClavesBovedaID pregunta a Sytue ID por `POST /internal/vault-keys/lookup`.
type ClavesBovedaID struct {
	url, secreto string
	http         *http.Client
}

// NuevasClavesBovedaID devuelve nil sin secreto: entonces no hay verificación de
// bóvedas y sus pruebas se rechazan.
func NuevasClavesBovedaID(base, secreto string) *ClavesBovedaID {
	if secreto == "" {
		return nil
	}
	return &ClavesBovedaID{
		url: strings.TrimRight(base, "/") + "/internal/vault-keys/lookup", secreto: secreto,
		http: &http.Client{Timeout: 3 * time.Second},
	}
}

// Activa devuelve la clave activa de la bóveda del ámbito. Sin caché: una
// autorización es siempre una comprobación fresca (ver VerifyMatrixStorageAction).
func (c *ClavesBovedaID) Activa(ctx context.Context, scope storage.Scope) (ClaveBoveda, error) {
	var out ClaveBoveda
	cuerpo, err := json.Marshal(map[string]string{"subject": scope.Subject, "tenant": scope.Tenant, "product": scope.Product})
	if err != nil {
		return out, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(cuerpo))
	if err != nil {
		return out, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.secreto)
	res, err := c.http.Do(req)
	if err != nil {
		return out, err
	}
	defer func() { _ = res.Body.Close() }()
	switch res.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return out, ErrSinClaveDeBoveda
	default:
		return out, fmt.Errorf("sytue id: claves de bóveda: HTTP %d", res.StatusCode)
	}
	var vista struct {
		Generation int64                         `json:"generation"`
		KeyID      string                        `json:"key_id"`
		PublicKey  recoverywrap.PQVaultVerifyKey `json:"public_key"`
		Reason     string                        `json:"reason"`
		CreatedAt  time.Time                     `json:"created_at"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 64<<10)).Decode(&vista); err != nil {
		return out, err
	}
	if id, err := vista.PublicKey.ID(); err != nil || id != vista.KeyID || vista.Generation < 1 {
		return out, errors.New("sytue id: clave de bóveda incoherente")
	}
	return ClaveBoveda{
		Registered: vaultproof.Registered{Generation: vista.Generation, KeyID: vista.KeyID, Public: vista.PublicKey},
		Motivo:     vista.Reason, Creada: vista.CreatedAt,
	}, nil
}
