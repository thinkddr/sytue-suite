// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"bytes"
	"context"
	"errors"
	"image"
	pngenc "image/png"
	"mime"
	"net/mail"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/apps/notes"
	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/eml"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// escribir teclea en el cuerpo como lo haría una persona.
func escribir(r *Redactor, s string) {
	z, _ := r.rico.Zona().(interface{ TypedRune(rune) })
	for _, c := range s {
		z.TypedRune(c)
	}
}

// TestElCuerpoConFormatoSaleEnHTML: lo escrito en negrita con la barra sale en la parte
// HTML del mensaje (`multipart/alternative`) y su texto en la parte de texto; el botón de
// negrita se enciende (`aria-pressed`).
func TestElCuerpoConFormatoSaleEnHTML(t *testing.T) {
	b, _, _ := buzonConVentana(t, servicioConTodas(), fyne.NewSize(1280, 800))
	b.abrirRedactor(Borrador{Para: "grace@example.org", Asunto: "Con formato"})
	r := b.redaccion.r
	if !r.VerFormato || r.barra == nil {
		t.Fatal("la barra de formato no sale desplegada de serie")
	}
	r.barra.aplicar("b", "")
	escribir(r, "Hola")
	if !strings.Contains(r.B.HTML, "<b>Hola</b>") || r.B.Cuerpo != "Hola" {
		t.Fatalf("cuerpo: HTML %q, texto %q", r.B.HTML, r.B.Cuerpo)
	}
	var negrita *botonOrden
	for _, bt := range r.barra.botones {
		if bt.orden == "b" {
			negrita = bt
		}
	}
	if negrita == nil || !negrita.Activo {
		t.Error("escribiendo en negrita, el botón de negrita no está pulsado")
	}
	_, s, crudo, err := b.componer(r.B, false)
	if err != nil || !strings.Contains(s.HTML, "<b>Hola</b>") {
		t.Fatalf("componer: %v %q", err, s.HTML)
	}
	m, _ := mail.ReadMessage(bytes.NewReader(crudo))
	if tipo, _, _ := mime.ParseMediaType(m.Header.Get("Content-Type")); tipo != "multipart/alternative" {
		t.Errorf("el mensaje es %s", tipo)
	}
	if x, err := eml.Leer(bytes.NewReader(crudo)); err != nil || x.Texto != "Hola" || !strings.Contains(x.HTML, "<b>Hola</b>") {
		t.Errorf("ida y vuelta: %q %q %v", x.Texto, x.HTML, err)
	}
}

// TestLaRespuestaEntraEnElEditorYSeEscribeEncima: la cita y la firma entran como texto
// (un renglón por `<div>`), el cursor queda arriba y, sin tocar el cuerpo, el mensaje
// sale como hasta ahora: solo texto.
func TestLaRespuestaEntraEnElEditorYSeEscribeEncima(t *testing.T) {
	b, _, _ := buzonConVentana(t, servicioConTodas(), fyne.NewSize(1280, 800))
	cuerpo := "\n\n> Hola:\n> ¿nos vemos?\n\n-- \nAda"
	b.abrirRedactor(Borrador{Para: "grace@example.org", Asunto: "Re: Hola", Cuerpo: cuerpo})
	r := b.redaccion.r
	if got := TextoDe(r.rico.Documento()); got != cuerpo {
		t.Errorf("el texto del editor no es el del borrador:\n%q\n%q", got, cuerpo)
	}
	if _, s, _, _ := b.componer(r.B, false); s.HTML != "" {
		t.Error("sin tocar el cuerpo ya sale HTML")
	}
	escribir(r, "Sí")
	if !strings.HasPrefix(r.B.Cuerpo, "Sí\n") || !strings.HasSuffix(r.B.Cuerpo, "-- \nAda") {
		t.Errorf("lo escrito no va encima de la cita: %q", r.B.Cuerpo)
	}
}

