// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"encoding/json"
	"io/fs"
	"slices"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// identidad traduce devolviendo la clave. Sirve para mirar QUÉ claves pide cada estado
// sin montar catálogos, que es justo lo que este guardián quiere comprobar.
func identidad(clave string) string { return clave }

// TestLosSeisEstadosDicenCosasDISTINTAS es el guardián de los seis estados, y no
// comprueba que existan —eso lo haría un `switch` exhaustivo y lo verifica el
// compilador— sino que cada uno DIGA algo distinto.
//
// Es la diferencia que importa: una pantalla con seis ramas que pintan el mismo «ha
// habido un error» tiene seis estados en el código y uno solo para quien la usa. «Sin
// conexión» y «la sesión ha caducado» piden cosas opuestas —esperar, o volver a
// entrar—, y confundirlas deja a alguien reintentando contra un servidor que está bien.
func TestLosSeisEstadosDicenCosasDISTINTAS(t *testing.T) {
	t.Parallel()
	// Listo va aparte: no tiene estado vacío porque pinta el documento.
	if titulo, _ := ClavesDelEstado(editor.Listo); titulo != "" {
		t.Errorf("Listo devuelve la clave %q; tendría que pintar el documento", titulo)
	}
	cinco := []editor.Estado{
		editor.Cargando, editor.Vacio, editor.Error,
		editor.SinConexion, editor.SinPermiso,
	}
	vistos := map[string]editor.Estado{}
	for _, e := range cinco {
		titulo, desc := ClavesDelEstado(e)
		if titulo == "" {
			t.Errorf("el estado %d no pide ninguna clave: estaría saliendo en blanco", e)
			continue
		}
		firma := titulo + "|" + desc
		if otro, repe := vistos[firma]; repe {
			t.Errorf("los estados %d y %d pintan lo MISMO (%s): para quien mira son un solo estado",
				otro, e, firma)
		}
		vistos[firma] = e
	}
}

// TestLasClavesDeLaPantallaExistenEnElCatalogo es el guardián barato que más vale de
// todos en esta suite: una clave que no existe NO explota, SE PINTA. El título de la
// ventana de Docus fue literalmente «app.name» durante un rato.
//
// Se comprueba contra el catálogo de la WEB, que es la fuente, y contra el inglés, que
// es el que `i18n.New` carga siempre de respaldo.
func TestLasClavesDeLaPantallaExistenEnElCatalogo(t *testing.T) {
	t.Parallel()
	crudo, err := fs.ReadFile(catalogos.Docs(), "en.json")
	if err != nil {
		t.Fatal(err)
	}
	// `map[string]any` y no `map[string]string`: el catálogo trae alguna entrada
	// ANIDADA (`bib.updated` es un objeto), y con `string` el Unmarshal falla entero
	// y el guardián se cae en vez de comprobar nada.
	var cat map[string]any
	if err := json.Unmarshal(crudo, &cat); err != nil {
		t.Fatal(err)
	}
	// Y el común, que es de donde salen `ui.cancel`, `forbidden.*` o `account.signIn`: la
	// app carga los dos (`i18n.New(l, i18n.Common(), catalogos.Docs())`).
	comun, err := fs.ReadFile(i18n.Common(), "en.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(comun, &cat); err != nil {
		t.Fatal(err)
	}

	pedidas := map[string]bool{}
	// Las de los seis estados.
	for _, e := range []editor.Estado{
		editor.Cargando, editor.Vacio, editor.Error,
		editor.SinConexion, editor.SinPermiso,
	} {
		titulo, desc := ClavesDelEstado(e)
		pedidas[titulo] = true
		if desc != "" {
			pedidas[desc] = true
		}
	}
	// Las de la hoja, la cinta y la paleta.
	pedidas["pdf.page.failed"] = true
	pedidas["pdf.page.loading"] = true
	pedidas["pdf.tools"] = true
	pedidas["pdf.blocked.annotate"] = true
	pedidas["pdf.ribbon.group.document"] = true
	pedidas["pdf.ribbon.group.security"] = true
	pedidas["pdf.ribbon.group.stroke"] = true
	pedidas["pdf.thickness"] = true
	// Las del panel lateral: la tira entera —también las pestañas sin portar, que
	// están en la tabla— y los textos de los tres paneles que se pintan.
	for _, x := range tiraDelPanel(&Contenido{Adjuntos: []Adjunto{{}}}) {
		pedidas[x.clave] = true
	}
	for _, e := range []editor.Estado{editor.SinSesion} {
		titulo, desc := ClavesDelEstado(e)
		pedidas[titulo], pedidas[desc] = true, true
	}
	for _, c := range []string{
		"ui.cancel", "pdf.comments.flatten.confirm",
		"pdf.outline.empty", "pdf.outline.noTarget", "pdf.attachments.empty",
		"pdf.page.n", "pdf.signatures.empty",
		"pdf.signatures.unnamed", "pdf.signatures.covers", "pdf.signatures.partial",
		"pdf.signatures.disclaimer",
	} {
		pedidas[c] = true
	}
	for _, r := range rotuloPestana {
		pedidas[r] = true
	}
	for _, h := range herramientas {
		pedidas[h.clave] = true
	}
	for _, c := range ColoresAnotacion {
		pedidas[c.Clave] = true
	}
	for _, c := range []string{
		"pdf.menu.organize", "pdf.menu.tools", "pdf.menu.stamp",
		"pdf.menu.compare", "pdf.menu.protect", "pdf.menu.sign",
	} {
		pedidas[c] = true
	}

	faltan := make([]string, 0)
	for c := range pedidas {
		if _, ok := cat[c]; !ok {
			faltan = append(faltan, c)
		}
	}
	slices.Sort(faltan)
	if len(faltan) > 0 {
		t.Errorf("%d claves que el código pide y el catálogo de la web NO tiene; se "+
			"pintarían tal cual en pantalla: %v", len(faltan), faltan)
	}
}

