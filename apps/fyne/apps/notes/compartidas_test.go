// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"net/url"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// «Compartido conmigo»: la lista trae las notas que me han dado; el aviso (`?shared=`)
// abre la suya; con «puede ver» la tarjeta es de solo lectura, con «puede editar» se
// escribe pero sin el «…»; y guardar va por el PUT de siempre, a su cuaderno.
func TestCompartidoConmigo(t *testing.T) {
	p, falso := pantallaConServicio(t, map[string]string{
		"GET /shared/notes": `{"notes":[` +
			`{"id":"n1","notebook_id":"x1","owner_sub":"ada","kind":"text","title":"Presupuesto","content":"<p>1000</p>","permission":"read","shared_by":"ada"},` +
			`{"id":"n2","notebook_id":"x2","owner_sub":"ada","kind":"text","title":"Billetes","content":"<p>AVE</p>","permission":"write","shared_by":"ada"}]}`,
		"PUT /notes/n2": `{"id":"n2","notebook_id":"x2","kind":"text","title":"Billetes de tren","content":"<p>AVE</p>"}`,
	})
	u, _ := url.Parse("sytue-notes:/notes/acme/?shared=n2")
	if !p.AtenderEnlace(u) || p.vista != VistaCompartidas {
		t.Fatal("el aviso de una nota compartida no abre «Compartido conmigo»")
	}
	p.cargarCompartidas()
	if len(p.comp.notas) != 2 || p.comp.abierta != "n2" {
		t.Fatalf("lista %+v, abierta %q", p.comp.notas, p.comp.abierta)
	}
	p.vistaCompartidas(p.variante())
	if l := p.comp.lienzo; l.SoloLectura || !l.SinMenu {
		t.Errorf("con edición: soloLectura=%v sinMenu=%v", l.SoloLectura, l.SinMenu)
	}
	p.abrirCompartida("n1")
	p.vistaCompartidas(p.variante())
	if !p.comp.lienzo.SoloLectura {
		t.Error("con «puede ver» la nota se escribe")
	}
	titulo := "Billetes de tren"
	if err := p.guardarCompartida("n2", Parche{Titulo: &titulo}); err != nil {
		t.Fatal(err)
	}
	if c := falso.ultima(t, "PUT /notes/n2"); c["notebook_id"] != "x2" || c["title"] != titulo {
		t.Errorf("al guardar llegó %+v", c)
	}
	if n, _ := p.compartida("n2"); n.Title != titulo {
		t.Errorf("tras guardar, la lista dice %q", n.Title)
	}
}

// «Compartir con personas» sale en el «…» de mis notas y en la barra de mis cuadernos,
// y en los de otra persona no.
func TestCompartirSoloLoMio(t *testing.T) {
	p, _ := pantallaConServicio(t, nil,
		api.Note{ID: "mia", NotebookID: "c1", OwnerSub: "yo"}, api.Note{ID: "ajena", NotebookID: "c1", OwnerSub: "ana"})
	p.Directorio, p.Yo = api.NewDirectorio("http://id.invalid", nil, nil), func() string { return "yo" }
	tiene := func(m *ui.Menu) bool {
		for _, o := range m.Elementos {
			if e, ok := o.(*ui.ElementoMenu); ok && e.Texto == p.tr.T("sharePeople.open", nil) {
				return true
			}
		}
		return false
	}
	mia, _ := p.nota("mia")
	ajena, _ := p.nota("ajena")
	if !tiene(p.menuNota(mia, false)) || tiene(p.menuNota(ajena, false)) {
		t.Error("«Compartir con personas» tiene que salir en mis notas y solo en ellas")
	}
	p.activo.Crudo.OwnerSub = "ana"
	p.sincronizar()
	if p.barra.AlCompartir != nil {
		t.Error("la barra ofrece compartir un cuaderno ajeno")
	}
	p.activo.Crudo.OwnerSub = "yo"
	p.sincronizar()
	if p.barra.AlCompartir == nil {
		t.Error("la barra no ofrece compartir mi cuaderno")
	}
}
