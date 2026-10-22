// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"slices"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// analisisDePrueba es el que el arnés de la web devuelve a `op: analizar-edicion`.
func analisisDePrueba() *AnalisisEdicion {
	return &AnalisisEdicion{
		Fragmentos: []FragmentoEditable{
			{ID: 1, Texto: "Contrato de prestación de servicios", AnchoDisponible: 241.6, Subconjunto: true},
			{ID: 2, Texto: "Madrid, 10 de septiembre de 2026", AnchoDisponible: 180.2},
		},
		Imagenes: []ImagenEditable{
			{ID: "Im1", Ancho: 1200, Alto: 400, Formato: "jpeg", TamanoBytes: 84210},
			{ID: "Im2", Ancho: 64, Alto: 64, Formato: "png", TamanoBytes: 300},
		},
		Capas:  []CapaDocumento{{ID: 1, Nombre: "Marca de agua", Visible: true}, {ID: 2}},
		Avisos: []string{"avisoSubconjunto", "avisoSinReflujo"},
	}
}

// panelEditarCon monta Editar a 1280×800 con ese análisis ya recibido.
func panelEditarCon(t *testing.T, a *AnalisisEdicion) (*panelEditar, fyne.Window) {
	t.Helper()
	p, w := pantallaConPanel(t, PanelEditar)
	e := p.panel.editarP
	if e == nil {
		t.Fatal("el panel de editar no se ha montado")
	}
	e.cargando, e.analisis, e.textos = false, a, map[int]string{}
	for _, f := range a.Fragmentos {
		e.textos[f.ID] = f.Texto
	}
	e.rehacer()
	w.Resize(fyne.NewSize(1280, 801))
	w.Resize(fyne.NewSize(1280, 800))
	return e, w
}

// TestEditarSeColocaComoEnLaWeb: Paneles.tsx real en Chromium, rem16 y Figtree.
// Avisos271×64 en155/231; leyendas en307/392; casillas UA13 centradas en filas py-0.5.
// Imágenes en416/448; campos513/565; fuente incompleta188,75,495; Aplicar en599 de271×28.
func TestEditarSeColocaComoEnLaWeb(t *testing.T) {
	_, w := panelEditarCon(t, analisisDePrueba())
	defer w.Close()
	var avisos, rotulos []fyne.CanvasObject
	var casillas []*casillaNativa
	var entradas []*ui.Entrada
	acciones := map[string][]*Accion{}
	todos(w.Content(), func(o fyne.CanvasObject) {
		switch x := o.(type) {
		case *ui.Parrafo:
			if x.Tinta == tema.Color("warning-text", tema.Claro) {
				avisos = append(avisos, x)
			}
		case *ui.Rotulo:
			if x.Texto == "Capas" || x.Texto == "Imágenes de esta página" || x.Texto == "fuente incompleta" {
				rotulos = append(rotulos, x)
			}
		case *casillaNativa:
			casillas = append(casillas, x)
		case *ui.Entrada:
			entradas = append(entradas, x)
		case *Accion:
			acciones[x.Texto] = append(acciones[x.Texto], x)
		}
	})
	if len(avisos) != 2 || len(rotulos) != 3 || len(casillas) != 2 || len(entradas) != 2 {
		t.Fatalf("faltan piezas: %d avisos, %d rótulos, %d casillas, %d campos", len(avisos), len(rotulos), len(casillas), len(entradas))
	}
	// El párrafo del aviso va dentro de su `p-2`.
	cerca(t, "el primer aviso", absoluta(avisos[0]).Y-8, 155)
	cerca(t, "el segundo aviso", absoluta(avisos[1]).Y-8, 231)
	cerca(t, "la leyenda de capas x", absoluta(rotulos[0]).X, 21)
	cerca(t, "la leyenda de capas y", absoluta(rotulos[0]).Y, 307)
	cerca(t, "la leyenda de imágenes y", absoluta(rotulos[1]).Y, 392)
	// La casilla nativa tiene caja de línea16 y cuadro UA13 centrado: 1,5 px más abajo.
	cerca(t, "la primera casilla x", absoluta(casillas[0]).X, 17)
	cerca(t, "la primera casilla", absoluta(casillas[0]).Y+1.5, 334.5)
	cerca(t, "la segunda casilla", absoluta(casillas[1]).Y+1.5, 354.5)
	casiTexto(t, "«fuente incompleta» x", absoluta(rotulos[2]).X, 188.75)
	cerca(t, "«fuente incompleta» y", absoluta(rotulos[2]).Y, 495)
	for i, y := range []float32{513, 565} {
		cerca(t, "el campo y", absoluta(entradas[i]).Y, y)
		cerca(t, "el campo ancho", entradas[i].Size().Width, 271)
	}
	sus, qui := acciones["Sustituir…"], acciones["Quitar"]
	if len(sus) != 2 || len(qui) != 2 {
		t.Fatalf("hay %d «Sustituir…» y %d «Quitar»; dos de cada", len(sus), len(qui))
	}
	casiTexto(t, "«Sustituir…» x", absoluta(sus[0]).X, 151.21875)
	cerca(t, "«Sustituir…» y", absoluta(sus[0]).Y, 416)
	casiTexto(t, "«Sustituir…» ancho", sus[0].Size().Width, 66.04688)
	cerca(t, "«Sustituir…» alto", sus[0].Size().Height, 26)
	casiTexto(t, "«Quitar» x", absoluta(qui[0]).X, 221.26563)
	cerca(t, "la segunda imagen", absoluta(sus[1]).Y, 448)
	ap := acciones["Aplicar los cambios"]
	if len(ap) != 1 {
		t.Fatal("no está «Aplicar los cambios»")
	}
	cerca(t, "«Aplicar» y", absoluta(ap[0]).Y, 599)
	cerca(t, "«Aplicar» ancho", ap[0].Size().Width, 271)
	cerca(t, "«Aplicar» alto", ap[0].Size().Height, 28)
}

