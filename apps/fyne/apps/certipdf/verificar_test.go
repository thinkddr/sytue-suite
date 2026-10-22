// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"encoding/json"
	"image/color"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// motorDeFirmas finge `/pdf/content` (un documento con dos firmas declaradas) y
// `/pdf/verify`, que contesta lo que se le ponga: el informe, o un 500 las veces que se
// diga. Cuenta las verificaciones y apunta si llegó el fichero.
type motorDeFirmas struct {
	informe       string
	fallar        atomic.Int32
	llamadas      atomic.Int32
	hayFichero    atomic.Bool
	cambios       atomic.Int32
	invalidaVista atomic.Bool
}

func (m *motorDeFirmas) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseMultipartForm(1 << 20)
	switch {
	case r.URL.Path == "/api/v1/pdf/content":
		_, _ = io.WriteString(w, `{"paginas":[{"numero":1,"ancho":100,"alto":140,"caja":[0,0,100,140]}],"total":1,
			"firmas":[{"nombre":"Ana Pérez","motivo":"Conforme","fecha":"2026-09-10","cubre_todo":true},
			{"fecha":"2026-09-12","cubre_todo":false}]}`)
	case r.URL.Path == "/api/v1/pdf/verify":
		m.llamadas.Add(1)
		_, f, err := r.FormFile("file")
		m.hayFichero.Store(err == nil && f != nil)
		if m.fallar.Add(-1) >= 0 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = io.WriteString(w, m.informe)
	case r.URL.Path == "/api/v1/pdf/protect":
		m.cambios.Add(1)
		if r.FormValue("invalidar_firmas") != "1" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_, _ = io.WriteString(w, `{"code":"pdf_invalidates_signatures","message":"Signed PDF"}`)
			return
		}
		m.invalidaVista.Store(true)
		_, _ = io.WriteString(w, "%PDF-modificado")
	case strings.HasPrefix(r.URL.Path, "/api/v1/folders/"):
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"f-1","name":"a.pdf"}`)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// informeDePrueba son tres firmas que cubren lo que la ficha sabe decir: una válida con
// su sello, una modificada sin nombre propio ni sello (el sujeto del certificado hace de
// nombre) y una sin determinar con un motivo que el catálogo no conoce.
const informeDePrueba = `{"anclas":false,"firmas":[
 {"nombre":"Ana Pérez","tipo":"pades","veredicto":"valida","integra":true,"cubre_todo":true,"confianza":"confiable",
  "firmante":{"sujeto":"Ana Pérez","emisor":"Izenpe","serie":"01","desde":"2026-01-01T00:00:00Z","hasta":"2028-01-01T00:00:00Z"},
  "sello":{"fecha":"2026-09-10T10:30:00Z","autoridad":"Izenpe TSA","integro":true,"confianza":"confiable"},
  "cambios":{"bytes_firmados":10,"bytes_totales":10,"revisiones":0,"alcance":"ninguno"}},
 {"tipo":"pades","veredicto":"modificada","motivos":["integridad","cambios"],"integra":false,"cubre_todo":false,"confianza":"no_confiable",
  "firmante":{"sujeto":"Luis Gómez","emisor":"FNMT","serie":"02","desde":"2026-02-01T00:00:00Z","hasta":"2027-02-01T00:00:00Z"},
  "cambios":{"bytes_firmados":10,"bytes_totales":30,"revisiones":1,"alcance":"otros","paginas":[9,5,6,7,3,3],"objetos":2}},
 {"tipo":"pades","veredicto":"indeterminada","motivos":["sin_anclas","inventado"],"integra":true,"cubre_todo":true,"confianza":"sin_anclas",
  "cambios":{"bytes_firmados":10,"bytes_totales":12,"revisiones":1,"alcance":"permitidos"}}]}`

// abrirConFirmas monta la pantalla sobre el motor fingido, con el panel «Firmas» abierto.
func abrirConFirmas(t *testing.T, m *motorDeFirmas) (*Pantalla, chan struct{}, func()) {
	t.Helper()
	srv := httptest.NewServer(m)
	doc := &Documento{pdf: api.NewPDF(srv.URL, srv.URL, nil, nil), ruta: "/PDF/a.pdf", fichero: api.File{Name: "a.pdf"}, datos: []byte("%PDF-")}
	p := NuevaPantalla(traductorES(t))
	p.Idioma = "es"
	w := ventanaSinRelleno(p.Objeto())
	w.Resize(fyne.NewSize(1280, 800))
	listo := make(chan struct{}, 16)
	p.trasRed = func() { listo <- struct{}{} }
	p.Abrir(doc)
	esperarRed(t, listo) // el contenido
	esperarRed(t, listo) // el primer lote
	fyne.DoAndWait(func() { p.panel.Abrir(PanelFirmas) })
	return p, listo, func() { w.Close(); srv.Close() }
}

// textosDelPanel son los párrafos y botones que se ven en el panel abierto, en orden.
func textosDelPanel(p *Pantalla) (textos []string, botones []*ui.Boton, parrafos []*ui.Parrafo) {
	fyne.DoAndWait(func() {
		todos(p.panel.Objeto(), func(o fyne.CanvasObject) {
			switch x := o.(type) {
			case *ui.Parrafo:
				textos = append(textos, x.Texto)
				parrafos = append(parrafos, x)
			case *ui.Boton:
				botones = append(botones, x)
			}
		})
	})
	return textos, botones, parrafos
}

func pulsarVerificar(t *testing.T, p *Pantalla, listo chan struct{}) {
	t.Helper()
	_, botones, _ := textosDelPanel(p)
	if len(botones) != 1 {
		t.Fatalf("el panel tiene %d botones; tiene que tener «Verificar las firmas»", len(botones))
	}
	fyne.DoAndWait(func() { botones[0].Tapped(nil) })
	esperarRed(t, listo)
}

// TestVerificarSoloCorreAlPulsar: abrir el panel enseña lo que las firmas DECLARAN, con
// el botón y el descargo, y NO pide nada al motor —verificar cuesta, cada firma resume el
// fichero entero—. Al pulsarlo, el informe sustituye a las fichas declaradas y trae la
// nota de la revocación; el descargo de «aquí no se comprueba» ya no tiene sentido.
func TestVerificarSoloCorreAlPulsar(t *testing.T) {
	m := &motorDeFirmas{informe: informeDePrueba}
	p, listo, cerrar := abrirConFirmas(t, m)
	defer cerrar()
	tr := traductorES(t)

	textos, botones, _ := textosDelPanel(p)
	if m.llamadas.Load() != 0 {
		t.Fatal("abrir el panel ya verificó: eso cuesta, y es de quien pulsa")
	}
	if len(botones) != 1 || botones[0].Texto != "Verificar las firmas" || botones[0].AccessibilityRole() != fyne.AccessibleRoleButton {
		t.Fatalf("falta el botón «Verificar las firmas»: %d botones", len(botones))
	}
	for _, quiero := range []string{
		"Ana Pérez", "Conforme", "2026-09-10", "Firma sin nombre",
		tr("pdf.signatures.covers", nil), tr("pdf.signatures.partial", nil), tr("pdf.signatures.disclaimer", nil),
	} {
		if !slices.Contains(textos, quiero) {
			t.Errorf("antes de verificar falta %q; hay %q", quiero, textos)
		}
	}

	pulsarVerificar(t, p, listo)
	if m.llamadas.Load() != 1 || !m.hayFichero.Load() {
		t.Fatalf("se hicieron %d verificaciones (con fichero: %v); tiene que ser una, con el PDF", m.llamadas.Load(), m.hayFichero.Load())
	}
	textos, botones, _ = textosDelPanel(p)
	if len(botones) != 0 {
		t.Errorf("con el informe ya no hay botón (la web tampoco lo deja): %d", len(botones))
	}
	if slices.Contains(textos, tr("pdf.signatures.disclaimer", nil)) {
		t.Error("con el informe sigue el descargo de «aquí no se verifica»")
	}
	for _, quiero := range []string{
		tr("pdf.verify.verdict.valida", nil), tr("pdf.verify.verdict.modificada", nil), tr("pdf.verify.verdict.indeterminada", nil),
		"Luis Gómez", // sin nombre propio, el sujeto del certificado
		tr("pdf.verify.reason.integridad", nil), tr("pdf.verify.reason.cambios", nil), tr("pdf.verify.reason.sin_anclas", nil),
		tr("pdf.verify.reason.formato", nil), // el motivo que no se conoce no se enseña en bruto
		tr("pdf.verify.certificate", i18n.Vars{"subject": "Ana Pérez", "issuer": "Izenpe"}),
		tr("pdf.verify.timestamp", i18n.Vars{"authority": "Izenpe TSA", "date": fechaCorta("2026-09-10T10:30:00Z", "es")}),
		tr("pdf.verify.noTimestamp", nil),
		tr("pdf.verify.changesAllowed", nil),
		tr("pdf.verify.changesPages", i18n.Vars{"pages": "3, 5-7, 9"}),
		tr("pdf.verify.changesObjects", nil),
		tr("pdf.verify.noAnchors", nil), // el despliegue no tiene en quién confiar
		tr("pdf.verify.revocation", nil),
	} {
		if !slices.Contains(textos, quiero) {
			t.Errorf("el informe no dice %q; dice %q", quiero, textos)
		}
	}
	for _, t1 := range textos {
		if strings.Contains(t1, "pdf.") || t1 == "inventado" {
			t.Errorf("sale una clave o un código en bruto: %q", t1)
		}
	}
	if textos[len(textos)-1] != tr("pdf.verify.revocation", nil) {
		t.Errorf("la nota de la revocación va al pie y lo último es %q", textos[len(textos)-1])
	}
}

// TestElVeredictoVaEnTextoYEnSuTono: «válida» en verde, «modificada» en rojo y «sin
// determinar» en la tinta normal —ni acusa ni da el visto bueno—; y el veredicto
// encabeza la ficha, antes del nombre.
func TestElVeredictoVaEnTextoYEnSuTono(t *testing.T) {
	m := &motorDeFirmas{informe: informeDePrueba}
	p, listo, cerrar := abrirConFirmas(t, m)
	defer cerrar()
	pulsarVerificar(t, p, listo)
	tr := traductorES(t)
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	_, _, parrafos := textosDelPanel(p)
	tinta := func(texto string) color.NRGBA {
		for _, x := range parrafos {
			if x.Texto == texto {
				return x.Tinta
			}
		}
		t.Fatalf("no hay un párrafo %q", texto)
		return color.NRGBA{}
	}
	for clave, token := range map[string]string{
		"pdf.verify.verdict.valida": "success-text", "pdf.verify.verdict.modificada": "danger-text",
		"pdf.verify.verdict.indeterminada": "text", "pdf.verify.changesPages": "",
	} {
		if token == "" {
			continue
		}
		if got, quiero := tinta(tr(clave, nil)), tema.Color(token, v); got != quiero {
			t.Errorf("%s se pinta %v y su tono es %s (%v)", clave, got, token, quiero)
		}
	}
	// Antes del nombre, en cada ficha.
	for i, x := range parrafos {
		if x.Texto == "Luis Gómez" && (i == 0 || parrafos[i-1].Texto != tr("pdf.verify.verdict.modificada", nil)) {
			t.Error("el veredicto no encabeza la ficha: antes del nombre hay otra cosa")
		}
	}
}

// TestUnFalloAlVerificarSeDiceYSePuedeReintentar: un 500 deja las fichas declaradas, dice
// que no se pudo (en el catálogo, no en bruto) y el botón vuelve a estar a mano; al
// repetir, llega el informe.
func TestUnFalloAlVerificarSeDiceYSePuedeReintentar(t *testing.T) {
	m := &motorDeFirmas{informe: informeDePrueba}
	m.fallar.Store(1)
	p, listo, cerrar := abrirConFirmas(t, m)
	defer cerrar()
	tr := traductorES(t)

	pulsarVerificar(t, p, listo)
	textos, botones, _ := textosDelPanel(p)
	if !slices.Contains(textos, tr("pdf.verify.error", nil)) {
		t.Errorf("tras el fallo no se dice que no se pudo: %q", textos)
	}
	if len(botones) != 1 || botones[0].Inerte || botones[0].Texto != "Verificar las firmas" {
		t.Fatal("tras el fallo el botón no está listo para reintentar")
	}
	pulsarVerificar(t, p, listo)
	textos, _, _ = textosDelPanel(p)
	if slices.Contains(textos, tr("pdf.verify.error", nil)) || !slices.Contains(textos, tr("pdf.verify.revocation", nil)) {
		t.Errorf("el segundo intento no deja el informe limpio: %q", textos)
	}
	if m.llamadas.Load() != 2 {
		t.Errorf("%d verificaciones; fueron dos pulsaciones", m.llamadas.Load())
	}
}

// TestMientrasVerificaElBotonDiceQueTrabajaYNoSePulsa: con la petición en vuelo el
// rótulo es «Verificando…» y el botón está inerte, así que un segundo clic no manda otra.
func TestMientrasVerificaElBotonDiceQueTrabajaYNoSePulsa(t *testing.T) {
	soltar := make(chan struct{})
	m := &motorDeFirmas{informe: informeDePrueba}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/pdf/verify" {
			<-soltar
		}
		m.ServeHTTP(w, r)
	}))
	defer srv.Close()
	doc := &Documento{pdf: api.NewPDF(srv.URL, srv.URL, nil, nil), ruta: "/PDF/a.pdf", fichero: api.File{Name: "a.pdf"}, datos: []byte("%PDF-")}
	p := NuevaPantalla(traductorES(t))
	w := ventanaSinRelleno(p.Objeto())
	defer w.Close()
	w.Resize(fyne.NewSize(1280, 800))
	listo := make(chan struct{}, 16)
	p.trasRed = func() { listo <- struct{}{} }
	p.Abrir(doc)
	esperarRed(t, listo)
	esperarRed(t, listo)
	fyne.DoAndWait(func() { p.panel.Abrir(PanelFirmas) })

	_, botones, _ := textosDelPanel(p)
	fyne.DoAndWait(func() { botones[0].Tapped(nil) })
	_, botones, _ = textosDelPanel(p)
	if len(botones) != 1 || botones[0].Texto != "Verificando las firmas…" || !botones[0].Inerte {
		t.Fatalf("mientras trabaja: %d botones, %q, inerte %v", len(botones), botones[0].Texto, botones[0].Inerte)
	}
	fyne.DoAndWait(func() { botones[0].Tapped(nil); p.panel.firmasP.verificar() })
	close(soltar)
	esperarRed(t, listo)
	if m.llamadas.Load() != 1 {
		t.Errorf("%d verificaciones; el segundo clic no tenía que mandar otra", m.llamadas.Load())
	}
}

// TestUnCambioDelDocumentoTiraElInforme: «Válida» sobre bytes que ya no son los
// verificados es lo único que esta pantalla no puede decir. Si el documento se vuelve a
// leer tras un cambio, el panel vuelve a las fichas declaradas y al botón; y un informe
// que estaba en vuelo y llega después no se enseña.
func TestUnCambioDelDocumentoTiraElInforme(t *testing.T) {
	m := &motorDeFirmas{informe: informeDePrueba}
	p, listo, cerrar := abrirConFirmas(t, m)
	defer cerrar()
	pulsarVerificar(t, p, listo)
	if _, botones, _ := textosDelPanel(p); len(botones) != 0 {
		t.Fatal("no llegó el informe")
	}
	fyne.DoAndWait(func() { p.panel.PonerContenido(p.Contenido) })
	textos, botones, _ := textosDelPanel(p)
	if len(botones) != 1 || slices.Contains(textos, traductorES(t)("pdf.verify.revocation", nil)) {
		t.Errorf("tras el cambio sigue el informe viejo: %d botones, %q", len(botones), textos)
	}

	// Uno en vuelo: se descarta al cambiar el documento.
	fyne.DoAndWait(func() { p.panel.firmasP.verificar(); p.panel.PonerContenido(p.Contenido) })
	esperarRed(t, listo)
	textos, botones, _ = textosDelPanel(p)
	if len(botones) != 1 || slices.Contains(textos, traductorES(t)("pdf.verify.revocation", nil)) {
		t.Errorf("un informe de antes del cambio se coló: %d botones", len(botones))
	}
}

// TestSinFirmasSoloDiceQueNoHay: como en la web, un documento sin firmar no ofrece
// verificar ni lleva descargo; solo dice que no está firmado.
func TestSinFirmasSoloDiceQueNoHay(t *testing.T) {
	p, w := pantallaConDocumento(t, &Contenido{Total: 1}, PanelFirmas)
	defer w.Close()
	textos, botones, _ := textosDelPanel(p)
	if len(botones) != 0 || len(textos) != 1 || textos[0] != "Este documento no está firmado." {
		t.Errorf("sin firmas hay %d botones y %q", len(botones), textos)
	}
}

// TestLasPiezasDeLaVerificacion: el rango de páginas, los códigos que no se conocen, la
// fecha y los tonos, que es donde se cuelan los errores de borde.
func TestLasPiezasDeLaVerificacion(t *testing.T) {
	for _, c := range []struct {
		in   []int
		want string
	}{{nil, ""}, {[]int{3}, "3"}, {[]int{9, 5, 6, 7, 3, 3}, "3, 5-7, 9"}, {[]int{1, 2}, "1-2"}, {[]int{4, 4}, "4"}} {
		if got := rangoDePaginas(c.in); got != c.want {
			t.Errorf("rangoDePaginas(%v) = %q; la web da %q", c.in, got, c.want)
		}
	}
	if claveDeMotivo("inventado") != "pdf.verify.reason.formato" || claveDeVeredicto("raro") != "pdf.verify.verdict.indeterminada" {
		t.Error("un código desconocido no cae en el genérico")
	}
	if tintaDeVeredicto("raro") != "text" || tintaDeVeredicto(VeredictoValida) != "success-text" || tintaDeVeredicto(VeredictoInvalida) != "danger-text" {
		t.Error("los tonos de los veredictos no son los de la web")
	}
	zonaDeFechas = time.UTC
	defer func() { zonaDeFechas = nil }()
	if got := fechaCorta("2026-09-10T10:30:00Z", "en"); got != "10 Sept 2026, 10:30" {
		t.Errorf("la fecha en inglés (en-GB, como la suite) es %q", got)
	}
	if got := fechaCorta("no es una fecha", "es"); got != "no es una fecha" {
		t.Errorf("una fecha ilegible se tira en vez de dejarse: %q", got)
	}
	if got := fechaCorta("", "es"); got != "" {
		t.Errorf("sin fecha: %q", got)
	}
	// El idioma de la pantalla puede no estar puesto (suelto, en una prueba): no se
	// enseña «, » en vez de la fecha.
	if got := fechaCorta("2026-09-10T10:30:00Z", ""); got == "" || got == ", " {
		t.Errorf("sin idioma la fecha sale %q", got)
	}
}

// TestElInformeDelMotorSeLeeEntero: el JSON del motor llega a los tipos de aquí sin
// perder campos —las etiquetas son las suyas— y `firmas: null` es una lista vacía.
func TestElInformeDelMotorSeLeeEntero(t *testing.T) {
	var r InformeVerificacion
	if err := json.Unmarshal([]byte(informeDePrueba), &r); err != nil {
		t.Fatal(err)
	}
	f := r.Firmas[1]
	if r.Anclas || len(r.Firmas) != 3 || f.Veredicto != VeredictoModificada || f.Firmante == nil || f.Firmante.Sujeto != "Luis Gómez" ||
		f.Cambios.Objetos != 2 || !slices.Equal(f.Cambios.Paginas, []int{9, 5, 6, 7, 3, 3}) || f.Cambios.Alcance != "otros" || f.CubreTodo {
		t.Errorf("el informe se leyó mal: %+v", r)
	}
	if r.Firmas[0].Sello == nil || !r.Firmas[0].Sello.Integro || r.Firmas[0].Sello.Autoridad != "Izenpe TSA" {
		t.Error("el sello no llegó")
	}
	var vacio InformeVerificacion
	if err := json.Unmarshal([]byte(`{"firmas":null,"anclas":true}`), &vacio); err != nil || len(vacio.Firmas) != 0 || !vacio.Anclas {
		t.Errorf("firmas null: %+v, %v", vacio, err)
	}
}

// TestLasClavesDeLaVerificacionExistenEnTodosLosIdiomas: cada veredicto, cada motivo y
// cada rótulo del panel está en el catálogo de todos los idiomas de la web: si el motor
// añade un código y aquí no, o un idioma se queda sin una frase, una pantalla de seguridad
// enseñaría una clave.
func TestLasClavesDeLaVerificacionExistenEnTodosLosIdiomas(t *testing.T) {
	claves := make([]string, 0, 16+len(motivosDelCatalogo))
	claves = append(claves,
		"pdf.verify.run", "pdf.verify.running", "pdf.verify.error", "pdf.verify.certificate", "pdf.verify.validity",
		"pdf.verify.timestamp", "pdf.verify.noTimestamp", "pdf.verify.changesAllowed", "pdf.verify.changesPages",
		"pdf.verify.changesObjects", "pdf.verify.noAnchors", "pdf.verify.revocation")
	for _, v := range []Veredicto{VeredictoValida, VeredictoIndeterminada, VeredictoModificada, VeredictoInvalida} {
		claves = append(claves, claveDeVeredicto(v))
	}
	for m := range motivosDelCatalogo {
		claves = append(claves, claveDeMotivo(m))
	}
	ficheros, err := fs.Glob(catalogos.Docs(), "*.json")
	if err != nil || len(ficheros) < 20 {
		t.Fatalf("catálogos de docs: %d ficheros, %v", len(ficheros), err)
	}
	for _, f := range ficheros {
		crudo, err := fs.ReadFile(catalogos.Docs(), f)
		if err != nil {
			t.Fatal(err)
		}
		var cat map[string]any
		if err := json.Unmarshal(crudo, &cat); err != nil {
			t.Fatal(err)
		}
		for _, c := range claves {
			if s, ok := cat[c].(string); !ok || s == "" {
				t.Errorf("%s no tiene %s", f, c)
			}
		}
	}
	// Dos motivos distintos no comparten frase (lo comprueba la web con su catálogo).
	vistas := map[string]string{}
	for m := range motivosDelCatalogo {
		if otro, ok := vistas[claveDeMotivo(m)]; ok {
			t.Errorf("los motivos %s y %s comparten clave", m, otro)
		}
		vistas[claveDeMotivo(m)] = m
	}
}

// botonesDeConfirmacion devuelve el diálogo abierto encima del lienzo y sus botones de
// cancelar y confirmar.
func botonesDeConfirmacion(t *testing.T, p *Pantalla) (*ui.Dialogo, *ui.Boton, *ui.Boton) {
	t.Helper()
	var dlg *ui.Dialogo
	var cancelar, confirmar *ui.Boton
	fyne.DoAndWait(func() {
		c := fyne.CurrentApp().Driver().CanvasForObject(p.raiz)
		if c == nil || c.Overlays().Top() == nil {
			return
		}
		todos(c.Overlays().Top(), func(o fyne.CanvasObject) {
			switch x := o.(type) {
			case *ui.Dialogo:
				dlg = x
			case *ui.Boton:
				switch x.Texto {
				case p.t("ui.cancel"):
					cancelar = x
				case p.t("pdf.signatures.invalidate.apply"):
					confirmar = x
				}
			}
		})
	})
	if dlg == nil || cancelar == nil || confirmar == nil {
		t.Fatalf("no está abierto el diálogo de confirmación de invalidar firmas (dlg=%v cancelar=%v confirmar=%v)", dlg, cancelar, confirmar)
	}
	return dlg, cancelar, confirmar
}

// TestEl409DeInvalidarFirmasPreguntaAntesDeRepetir: cuando el motor contesta 409
// `pdf_invalidates_signatures`, la pantalla abre la confirmación nativa. Si se cancela,
// avisa con `hecho(false)` y no repite la petición; si se confirma, repite con
// `invalidar_firmas=1` y avisa con `hecho(true)`.
func TestEl409DeInvalidarFirmasPreguntaAntesDeRepetir(t *testing.T) {
	m := &motorDeFirmas{informe: informeDePrueba}
	p, listo, cerrar := abrirConFirmas(t, m)
	defer cerrar()

	cambio := api.PDFCambio{Op: "metadatos", Datos: map[string]string{"title": "Revisado"}}
	cancelado := make(chan bool, 1)
	fyne.DoAndWait(func() {
		p.Aplicar(cambio, "Metadatos guardados", func(ok bool) { cancelado <- ok })
	})
	esperarRed(t, listo)
	dlg, cancelar, _ := botonesDeConfirmacion(t, p)
	if dlg.Titulo != p.t("pdf.signatures.invalidate.apply") {
		t.Errorf("título del diálogo = %q; quiero %q", dlg.Titulo, p.t("pdf.signatures.invalidate.apply"))
	}
	fyne.DoAndWait(func() { cancelar.Tapped(nil) })
	select {
	case ok := <-cancelado:
		if ok {
			t.Error("cancelar la confirmación devolvió hecho(true)")
		}
	default:
		t.Fatal("cancelar la confirmación de invalidar firmas no llamó a hecho(false)")
	}
	if m.cambios.Load() != 1 || m.invalidaVista.Load() {
		t.Fatalf("tras cancelar: cambios=%d invalidaVista=%v", m.cambios.Load(), m.invalidaVista.Load())
	}

	confirmado := make(chan bool, 1)
	fyne.DoAndWait(func() {
		p.Aplicar(cambio, "Metadatos guardados", func(ok bool) { confirmado <- ok })
	})
	esperarRed(t, listo)
	_, _, confirmar := botonesDeConfirmacion(t, p)
	fyne.DoAndWait(func() { confirmar.Tapped(nil) })
	esperarRed(t, listo) // segunda llamada a /pdf/protect con invalidar_firmas=1
	esperarRed(t, listo) // recarga de contenido
	esperarRed(t, listo) // recarga de lote
	select {
	case ok := <-confirmado:
		if !ok {
			t.Error("confirmar la invalidación devolvió hecho(false)")
		}
	default:
		t.Fatal("confirmar la invalidación no llamó a hecho(true)")
	}
	if m.cambios.Load() != 3 || !m.invalidaVista.Load() {
		t.Errorf("tras confirmar: cambios=%d (quiero 3) invalidaVista=%v", m.cambios.Load(), m.invalidaVista.Load())
	}
}
