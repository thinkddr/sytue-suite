// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"image/color"
	"image/png"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/software"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/editor/docus/rico"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// sinSello cambia cada `data-ts` por «T»: el sello es la hora de la tecla.
var reSello = regexp.MustCompile(`data-ts="\d+"`)

func sinSello(s string) string { return reSello.ReplaceAllString(s, `data-ts="T"`) }

const firmaAna = ` data-track="" data-author="Ana" data-author-id="u1" data-ts="T" style="--sytue-track:var(--sytue-avatar-5)"`

// TestElFolioRastreaLoQueSeTeclea: con el modo encendido (la sentinela), teclear inserta
// en un `<ins>` de quien ha entrado, Retroceso marca en vez de borrar y el cursor salta lo
// marcado, Intro sigue partiendo el párrafo, y el modo se guarda con el documento sin
// entrar en el deshacer. Se vio en ROJO quitando `f.rastreando()` de `retroceso`: la «a»
// desaparecía del documento.
func TestElFolioRastreaLoQueSeTeclea(t *testing.T) {
	f := folioCon(t, rico.Sentinela+"<p>hola</p>")
	f.Autor = rico.Autor{Nombre: "Ana", ID: "u1"}
	if !f.EnRastreo() {
		t.Fatal("la sentinela enciende el control de cambios")
	}
	f.mover(punto{0, 4}, false)
	f.escribir("!")
	f.escribir("?")
	if got := sinSello(f.Serializar()); got != rico.Sentinela+`<p>hola<ins`+firmaAna+`>!?</ins></p>` {
		t.Errorf("teclear: %s", got)
	}
	f.mover(punto{0, 4}, false)
	f.retroceso()
	f.retroceso()
	if got := sinSello(f.Serializar()); got != rico.Sentinela+`<p>ho<del`+firmaAna+`>la</del><ins`+firmaAna+`>!?</ins></p>` || f.cur != (punto{0, 2}) {
		t.Errorf("Retroceso marca: %s, cursor %v", got, f.cur)
	}
	f.mover(punto{0, 6}, false)
	f.retroceso() // lo propio sin aceptar se retira
	if got := sinSello(f.Serializar()); !strings.Contains(got, `>!</ins>`) || f.cur != (punto{0, 5}) {
		t.Errorf("retirar lo propio: %s, cursor %v", got, f.cur)
	}
	f.mover(punto{0, 4}, false)
	f.retroceso() // sobre lo borrado, solo salta
	if f.cur != (punto{0, 2}) {
		t.Errorf("el cursor salta lo borrado: %v", f.cur)
	}
	f.mover(punto{0, 2}, false)
	f.intro()
	if n := len(f.bloques); n != 2 {
		t.Errorf("Intro parte el párrafo también rastreando: %d bloques", n)
	}
	// Apagar se guarda (la sentinela se va) y no se deshace.
	antes := f.Cambios()
	f.AlternarRastreo()
	if f.EnRastreo() || strings.HasPrefix(f.Serializar(), rico.Sentinela) || f.Cambios() != antes+1 {
		t.Errorf("apagar: %v, %s", f.EnRastreo(), f.Serializar())
	}
	f.deshacer()
	if f.EnRastreo() {
		t.Error("deshacer no vuelve a encender el control de cambios")
	}
	f.escribir("x")
	if got := sinSello(f.Serializar()); got != `<p>hox<del`+firmaAna+`>la</del><ins`+firmaAna+`>!</ins></p>` {
		t.Errorf("apagado, lo tecleado no se marca: %s", got)
	}
}

