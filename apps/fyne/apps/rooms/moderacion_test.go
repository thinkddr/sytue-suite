// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"slices"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// servicioModerador es el doble de la sala MÁS las rutas de denunciar y bloquear.
type servicioModerador struct {
	*servicioFalso
	bloqueos []api.RoomsBlock
}

func (s *servicioModerador) ReportMessage(_ context.Context, m, motivo, nota string) error {
	s.apuntar("ReportMessage", m, motivo, nota)
	return nil
}

func (s *servicioModerador) ReportMember(_ context.Context, c, p, motivo, nota string) error {
	s.apuntar("ReportMember", c, p, motivo, nota)
	return nil
}

func (s *servicioModerador) Block(_ context.Context, c, p string) (api.RoomsBlock, error) {
	s.apuntar("Block", c, p)
	return api.RoomsBlock{Subject: p, Name: p}, nil
}

func (s *servicioModerador) Blocks(context.Context) ([]api.RoomsBlock, error) {
	return s.bloqueos, nil
}

func (s *servicioModerador) Unblock(_ context.Context, p string) error {
	s.apuntar("Unblock", p)
	return nil
}

func (s *servicioModerador) Reports(context.Context) ([]api.RoomsReport, error) { return nil, nil }

func (s *servicioModerador) ResolveReport(_ context.Context, id, accion string) error {
	s.apuntar("ResolveReport", id, accion)
	return nil
}

// rotulos son los textos de las entradas de un menú, sin los separadores.
func rotulos(entradas []fyne.CanvasObject) []string {
	var out []string
	for _, e := range entradas {
		if it, ok := e.(*ui.ElementoMenu); ok {
			out = append(out, it.Texto)
		}
	}
	return out
}

func mensajeDeLaLista(t *testing.T, sa *Sala, id string) Mensaje {
	t.Helper()
	ms := sa.P.Canal.Historial.Mensajes
	i := slices.IndexFunc(ms, func(m Mensaje) bool { return m.ID == id })
	if i < 0 {
		t.Fatalf("el mensaje %s no está en el historial", id)
	}
	return ms[i]
}

// Los mensajes de una persona bloqueada se ven PLEGADOS, su menú ofrece mostrarlos,
// denunciarlos y desbloquear; los propios no ofrecen ni denunciar ni bloquear.
func TestSala_PliegaLosMensajesDeUnaPersonaBloqueada(t *testing.T) {
	s := servicioConDos()
	sa, _ := salaDePrueba(t, s)
	mod := &servicioModerador{servicioFalso: s, bloqueos: []api.RoomsBlock{{Subject: "ana", Name: "Ana"}}}
	sa.S = mod
	sa.Arrancar(context.Background())

	plegado := sa.P.T.T("block.folded", nil)
	m1 := mensajeDeLaLista(t, sa, "m1")
	if m1.Cuerpo != plegado {
		t.Fatalf("el mensaje de la bloqueada se lee: %q", m1.Cuerpo)
	}
	if m4 := mensajeDeLaLista(t, sa, "m4"); m4.Cuerpo != "vale" {
		t.Fatalf("se ha plegado el mensaje de quien no está bloqueado: %q", m4.Cuerpo)
	}

	menu := rotulos(sa.menuDe(m1))
	for _, quiero := range []string{"block.show", "report.message"} {
		if !slices.Contains(menu, sa.P.T.T(quiero, nil)) {
			t.Fatalf("al menú de un mensaje plegado le falta %q: %v", quiero, menu)
		}
	}

	// «Mostrar» lo abre, y solo ese.
	for _, e := range sa.menuDe(m1) {
		if it, ok := e.(*ui.ElementoMenu); ok && it.Texto == sa.P.T.T("block.show", nil) {
			it.AlPulsar()
		}
	}
	if m1 = mensajeDeLaLista(t, sa, "m1"); m1.Cuerpo != "hola" {
		t.Fatalf("tras «Mostrar» el mensaje sigue plegado: %q", m1.Cuerpo)
	}

	// Desbloquear va al servicio y despliega lo demás.
	sa.mostrados = nil
	sa.AlternarBloqueo("c1", "ana", "Ana", true)
	if !s.pidio("Unblock(ana)") || sa.bloqueado("ana") {
		t.Fatalf("desbloquear no llegó al servicio o no se apuntó: %v", s.llamadas)
	}
	if m1 = mensajeDeLaLista(t, sa, "m1"); m1.Cuerpo != "hola" {
		t.Fatalf("tras desbloquear el mensaje sigue plegado: %q", m1.Cuerpo)
	}

	// Sobre lo PROPIO no hay ni denuncia ni bloqueo.
	propio := Mensaje{ID: "mio", Autor: Autor{ID: "yo", Nombre: "Yo"}}
	if menu := rotulos(sa.menuDe(propio)); slices.Contains(menu, sa.P.T.T("report.message", nil)) {
		t.Fatalf("un mensaje propio ofrece denunciarse: %v", menu)
	}
}

// Sin las rutas de moderación (un servicio que no las tiene), la sala no las ofrece:
// no hay entrada que lleve a ninguna parte.
func TestSala_SinModeradorNoOfreceDenunciar(t *testing.T) {
	s := servicioConDos()
	sa, _ := salaDePrueba(t, s)
	sa.Arrancar(context.Background())
	if menu := rotulos(sa.menuDe(mensajeDeLaLista(t, sa, "m1"))); slices.Contains(menu, sa.P.T.T("report.message", nil)) {
		t.Fatalf("sin moderación la sala ofrece denunciar: %v", menu)
	}
}

// La web separa el texto corto de la acción y su aria-label. Mostrar el nombre
// completo en el botón dejaba la fila de bloqueados fuera de un teléfono de320px.
func TestLaListaDeBloqueadosCabeEnElTelefonoYAnunciaElNombre(t *testing.T) {
	s := servicioConDos()
	sa, _, w := salaEn(t, s, 320)
	nombre := "Ana María de los Ángeles y del Valle"
	sa.S = &servicioModerador{servicioFalso: s, bloqueos: []api.RoomsBlock{{Subject: "ana", Name: nombre}}}
	sa.Arrancar(context.Background())
	w.Resize(fyne.NewSize(320, 568))
	sa.AbrirBloqueados()
	if sa.pregunta == nil {
		t.Fatal("no se abrió la lista de bloqueados")
	}
	t.Cleanup(sa.pregunta.Cerrar)
	cuerpo := sa.pregunta.Cuerpo.(*fyne.Container)
	fila := cuerpo.Objects[len(cuerpo.Objects)-1].(*fyne.Container)
	boton := fila.Objects[1].(*ui.Boton)
	if boton.Texto != sa.P.T.T("block.unblockShort", nil) || boton.AccessibilityLabel() != "Desbloquear a "+nombre {
		t.Errorf("se confunden el texto y el nombre accesible: %q / %q", boton.Texto, boton.AccessibilityLabel())
	}
	if boton.Position().X < 0 || boton.Position().X+boton.Size().Width > fila.Size().Width || fila.Objects[0].Size().Width <= 0 {
		t.Errorf("nombre o acción fuera de la fila: fila=%v, acción=%v/%v, nombre=%v", fila.Size(), boton.Position(), boton.Size(), fila.Objects[0].Size())
	}
}
