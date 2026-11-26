// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/local/archivos"
)

// equipoDoble apunta lo que la pantalla le pide a la edición local.
type equipoDoble struct {
	mu                       sync.Mutex
	abiertos, copiados, vist []string
}

func (e *equipoDoble) enEquipo() *EnEquipo {
	apuntar := func(l *[]string, s string) {
		e.mu.Lock()
		defer e.mu.Unlock()
		*l = append(*l, s)
	}
	return &EnEquipo{
		Texto: func(clave string) string { return "«" + clave + "»" },
		Abrir: func(el Elemento) error { apuntar(&e.abiertos, el.Nombre); return nil },
		CopiarA: func(el Elemento, listo func(string, error)) {
			apuntar(&e.copiados, el.Nombre)
			listo("/destino/"+el.Nombre, nil)
		},
		MostrarEnCarpeta: func(el Elemento) error { apuntar(&e.vist, el.Nombre); return nil },
		CrearDocumento: func(context.Context, string, api.DocKind) (api.File, error) {
			return api.File{ID: "nuevo", Name: "Documento sin título.docx"}, nil
		},
	}
}

func (e *equipoDoble) lista(l *[]string) []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return slices.Clone(*l)
}

// TestEnElEquipoElMenuNoComparteYCopiaA: sin organización ni servidor no hay compartir,
// enlaces, clasificar ni llevarlo a otras apps (DR-05), y «Descargar» es «Copiar a…» y
// «Mostrar en la carpeta» (DR-08). Con la IA de la persona, «Resumir» sí (DR-10).
func TestEnElEquipoElMenuNoComparteYCopiaA(t *testing.T) {
	tr := traductor(t)
	p := NuevaPantalla(tr, nil, nil, nil)
	p.EnEquipo = (&equipoDoble{}).enEquipo()
	k := func(claves ...string) []string {
		out := make([]string, 0, len(claves))
		for _, c := range claves {
			if c[0] == '«' {
				out = append(out, c)
				continue
			}
			out = append(out, tr.T(c, nil))
		}
		return out
	}
	notas := Elemento{ID: "f1", Nombre: "notas.md"}
	pdf := Elemento{ID: "f2", Nombre: "informe.pdf"}
	carpeta := Elemento{ID: "c1", Nombre: "Contratos", Carpeta: true}

	if got, quiero := textos(p.AccionesDe(notas, false, false)), k("«copiarA»", "«mostrarCarpeta»", "action.rename", "action.move",
		"action.copy", "action.versions", "details.action", "action.addFavorite", "action.delete"); !slices.Equal(got, quiero) {
		t.Errorf("sin IA:\n got %q\nwant %q", got, quiero)
	}
	p.IA = FuncionIA(func(context.Context, api.PeticionIA) (string, error) { return "", nil })
	if got, quiero := textos(p.AccionesDe(notas, false, true)), k("«copiarA»", "«mostrarCarpeta»", "action.rename", "action.move",
		"action.copy", "action.versions", "details.action", "ai.summarize", "action.removeFavorite", "action.delete"); !slices.Equal(got, quiero) {
		t.Errorf("con IA:\n got %q\nwant %q", got, quiero)
	}
	if got := textos(p.AccionesDe(pdf, false, false)); !slices.Contains(got, tr.T("pdf.openInPdf", nil)) {
		t.Errorf("un PDF se abre en CertiPDF: %q", got)
	}
	if got, quiero := textos(p.AccionesDe(carpeta, false, false)), k("«copiarA»", "«mostrarCarpeta»", "action.rename", "action.move",
		"action.copy", "details.action", "action.addFavorite", "action.delete"); !slices.Equal(got, quiero) {
		t.Errorf("carpeta:\n got %q\nwant %q", got, quiero)
	}
	if got, quiero := textos(p.accionesPlano(notas, false)), k("«copiarA»", "«mostrarCarpeta»", "action.showInFolder"); !slices.Equal(got, quiero) {
		t.Errorf("recientes:\n got %q\nwant %q", got, quiero)
	}
	if FuncionIA(nil) != nil {
		t.Error("sin IA no hay asistente")
	}
}