// TestLaCintaEsLaDeLaWeb compara las tablas de la cinta con las de `Barra.tsx`.
//
// Las dos cosas que vigila se rompen en silencio: una herramienta que desaparece de su
// pestaña simplemente deja de existir para quien la busca, y censurar fuera de
// «Firmar» vuelve a quedar al lado del resaltador, que es de donde se le sacó a
// propósito por ser lo único de la cinta que DESTRUYE contenido.
func TestLaCintaEsLaDeLaWeb(t *testing.T) {
	t.Parallel()

	// `EN_PESTANA` de `Barra.tsx`, tal cual.
	quiero := map[PestanaCinta][]Herramienta{
		PestInicio:     {Mano, Resaltar, Subrayar, Tachar, Nota},
		PestAnotar:     {Lapiz, Rectangulo, Elipse, Linea, Flecha, Medida, TextoLibre, Sello},
		PestFormulario: {CampoNuevo, Recorte},
		PestOrganizar:  nil,
		PestFirmar:     {Censura},
		PestVer:        nil,
	}
	for p, hs := range quiero {
		if !slices.Equal(enPestana[p], hs) {
			t.Errorf("pestaña %d: %v, la web pone %v", p, enPestana[p], hs)
		}
	}

	// Toda herramienta de la tabla tiene que estar EN alguna pestaña: una que no esté
	// no se puede elegir, y su fila en `herramientas` engaña al siguiente que la lea.
	for _, h := range herramientas {
		en := false
		for _, hs := range enPestana {
			if slices.Contains(hs, h.id) {
				en = true
			}
		}
		if !en {
			t.Errorf("la herramienta %q no está en ninguna pestaña: es inalcanzable", h.clave)
		}
	}

	// `conTrazo` es, LITERAL, la línea de `Barra.tsx`:
	//
	//	const conTrazo = herramientas.some((h) => h.id !== 'mano' && h.id !== 'recorte');
	//
	// Así que Formulario SÍ ofrece trazo: «campo» no es ni la mano ni el recorte. Lo
	// escribo con el resultado de la web y no con el que yo esperaba, porque la
	// primera versión de este test afirmaba lo contrario —me pareció que un campo de
	// formulario no dibuja— y habría acabado «arreglando» el código para que dejara
	// de parecerse a la web.
	quieroTrazo := map[PestanaCinta]bool{
		PestInicio:     true,  // resaltar, subrayar, tachar, nota
		PestAnotar:     true,  // todas dibujan
		PestFormulario: true,  // campo sí; recorte no, pero basta con una
		PestOrganizar:  false, // sin herramientas
		PestFirmar:     false, // censura: su barra es negra siempre
		PestVer:        false, // sin herramientas
	}
	for p, q := range quieroTrazo {
		if hay := conTrazo(enPestana[p]); hay != q {
			t.Errorf("pestaña %d: conTrazo=%v, la web da %v", p, hay, q)
		}
	}
}

