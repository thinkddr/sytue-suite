// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

// ---------------------------------------------------------------------------
// Previews cerradas con contraseña
// ---------------------------------------------------------------------------
//
// Una preview publica en internet la rama de alguien SIN REVISAR, con sus datos
// de prueba, en una URL que Google puede indexar. Salía abierta igual que
// producción y la única defensa era no crearla.
//
// La contraseña se genera aquí y se enseña UNA vez: lo que se guarda es la línea
// htpasswd con su bcrypt, que es lo que consume el middleware de Traefik. No hay
// forma de recuperarla —ni para nosotros—, y por eso la interfaz lo dice antes de
// cerrar el aviso.

// previewPasswordBytes es la entropía de la contraseña generada. 16 bytes en
// base64 sin relleno son 22 caracteres: se puede pegar en un chat de equipo sin
// que ocupe media pantalla, y adivinarla no está sobre la mesa.
const previewPasswordBytes = 16

// ProtectPreviews cierra las previews del proyecto con una contraseña nueva y la
// devuelve en claro (una sola vez).
//
// Regenerar sobre una protección ya activa es el camino normal para rotarla: la
// anterior deja de valer en el acto, que es justo lo que se pide cuando la
// contraseña se ha ido de las manos.
//
// El cambio NO afecta a las previews YA desplegadas hasta que el reconciliador
// las vuelva a tocar; lo dice la interfaz, en vez de prometer un efecto inmediato
// que el clúster no da.
func (s *Service) ProtectPreviews(ctx context.Context, id uuid.UUID) (string, error) {
	raw := make([]byte, previewPasswordBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("service: generar contraseña de preview: %w", err)
	}
	password := base64.RawURLEncoding.EncodeToString(raw)

	// bcrypt con el coste por defecto: es lo que entiende el BasicAuth de Traefik
	// y lo que evita que un hash filtrado se rompa en un rato.
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("service: cifrar contraseña de preview: %w", err)
	}
	line := domain.PreviewAuthUser + ":" + string(hashed)
	if _, err := s.store.UpdateProject(ctx, id, store.UpdateProjectRecord{
		PreviewAuthHash: &line,
	}); err != nil {
		return "", err
	}
	// Queda registrado el HECHO, nunca el valor: es una credencial.
	s.audit(ctx, "project.preview_protection.enable", id.String(), nil)
	return password, nil
}

// UnprotectPreviews vuelve a dejar las previews abiertas.
func (s *Service) UnprotectPreviews(ctx context.Context, id uuid.UUID) error {
	empty := ""
	if _, err := s.store.UpdateProject(ctx, id, store.UpdateProjectRecord{
		PreviewAuthHash: &empty,
	}); err != nil {
		return err
	}
	// Abrir a internet lo que estaba cerrado es exactamente lo que una auditoría
	// tiene que poder reconstruir.
	s.audit(ctx, "project.preview_protection.disable", id.String(), nil)
	return nil
}
