// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package tienda

import (
	"context"
	"regexp"
	"sort"
	"strings"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/validation"
	"git.sytue.eu/sytue/one/services/pages/internal/domain"
	"git.sytue.eu/sytue/one/services/pages/internal/ir"
)

// La bandeja de formularios (decisión del dueño del 2026-10-16): cada formulario de la
// web con sus respuestas, y a quién avisa por correo. Hasta entonces los envíos se
// guardaban y nadie los veía.

// maxAvisadosFormulario es el tope de direcciones a las que avisa un formulario: el aviso
// sale por el relé de la suite y no es una lista de correo.
const maxAvisadosFormulario = 10

// MaxEnviosBandeja es cuántas respuestas, las más nuevas, enseña y exporta la bandeja.
// ponytail: sin paginar; con más de mil respuestas en un formulario, paginar por fecha.
const MaxEnviosBandeja = 1000

var reFormID = regexp.MustCompile(`^[a-z0-9]{10}$`)

// Formularios lista los formularios de la versión en vivo y los que, sin estar ya en
// ella, guardan respuestas. Los de la web van primero, por su identificador.
func (s *Servicio) Formularios(ctx context.Context, tenant string, siteID uuid.UUID) ([]domain.FormInbox, error) {
	raw, resumen, err := s.store.Formularios(ctx, tenant, siteID)
	if err != nil {
		return nil, err
	}
	enVivo := map[string]*ir.Nodo{}
	if raw != nil {
		if sitio, err := ir.Decodificar(raw); err == nil {
			enVivo = ir.Formularios(sitio)
		}
	}
	out := []domain.FormInbox{}
	for id, n := range enVivo {
		f := domain.FormInbox{FormID: id, Name: n.Name, Live: true, Fields: n.Fields}
		if r, ok := resumen[id]; ok {
			f.Notify, f.Total = correos(r.Notify), r.Total
		}
		out = append(out, f)
	}
	for id, r := range resumen {
		if _, ok := enVivo[id]; !ok && r.Total > 0 {
			out = append(out, domain.FormInbox{FormID: id, Notify: correos(r.Notify), Total: r.Total})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Live != out[j].Live {
			return out[i].Live
		}
		return out[i].FormID < out[j].FormID
	})
	for i := range out {
		if out[i].Fields == nil {
			out[i].Fields = []ir.CampoFormulario{}
		}
		if out[i].Notify == nil {
			out[i].Notify = []string{}
		}
	}
	return out, nil
}

func correos(ps []domain.StoreNotifyPerson) []string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		if p.Email != "" {
			out = append(out, p.Email)
		}
	}
	return out
}

// GuardarAvisosFormulario fija las direcciones a las que avisa un formulario: el
// despachador manda a cada una el aviso de cada respuesta, con el tope de 200 al día por
// sitio (WEB-07). Una lista vacía deja de avisar; las respuestas se siguen guardando.
func (s *Servicio) GuardarAvisosFormulario(ctx context.Context, tenant string, siteID uuid.UUID, formID string, lista []string) error {
	if !reFormID.MatchString(formID) {
		return domain.ErrFormularioNoExiste
	}
	if len(lista) > maxAvisadosFormulario {
		return domain.ErrAvisadosInvalidos
	}
	vistos := map[string]bool{}
	notify := []domain.StoreNotifyPerson{}
	for _, c := range lista {
		c = strings.ToLower(strings.TrimSpace(c))
		if !validation.IsEmail(c) {
			return domain.ErrAvisadosInvalidos
		}
		if !vistos[c] {
			vistos[c] = true
			notify = append(notify, domain.StoreNotifyPerson{Email: c})
		}
	}
	return s.store.GuardarAvisosFormulario(ctx, tenant, siteID, formID, notify)
}

// EnviosFormulario devuelve las respuestas de un formulario, las más nuevas primero.
func (s *Servicio) EnviosFormulario(ctx context.Context, tenant string, siteID uuid.UUID, formID string) ([]domain.FormSubmission, error) {
	if !reFormID.MatchString(formID) {
		return nil, domain.ErrFormularioNoExiste
	}
	return s.store.ListarEnvios(ctx, tenant, siteID, formID, MaxEnviosBandeja)
}

// EtiquetaCampo es el rótulo de un campo en un idioma, con el mismo respaldo que el
// resto de textos de la web.
func EtiquetaCampo(c ir.CampoFormulario, idioma string) string {
	if t := textoEnIdioma(c.Label, idioma); t != "" {
		return t
	}
	return c.ID
}