// TestElGrupoDeTrazoEsElDeLaWeb comprueba las tres cosas del grupo «Trazo» que se
// rompen en silencio: que esté donde `conTrazo` dice y solo ahí, que sus seis muestras
// lleven los seis colores de `COLORES` con UNA sola marcada, y que pulsar una cambie el
// color elegido — un selector que se pinta pero no elige es peor que no tenerlo, porque
// todo lo que se anote saldrá del color de por defecto sin que nadie lo note.
func TestElGrupoDeTrazoEsElDeLaWeb(t *testing.T) {
	// Sin t.Parallel: mide texto, y el conformador de Fyne comparte búfer entre llamadas
	// (visto con -race: harfbuzz.(*Buffer).Clear desde dos pruebas a la vez).
	for _, p := range []PestanaCinta{PestInicio, PestAnotar, PestFormulario, PestOrganizar, PestFirmar, PestVer} {
		b := NuevaBarra(identidad)
		b.Elegir(p)
		hay := false
		for _, g := range gruposDe(b) {
			if g.Rotulo == "pdf.ribbon.group.stroke" {
				hay = true
			}
		}
		if quiero := conTrazo(enPestana[p]); hay != quiero {
			t.Errorf("pestaña %d: grupo «Trazo» presente=%v, conTrazo dice %v", p, hay, quiero)
		}
	}

	b := NuevaBarra(identidad)
	if b.Grosor != GrosorPorDefecto {
		t.Errorf("la cinta nace con grosor %d; `PdfApp.tsx` arranca en %d", b.Grosor, GrosorPorDefecto)
	}
	muestras := b.muestrasDeColor()
	if len(muestras) != len(ColoresAnotacion) {
		t.Fatalf("%d muestras para %d colores: alguna no se pinta y su color es inalcanzable",
			len(muestras), len(ColoresAnotacion))
	}
	elegidas := 0
	for i, o := range muestras {
		m, ok := sinPista(o).(*ui.MuestraColor)
		if !ok {
			t.Fatalf("la muestra %d es %T y no *ui.MuestraColor", i, o)
		}
		// `pdf/Barra.tsx` fija `h-[24px] w-[24px]`, incluida la muestra elegida.
		if got := m.MinSize(); got != fyne.NewSize(24, 24) {
			t.Errorf("la muestra %d mide %v; Barra.tsx declara 24×24 px", i, got)
		}
		if m.Elegida {
			elegidas++
		}
	}
	if elegidas != 1 {
		t.Errorf("hay %d muestras marcadas; `aria-pressed` es de una sola", elegidas)
	}

	// Y que elegir ELIJA: la tercera del grupo es el azul.
	const tercera = 2
	sinPista(b.muestrasDeColor()[tercera]).(*ui.MuestraColor).Tapped(nil)
	if b.Color != tercera {
		t.Errorf("tras pulsar la muestra %d el color de la cinta es %d", tercera, b.Color)
	}
	if got := sinPista(b.muestrasDeColor()[tercera]).(*ui.MuestraColor).Elegida; !got {
		t.Error("la muestra pulsada no se pinta marcada: el color elegido no se ve por ninguna parte")
	}
}