// TestLaRevisionAceptaYRechaza: el panel lista los cambios, «Siguiente» resalta el primero
// en el folio, aceptar uno lo consolida y pasa al que ocupa su sitio, y rechazar todos
// devuelve el texto de antes. Pinta el panel entero, que es donde un token inventado
// entraría en pánico.
func TestLaRevisionAceptaYRechaza(t *testing.T) {
	const doc = `<p>a<ins data-track="" data-author="Ana" data-author-id="u1" data-ts="1700000000000">b</ins>` +
		`<del data-track="" data-author="Luis" data-author-id="u2" data-ts="1700000000000">c</del>d</p>`
	ctx := &editor.Contexto{Tipo: editor.Documento, Zoom: 100, Regla: true, Estado: editor.Listo, Panel: editor.PanelRevision}
	f := NuevoFolio(ctx)
	f.Programar = func(time.Duration, func()) {}
	ctx.Lienzo = f
	ctx.Paneles = panelesDe(f, nil, "es")
	if !f.Cargar(doc) {
		t.Fatal("el folio no entiende las marcas de la web")
	}
	m := editor.NuevoMarco(ctx, registroDocus("es"), nil)
	c := software.NewCanvas()
	c.SetPadded(false)
	c.SetContent(m)
	c.Resize(fyne.NewSize(1280, 744))
	r := f.revisor
	if r == nil || len(r.lista.Objects) != 2 || r.contador.Texto != "revision.position" {
		t.Fatalf("el panel: %v", r)
	}
	r.siguiente.Tapped(nil)
	if r.activa != 0 || len(f.revisado) != 1 || f.revisado[0] != (coincidencia{0, 1, 2}) {
		t.Errorf("«Siguiente» resalta el primero: activa %d, %+v", r.activa, f.revisado)
	}
	img := c.Capture()
	if ruta := os.Getenv("SYTUE_CAPTURA_REVISION"); ruta != "" {
		fh, err := os.Create(ruta)
		if err != nil {
			t.Fatal(err)
		}
		defer fh.Close()
		_ = png.Encode(fh, img)
	}
	r.resolverUno(0, true)
	if got := f.Serializar(); got != `<p>ab<del data-track="" data-author="Luis" data-author-id="u2" data-ts="1700000000000">c</del>d</p>` || r.activa != 0 {
		t.Errorf("aceptar el primero: %s (activa %d)", got, r.activa)
	}
	if !f.ResolverCambios(false) || f.Serializar() != `<p>abcd</p>` || len(f.CambiosRastreados()) != 0 {
		t.Errorf("rechazar todo: %s", f.Serializar())
	}
	r.pintar()
	c.Capture()
	f.deshacer()
	if len(f.CambiosRastreados()) != 1 {
		t.Errorf("rechazar todo se deshace de un paso: %d cambios", len(f.CambiosRastreados()))
	}
}

// TestLasTarjetasNoSalenGrises: una tarjeta sin fondo propio deja ver el del panel, y la
// del cambio en foco lleva `bg-selected` compuesto sobre él. Se vio en ROJO con el
// relleno transparente que tenían: la caja pinta el borde debajo del relleno y el
// interior salía del gris del borde (las de comentarios, igual).
func TestLasTarjetasNoSalenGrises(t *testing.T) {
	f := folioCon(t, `<p>a<ins data-track="" data-author="Ana" data-author-id="u1" data-ts="1">b</ins></p>`)
	r := nuevoRevisor(f, nil, "es")
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	for activa, quiero := range map[int]color.NRGBA{
		-1: tema.Color("surface", v),
		0:  tokens.SobreSRGB(tema.Color("selected", v), tema.Color("surface", v)),
	} {
		r.activa = activa
		c := software.NewCanvas()
		c.SetPadded(false)
		c.SetContent(r.tarjeta(0, f.CambiosRastreados()[0], v))
		c.Resize(fyne.NewSize(anchoCuerpoPanel, 90))
		img := c.Capture()
		escala := float32(img.Bounds().Dx()) / anchoCuerpoPanel
		got := color.NRGBAModel.Convert(img.At(int((anchoCuerpoPanel-4)*escala), int(40*escala))).(color.NRGBA)
		if diferencia(got, quiero) > 2 {
			t.Errorf("activa %d: el interior de la tarjeta es %v y tiene que ser %v", activa, got, quiero)
		}
	}
}

func diferencia(a, b color.NRGBA) int {
	d := 0
	for _, x := range [][2]uint8{{a.R, b.R}, {a.G, b.G}, {a.B, b.B}} {
		d = max(d, abs8(x[0], x[1]))
	}
	return d
}

func abs8(a, b uint8) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}

// TestLaNotaAlFinalDesdeLaCinta: «Insertar nota al final» deja el cursor en la nota con su
// texto provisional seleccionado —sin el enlace de vuelta—, y teclear lo sustituye.
func TestLaNotaAlFinalDesdeLaCinta(t *testing.T) {
	f := folioCon(t, `<p>hola</p>`)
	f.mover(punto{0, 4}, false)
	f.Mandar(editor.NotaFinal, "")
	a, b := f.orden()
	if a != (punto{2, 2}) || b.b != 2 || b.p != f.largo(2) {
		t.Fatalf("la selección de la nota nueva: %v – %v", a, b)
	}
	f.escribir("Mi nota")
	if got := f.Serializar(); !strings.Contains(got, `title="ref.endnote.back">i.</a> Mi nota</li>`) {
		t.Errorf("la nota al final: %s", got)
	}
}