// TestElEnlaceSePideComoEnLaWeb: el panel propone lo seleccionado como rótulo; una
// dirección sin `http(s)://` ni `mailto:` no enlaza nada; una buena, sí.
func TestElEnlaceSePideComoEnLaWeb(t *testing.T) {
	b, _, _ := buzonConVentana(t, servicioConTodas(), fyne.NewSize(1280, 800))
	b.abrirRedactor(Borrador{Para: "grace@example.org"})
	r := b.redaccion.r
	r.barra.aplicar("enlace", "")
	if r.panel == nil {
		t.Fatal("el botón de enlace no abre el panel")
	}
	r.aplicarEnlace("javascript:alert(1)", "mal")
	if r.panel != nil || strings.Contains(r.B.HTML, "href") {
		t.Fatalf("una dirección que no es http ni mailto se enlaza: %q", r.B.HTML)
	}
	r.abrirEnlace()
	r.aplicarEnlace("https://sytue.eu", "Sytue")
	if !strings.Contains(r.B.HTML, `href="https://sytue.eu"`) || !strings.Contains(r.B.HTML, ">Sytue</a>") {
		t.Errorf("el enlace no entra: %q", r.B.HTML)
	}
}

// TestAaPliegaLaBarraYSeRecuerda: «Formato del texto» pliega la barra y el siguiente
// redactor sale como se dejó.
func TestAaPliegaLaBarraYSeRecuerda(t *testing.T) {
	b, _, _ := buzonConVentana(t, servicioConTodas(), fyne.NewSize(1280, 800))
	b.recordarFormato(true)
	t.Cleanup(func() { b.recordarFormato(true) })
	b.abrirRedactor(Borrador{})
	alto := b.redaccion.r.contenedor.MinSize().Height
	b.redaccion.r.AlternarFormato()
	b.recordarFormato(b.redaccion.r.VerFormato)
	if b.redaccion.r.VerFormato || b.redaccion.r.contenedor.MinSize().Height >= alto {
		t.Fatal("plegar la barra no la quita")
	}
	b.cerrar(b.redaccion)
	b.abrirRedactor(Borrador{})
	if b.redaccion.r.VerFormato {
		t.Error("la barra plegada no se recuerda")
	}
}

// TestElBorradorVuelveConSuFormato: un borrador guardado con HTML se reabre con su
// formato; una tabla de la web vuelve entera (se pinta y se conserva tal cual, aunque
// no se edite por dentro); y uno que el editor no sabría reescribir igual —una clase
// que no está en el modelo— se abre por su texto, para no estropearlo al guardar.
func TestElBorradorVuelveConSuFormato(t *testing.T) {
	r := NuevoRedactor(traductor(t), "es", Borrador{Cuerpo: "Hola", HTML: "<p>Hola <b>equipo</b></p>"}, nil, 600)
	if !strings.Contains(r.rico.HTML(), "<b>equipo</b>") {
		t.Errorf("el formato del borrador se pierde: %q", r.rico.HTML())
	}
	tabla := "<table><tbody><tr><td>a</td><td>b</td></tr></tbody></table>"
	r = NuevoRedactor(traductor(t), "es", Borrador{Cuerpo: "a b", HTML: tabla}, nil, 600)
	if !strings.Contains(r.rico.HTML(), tabla) {
		t.Errorf("la tabla del borrador no vuelve entera: %q", r.rico.HTML())
	}
	// Por su texto es un `<div>` por renglón (HTMLDeTexto), que es lo que hace la web al
	// reabrir CUALQUIER borrador (readDraft solo lee el texto).
	r = NuevoRedactor(traductor(t), "es", Borrador{Cuerpo: "Hola", HTML: `<p class="MsoNormal"><b>Hola</b></p>`}, nil, 600)
	if got := r.rico.HTML(); strings.Contains(got, "<p") || strings.Contains(got, "<b>") || TextoDe(r.rico.Documento()) != "Hola" {
		t.Errorf("un HTML que no se reescribe igual se abre para editarlo: %q", got)
	}
}

// TestLosRenglonesDelCuerpoSonLosDeLaWeb: `text-base leading-relaxed`, 19,90625 entre
// renglón y renglón. Con el `widget.Entry` de Fyne salían a 14,70.
func TestLosRenglonesDelCuerpoSonLosDeLaWeb(t *testing.T) {
	m := notes.Maquetar(notes.Leer(HTMLDeTexto("uno\ndos")), 600, tema.Claro)
	if len(m.Lineas) != 2 || m.Lineas[1].Y-m.Lineas[0].Y != LineaCuerpo {
		t.Fatalf("renglones: %+v", m.Lineas)
	}
}

