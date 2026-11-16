// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// Plantillas de respuesta (D-MAIL-06) en el repositorio privado. Son texto que
// escribe quien usa la cuenta, como la firma, así que viven cifradas junto a ella y
// no en la tabla en claro del servicio. Un solo documento con la lista entera: el
// tope (100) y el nombre único se comprueban sobre la misma instantánea que se
// escribe, y dos dispositivos que escriben a la vez dejan dos cabezas que se leen
// como ErrConflict, igual que la firma.

const (
	// TemplateNameMax, TemplateBodyMaxBytes y TemplatesMax son los topes del servicio
	// (services/mail/internal/domain/template.go): la web los comparte.
	TemplateNameMax      = 100
	TemplateBodyMaxBytes = 16 << 10
	TemplatesMax         = 100
)

var (
	// ErrTemplateExists: ya hay una plantilla con ese nombre (sin distinguir mayúsculas).
	ErrTemplateExists = errors.New("mail: reply template already exists")
	// ErrTemplateLimit: la cuenta ya tiene TemplatesMax plantillas.
	ErrTemplateLimit = errors.New("mail: reply template limit reached")
	// ErrInvalidTemplate: nombre vacío, demasiado largo o con controles; cuerpo
	// demasiado largo o no UTF-8.
	ErrInvalidTemplate = errors.New("mail: invalid reply template")
)

// ReplyTemplate es una plantilla de respuesta: un texto PLANO con nombre.
type ReplyTemplate struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Body      string    `json:"body"`
	UpdatedAt time.Time `json:"updated_at"`
}

// normalizeTemplate es NormalizarTemplate del servicio: recorta el nombre y no el cuerpo.
func normalizeTemplate(name, body string) (string, string, error) {
	name = strings.TrimSpace(name)
	if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > TemplateNameMax {
		return "", "", ErrInvalidTemplate
	}
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

func (r *Repository) templates(ctx context.Context, s storage.AtomicCatalogSnapshot) ([]ReplyTemplate, error) {
	raw, err := r.document(ctx, s, "templates", "templates")
	if errors.Is(err, ErrNotFound) {
		return []ReplyTemplate{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	var out []ReplyTemplate
	if json.Unmarshal(raw, &out) != nil || len(out) > TemplatesMax {
		return nil, ErrRecord
	}
	seen := map[string]bool{}
	for _, value := range out {
		if _, err := uuid.Parse(value.ID); err != nil || seen[value.ID] {
			return nil, ErrRecord
		}
		seen[value.ID] = true
		if name, body, err := normalizeTemplate(value.Name, value.Body); err != nil || name != value.Name || body != value.Body {
			return nil, ErrRecord
		}
	}
	if out == nil {
		out = []ReplyTemplate{}
	}
	return out, nil
}

func (r *Repository) writeTemplates(ctx context.Context, s storage.AtomicCatalogSnapshot, values []ReplyTemplate) error {
	slices.SortFunc(values, func(a, b ReplyTemplate) int {
		if c := strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
	payload, err := json.Marshal(values)
	if err != nil {
		return err
	}
	defer clear(payload)
	return r.writeDocument(ctx, s, "templates", "templates", payload, "live")
}

// Templates son las plantillas de la cuenta, por nombre.
func (r *Repository) Templates(parent context.Context) ([]ReplyTemplate, error) {
	var out []ReplyTemplate
	err := r.run(parent, true, func(ctx context.Context, s storage.AtomicCatalogSnapshot) error {
		values, err := r.templates(ctx, s)
		out = values
		return err
	})
	return out, err
}

// SaveTemplate crea (id vacío) o cambia una plantilla y la devuelve como quedó.
func (r *Repository) SaveTemplate(parent context.Context, id, name, body string) (ReplyTemplate, error) {
	name, body, err := normalizeTemplate(name, body)
	if err != nil {
		return ReplyTemplate{}, err
	}
	var saved ReplyTemplate
	err = r.run(parent, true, func(ctx context.Context, s storage.AtomicCatalogSnapshot) error {
		values, err := r.templates(ctx, s)
		if err != nil {
			return err
		}
		index := -1
		for i, value := range values {
			if value.ID == id && id != "" {
				index = i
				continue
			}
			if strings.EqualFold(value.Name, name) {
				return ErrTemplateExists
			}
		}
		saved = ReplyTemplate{ID: id, Name: name, Body: body, UpdatedAt: time.Now().UTC()}
		switch {
		case id == "":
			if len(values) >= TemplatesMax {
				return ErrTemplateLimit
			}
			saved.ID = uuid.NewString()
			values = append(values, saved)
		case index < 0:
			return ErrNotFound
		default:
			values[index] = saved
		}
		return r.writeTemplates(ctx, s, values)
	})
	if err != nil {
		return ReplyTemplate{}, err
	}
	return saved, nil
}

// DeleteTemplate borra una plantilla. Una que ya no está es ErrNotFound.
func (r *Repository) DeleteTemplate(parent context.Context, id string) error {
	return r.run(parent, true, func(ctx context.Context, s storage.AtomicCatalogSnapshot) error {
		values, err := r.templates(ctx, s)
		if err != nil {
			return err
		}
		index := slices.IndexFunc(values, func(value ReplyTemplate) bool { return value.ID == id })
		if index < 0 {
			return ErrNotFound
		}
		return r.writeTemplates(ctx, s, slices.Delete(values, index, index+1))
	})
}
