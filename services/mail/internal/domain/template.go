// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Template es una plantilla de respuesta de la cuenta (migración 0021): un texto
// con nombre que el redactor inserta donde está el cursor. Texto PLANO, como la
// firma: el cliente lo escapa al convertirlo en HTML, así que aquí nunca se
// guarda marcado que luego acabe sin sanear en el cuerpo de un correo.
type Template struct {
	ID        uuid.UUID `json:"id"`
	Tenant    string    `json:"-"`
	AccountID uuid.UUID `json:"-"`
	Name      string    `json:"name"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

const (
	// TemplateNameMax es el tope del nombre, en caracteres: es el rótulo de una
	// entrada de menú, no un asunto.
	TemplateNameMax = 100
	// TemplateBodyMaxBytes acota el cuerpo. 16 KiB son varias páginas de texto:
	// de sobra para una respuesta tipo, y evita que la tabla sirva de almacén.
	TemplateBodyMaxBytes = 16 << 10
	// TemplatesPorCuenta es cuántas puede tener una cuenta. Un menú con más de cien
	// entradas ya no se recorre, y sin tope cualquiera podría llenar la base.
	TemplatesPorCuenta = 100
)

var (
	// ErrTemplateExists: ya hay una plantilla con ese nombre en la cuenta (sin
	// distinguir mayúsculas).
	ErrTemplateExists = errors.New("mail: la plantilla ya existe")
	// ErrTemplateLimite: la cuenta ya tiene TemplatesPorCuenta plantillas.
	ErrTemplateLimite = errors.New("mail: límite de plantillas alcanzado")
	// ErrInvalidTemplate: nombre vacío o demasiado largo, o cuerpo demasiado largo.
	ErrInvalidTemplate = errors.New("mail: plantilla inválida")
)

// NormalizarTemplate recorta el nombre y valida nombre y cuerpo. El cuerpo NO se
// recorta: los saltos de línea del final pueden ser a propósito (dejan sitio para
// seguir escribiendo debajo).
func NormalizarTemplate(name, body string) (string, string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > TemplateNameMax || !utf8.ValidString(name) {
		return "", "", ErrInvalidTemplate
	}
	// Sin caracteres de control en el nombre: es una línea de menú.
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return "", "", ErrInvalidTemplate
		}
	}
	if len(body) > TemplateBodyMaxBytes || !utf8.ValidString(body) || strings.ContainsRune(body, 0) {
		return "", "", ErrInvalidTemplate
	}
	return name, body, nil
}
