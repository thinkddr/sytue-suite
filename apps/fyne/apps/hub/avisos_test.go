// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// Números DERIVADOS de las clases de `NotificationsInbox.tsx` con el rem de 16, no
// medidos: la sonda del portal no recorre la bandeja todavía. La cuenta va escrita al
// lado para que se pueda revisar sin abrir el código.
const (
	// `py-3` 24 + título 18 + `mt-0.5` 2 + cuerpo 18 + `mt-1` 4 + meta 16.
	altoAvisoCompleto = 82
	// Sin cuerpo: 24 + 18 + 4 + 16. La marca (2 + 18) no llega.
	altoAvisoSinCuerpo = 62
	// Carga: `py-3` 24 + la columna de 14 + `gap-2` 8 + 12.
	altoFilaCarga = 58
	// La barra con «Marcar todas»: el botón `h-9` (36) manda sobre la píldora.
	altoBarraConBoton = 36
	// Las reglas sin su párrafo, a dos columnas, con `tema.Rem`: la cabecera (`mt-2`,
	// filete, `pt-6`, h2, `mt-1`, `mb-4`) 75; cinco leyendas de 16 con su `mb-3` de 12;
	// diez filas de interruptores de 22 (4+7+2+3+2 orígenes de dos en dos), cinco
	// `gap-y-3` de 12 dentro de los grupos y cuatro `gap-6` de 24 entre ellos.
	// Y la fila fija de Sytue ID: `gap-6` 24 + interruptor 22 + `gap-0.5` 2 (sin su nota).
	altoReglasSinParrafo = 75 + 5*(16+12) + 10*22 + 5*12 + 4*24 + 24 + 22 + 2
)

func avisosDePrueba() []api.HubNotification {
	ayer := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	return []api.HubNotification{
		{ID: "1", Source: "mail", Title: "Correo nuevo de Grace", Body: "Acta de la reunión", URL: "https://mail.sytue.eu/m/1", CreatedAt: ayer},
		{ID: "2", Source: "drive", Title: "Ana ha compartido «Presupuesto»", Read: true, CreatedAt: ayer},
		{ID: "3", Source: "mail", Title: "Recordatorio", Body: "Mañana a las 10", CreatedAt: ayer},
	}
}

func bandejaDePrueba(t *testing.T) (*Avisos, *avisosRenderer) {
	t.Helper()
	a := NuevaAvisos(traductor(t))
	a.Ahora = time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	a.Estado, a.Items = Listo, avisosDePrueba()
	r := test.WidgetRenderer(a).(*avisosRenderer)
	return a, r
}