// TestLaBarraMideLoDeLaWeb: MEDIDO en el arnés a 1280×800 (el redactor de 676 de
// cuerpo): botones de 24,5 a `gap-0.5` (el segundo en 26,25), el pelo separador de 1 × 17,5
// a 3,5 de su hueco (`mx-1`), la barra en dos renglones de 58,75 con su pelo de abajo y
// el primer renglón del cuerpo 10,5 por debajo (`py-3`) y 3,5 a la derecha (`px-1`). En
// el móvil (360), una sola fila que se desplaza de lado.
// Raíz16: botón w-7=28, gap-0.5=2, separador h-5=20/mx-1=4;
// dos filas28 + gap2 + py-1×2 + border-b=67; móvil28+8+1=37.
func TestLaBarraMideLoDeLaWeb(t *testing.T) {
	b, _, _ := buzonConVentana(t, servicioConTodas(), fyne.NewSize(1280, 800))
	b.abrirRedactor(Borrador{Para: "grace@example.org"})
	r := b.redaccion.r
	r.contenedor.Refresh()
	caja := r.barra.caja
	casi := func(que string, got, quiero float32) {
		t.Helper()
		if d := got - quiero; d > 0.01 || d < -0.01 {
			t.Errorf("%s: %g y en la web %g", que, got, quiero)
		}
	}
	casi("lado del botón", caja.Objects[0].Size().Width, 28)
	casi("x del segundo botón", caja.Objects[1].Position().X, 30)
	sep, _ := caja.Objects[2].(*fyne.Container)
	casi("x del separador", caja.Objects[2].Position().X+sep.Objects[0].Position().X, 64)
	casi("alto del separador", sep.Objects[0].Size().Height, 20)
	casi("alto de la barra con su pelo", r.barra.alto(r.ancho)+1, 67)
	casi("renglón del cuerpo bajo la barra", r.cuerpo.Position().Y+anilloCuerpo+notes.PYZonaEd-(caja.Position().Y+67), 12)
	casi("x del texto del cuerpo", r.cuerpo.Position().X+anilloCuerpo+notes.PXZonaEd, 4)

	bm, _, _ := buzonConVentana(t, servicioConTodas(), fyne.NewSize(360, 740))
	bm.abrirRedactor(Borrador{Para: "grace@example.org"})
	rm := bm.redaccion.r
	if rm.barra.tira == nil {
		t.Fatal("en el móvil la barra se parte en vez de desplazarse")
	}
	casi("alto de la barra en el móvil", rm.barra.alto(rm.ancho)+1, 37)
}

// botonDelPanel es el botón del panel abierto con ese rótulo.
func botonDelPanel(t *testing.T, r *Redactor, rotulo string) *ui.Boton {
	t.Helper()
	var hallado *ui.Boton
	var buscar func([]fyne.CanvasObject)
	buscar = func(objs []fyne.CanvasObject) {
		for _, o := range objs {
			switch x := o.(type) {
			case *ui.Boton:
				if x.Texto == rotulo {
					hallado = x
				}
			case *fyne.Container:
				buscar(x.Objects)
			}
		}
	}
	if r.panel != nil {
		buscar(r.panel.caja.Objects)
	}
	if hallado == nil {
		t.Fatalf("el panel no tiene «%s»", rotulo)
	}
	return hallado
}

