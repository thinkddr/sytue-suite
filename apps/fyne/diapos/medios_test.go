// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2/canvas"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// TestLosMediosSonLosDeLaWeb: `mediaKindFor` y `safeMediaSrc` con sus casos del borde.
func TestLosMediosSonLosDeLaWeb(t *testing.T) {
	for entrada, quiero := range map[string]string{
		"video/mp4": "video", "audio/ogg": "audio", "https://x.es/clip.MOV?v=2#t": "video",
		"voz.opus": "audio", "foto.png": "", "": "",
	} {
		if got := TipoMedio(entrada); got != quiero {
			t.Errorf("TipoMedio(%q) = %q, quiero %q", entrada, got, quiero)
		}
	}
	for entrada, quiero := range map[string]string{
		"ejemplo.com/v.mp4":             "https://ejemplo.com/v.mp4",
		"\thttp://a.es/b.mp3":           "http://a.es/b.mp3",
		"javascript:alert(1)":           "",
		"file:///etc/passwd":            "",
		"data:video/mp4;base64,AAAA":    "data:video/mp4;base64,AAAA",
		"data:text/html;base64,PHNjcj4": "",
	} {
		if got := OrigenMedio(entrada); got != quiero {
			t.Errorf("OrigenMedio(%q) = %q, quiero %q", entrada, got, quiero)
		}
	}
}

// TestElPanelDeMediosInsertaComoLaWeb: `MediaPanel` tiene `w-64` (256), campo y tipos
// de `h-7` (28), etiqueta de 16 y `gap-0.5` (2), y botón pequeño de 32 con rem 16.
// «Insertar URL» va apagado hasta que hay dirección, una `.mp3` elige
// «Audio» sola, y el audio nace en su barra de abajo con solo las banderas que se
// apartan del defecto.
//
// VISTO EN ROJO (2026-09-23, en el frontal): la caja de los tipos nacía sin borde (se
// creaba con el borde transparente y `ui.Caja` ya no lo pinta), y lo mismo «Cerrar
// patrón».
func TestElPanelDeMediosInsertaComoLaWeb(t *testing.T) {
	e, _, w := montar(t, `{"slides":[{"id":"a","boxes":[]}]}`)
	var disparador *editor.BotonCinta
	for _, p := range buscar[*editor.PestanaCinta](w.Canvas().Content()) {
		if p.Rotulo == "Insertar" {
			test.Tap(p)
		}
	}
	for _, b := range buscar[*editor.BotonCinta](w.Canvas().Content()) {
		if b.Rotulo == "Insertar audio o vídeo" {
			disparador = b
		}
	}
	if disparador == nil || disparador.Inerte {
		t.Fatal("«Insertar audio o vídeo» está en la cinta y se puede pulsar")
	}
	test.Tap(disparador)
	paneles := buscar[*ui.Entrada](w.Canvas().Overlays().Top())
	if len(paneles) != 1 {
		t.Fatalf("el panel tiene %d campos y es uno", len(paneles))
	}
	url := paneles[0]
	if pos, tam := url.Position(), url.Size(); pos.Y != 18 || tam != fyne.NewSize(256, 28) {
		t.Errorf("el campo está en %v con %v; en la web, bajo la etiqueta y de 256 × 28", pos, tam)
	}
	var insertar *ui.Boton
	for _, b := range buscar[*ui.Boton](w.Canvas().Overlays().Top()) {
		if b.Texto == "Insertar URL" {
			insertar = b
		}
	}
	if insertar == nil || !insertar.Inerte {
		t.Fatal("sin dirección, «Insertar URL» va apagado")
	}
	if tam := insertar.Size(); tam.Height != 32 {
		t.Errorf("«Insertar URL» mide %v de alto y en la web 32", tam.Height)
	}
	opciones := buscar[*opcionMedio](w.Canvas().Overlays().Top())
	if len(opciones) != 2 || opciones[0].Size() != fyne.NewSize(126, 28) {
		t.Errorf("los dos tipos miden %v y en la web (256 − 4) / 2 × 28 = 126 × 28", opciones[0].Size())
	}
	if r := test.WidgetRenderer(opciones[1]).(*opcionMedioRenderer); len(r.caja.Objetos()) != 2 {
		t.Error("la caja de «Audio» nació sin borde: `ui.Caja` solo pinta el que trae al nacer")
	}
	guarda(t, "medio.png", w.Canvas().Capture())
	url.PonerTexto("radio.ejemplo.es/programa.mp3")
	if insertar.Inerte || !opciones[1].activa {
		t.Error("con una .mp3, «Insertar URL» se enciende y el tipo pasa a «Audio»")
	}
	for _, c := range buscar[*casillaMedio](w.Canvas().Overlays().Top()) {
		if strings.HasPrefix(c.texto, "Repetir") {
			test.Tap(c)
		}
	}
	test.Tap(insertar)
	cs := e.m.Diapos[0].Cajas
	if len(cs) != 1 {
		t.Fatalf("insertar deja %d cajas", len(cs))
	}
	c := cs[0]
	if c.Tipo != "media" || c.TipoMedio != "audio" || c.Src != "https://radio.ejemplo.es/programa.mp3" ||
		c.X != 18 || c.Y != 76 || c.W != 64 || c.H != 12 || c.Bucle == nil || c.AutoPlay != nil || c.Controles != nil {
		t.Errorf("el audio nace %+v", c)
	}
	if w.Canvas().Overlays().Top() != nil {
		t.Error("insertar cierra el panel")
	}
}