// TestFirmarEsLaPestanaDeFirmar es `pdf/Barra.test.tsx`: la pestaña se llamaba
// «Proteger», como el botón de la contraseña que lleva dentro, y quien venía a firmar no
// la encontraba (orden del dueño, 2026-10-16). Se fija lo que se ve: el rótulo, que lo
// primero es firmar y que la contraseña sigue a mano con su propio nombre.
func TestFirmarEsLaPestanaDeFirmar(t *testing.T) {
	// Sin t.Parallel: mide texto (ver TestElGrupoDeTrazoEsElDeLaWeb).
	if got := rotuloPestana[PestFirmar]; got != "pdf.menu.sign" {
		t.Errorf("la pestaña de firmar se rotula con %q y no con «Firmar» (pdf.menu.sign)", got)
	}
	for p, r := range rotuloPestana {
		if r == "pdf.menu.protect" {
			t.Errorf("la pestaña %d vuelve a llamarse como el botón de la contraseña", p)
		}
	}

	b := NuevaBarra(identidad)
	b.Elegir(PestFirmar)
	grupos := gruposDe(b)
	rotulos, botones := make([]string, 0, len(grupos)), make([]string, 0, 3)
	for _, g := range grupos {
		rotulos = append(rotulos, g.Rotulo)
		for _, c := range g.Controles {
			if a, ok := sinPista(c).(*Accion); ok {
				botones = append(botones, a.Texto)
			}
		}
	}
	if quiero := []string{"pdf.panel.signatures", "pdf.ribbon.group.security", "pdf.tools"}; !slices.Equal(rotulos, quiero) {
		t.Errorf("grupos de Firmar: %v; la web pone %v", rotulos, quiero)
	}
	if quiero := []string{"pdf.menu.sign", "pdf.menu.requestSignature", "pdf.menu.protect"}; !slices.Equal(botones, quiero) {
		t.Errorf("botones de Firmar: %v; la web pone %v", botones, quiero)
	}

	// Y cada uno abre SU diálogo.
	var abiertos []Dialogo
	b.AlPulsarDialogo = func(d Dialogo) { abiertos = append(abiertos, d) }
	for _, g := range gruposDe(b) {
		for _, c := range g.Controles {
			if a, ok := sinPista(c).(*Accion); ok {
				a.Tapped(nil)
			}
		}
	}
	if quiero := []Dialogo{DialogoFirmar, DialogoPedirFirma, DialogoProteger}; !slices.Equal(abiertos, quiero) {
		t.Errorf("los botones de Firmar abren %v; la web, %v", abiertos, quiero)
	}
}

// gruposDe son los grupos de la banda de la cinta tal y como se van a pintar.
func gruposDe(b *Barra) []*editor.Grupo {
	var out []*editor.Grupo
	c, ok := b.bandaDeGrupos().(*fyne.Container)
	if !ok {
		return nil
	}
	for _, o := range c.Objects {
		if g, ok := o.(*editor.Grupo); ok {
			out = append(out, g)
		}
	}
	return out
}