// esperarImagen espera a que la imagen de `src` llegue al cuerpo, que la pide en su rutina.
func esperarImagen(t *testing.T, r *Redactor, src string) {
	t.Helper()
	for range 200 {
		fyne.DoAndWait(func() {})
		if r.imagenes[src] != nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("la imagen %s no llega al cuerpo", src)
}

// TestFuenteYTamanoSonLosDeLaWeb: los desplegables de tipografía y tamaño escriben su
// `font-family` y su `font-size` (en puntos, el rótulo de `TAMANOS`) sobre lo seleccionado,
// y vuelven a su rótulo; van antes que el de estilo, como en la web.
func TestFuenteYTamanoSonLosDeLaWeb(t *testing.T) {
	b, _, _ := buzonConVentana(t, servicioConTodas(), fyne.NewSize(1280, 800))
	b.abrirRedactor(Borrador{Para: "grace@example.org"})
	r := b.redaccion.r
	tr := traductor(t)
	var rotulos []string
	for _, o := range r.barra.caja.Objects {
		if s, ok := o.(*editor.SelectorCinta); ok {
			rotulos = append(rotulos, s.Opciones[0].Rotulo)
		}
	}
	if quiero := []string{tr.T("editor.font", nil), tr.T("editor.size", nil), tr.T("editor.style", nil)}; len(rotulos) < 3 || !slices.Equal(rotulos[:3], quiero) {
		t.Errorf("los desplegables son %q", rotulos)
	}
	escribir(r, "Hola")
	sc, _ := r.rico.Zona().(fyne.Shortcutable)
	sc.TypedShortcut(&fyne.ShortcutSelectAll{})
	r.barra.aplicar("fuente", "Georgia, serif")
	r.barra.aplicar("tamano", "18pt")
	if !strings.Contains(r.B.HTML, `<span style="font-family:Georgia, serif;font-size:18pt">Hola</span>`) {
		t.Errorf("tipografía y tamaño: %q", r.B.HTML)
	}
}

// TestLaTablaTraeSuBarra: «Insertar tabla» mete la de 3×3; la barra de tabla (rótulo,
// cuatro de insertar y tres de eliminar) sale SOLO con el cursor en una celda —también
// con la de formato plegada, como en la web—, sus botones la cambian y, borrada, se va.
func TestLaTablaTraeSuBarra(t *testing.T) {
	b, _, _ := buzonConVentana(t, servicioConTodas(), fyne.NewSize(1280, 800))
	b.abrirRedactor(Borrador{Para: "grace@example.org"})
	r := b.redaccion.r
	enLaColumna := func() bool { return slices.Contains(r.contenedor.Objects, fyne.CanvasObject(r.barra.tabla.caja)) }
	r.barra.aplicar("tabla", "")
	if n := strings.Count(r.B.HTML, "<tr"); n != 3 || strings.Count(r.B.HTML, "<td") != 9 {
		t.Fatalf("la tabla de partida tiene %d filas: %q", n, r.B.HTML)
	}
	if r.verTabla || enLaColumna() {
		t.Fatal("la barra de tabla sale con el cursor fuera de la tabla")
	}
	r.AlternarFormato() // plegada la de formato, la de tabla sigue saliendo
	z := r.rico.Zona()
	z.TypedKey(&fyne.KeyEvent{Name: fyne.KeyUp})
	if !r.verTabla || !enLaColumna() {
		t.Fatal("con el cursor en una celda no sale la barra de tabla")
	}
	if n := len(r.barra.tabla.caja.Objects); n != 1+4+1+3 {
		t.Errorf("la barra de tabla tiene %d piezas", n)
	}
	r.barra.aplicar("filaDebajo", "")
	if n := strings.Count(r.B.HTML, "<tr"); n != 4 {
		t.Errorf("«Insertar fila debajo» deja %d filas", n)
	}
	r.barra.aplicar("borrarTabla", "")
	if strings.Contains(r.B.HTML, "<table") || r.verTabla || enLaColumna() {
		t.Errorf("borrada la tabla: barra %v, %q", r.verTabla, r.B.HTML)
	}
}

// TestLaImagenEntraDelEquipoOPorDireccion: el panel de imagen ofrece primero el fichero;
// el elegido (solo las imágenes) entra en el texto como `cid:` de una parte del mensaje,
// que sale DENTRO del cuerpo (`multipart/related`) y se pinta en el editor. Por dirección,
// tras «URL», solo `https://`.
func TestLaImagenEntraDelEquipoOPorDireccion(t *testing.T) {
	b, _, _ := buzonConVentana(t, servicioConTodas(), fyne.NewSize(1280, 800))
	tr := traductor(t)
	dir := t.TempDir()
	png := dir + "/foto.png"
	var buf bytes.Buffer
	_ = pngenc.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 40, 20)))
	if err := os.WriteFile(png, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/nota.txt", []byte("no"), 0o600); err != nil {
		t.Fatal(err)
	}
	b.ElegirImagenes = func() ([]string, error) { return []string{png, dir + "/nota.txt"}, nil }
	bajadas := 0
	antes := bajarImagen
	bajarImagen = func(context.Context, string) (image.Image, error) {
		bajadas++
		return image.NewRGBA(image.Rect(0, 0, 30, 10)), nil
	}
	t.Cleanup(func() { bajarImagen = antes })

	b.abrirRedactor(Borrador{Para: "grace@example.org"})
	r := b.redaccion.r
	r.barra.aplicar("imagen", "")
	test.Tap(botonDelPanel(t, r, tr.T("smime.chooseFile", nil)))
	if r.panel != nil || len(r.B.Adjuntos) != 1 || !strings.HasPrefix(r.B.Adjuntos[0].CID, "img") {
		t.Fatalf("del equipo: panel %v, adjuntos %+v", r.panel != nil, r.B.Adjuntos)
	}
	src := "cid:" + r.B.Adjuntos[0].CID
	if !strings.Contains(r.B.HTML, `src="`+src+`"`) {
		t.Fatalf("el cuerpo no cita la imagen: %q", r.B.HTML)
	}
	esperarImagen(t, r, src)
	_, _, crudo, err := b.componer(r.B, false)
	m, _ := mail.ReadMessage(bytes.NewReader(crudo))
	if tipo, _, _ := mime.ParseMediaType(m.Header.Get("Content-Type")); err != nil || tipo != "multipart/related" {
		t.Errorf("el mensaje es %s (%v)", tipo, err)
	}

	r.barra.aplicar("imagen", "")
	test.Tap(botonDelPanel(t, r, tr.T("editor.imageByUrl", nil)))
	if r.panel == nil || r.panel.url == nil {
		t.Fatal("«URL» no pide la dirección")
	}
	r.aplicarImagen("http://sytue.eu/a.png")
	if strings.Contains(r.B.HTML, "http://") {
		t.Errorf("una imagen http entra: %q", r.B.HTML)
	}
	r.abrirImagen()
	test.Tap(botonDelPanel(t, r, tr.T("editor.imageByUrl", nil)))
	r.panel.url.PonerTexto("https://sytue.eu/a.png")
	test.Tap(botonDelPanel(t, r, tr.T("editor.imageApply", nil)))
	if !strings.Contains(r.B.HTML, `src="https://sytue.eu/a.png"`) {
		t.Fatalf("la imagen por dirección no entra: %q", r.B.HTML)
	}
	esperarImagen(t, r, "https://sytue.eu/a.png")
	if bajadas != 1 {
		t.Errorf("la imagen se ha bajado %d veces", bajadas)
	}
}