// TestLaBandejaMideLoQueDicenSusClases ata la geometría a las clases de la web.
//
// Visto en ROJO a mano el 2026-09-22 (en el frontal): quitando `SangriaMetaAviso` de
// AltoFilaAviso —el `mt-1` de la línea de origen—, la fila completa pasa a 68,25 y el
// test lo dice con los dos números.
func TestLaBandejaMideLoQueDicenSusClases(t *testing.T) {
	a, r := bandejaDePrueba(t)
	img := pintaWidget(t, a, r.alto)
	guardar(t, img, "avisos-exito")
	_, ancho := CajaContenido(ventana)

	if len(r.filas) != 3 {
		t.Fatalf("hay %d filas y son tres avisos", len(r.filas))
	}
	if got := AltoFilaAviso(r.filas[0], ancho); !cerca(got, altoAvisoCompleto) {
		t.Errorf("la fila con cuerpo mide %v y sus clases dan %v", got, altoAvisoCompleto)
	}
	if got := AltoFilaAviso(r.filas[1], ancho); !cerca(got, altoAvisoSinCuerpo) {
		t.Errorf("la fila sin cuerpo mide %v y sus clases dan %v", got, altoAvisoSinCuerpo)
	}
	if got := AltoFilaEsqueleto(); !cerca(got, altoFilaCarga) {
		t.Errorf("la fila de carga mide %v y sus clases dan %v", got, altoFilaCarga)
	}
	if got := r.barra(0, 0, ancho, false); !cerca(got, altoBarraConBoton) {
		t.Errorf("la barra mide %v y sus clases dan %v", got, altoBarraConBoton)
	}
	if got := r.altoReglas() - ui.AltoParrafo(r.textoReglas, r.textoReglas.Ancho) -
		ui.AltoParrafo(r.notaFijo, r.notaFijo.Ancho); !cerca(got, altoReglasSinParrafo) {
		t.Errorf("las reglas miden %v sin el párrafo y sus clases dan %v", got, altoReglasSinParrafo)
	}
	// Lo que Fyne coloca es lo que se declara: la segunda fila empieza donde acaba la
	// primera más el filete de `divide-y`.
	f0, f1 := r.filas[0], r.filas[1]
	if got := f1.Position().Y - f0.Position().Y; !cerca(got, altoAvisoCompleto+tema.Borde) {
		t.Errorf("de la primera fila a la segunda hay %v y tiene que haber %v", got, altoAvisoCompleto+tema.Borde)
	}

	// El punto de sin leer se PINTA en `primary`: `bg-primary h-2 w-2`, en el centro de
	// su botón (`p-2`), que va a la derecha de la fila con su `mt-3`.
	x := int(f0.Position().X + ancho - LadoBotonLeido/2)
	y := int(f0.Position().Y + MargenYAviso + LadoBotonLeido/2)
	c, g, b, _ := img.At(x, y).RGBA()
	got := color.NRGBA{R: uint8(c >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: 255}
	if quiero := tema.Color("primary", tema.Claro); !parecido(got, quiero) {
		t.Errorf("el punto de sin leer pinta %v en (%d,%d) y `primary` es %v", got, x, y, quiero)
	}
}

// TestLaPildoraInactivaEnsenaLaPagina: un filtro sin poner es un aro de `border` sobre
// el papel, no una pastilla gris. Vale para la bandeja y para la actividad, que
// comparten la pieza.
//
// Visto en ROJO a mano el 2026-09-22 (en el frontal): con el relleno transparente de
// antes, el interior de «Drive» pinta el gris de `border` y el test lo dice.
func TestLaPildoraInactivaEnsenaLaPagina(t *testing.T) {
	a, r := bandejaDePrueba(t)
	img := pintaWidget(t, a, r.alto)
	p := r.filtros[3] // «Drive», inactiva
	px := func(x, y float32) color.NRGBA {
		c, g, b, _ := img.At(int(x), int(y)).RGBA()
		return color.NRGBA{R: uint8(c >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: 255}
	}
	// Por dentro del borde, entre la marca y el rótulo no hay tinta: a la derecha del
	// rótulo, a media altura.
	dentro := px(p.Position().X+p.Size().Width-MargenPildora/2-tema.Borde, p.Position().Y+p.Size().Height/2)
	if quiero := tema.Color("bg", tema.Claro); !parecido(dentro, quiero) {
		t.Errorf("dentro de una píldora inactiva pinta %v y el papel `bg` es %v", dentro, quiero)
	}
}

func parecido(a, b color.NRGBA) bool {
	d := func(x, y uint8) bool { return int(x)-int(y) > 2 || int(y)-int(x) > 2 }
	return !d(a.R, b.R) && !d(a.G, b.G) && !d(a.B, b.B)
}

// TestLaBandejaEnSusSeisEstados: los seis se pintan, y ninguno se pinta como otro.
func TestLaBandejaEnSusSeisEstados(t *testing.T) {
	visto := map[string]string{}
	for _, caso := range []struct {
		nombre string
		e      Estado
		items  []api.HubNotification
	}{
		{"carga", Cargando, nil},
		{"lista", Listo, avisosDePrueba()},
		{"vacio", Listo, nil},
		{"error", Fallo, nil},
		{"sin-conexion", SinConexion, nil},
		{"prohibido", Prohibido, nil},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			a := NuevaAvisos(traductor(t))
			a.AlReintentar = func() {}
			a.Estado, a.Items = caso.e, caso.items
			r := test.WidgetRenderer(a).(*avisosRenderer)
			guardar(t, pintaWidget(t, a, r.alto), "avisos-"+caso.nombre)
			titulo := ""
			if r.aviso != nil {
				titulo = r.aviso.Titulo
			}
			firma := fmt.Sprintf("esq=%d filas=%d aviso=%q alto=%.2f", len(r.esqueleto), len(r.filas), titulo, r.alto(ventana))
			if otro, repe := visto[firma]; repe {
				t.Errorf("el estado %q se pinta igual que %q: %s", caso.nombre, otro, firma)
			}
			visto[firma] = caso.nombre
		})
	}
}

// TestLosFiltrosDeLaBandeja: «sin leer» deja lo no leído, el producto solo lo suyo, y
// los filtros de producto son los que HAN avisado, sin repetir.
//
// Visto en ROJO a mano el 2026-09-22 (en el frontal): quitando el `visto` de
// hacerBarra, Mail sale dos veces y el test lo dice.
func TestLosFiltrosDeLaBandeja(t *testing.T) {
	a, r := bandejaDePrueba(t)
	// Todas, Sin leer y un filtro por producto presente: Mail y Drive.
	if len(r.filtros) != 4 {
		t.Errorf("hay %d filtros y tienen que ser 4 (Todas, Sin leer, Mail, Drive)", len(r.filtros))
	}
	if r.marcar == nil {
		t.Error("con avisos sin leer no se ofrece «Marcar todas»")
	}
	if n := len(Visibles(a.Items, true, "")); n != 2 {
		t.Errorf("sin leer quedan %d y son 2", n)
	}
	if n := len(Visibles(a.Items, false, "drive")); n != 1 {
		t.Errorf("de Drive quedan %d y es 1", n)
	}
	if n := len(Visibles(a.Items, true, "drive")); n != 0 {
		t.Errorf("sin leer de Drive quedan %d y no hay ninguno", n)
	}
	// Pulsar el filtro de un producto lo pone, y pulsarlo otra vez lo quita.
	test.Tap(r.filtros[2])
	if a.Producto != "mail" || len(r.filas) != 2 {
		t.Errorf("con el filtro de Mail: producto %q y %d filas", a.Producto, len(r.filas))
	}
	test.Tap(r.filtros[2])
	if a.Producto != "" {
		t.Errorf("pulsar otra vez Mail no quita el filtro: %q", a.Producto)
	}
	// Todo leído: ni «Marcar todas» ni avisos que mostrar con «sin leer».
	for i := range a.Items {
		a.Items[i].Read = true
	}
	a.SoloSinLeer = true
	a.Refresh()
	if r.marcar != nil || r.aviso == nil || r.aviso.Titulo != a.T.T("inbox.emptyUnreadTitle", nil) {
		t.Errorf("con todo leído y el filtro puesto: marcar=%v aviso=%+v", r.marcar != nil, r.aviso)
	}
}