// TestEditarSinTextoLoDice: sin texto editable, p-2 deja el párrafo en8,155 a rem16.
func TestEditarSinTextoLoDice(t *testing.T) {
	_, w := panelEditarCon(t, &AnalisisEdicion{})
	defer w.Close()
	var dice *ui.Parrafo
	todos(w.Content(), func(o fyne.CanvasObject) {
		if x, ok := o.(*ui.Parrafo); ok && x.Texto == "En esta página no hay texto que editar." {
			dice = x
		}
	})
	if dice == nil {
		t.Fatal("una página sin texto no lo dice")
	}
	cerca(t, "el párrafo y", absoluta(dice).Y, 155)
}

// TestEditarMandaSoloLoQueCambio: `op: editar-texto` lleva los fragmentos cuyo texto
// cambió, con su página y su id.
func TestEditarMandaSoloLoQueCambio(t *testing.T) {
	fs := analisisDePrueba().Fragmentos
	textos := map[int]string{1: "Contrato de obra", 2: "Madrid, 10 de septiembre de 2026"}
	if got := fragmentosCambiados(fs, textos); !slices.Equal(got, []cambioDeTexto{{ID: 1, Nuevo: "Contrato de obra"}}) {
		t.Errorf("se mandaría %+v; solo cambió el primero", got)
	}
	textos[1] = fs[0].Texto
	if got := fragmentosCambiados(fs, textos); len(got) != 0 {
		t.Errorf("sin cambios se mandaría %+v", got)
	}
}

// TestLaLeyendaSeSientaSobreElBorde: el borde del `<fieldset>` va a media línea de la
// leyenda menos medio hairline: (16 - 1) / 2 = 7,5. El hueco empieza después del borde
// y p-2, en9. Paneles.tsx real en Chromium con Figtree y rem16 mide38,78125 px
// para la leyenda Capas, incluidos sus8 de px-1. No se mide contra ui.Medir.
func TestLaLeyendaSeSientaSobreElBorde(t *testing.T) {
	f := nuevaLeyenda("Capas", container.New(ui.Fila{}), 237)
	w := ventanaSinRelleno(f)
	defer w.Close()
	w.Resize(fyne.NewSize(237, 60))
	dentro := f.(*fyne.Container).Objects[0].(*fyne.Container)
	cerca(t, "el borde de arriba", dentro.Objects[0].Position().Y, 7.5)
	hueco := dentro.Objects[2]
	cerca(t, "el hueco empieza", hueco.Position().X, 9)
	casiTexto(t, "el hueco mide", hueco.Size().Width, 38.78125)
}

// TestEditarVuelveAMirarAlCambiarDePagina: lo editable es de UNA página; pasar a otra
// con el panel abierto vuelve a preguntar por ella.
func TestEditarVuelveAMirarAlCambiarDePagina(t *testing.T) {
	e, w := panelEditarCon(t, analisisDePrueba())
	defer w.Close()
	e.l.PaginaActual = 2
	e.l.RefrescarActual()
	if e.pagina != 2 || e.analisis != nil {
		t.Errorf("tras pasar a la 2 el análisis es de la %d (%v)", e.pagina, e.analisis != nil)
	}
}

// Un fragmento cuya fuente no dice qué letras usa lo avisa encima del campo, en
// vez de «fuente incompleta», como la web.
func TestEditarAvisaDeLetrasDesconocidas(t *testing.T) {
	a := analisisDePrueba()
	a.Fragmentos[0].LetrasDesconocidas = true
	_, w := panelEditarCon(t, a)
	defer w.Close()
	var vistos []string
	todos(w.Content(), func(o fyne.CanvasObject) {
		if r, ok := o.(*ui.Rotulo); ok && (r.Texto == "letras desconocidas" || r.Texto == "fuente incompleta") {
			vistos = append(vistos, r.Texto)
		}
	})
	if !slices.Equal(vistos, []string{"letras desconocidas"}) {
		t.Errorf("rótulos = %q, quería solo «letras desconocidas»", vistos)
	}
}