// TestElCuerpoSeDesplazaPasadoSuTope: el cuerpo crece hasta `min(28rem, 45 % de la
// ventana)` —360 en una de 800 de alto— y a partir de ahí se desplaza por dentro,
// siguiendo al cursor: al final, se ve el final; al principio, el principio. Lo que se
// suelta fuera de lo que se ve no entra.
func TestElCuerpoSeDesplazaPasadoSuTope(t *testing.T) {
	b, _, _ := buzonConVentana(t, servicioConTodas(), fyne.NewSize(1280, 800))
	b.abrirRedactor(Borrador{Para: "grace@example.org"})
	r := b.redaccion.r
	z := r.rico.Zona()
	for i := range 40 {
		escribir(r, "renglón "+strconv.Itoa(i))
		z.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	}
	caja := r.cuerpo.Size().Height + 2*rellenoCuerpoY - 2*notes.PYZonaEd - 2*anilloCuerpo
	if d := caja - 360; d > 0.01 || d < -0.01 {
		t.Errorf("el cuerpo mide %g y en la web %g", caja, float32(360))
	}
	contenido := r.cuerpo.Content.Size().Height
	if contenido <= r.cuerpo.Size().Height || r.cuerpo.Offset.Y+r.cuerpo.Size().Height < contenido-2*LineaCuerpo {
		t.Errorf("escribiendo al final no se ve el final: desplazado %g de %g (se ven %g)", r.cuerpo.Offset.Y, contenido, r.cuerpo.Size().Height)
	}
	r.rico.AlPrincipio()
	if r.cuerpo.Offset.Y != 0 {
		t.Errorf("al principio sigue desplazado %g", r.cuerpo.Offset.Y)
	}
	// Lo que se suelta justo debajo de lo que se ve no cae en la parte oculta del cuerpo.
	debajo := fyne.CurrentApp().Driver().AbsolutePositionForObject(r.cuerpo).AddXY(5, r.cuerpo.Size().Height+2)
	if antes := r.B.Cuerpo; b.Soltar("fuera", debajo) || r.B.Cuerpo != antes {
		t.Error("lo soltado bajo el cuerpo visible ha entrado en él")
	}
}