// TestLasReglasDeSilencio: el interruptor dice «recibir» y el Hub guarda «silenciado»,
// y solo cuenta la regla del origen ENTERO.
func TestLasReglasDeSilencio(t *testing.T) {
	reglas := []api.HubNotificationPref{{Source: "mail", Muted: true}, {Source: "drive", Kind: "share", Muted: true}}
	if !Silenciado(reglas, "mail") || Silenciado(reglas, "drive") {
		t.Errorf("mail silenciado=%v (sí) y drive=%v (no: solo un tipo)", Silenciado(reglas, "mail"), Silenciado(reglas, "drive"))
	}
	reglas = ConRegla(reglas, "mail", false)
	if Silenciado(reglas, "mail") || len(reglas) != 2 {
		t.Errorf("tras volver a encender Mail: %+v", reglas)
	}
	a := NuevaAvisos(traductor(t))
	a.Estado, a.Reglas = Listo, []api.HubNotificationPref{{Source: "rooms", Muted: true}}
	var origen string
	var silencio bool
	a.AlSilenciar = func(o string, s bool) { origen, silencio = o, s }
	r := test.WidgetRenderer(a).(*avisosRenderer)
	if len(r.conmutadores) != len(OrigenesSilenciables) {
		t.Fatalf("hay %d interruptores y son %d", len(r.conmutadores), len(OrigenesSilenciables))
	}
	if r.conmutadores["rooms"].Encendido || !r.conmutadores["mail"].Encendido {
		t.Error("Rooms silenciado sale encendido, o Mail sin regla sale apagado")
	}
	if r.conmutadores["mail"].Etiqueta != "Sytue Mail" {
		t.Errorf("el interruptor de Mail se llama %q y en la web «Sytue Mail»", r.conmutadores["mail"].Etiqueta)
	}
	test.Tap(r.conmutadores["mail"])
	if origen != "mail" || !silencio {
		t.Errorf("apagar Mail pide al Hub %q silenciado=%v", origen, silencio)
	}
	// Sytue ID sale al final, encendido e inerte, con su porqué: como en la web.
	if r.fijo == nil || !r.fijo.Encendido || !r.fijo.Disabled() || !seVe(r, r.fijo) {
		t.Fatal("el interruptor de Sytue ID no sale fijo y encendido")
	}
	if r.notaFijo == nil || !seVe(r, r.notaFijo) || r.notaFijo.Texto == "" || r.notaFijo.Texto == "inbox.prefsAlwaysOn" {
		t.Error("falta la nota «Siempre activos por tu seguridad» bajo Sytue ID")
	}
	if objs := r.Objects(); objs[len(objs)-2] != r.fijo {
		t.Error("Sytue ID no sale al final de las reglas")
	}
	origen = ""
	test.Tap(r.fijo)
	if origen != "" || !r.fijo.Encendido {
		t.Errorf("pulsar Sytue ID pide al Hub %q o lo apaga", origen)
	}
}

// TestLosGruposDeAvisosSonLosDeLaWeb: la lista nativa es `ORIGENES_DE_AVISOS` de
// @sytue/shell, grupo a grupo y en el mismo orden.
//
// Visto en ROJO a mano el 2026-11-10: con la lista de seis de antes no casa ni el
// primer grupo.
func TestLosGruposDeAvisosSonLosDeLaWeb(t *testing.T) {
	ts := webref.Read(t, "apps/frontend/packages/shell/src/avisos.ts")
	i := strings.Index(ts, "export const ORIGENES_DE_AVISOS")
	if i < 0 {
		t.Fatal("avisos.ts ya no exporta ORIGENES_DE_AVISOS: revisar el espejo")
	}
	bloque := ts[i:]
	_, bloque, ok := strings.Cut(bloque, "= [")
	bloque, _, ok2 := strings.Cut(bloque, "];")
	if !ok || !ok2 {
		t.Fatal("ORIGENES_DE_AVISOS ya no es una lista literal: revisar el espejo")
	}
	web := make([]GrupoAvisos, 0, len(GruposDeAvisos))
	for _, m := range regexp.MustCompile(`(?s)grupo:\s*'(\w+)',\s*origenes:\s*\[([^\]]*)\]`).FindAllStringSubmatch(bloque, -1) {
		g := GrupoAvisos{Grupo: m[1]}
		for _, o := range regexp.MustCompile(`'(\w+)'`).FindAllStringSubmatch(m[2], -1) {
			g.Origenes = append(g.Origenes, o[1])
		}
		web = append(web, g)
	}
	if len(web) == 0 || !reflect.DeepEqual(web, GruposDeAvisos) {
		t.Errorf("los grupos nativos son\n%v\ny los de la web\n%v", GruposDeAvisos, web)
	}
}

// seVe dice si el objeto está entre lo que la pantalla pinta.
func seVe(r *avisosRenderer, o fyne.CanvasObject) bool { return slices.Contains(r.Objects(), o) }