// TestSinPermisoSoloQuedaLaMano comprueba el cortafuegos de los permisos: con un
// documento que pide que no se anote, solo la mano queda viva y las demás salen
// INERTES y con el motivo por rótulo, no grises y calladas.
//
// Se comprueba en los dos sentidos, y la segunda mitad es la que hace que valga: sin
// ella, un `Inerte = true` puesto siempre pasaría la primera tan campante.
func TestSinPermisoSoloQuedaLaMano(t *testing.T) {
	// Sin t.Parallel: mide texto, y el conformador de Fyne comparte búfer entre llamadas
	// (visto con -race: harfbuzz.(*Buffer).Clear desde dos pruebas a la vez).
	casos := []struct {
		puede         bool
		quieroInertes int
	}{
		{true, 0},
		{false, len(enPestana[PestInicio]) - 1}, // todas menos la mano
	}
	for _, c := range casos {
		b := NuevaBarra(identidad)
		b.PuedeAnotar = c.puede
		inertes, motivos := 0, 0
		for _, o := range b.botonesDeHerramienta(enPestana[PestInicio]) {
			pista, ok := o.(*ui.Pista)
			if !ok {
				t.Fatalf("un botón de herramienta es %T y no lleva pista (el `title` de la web)", o)
			}
			bi, ok := pista.Hijo.(*ui.BotonIcono)
			if !ok {
				t.Fatalf("un botón de herramienta es %T y no *ui.BotonIcono", pista.Hijo)
			}
			if bi.Inerte {
				inertes++
				// El motivo va en la pista; el nombre sigue siendo el de la herramienta.
				if pista.Rotulo == "pdf.blocked.annotate" && bi.Rotulo != "pdf.blocked.annotate" {
					motivos++
				}
			}
		}
		if inertes != c.quieroInertes {
			t.Errorf("PuedeAnotar=%v: %d botones inertes, esperaba %d", c.puede, inertes, c.quieroInertes)
		}
		if motivos != inertes {
			t.Errorf("PuedeAnotar=%v: %d de %d botones inertes dicen POR QUÉ; un botón "+
				"gris sin motivo hace que se pruebe tres veces", c.puede, motivos, inertes)
		}
	}
}

// TestElCampoDeLaClaveEsElDeLaWeb: `px-3 py-2 text-sm` con su borde son 36 de alto
// (18 + 2 × 8 + 2). La entrada `md` de la suite también mide 36, pero su letra es
// `text-base`: la talla de 13 px distingue el contrato aunque coincida la altura.
func TestElCampoDeLaClaveEsElDeLaWeb(t *testing.T) {
	var objs []fyne.CanvasObject
	todos(nuevoFormularioClave(func(c string) string { return c }, "", func(string) {}),
		func(o fyne.CanvasObject) { objs = append(objs, o) })
	e, ok := primero[*ui.Entrada](objs)
	if !ok {
		t.Fatal("el formulario de la contraseña no lleva campo")
	}
	if got := e.MinSize().Height; !cerca32(got, 36) {
		t.Errorf("el campo de la contraseña mide %g de alto; la web, 36", got)
	}
	if e.Propia == nil || e.Propia.Texto.Tam != 13 {
		t.Errorf("el campo de la contraseña no conserva text-sm (0,8125rem = 13 px): %v", e.Propia)
	}
}

// sinPista es el control que envuelve una pista de la cinta, o el objeto tal cual.
func sinPista(o fyne.CanvasObject) fyne.CanvasObject {
	if p, ok := o.(*ui.Pista); ok {
		return p.Hijo
	}
	return o
}

// TestLasPistasDeLaCintaVanAlPortalYSeLimpian: la pista de un botón de la cinta se pinta
// en el portal de la pantalla, fuera del desplazador que la recortaría, y rehacer la
// cinta vacía el portal: la pista abierta era de un botón que ya no está.
func TestLasPistasDeLaCintaVanAlPortalYSeLimpian(t *testing.T) {
	p := NuevaPantalla(traductorES(t))
	w := ventanaSinRelleno(p.Objeto())
	defer w.Close()
	w.Resize(fyne.NewSize(1280, 800))
	fyne.DoAndWait(func() { p.PonerContenido(documentoDePrueba()) })
	var pista *ui.Pista
	todos(p.raiz, func(o fyne.CanvasObject) {
		if x, ok := o.(*ui.Pista); ok && x.Rotulo == p.t("pdf.tool.highlight") {
			pista = x
		}
	})
	if pista == nil {
		t.Fatal("el botón de resaltar no lleva pista")
	}
	fyne.DoAndWait(pista.Abrir)
	if len(p.portal.Objects) == 0 {
		t.Fatal("la pista abierta no está en el portal")
	}
	fyne.DoAndWait(p.Recomponer)
	if len(p.portal.Objects) != 0 {
		t.Errorf("tras rehacer la cinta quedan %d objetos de una pista huérfana en el portal", len(p.portal.Objects))
	}
}