// TestElTextoPlanoNoLlevaLasImagenes: la imagen ocupa una letra en el modelo
// (`notes.Objeto`) que no llega a la parte de texto, y las celdas de una fila van
// separadas por un tabulador.
func TestElTextoPlanoNoLlevaLasImagenes(t *testing.T) {
	d := notes.Leer(`<p>Mira <img src="cid:a"> esto</p><table><tr><td>uno</td><td>dos</td></tr><tr><td>tres</td><td>cuatro</td></tr></table>`)
	if got, quiero := TextoDe(d), "Mira  esto\nuno\tdos\ntres\tcuatro"; got != quiero {
		t.Errorf("texto %q, quiero %q", got, quiero)
	}
}

// TestLasAccionesDelAsistenteSonLasDeLaWeb: las siete acciones del redactor, en su orden
// y con la directiva de `lib/ai.ts` letra por letra —un preámbulo del modelo en el correo
// es lo que evita el «Devuelve solo el texto»—, y el `traducir` de `construirPeticion`.
func TestLasAccionesDelAsistenteSonLasDeLaWeb(t *testing.T) {
	fuente := webref.Read(t, "apps/frontend/apps/mail/src/lib/ai.ts")
	re := regexp.MustCompile(`(?m)^ {2}(\w+):\s*\n?\s*'([^']*)',`)
	web := map[string]string{}
	for _, m := range re.FindAllStringSubmatch(fuente, -1) {
		web[m[1]] = m[2]
	}
	if len(web) != 6 {
		t.Fatalf("de lib/ai.ts salen %d directivas: el patrón ya no vale", len(web))
	}
	var acciones []string
	for _, a := range accionesIA {
		if a.accion != "" {
			acciones = append(acciones, a.accion)
		}
	}
	if quiero := "'mejorar' | 'corregir' | 'acortar' | 'formal' | 'cercano' | 'responder' | 'traducir'"; "'"+strings.Join(acciones, "' | '")+"'" != quiero ||
		!strings.Contains(fuente, quiero) {
		t.Errorf("las acciones son %q", acciones)
	}
	for accion, directiva := range web {
		if directivasIA[accion] != directiva {
			t.Errorf("«%s» dice aquí %q y en la web %q", accion, directivasIA[accion], directiva)
		}
	}
	if q := PeticionIA("traducir", "  hola "); q.Instruccion != "Traduce este correo a inglés. Devuelve solo la traducción." ||
		q.Tipo != "email" || q.Contexto != "hola" || !strings.Contains(fuente, "kind: 'email'") {
		t.Errorf("traducir: %+v", q)
	}
}