// TestLasReglasEnteras: todos los orígenes, por familias, con el nombre de cada uno, y el
// ámbito de las reglas dicho; una cuenta personal no ve ERP, Bases ni Web.
func TestLasReglasEnteras(t *testing.T) {
	a := NuevaAvisos(traductor(t))
	a.Estado, a.Organizacion = Listo, "Acme"
	r := test.WidgetRenderer(a).(*avisosRenderer)
	for _, o := range []string{"erp", "cast", "pages", "photos", "integrations"} {
		if c := r.conmutadores[o]; c == nil || !seVe(r, c) {
			t.Errorf("no hay interruptor para %q", o)
		}
	}
	if got := r.conmutadores["integrations"].Etiqueta; got != a.T.T("inbox.prefsSource.integrations", nil) {
		t.Errorf("Integraciones se llama %q", got)
	}
	if len(r.leyendas) != 5 || r.leyendas[0].Text != strings.ToUpper(a.T.T("inbox.prefsGroup.communication", nil)) {
		t.Errorf("las leyendas no son las cinco familias: %d", len(r.leyendas))
	}
	if !strings.Contains(r.textoReglas.Texto, "Acme") {
		t.Errorf("el texto no dice de qué organización son las reglas: %q", r.textoReglas.Texto)
	}

	a.Personal = true
	a.Refresh()
	for _, o := range SoloOrganizaciones {
		if seVe(r, r.conmutadores[o]) {
			t.Errorf("una cuenta personal ve el interruptor de %q", o)
		}
	}
	if strings.Contains(r.textoReglas.Texto, "Acme") || !strings.Contains(r.textoReglas.Texto, a.T.T("inbox.prefsScopePersonal", nil)) {
		t.Errorf("una cuenta personal no dice que sus reglas son las de su cuenta: %q", r.textoReglas.Texto)
	}
	if len(r.leyendas) != 4 {
		t.Errorf("sin ERP ni Bases el grupo de negocio sobra y hay %d leyendas", len(r.leyendas))
	}
}

// TestLasReglasSinCargarNoOfrecenInterruptores: mientras cargan, fantasmas sin nada que
// pulsar; si fallan, el aviso con «Reintentar» y NINGÚN interruptor, que saldrían
// encendidos aunque hubiera cosas calladas.
//
// Visto en ROJO a mano el 2026-11-10: sin `CargaReglas`, los diecinueve interruptores
// salen pintados y encendidos en los dos estados.
func TestLasReglasSinCargarNoOfrecenInterruptores(t *testing.T) {
	a := NuevaAvisos(traductor(t))
	a.Estado, a.CargaReglas = Listo, ReglasCargando
	r := test.WidgetRenderer(a).(*avisosRenderer)
	for o, c := range r.conmutadores {
		if seVe(r, c) {
			t.Errorf("cargando, se ofrece el interruptor de %q", o)
		}
	}
	if seVe(r, r.fijo) {
		t.Error("cargando, ya sale la fila fija de Sytue ID")
	}
	if len(r.fantasmas) != len(OrigenesSilenciables) {
		t.Errorf("cargando hay %d fantasmas y son %d orígenes", len(r.fantasmas), len(OrigenesSilenciables))
	}

	reintentos := 0
	a.CargaReglas, a.AlReintentarReglas = ReglasFallo, func() { reintentos++ }
	a.Refresh()
	for o, c := range r.conmutadores {
		if seVe(r, c) {
			t.Errorf("con la carga fallida, se ofrece el interruptor de %q", o)
		}
	}
	if r.falloReglas == nil || r.falloReglas.Tono != ui.AyudaPeligro || r.falloReglas.Titulo != a.T.T("inbox.prefsLoadError", nil) {
		t.Fatalf("con la carga fallida no se dice: %+v", r.falloReglas)
	}
	test.Tap(r.reintentar)
	if reintentos != 1 {
		t.Errorf("«Reintentar» pide las reglas %d veces", reintentos)
	}
	img := pintaWidget(t, a, r.alto)
	guardar(t, img, "avisos-reglas-fallo")
}

// TestLasReglasConElHubQueFalla: si la carga falla lo dice y reintentar las trae; si el
// guardado falla, el interruptor vuelve a donde estaba y su fila lo dice.
//
// Visto en ROJO a mano el 2026-11-10: con el `_, _ =` de antes en `silenciar`, Drive
// se queda silenciado en pantalla aunque el Hub no lo haya guardado.
func TestLasReglasConElHubQueFalla(t *testing.T) {
	d := &buzonDoble{
		items:       avisosDePrueba(),
		reglas:      []api.HubNotificationPref{{Source: "mail", Muted: true}},
		flujo:       make(chan api.HubNotification),
		falloReglas: errors.New("503"),
	}
	v := NuevaVista(traductor(t), nil, time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)).ConBuzon(d)
	lee := func(f func(a *Avisos) bool) func() bool {
		return func() bool {
			v.mu.Lock()
			defer v.mu.Unlock()
			return f(v.avisos)
		}
	}
	v.Ir(AAvisos)
	defer v.Ir(AInicio)
	esperar(t, "la carga fallida", lee(func(a *Avisos) bool { return a.Estado == Listo && a.CargaReglas == ReglasFallo }))

	d.mira(func() bool { d.falloReglas = nil; return true })
	fyne.DoAndWait(func() { v.avisos.AlReintentarReglas() })
	esperar(t, "reintentar", lee(func(a *Avisos) bool { return a.CargaReglas == ReglasListas && Silenciado(a.Reglas, "mail") }))

	d.mira(func() bool { d.falloGuardar = errors.New("500"); return true })
	r := test.WidgetRenderer(v.avisos).(*avisosRenderer)
	fyne.DoAndWait(func() { test.Tap(r.conmutadores["drive"]) })
	esperar(t, "el guardado fallido", lee(func(a *Avisos) bool { return a.FalloRegla["drive"] && !a.GuardandoRegla["drive"] }))
	if lee(func(a *Avisos) bool { return Silenciado(a.Reglas, "drive") || !Silenciado(a.Reglas, "mail") })() {
		t.Error("tras el fallo Drive sigue silenciado, o se ha perdido la regla de Mail")
	}
	fyne.DoAndWait(func() {
		if !r.conmutadores["drive"].Encendido {
			t.Error("el interruptor de Drive no ha vuelto a su sitio")
		}
		if p := r.fallosFila["drive"]; p == nil || !seVe(r, p) || p.Texto != v.avisos.T.T("inbox.prefsSaveError", nil) {
			t.Error("la fila de Drive no dice que no se ha guardado")
		}
	})

	// Un guardado bueno borra el aviso de la fila.
	d.mira(func() bool { d.falloGuardar = nil; return true })
	fyne.DoAndWait(func() { test.Tap(r.conmutadores["drive"]) })
	esperar(t, "el guardado bueno", lee(func(a *Avisos) bool {
		return !a.FalloRegla["drive"] && !a.GuardandoRegla["drive"] && Silenciado(a.Reglas, "drive")
	}))
}