// TestEnElEquipoAbrirNoDescarga: un documento va a su app (DR-07), una foto a la vista
// previa (DR-11), y «Descargar» copia a otra carpeta (DR-08).
func TestEnElEquipoAbrirNoDescarga(t *testing.T) {
	tr := traductor(t)
	// Con servicio: la vista previa lee de él. Sin ventana no se pinta, pero tampoco
	// se manda la foto a ninguna app.
	p := NuevaPantalla(tr, api.NewDrive("https://sin-servidor.invalid", nil, nil), nil, nil)
	e := &equipoDoble{}
	p.EnEquipo = e.enEquipo()
	p.abrir(Elemento{ID: "f1", Nombre: "acta.docx"})
	p.abrir(Elemento{ID: "f2", Nombre: "demo.mp4"})
	p.abrir(Elemento{ID: "f3", Nombre: "foto.jpg"})
	esperar()
	if got := e.lista(&e.abiertos); !slices.Equal(got, []string{"acta.docx", "demo.mp4"}) && !slices.Equal(got, []string{"demo.mp4", "acta.docx"}) {
		t.Errorf("abiertos con su app: %q (la foto va a la vista previa)", got)
	}
	if TipoVista(Elemento{ID: "f3", Nombre: "foto.jpg"}, true) != "imagen" {
		t.Error("la foto tiene vista previa en el equipo")
	}
	p.descargar(Elemento{ID: "f1", Nombre: "acta.docx"})
	esperar()
	if got := e.lista(&e.copiados); !slices.Equal(got, []string{"acta.docx"}) {
		t.Errorf("descargar es copiar a: %q", got)
	}
}

// TestEnElEquipoNuevoCreaFicheros: «Nuevo» trae la carpeta y los tres documentos, sin la
// vista de Bases, y crear uno lo abre en su app (DR-06).
func TestEnElEquipoNuevoCreaFicheros(t *testing.T) {
	tr := traductor(t)
	p := NuevaPantalla(tr, nil, nil, nil)
	e := &equipoDoble{}
	p.EnEquipo = e.enEquipo()
	var rotulos []string
	for _, o := range p.elementosNuevo() {
		if m, ok := o.(*ui.ElementoMenu); ok {
			rotulos = append(rotulos, m.Texto)
		}
	}
	quiero := []string{tr.T("files.new.folder", nil), tr.T("files.new.document", nil), tr.T("files.new.spreadsheet", nil), tr.T("files.new.presentation", nil)}
	if !slices.Equal(rotulos, quiero) {
		t.Fatalf("Nuevo:\n got %q\nwant %q", rotulos, quiero)
	}
	p.nuevoEnEquipo(api.DocText)
	for range 50 {
		esperar()
		if len(e.lista(&e.abiertos)) > 0 {
			break
		}
	}
	if got := e.lista(&e.abiertos); !slices.Equal(got, []string{"Documento sin título.docx"}) {
		t.Errorf("el documento nuevo se abre en su app: %q", got)
	}
}

// TestResumirConLaIADeLaPersona: «Resumir» lee el fichero del disco y se lo pasa a la
// función de IA de la cáscara local (DR-10).
func TestResumirConLaIADeLaPersona(t *testing.T) {
	dir := t.TempDir()
	disco, err := archivos.Abrir(filepath.Join(dir, "Drive"), archivos.Opciones{Instalacion: "i"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(disco.Raiz(), "acta.txt"), []byte("Se aprueba el presupuesto."), 0o600); err != nil {
		t.Fatal(err)
	}
	svc, err := api.NewDrive("https://sin-servidor.invalid", nil, nil).WithPrivateData(disco)
	if err != nil {
		t.Fatal(err)
	}
	raiz, err := svc.Root(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	hijos, err := svc.Children(t.Context(), raiz.ID, "")
	if err != nil || len(hijos.Files) != 1 {
		t.Fatal(hijos, err)
	}
	var pedido api.PeticionIA
	ia := FuncionIA(func(_ context.Context, q api.PeticionIA) (string, error) {
		pedido = q
		return "Un presupuesto aprobado.", nil
	})
	got, err := Resumir(t.Context(), svc, ia, DeFichero(hijos.Files[0]), "puntos")
	if err != nil || got != "Un presupuesto aprobado." {
		t.Fatal(got, err)
	}
	if pedido.Contexto != "Se aprueba el presupuesto." || pedido.Accion != "puntos" {
		t.Fatalf("petición: %+v", pedido)
	}
}