// TestLaCasillaDelMedioSigueAlTema: sin marcar es la casilla de Chromium de ese tema
// (con `color-scheme: dark`, borde #858585 sobre #3b3b3b), no la blanca en oscuro.
func TestLaCasillaDelMedioSigueAlTema(t *testing.T) {
	t.Cleanup(func() { tema.Elegir("") })
	for _, v := range []tema.Variante{tema.Claro, tema.Oscuro} {
		tema.Elegir(map[tema.Variante]string{tema.Claro: "claro", tema.Oscuro: "oscuro"}[v])
		r := test.TempWidgetRenderer(t, nuevaCasillaMedio("Bucle", false, func(bool) {})).(*casillaMedioRenderer)
		o := r.pieza.Objetos()
		borde, dentro := o[0].(*canvas.Rectangle), o[1].(*canvas.Rectangle)
		if quiero := ui.CasillaNativa(v); dentro.FillColor != quiero.Fondo || borde.FillColor != quiero.Borde {
			t.Errorf("variante %v: fondo %v y borde %v; Chromium, %v y %v", v, dentro.FillColor, borde.FillColor, quiero.Fondo, quiero.Borde)
		}
	}
}

// TestElMedioSeReproduceAlPulsarloEnLaPresentacion: al presentar, un audio o un vídeo se
// pulsa y se reproduce —con el reproductor del sistema, que es el único que hay—: uno
// incrustado se deja en un fichero con sus bytes, uno enlazado va a su dirección.
//
// VISTO EN ROJO (2026-09-26, en el frontal): sin la capa pulsable de los medios, la caja
// no tenía nada que pulsar y el medio no se oía ni se veía.
func TestElMedioSeReproduceAlPulsarloEnLaPresentacion(t *testing.T) {
	m, err := Leer([]byte(`{"slides":[{"id":"s","boxes":[
		{"id":"a","kind":"media","mediaType":"audio","src":"data:audio/mpeg;base64,SUQzBAA=","html":"","x":10,"y":10,"w":30,"h":20}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	p := NuevaPresentacion(m, 0, traductor(t))
	var pulsada string
	p.AbrirMedio = func(c Caja) { pulsada = c.ID }
	w := test.NewTempWindow(t, p)
	w.Resize(fyne.NewSize(960, 540))
	if len(p.enlaces) != 1 {
		t.Fatalf("la presentación tiene %d cajas pulsables y el audio es una", len(p.enlaces))
	}
	test.Tap(p.enlaces[0])
	if pulsada != "a" {
		t.Fatal("pulsar el audio no lo reproduce")
	}
	dir := t.TempDir()
	ruta, web, ok := destinoMedio(m.Diapos[0].Cajas[0], dir)
	if b, _ := os.ReadFile(ruta); !ok || web != nil || string(b) != "ID3\x04\x00" {
		t.Errorf("el audio incrustado se deja en %q con %q", ruta, b)
	}
	if _, web, ok := destinoMedio(Caja{ID: "v", Src: "https://sytue.eu/v.mp4"}, dir); !ok || web.String() != "https://sytue.eu/v.mp4" {
		t.Error("un vídeo enlazado va a su dirección")
	}
	if _, _, ok := destinoMedio(Caja{ID: "x", Src: "javascript:alert(1)"}, dir); ok {
		t.Error("lo que no es un medio no se abre")
	}
}

// TestElMedioIncrustadoNoEligeNiDondeNiComoSeAbre: del documento solo salen los bytes del
// medio. El `id` de la caja y el MIME del `data:` los escribe quien edita la presentación;
// con ellos en la ruta, un `id` con barras dejaba el fichero fuera de la carpeta y el MIME
// elegía la extensión, que es lo que decide qué programa lo abre (NAT-01).
func TestElMedioIncrustadoNoEligeNiDondeNiComoSeAbre(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "medios")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	en64 := func(tipo string) string {
		return "data:" + tipo + ";base64," + base64.StdEncoding.EncodeToString([]byte("ID3"))
	}
	ruta, _, ok := destinoMedio(Caja{ID: "/../../fuera", Tipo: "media", Src: en64("audio/mpeg")}, dir)
	if !ok || filepath.Dir(ruta) != dir || filepath.Ext(ruta) != ".mp3" || strings.Contains(ruta, "fuera") {
		t.Errorf("el audio se deja en %q (ok=%v): tiene que quedar en %q, con un nombre de la app y .mp3", ruta, ok, dir)
	}
	if ruta, _, ok := destinoMedio(Caja{ID: "v", Src: en64("Video/OGG; codecs=theora")}, dir); !ok || filepath.Ext(ruta) != ".ogv" {
		t.Errorf("un vídeo con parámetros en su tipo se deja en %q (ok=%v), y es .ogv", ruta, ok)
	}
	for _, tipo := range []string{"text/javascript", "text/html", "application/octet-stream", "audio/no-existe", ""} {
		if ruta, _, ok := destinoMedio(Caja{ID: "x", Tipo: "media", Src: en64(tipo)}, dir); ok {
			t.Errorf("un `data:%s` no es un medio de la lista y se ha dejado en %q", tipo, ruta)
		}
	}
	if fuera, _ := os.ReadDir(base); len(fuera) != 1 {
		t.Errorf("en la carpeta de arriba hay %d entradas: solo puede estar la de los medios", len(fuera))
	}
	if dentro, _ := os.ReadDir(dir); len(dentro) != 2 {
		t.Errorf("en la carpeta de los medios hay %d ficheros y se han dejado dos", len(dentro))
	}
}