// TestRecargarAvisosTraeLoQueGuardoElReceptor: el receptor de la entrada privada guarda
// un aviso nuevo en el almacén privado y la bandeja abierta lo enseña al recargar; con
// la bandeja cerrada no se pide nada.
func TestRecargarAvisosTraeLoQueGuardoElReceptor(t *testing.T) {
	d := &buzonDoble{items: avisosDePrueba(), flujo: make(chan api.HubNotification)}
	v := NuevaVista(traductor(t), nil, time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)).ConBuzon(d)
	v.RecargarAvisos() // cerrada: nada
	lee := func(f func(a *Avisos) bool) func() bool {
		return func() bool {
			v.mu.Lock()
			defer v.mu.Unlock()
			return f(v.avisos)
		}
	}
	v.Ir(AAvisos)
	defer v.Ir(AInicio)
	antes := len(avisosDePrueba())
	esperar(t, "la primera carga", lee(func(a *Avisos) bool { return a.Estado == Listo && len(a.Items) == antes }))
	d.mira(func() bool {
		d.items = append([]api.HubNotification{{ID: "privado", Source: "drive", Type: "security.storage.released", Title: "Guardado en tu almacén"}}, d.items...)
		return true
	})
	v.RecargarAvisos()
	esperar(t, "la recarga", lee(func(a *Avisos) bool { return len(a.Items) == antes+1 && a.Items[0].ID == "privado" }))
}

// buzonDoble es un Hub de mentira para la bandeja, seguro entre gorutinas.
type buzonDoble struct {
	falloReglas  error // si no es nil, NotificationPrefs falla
	falloGuardar error // si no es nil, SetNotificationPref falla
	mu           sync.Mutex
	items        []api.HubNotification
	reglas       []api.HubNotificationPref
	leidos       []string
	todas        int
	todasDe      []string // los orígenes de «Marcar todas» con un producto elegido
	desleidos    []string
	falloDesleer error // si no es nil, MarkUnread falla
	puestas      []api.HubNotificationPref
	flujo        chan api.HubNotification
	mas          []api.HubNotification // la página de detrás
	antes        []string              // a partir de qué id se pidió
	soltar       chan struct{}         // si no es nil, «Cargar más» espera a que se cierre
	cargas       int                   // cuántas veces se pidió la primera página
}

func (b *buzonDoble) Notifications(context.Context, int) ([]api.HubNotification, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.cargas++
	return slices.Clone(b.items), nil
}

