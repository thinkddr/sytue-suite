// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"context"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// conSuite le da a la pantalla los clientes de Drive y Rooms contra el mismo servicio
// falso, como hace el `main` contra los de verdad.
func conSuite(t *testing.T, p *Pantalla, respuestas map[string]string) *servicioFalso {
	t.Helper()
	falso := &servicioFalso{respuesta: respuestas}
	srv := httptest.NewServer(falso)
	t.Cleanup(srv.Close)
	p.Suite = &api.Elegir{Apex: api.ApexProd, Drive: api.NewDrive(srv.URL, nil, nil), Rooms: api.NewRooms(srv.URL, nil, nil)}
	return falso
}

// «Compartir en Rooms» va entre «Exportar» y «Qué apunta aquí», en la tarjeta y en el
// árbol, como en NoteCard.tsx y NoteTree.tsx. Sin con qué publicar, no se ofrece.
func TestCompartirEnRoomsEstaEnSuSitio(t *testing.T) {
	p, _ := pantallaConServicio(t, nil, api.Note{ID: "a", NotebookID: "c1"})
	n, _ := p.nota("a")
	tr := p.tr.T
	textos := func(m *ui.Menu) []string {
		var out []string
		for _, e := range m.Elementos {
			if it, ok := e.(*ui.ElementoMenu); ok {
				out = append(out, it.Texto)
			}
		}
		return out
	}
	if slices.Contains(textos(p.menuNota(n, false)), tr("share.toRoom", nil)) {
		t.Error("sin clientes de la suite se ofrece «Compartir en Rooms», que siempre fallaría")
	}
	conSuite(t, p, nil)
	for _, arbol := range []bool{false, true} {
		got := textos(p.menuNota(n, arbol))
		i := slices.Index(got, tr("share.toRoom", nil))
		if i < 1 || got[i-1] != tr("note.export", nil) || got[i+1] != tr("backlinks.title", nil) {
			t.Errorf("árbol=%v: «Compartir en Rooms» no va entre «Exportar» y «Qué apunta aquí»: %v", arbol, got)
		}
	}
}

// Lo que llega a la sala es el ENLACE de la nota con su título, nunca su contenido, y el
// aviso dice dónde se ha compartido.
func TestCompartirEnSalaPublicaElEnlaceYNoElContenido(t *testing.T) {
	p, _ := pantallaConServicio(t, nil,
		api.Note{ID: "a", NotebookID: "c1", Title: "Presupuesto", Content: "<p>secreto</p>"},
		api.Note{ID: "b", NotebookID: "c1"})
	falso := conSuite(t, p, map[string]string{"POST /channels/s1/messages": `{"id":"m1"}`})
	n, _ := p.nota("a")
	p.publicarEnSala("c1", n, api.Elegible{ID: "s1", Nombre: "General"})
	cuerpo := falso.ultima(t, "POST /channels/s1/messages")
	quiero := "📝 Presupuesto\nhttps://notes.sytue.eu/notes/?notebook=c1&note=a"
	if cuerpo["body"] != quiero {
		t.Errorf("a la sala llega %q y tiene que ser %q", cuerpo["body"], quiero)
	}
	if strings.Contains(cuerpo["body"].(string), "secreto") {
		t.Error("el contenido de la nota ha viajado a la sala")
	}
	// Una nota sin título se publica como «Sin título», del catálogo.
	sin, _ := p.nota("b")
	p.publicarEnSala("c1", sin, api.Elegible{ID: "s1", Nombre: "General"})
	if got := falso.ultima(t, "POST /channels/s1/messages")["body"].(string); !strings.HasPrefix(got, "📝 "+p.tr.T("note.untitled", nil)+"\n") {
		t.Errorf("sin título se publica %q", got)
	}
}

// La herramienta de enlaces ofrece los ficheros de Drive que casan —no las carpetas—,
// con su enlace canónico y rotulados «Sytue Drive».
func TestLaHerramientaDeEnlacesOfreceFicherosDeDrive(t *testing.T) {
	p, _ := pantallaConServicio(t, nil)
	conSuite(t, p, map[string]string{
		"QUERY /search": `{"files":[{"id":"F9","name":"Actas","is_directory":true},
			{"id":"f1","name":"acta.pdf","is_directory":false,"size_bytes":10}]}`,
	})
	ctx, cancelar := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelar()
	ops := p.opcionesDrive(ctx, "acta")
	if len(ops) != 1 || ops[0].url != "https://drive.sytue.eu/drive/files/f1" || ops[0].rotulo != "acta.pdf" ||
		ops[0].cuaderno != MarcaDrive || ops[0].tipo != enlaceDrive {
		t.Fatalf("la herramienta de enlaces ofrece %+v", ops)
	}
	// Van detrás de las notas y bajo su rótulo, como en LinkTool.tsx.
	nota := opcionEnlace{url: "u", rotulo: "Mudanza", nota: "Mudanza", tipo: enlaceLocal}
	col := p.filasEnlace([]opcionEnlace{nota, ops[0]}, 0, false, func(opcionEnlace) {}, p.variante())
	// «Este cuaderno», la nota, «Sytue Drive» y el fichero: cuatro filas.
	if n := len(col.(*fyne.Container).Objects); n != 4 {
		t.Errorf("la lista tiene %d filas y son cuatro: cada grupo con su rótulo", n)
	}
}
