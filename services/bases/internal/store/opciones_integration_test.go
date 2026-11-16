// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store

import (
	"slices"
	"testing"

	"git.sytue.eu/sytue/one/services/bases/internal/domain"
)

func (e *entorno) campos() []domain.Campo {
	e.t.Helper()
	ts, err := e.s.ListarTablas(e.ctx, ana, e.base.ID)
	e.ok(err)
	return ts[0].Fields
}

// Quitar una opción no borra el valor de los registros que la usan: se queda hasta
// que alguien lo cambie, y se puede seguir escribiendo junto a las demás.
func TestCambiarOpcionesConservaLasRetiradas(t *testing.T) {
	e := nuevo(t)
	est, err := e.s.CrearCampo(e.ctx, ana, e.tabl.ID, domain.EntradaCampo{Name: "Estado", Kind: domain.TipoSeleccion, Choices: []string{"nuevo", "viejo"}})
	e.ok(err)
	tags, err := e.s.CrearCampo(e.ctx, ana, e.tabl.ID, domain.EntradaCampo{Name: "Etiquetas", Kind: domain.TipoMultiple, Choices: []string{"a", "z"}})
	e.ok(err)
	r := e.registro(ana, e.tabl.ID, dato(est.ID, `"viejo"`, tags.ID, `["z"]`))

	e.ok(e.s.CambiarOpciones(e.ctx, ana, est.ID, []string{" urgente ", "nuevo"}))
	e.ok(e.s.CambiarOpciones(e.ctx, ana, tags.ID, []string{"a", "b"}))
	for _, c := range e.campos() {
		if c.ID == est.ID && !slices.Equal(c.Choices, []string{"urgente", "nuevo"}) {
			t.Errorf("opciones guardadas: %v", c.Choices)
		}
	}
	got, err := e.s.ObtenerRegistro(e.ctx, ana, r.ID)
	e.ok(err)
	if string(got.Data[est.ID]) != `"viejo"` {
		t.Errorf("el valor retirado se ha perdido: %s", got.Data[est.ID])
	}
	// Añadir «b» a una múltiple que tiene la retirada «z» no falla.
	got, err = e.s.ActualizarRegistro(e.ctx, ana, r.ID, got.Rev, dato(tags.ID, `["z","b"]`), nil)
	e.ok(err)
	if string(got.Data[tags.ID]) != `["b","z"]` {
		t.Errorf("múltiple: %s", got.Data[tags.ID])
	}
	// Pero otra retirada que no tenía, no; y por celda pasa lo mismo.
	_, err = e.s.ActualizarRegistro(e.ctx, ana, r.ID, got.Rev, dato(est.ID, `"inventada"`), nil)
	esperar(t, err, domain.ErrInvalido, "una opción que no existe")
	_, err = e.s.EscribirCelda(e.ctx, ana, r.ID, Celda{FieldID: tags.ID, Value: []byte(`["a","b","z"]`)})
	e.ok(err)

	esperar(t, e.s.CambiarOpciones(e.ctx, ana, e.txt.ID, []string{"x"}), domain.ErrInvalido, "opciones en un texto")
	esperar(t, e.s.CambiarOpciones(e.ctx, ana, est.ID, []string{"x", "x"}), domain.ErrInvalido, "opciones repetidas")
	esperar(t, e.s.CambiarOpciones(e.ctx, ana, est.ID, nil), domain.ErrInvalido, "sin opciones")
	esperar(t, e.s.CambiarOpciones(e.ctx, bea, est.ID, []string{"x"}), domain.ErrNoExiste, "quien no ve la base")
	esperar(t, e.s.CambiarOpciones(e.ctx, gus, est.ID, []string{"x"}), domain.ErrNoExiste, "otra organización")
}

func TestReordenarCampos(t *testing.T) {
	e := nuevo(t)
	fecha, err := e.s.CrearCampo(e.ctx, ana, e.tabl.ID, domain.EntradaCampo{Name: "Fecha", Kind: domain.TipoFecha})
	e.ok(err)
	orden := []string{fecha.ID, e.txt.ID, e.num.ID}
	e.ok(e.s.ReordenarCampos(e.ctx, ana, e.tabl.ID, orden))
	var ids []string
	for _, c := range e.campos() {
		ids = append(ids, c.ID)
	}
	if !slices.Equal(ids, orden) {
		t.Errorf("orden: %v, quería %v", ids, orden)
	}
	// Un orden que no es exactamente el de la tabla (falta uno, sobra uno) no se aplica.
	esperar(t, e.s.ReordenarCampos(e.ctx, ana, e.tabl.ID, orden[:2]), domain.ErrConflicto, "falta un campo")
	esperar(t, e.s.ReordenarCampos(e.ctx, ana, e.tabl.ID, []string{fecha.ID, e.txt.ID, fecha.ID}), domain.ErrInvalido, "repetido")
	esperar(t, e.s.ReordenarCampos(e.ctx, ana, e.tabl.ID, []string{fecha.ID, e.txt.ID, "00000000-0000-0000-0000-000000000000"}), domain.ErrConflicto, "uno ajeno")
	esperar(t, e.s.ReordenarCampos(e.ctx, gus, e.tabl.ID, orden), domain.ErrNoExiste, "otra organización")
}