func (b *buzonDoble) NotificationsAntes(_ context.Context, _ int, antes api.HubNotification) ([]api.HubNotification, error) {
	b.mu.Lock()
	b.antes = append(b.antes, antes.ID)
	espera := b.soltar
	b.mu.Unlock()
	if espera != nil {
		<-espera
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return slices.Clone(b.mas), nil
}

func (b *buzonDoble) StreamNotifications(ctx context.Context, cada func(api.HubNotification)) error {
	for {
		select {
		case n := <-b.flujo:
			cada(n)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (b *buzonDoble) MarkRead(_ context.Context, id string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.leidos = append(b.leidos, id)
	return nil
}

func (b *buzonDoble) MarkAllRead(context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.todas++
	return nil
}

func (b *buzonDoble) MarkAllReadDe(ctx context.Context, origen string) error {
	if origen == "" {
		return b.MarkAllRead(ctx)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.todasDe = append(b.todasDe, origen)
	return nil
}

func (b *buzonDoble) MarkUnread(_ context.Context, id string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.desleidos = append(b.desleidos, id)
	return b.falloDesleer
}

func (b *buzonDoble) NotificationPrefs(context.Context) ([]api.HubNotificationPref, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.falloReglas != nil {
		return nil, b.falloReglas
	}
	return b.reglas, nil
}

func (b *buzonDoble) SetNotificationPref(_ context.Context, p api.HubNotificationPref) (api.HubNotificationPref, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.puestas = append(b.puestas, p)
	if b.falloGuardar != nil {
		return api.HubNotificationPref{}, b.falloGuardar
	}
	return p, nil
}

func (b *buzonDoble) mira(f func() bool) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return f()
}

// esperar da hasta un segundo a que algo que va en segundo plano ocurra.
func esperar(t *testing.T, que string, f func() bool) {
	t.Helper()
	// Plazo por reloj, no por vueltas: con toda la suite en paralelo un segundo no
	// bastaba y el foco llegaba tarde (TestConSinLeerElFocoPasaAlVecino).
	for limite := time.Now().Add(5 * time.Second); ; {
		fyne.DoAndWait(func() {}) // lo que el fondo manda pintar corre en la rutina del test
		if f() {
			return
		}
		if time.Now().After(limite) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("no ha pasado: %s", que)
}

// TestLaBandejaHablaConElHub: cargar, lo que llega por el flujo y las tres acciones.
//
// Visto en ROJO a mano el 2026-09-22 (en el frontal): quitando el borrado del repetido
// en llegaAviso, el aviso que reenvía el flujo sale dos veces y el test lo dice.
func TestLaBandejaHablaConElHub(t *testing.T) {
	d := &buzonDoble{
		items:  avisosDePrueba(),
		reglas: []api.HubNotificationPref{{Source: "mail", Muted: true}},
		flujo:  make(chan api.HubNotification),
	}
	v := NuevaVista(traductor(t), nil, time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)).ConBuzon(d)
	lee := func(f func(a *Avisos) bool) func() bool {
		return func() bool {
			v.mu.Lock()
			defer v.mu.Unlock()
			return f(v.avisos)
		}
	}

	v.Ir(AAvisos)
	if v.Destino() != AAvisos {
		t.Fatalf("Ir(AAvisos) deja la vista en %v", v.Destino())
	}
	esperar(t, "carga", lee(func(a *Avisos) bool { return a.Estado == Listo && len(a.Items) == 3 && len(a.Reglas) == 1 }))

	// El flujo reenvía el primero con otro título: va delante y no se repite.
	d.flujo <- api.HubNotification{ID: "1", Source: "mail", Title: "Otra vez"}
	d.flujo <- api.HubNotification{ID: "9", Source: "rooms", Title: "Mensaje en #general"}
	esperar(t, "lo del flujo", lee(func(a *Avisos) bool {
		return len(a.Items) == 4 && a.Items[0].ID == "9" && a.Items[1].Title == "Otra vez"
	}))

	v.abrirAviso(api.HubNotification{ID: "3"})
	esperar(t, "marcar uno", func() bool { return d.mira(func() bool { return slices.Contains(d.leidos, "3") }) })

	v.marcarTodas()
	esperar(t, "marcar todas", func() bool { return d.mira(func() bool { return d.todas == 1 }) })
	if !lee(func(a *Avisos) bool { return a.sinLeer() == 0 })() {
		t.Error("tras marcar todas quedan avisos sin leer en pantalla")
	}

	v.silenciar("drive", true)
	esperar(t, "silenciar", func() bool {
		return d.mira(func() bool {
			return len(d.puestas) == 1 && d.puestas[0] == api.HubNotificationPref{Source: "drive", Muted: true}
		})
	})
	if !lee(func(a *Avisos) bool { return Silenciado(a.Reglas, "drive") && Silenciado(a.Reglas, "mail") })() {
		t.Error("tras silenciar Drive la pantalla no lo refleja, o ha perdido la regla de Mail")
	}

	// Al irse, el flujo se corta.
	v.Ir(AInicio)
	v.mu.Lock()
	escucha := v.pararFlujo != nil
	v.mu.Unlock()
	if escucha {
		t.Error("tras salir de la bandeja el flujo sigue abierto")
	}
}

// TestLaBandejaCargaMas: con una página llena sale «Cargar más», pide lo de detrás del
// último (por su id, que es lo que el Hub necesita para no perder los del mismo
// instante), lo añade sin repetir y, con una página corta, el botón se va.
func TestLaBandejaCargaMas(t *testing.T) {
	llena := make([]api.HubNotification, MaxAvisosBandeja)
	for i := range llena {
		llena[i] = api.HubNotification{ID: "n" + strconv.Itoa(i), Source: "mail", Title: "x", Read: true}
	}
	d := &buzonDoble{
		items: llena,
		mas:   []api.HubNotification{llena[len(llena)-1], {ID: "viejo", Source: "docs", Title: "y", Read: true}},
		flujo: make(chan api.HubNotification),
	}
	v := NuevaVista(traductor(t), nil, time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)).ConBuzon(d)
	lee := func(f func(a *Avisos) bool) func() bool {
		return func() bool {
			v.mu.Lock()
			defer v.mu.Unlock()
			return f(v.avisos)
		}
	}
	v.Ir(AAvisos)
	esperar(t, "página llena con «Cargar más»", lee(func(a *Avisos) bool { return a.Estado == Listo && a.HayMas }))
	var boton *ui.Boton
	fyne.DoAndWait(func() {
		for _, o := range test.WidgetRenderer(v.avisos).Objects() {
			if b, ok := o.(*ui.Boton); ok && b.Texto == v.avisos.T.T("notifications.loadMore", nil) {
				boton = b
			}
		}
	})
	if boton == nil {
		t.Fatal("con más avisos detrás, la bandeja no enseña «Cargar más»")
	}
	v.avisos.AlCargarMas()
	esperar(t, "la página de detrás", lee(func(a *Avisos) bool { return len(a.Items) == MaxAvisosBandeja+1 && !a.HayMas }))
	if !d.mira(func() bool { return slices.Equal(d.antes, []string{llena[len(llena)-1].ID}) }) {
		t.Errorf("pidió lo de detrás de %v, quiero el último de la lista", d.antes)
	}
	if !lee(func(a *Avisos) bool { return a.Items[len(a.Items)-1].ID == "viejo" })() {
		t.Error("lo de detrás no va al final")
	}
	v.Ir(AInicio)
}

// TestCargarMasConLaListaViva: con un filtro que no deja ver nada el botón sigue (lo que
// se busca puede estar detrás); dos pulsaciones seguidas son UNA petición; y una página
// que vuelve cuando la lista ya se recargó se tira, no se pega a la nueva.
func TestCargarMasConLaListaViva(t *testing.T) {
	llena := make([]api.HubNotification, MaxAvisosBandeja)
	for i := range llena {
		llena[i] = api.HubNotification{ID: "n" + strconv.Itoa(i), Source: "mail", Title: "x", Read: true}
	}
	d := &buzonDoble{
		items:  llena,
		mas:    []api.HubNotification{{ID: "viejo", Source: "docs", Title: "y", Read: true}},
		flujo:  make(chan api.HubNotification),
		soltar: make(chan struct{}),
	}
	v := NuevaVista(traductor(t), nil, time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)).ConBuzon(d)
	lee := func(f func(a *Avisos) bool) func() bool {
		return func() bool {
			v.mu.Lock()
			defer v.mu.Unlock()
			return f(v.avisos)
		}
	}
	v.Ir(AAvisos)
	esperar(t, "carga", lee(func(a *Avisos) bool { return a.Estado == Listo && a.HayMas }))

	fyne.DoAndWait(func() { v.avisos.SoloSinLeer = true; v.avisos.Refresh() })
	var boton bool
	fyne.DoAndWait(func() {
		for _, o := range test.WidgetRenderer(v.avisos).Objects() {
			if b, ok := o.(*ui.Boton); ok && b.Texto == v.avisos.T.T("notifications.loadMore", nil) {
				boton = true
			}
		}
	})
	if !boton {
		t.Error("con el filtro «sin leer» vacío y más avisos detrás, no hay «Cargar más»")
	}
	if lee(func(a *Avisos) bool { return a.T.T("notifications.noneLoaded", nil) == "notifications.noneLoaded" })() {
		t.Error("la clave del vacío con más detrás no está en el catálogo")
	}
	var titulo string
	fyne.DoAndWait(func() {
		for _, o := range test.WidgetRenderer(v.avisos).Objects() {
			if vc, ok := o.(*ui.Vacio); ok {
				titulo = vc.Titulo
			}
		}
	})
	if titulo != v.avisos.T.T("notifications.noneLoaded", nil) {
		t.Errorf("con más detrás el vacío dice %q: «estás al día» sería falso", titulo)
	}

	v.avisos.AlCargarMas()
	v.avisos.AlCargarMas()
	esperar(t, "la primera petición", func() bool { return d.mira(func() bool { return len(d.antes) >= 1 }) })
	if n := d.mira(func() bool { return len(d.antes) == 1 }); !n {
		t.Errorf("dos pulsaciones seguidas pidieron %v: quiero una", d.antes)
	}
	v.avisos.AlReintentar() // se recarga mientras la página vuelve
	esperar(t, "la recarga", func() bool {
		return d.mira(func() bool { return d.cargas == 2 }) &&
			lee(func(a *Avisos) bool { return a.Estado == Listo && len(a.Items) == MaxAvisosBandeja })()
	})
	close(d.soltar)
	for range 30 { // lo que manda pintar el fondo corre aquí; se le da tiempo de sobra
		fyne.DoAndWait(func() {})
		if lee(func(a *Avisos) bool { return len(a.Items) != MaxAvisosBandeja })() {
			t.Fatal("la página pedida antes de recargar se pegó a la lista nueva")
		}
		time.Sleep(10 * time.Millisecond)
	}
	v.Ir(AInicio)
}

// TestElPuntoMarcaUnoSinAbrirlo: el punto azul es un botón «Marcar como leído». Marca
// ESE aviso —al Hub y en pantalla—, no abre nada aunque el aviso lleve a otra app, deja
// los demás como estaban y, ya leído, el botón se va.
//
// Visto en ROJO a mano el 2026-11-05: enganchando el botón a `AlAbrir` en vez de a
// `AlMarcarLeido`, pulsar el punto abre el correo y el test lo dice.
func TestElPuntoMarcaUnoSinAbrirlo(t *testing.T) {
	d := &buzonDoble{items: avisosDePrueba(), flujo: make(chan api.HubNotification)}
	ab := &abridorDoble{}
	v := NuevaVista(traductor(t), nil, time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)).ConBuzon(d)
	v.abrir = ab
	lee := func(f func(a *Avisos) bool) func() bool {
		return func() bool {
			v.mu.Lock()
			defer v.mu.Unlock()
			return f(v.avisos)
		}
	}
	botones := func() []*BotonLeido {
		var out []*BotonLeido
		fyne.DoAndWait(func() {
			for _, o := range test.WidgetRenderer(v.avisos).Objects() {
				if b, ok := o.(*BotonLeido); ok {
					out = append(out, b)
				}
			}
		})
		return out
	}
	v.Ir(AAvisos)
	esperar(t, "carga", lee(func(a *Avisos) bool { return a.Estado == Listo && len(a.Items) == 3 }))

	bs := botones()
	if len(bs) != 2 {
		t.Fatalf("hay %d botones de punto y son dos avisos sin leer", len(bs))
	}
	if quiero := v.avisos.T.T("inbox.markRead", nil); bs[0].AccessibilityLabel() != quiero || quiero == "inbox.markRead" {
		t.Errorf("el punto se llama %q y tiene que llamarse %q", bs[0].AccessibilityLabel(), quiero)
	}
	if bs[0].AccessibilityRole() != fyne.AccessibleRoleButton {
		t.Error("el punto no se anuncia como botón")
	}
	test.Tap(bs[0]) // el del primero, que lleva a Mail
	esperar(t, "marcarlo en el Hub", func() bool { return d.mira(func() bool { return slices.Equal(d.leidos, []string{"1"}) }) })
	esperar(t, "marcarlo en pantalla", lee(func(a *Avisos) bool { return a.Items[0].Read }))
	if !lee(func(a *Avisos) bool { return !a.Items[2].Read })() {
		t.Error("marcar uno ha marcado también otro")
	}
	if len(ab.abiertas) != 0 {
		t.Errorf("marcar como leído ha abierto %v", ab.abiertas)
	}
	if n := len(botones()); n != 1 {
		t.Errorf("ya leído, quedan %d botones de punto y es uno", n)
	}
	v.Ir(AInicio)
}

// TestElPuntoVaDetrasDeSuFila: con Tab, cada «Marcar como leído» va justo detrás de su
// aviso —fila, punto, fila, punto—, que es el orden del DOM en la web; y al marcar con el
// foco puesto, el botón se va y el foco pasa a SU fila, no a ninguna parte.
//
// Visto en ROJO a mano el 2026-11-05: con los botones al final de `recomponer` (como
// antes), el Tab desde la primera fila cae en la segunda; y sin `devolverFoco`, tras
// marcar el foco se queda en nil. El test dice las dos cosas.
func TestElPuntoVaDetrasDeSuFila(t *testing.T) {
	d := &buzonDoble{items: avisosDePrueba(), flujo: make(chan api.HubNotification)}
	v := NuevaVista(traductor(t), nil, time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)).ConBuzon(d)
	ven := test.NewWindow(v.avisos)
	t.Cleanup(ven.Close)
	ven.Resize(fyne.NewSize(1280, 800))
	c := ven.Canvas()
	v.Ir(AAvisos)
	esperar(t, "carga", func() bool {
		v.mu.Lock()
		defer v.mu.Unlock()
		return v.avisos.Estado == Listo && len(v.avisos.Items) == 3
	})
	var r *avisosRenderer
	fyne.DoAndWait(func() { r = test.WidgetRenderer(v.avisos).(*avisosRenderer) })

	var foco fyne.Focusable
	fyne.DoAndWait(func() {
		c.Focus(r.filas[0])
		c.FocusNext()
		foco = c.Focused()
	})
	if foco != fyne.Focusable(r.leidos[0]) {
		t.Fatalf("Tab desde la primera fila va a %T y tenía que ir a su punto", foco)
	}
	fyne.DoAndWait(func() { r.leidos[0].TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn}) })
	esperar(t, "el foco en su fila", func() bool {
		var ok bool
		fyne.DoAndWait(func() {
			ok = len(r.filas) == 3 && r.ids[0] == "1" && c.Focused() == fyne.Focusable(r.filas[0])
		})
		return ok
	})
	v.Ir(AInicio)
}