// TestElAsistenteReescribeElCorreo: con Sytue AI, el botón va al final de la barra; la
// acción manda el correo entero (sin selección) en el idioma de la sesión, el botón se
// apaga con la rueda mientras contesta, lo que vuelve sustituye el cuerpo y se deshace de
// una vez. Si falla, el cuerpo no se toca y se dice bajo la barra hasta la siguiente.
// Sin servicio, no hay botón.
func TestElAsistenteReescribeElCorreo(t *testing.T) {
	b, _, _ := buzonConVentana(t, servicioConTodas(), fyne.NewSize(1280, 800))
	b.abrirRedactor(Borrador{Para: "grace@example.org"})
	if b.redaccion.r.barra.ia != nil {
		t.Fatal("sin Sytue AI sale el botón del asistente")
	}
	b.cerrar(b.redaccion)

	var pedido api.PeticionIA
	seguir := make(chan struct{})
	respuesta, fallo := "Hola, Grace: te escribo por la reunión.", error(nil)
	b.Asistir = func(_ context.Context, q api.PeticionIA) (string, error) {
		pedido = q
		<-seguir
		return respuesta, fallo
	}
	b.abrirRedactor(Borrador{Para: "grace@example.org"})
	r := b.redaccion.r
	objs := r.barra.caja.Objects
	if r.barra.ia == nil || objs[len(objs)-1] != fyne.CanvasObject(r.barra.hueco) {
		t.Fatal("el asistente no va al final de la barra")
	}
	escribir(r, "ola grace, lo de la reunion")
	esperar := func(listo func() bool) {
		t.Helper()
		for range 400 {
			fyne.DoAndWait(func() {})
			if listo() {
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
		t.Fatal("el asistente no acaba")
	}
	r.PedirIA("mejorar")
	if r.barra.rueda == nil || r.barra.hueco.Objects[0] == fyne.CanvasObject(r.barra.ia) {
		t.Error("mientras contesta, el botón no está apagado con la rueda")
	}
	seguir <- struct{}{}
	esperar(func() bool { return r.B.Cuerpo == respuesta })
	if pedido.Accion != "mejorar" || pedido.Tipo != "email" || pedido.Contexto != "ola grace, lo de la reunion" || pedido.Idioma != "es" {
		t.Errorf("la petición: %+v", pedido)
	}
	if r.barra.rueda != nil || r.iaFallo {
		t.Error("acabado, el botón sigue apagado o hay aviso")
	}
	sc, _ := r.rico.Zona().(fyne.Shortcutable)
	sc.TypedShortcut(&fyne.ShortcutUndo{})
	if r.B.Cuerpo != "ola grace, lo de la reunion" {
		t.Errorf("deshacer no vuelve de una vez: %q", r.B.Cuerpo)
	}

	fallo = errors.New("sin servicio")
	r.PedirIA("acortar")
	seguir <- struct{}{}
	esperar(func() bool { return r.iaFallo })
	if r.B.Cuerpo != "ola grace, lo de la reunion" || !slices.Contains(r.contenedor.Objects, fyne.CanvasObject(r.avisoIA)) {
		t.Errorf("con fallo: cuerpo %q, aviso en la columna %v", r.B.Cuerpo, slices.Contains(r.contenedor.Objects, fyne.CanvasObject(r.avisoIA)))
	}
	fallo = nil
	r.PedirIA("corregir")
	if r.iaFallo {
		t.Error("una petición nueva no quita el aviso de la anterior")
	}
	seguir <- struct{}{}
	esperar(func() bool { return r.B.Cuerpo == respuesta })
}

// TestLosTitulosDelAsistenteVanEnNegrita: en un correo, un título que devuelve el
// asistente se escribe como un párrafo en negrita en su línea (`escribirBloques` de la
// web), no como `<h2>`; las viñetas siguen siendo lista y la cursiva del título se queda.
func TestLosTitulosDelAsistenteVanEnNegrita(t *testing.T) {
	b, _, _ := buzonConVentana(t, servicioConTodas(), fyne.NewSize(1280, 800))
	b.Asistir = func(context.Context, api.PeticionIA) (string, error) {
		return "## Plan *nuevo*\n\n- uno\n- dos", nil
	}
	b.abrirRedactor(Borrador{Para: "grace@example.org"})
	r := b.redaccion.r
	escribir(r, "notas")
	r.rico.Zona().TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn}) // en una línea nueva
	r.PedirIA("mejorar")
	for range 400 {
		fyne.DoAndWait(func() {})
		if strings.Contains(r.B.HTML, "uno") {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	h := r.B.HTML
	if strings.Contains(h, "<h") || !strings.Contains(h, "<div><b>Plan </b><b><i>nuevo</i></b></div>") ||
		!strings.Contains(h, "<li>uno</li><li>dos</li>") {
		t.Errorf("el cuerpo: %q", h)
	}
}