// TestConSinLeerElFocoPasaAlVecino: con «Sin leer», el aviso marcado SALE de la lista con
// su fila, y el foco pasa a la del siguiente —que ocupa ahora su puesto— o a la del
// anterior si era el último. Como `MarcarLeido` en la web.
//
// Visto en ROJO a mano el 2026-11-06: sin el respaldo por puesto de `devolverFoco`, tras
// marcar el foco se queda en nil.
func TestConSinLeerElFocoPasaAlVecino(t *testing.T) {
	ayer := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	items := []api.HubNotification{
		{ID: "a", Source: "mail", Title: "Uno", CreatedAt: ayer},
		{ID: "b", Source: "mail", Title: "Dos", CreatedAt: ayer},
		{ID: "c", Source: "mail", Title: "Tres", CreatedAt: ayer},
	}
	d := &buzonDoble{items: items, flujo: make(chan api.HubNotification)}
	v := NuevaVista(traductor(t), nil, time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)).ConBuzon(d)
	ven := test.NewWindow(v.avisos)
	t.Cleanup(ven.Close)
	ven.Resize(fyne.NewSize(1280, 800))
	c := ven.Canvas()
	v.Ir(AAvisos)
	esperar(t, "carga", func() bool {
		v.mu.Lock()
		defer v.mu.Unlock()
		return v.avisos.Estado == Listo && len(v.avisos.Items) == 3
	})
	var r *avisosRenderer
	fyne.DoAndWait(func() {
		v.mu.Lock()
		v.avisos.SoloSinLeer = true
		v.mu.Unlock()
		v.avisos.Refresh()
		r = test.WidgetRenderer(v.avisos).(*avisosRenderer)
	})

	marcar := func(id string) {
		t.Helper()
		fyne.DoAndWait(func() {
			for i, x := range r.ids {
				if x == id {
					c.Focus(r.leidos[i])
					r.leidos[i].TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
					return
				}
			}
			t.Fatalf("no está la fila de %q", id)
		})
	}
	enfocada := func(id string) func() bool {
		return func() bool {
			var ok bool
			fyne.DoAndWait(func() {
				for i, x := range r.ids {
					if x == id {
						ok = c.Focused() == fyne.Focusable(r.filas[i])
					}
				}
			})
			return ok
		}
	}

	marcar("c") // el último: el foco va al anterior
	esperar(t, "el foco en «b»", enfocada("b"))
	marcar("a") // el primero: el foco va al siguiente, que ocupa su puesto
	esperar(t, "el foco en «b»", func() bool {
		var n int
		fyne.DoAndWait(func() { n = len(r.filas) })
		return n == 1 && enfocada("b")()
	})
	v.Ir(AInicio)
}
