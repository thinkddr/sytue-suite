// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"context"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # «Traer mis datos»
//
// `ImportData` de @sytue/shell: el asistente de importación DENTRO del producto. El motor
// de migración (Google Workspace / Microsoft 365 → Sytue) solo tenía puerta en la consola
// de administración, y quien abre Drive vacío no va a pedirle a su administración que le
// migre los archivos: necesita el botón donde está mirando.
//
// Una sola pieza para las apps que traen contenido personal (Drive, Contacts, Mail,
// Calendar): lo que cambia es QUÉ trae, y eso es `dominios`. No aparece si la
// organización no ha conectado ningún origen —un botón que siempre falla es peor que
// ninguno— ni mientras se averigua, porque un bloque que aparece y desaparece al cargar
// da la sensación de que algo se rompió.
//
// Es ESTADO, no un widget: quien la monta la pinta con `Objeto` en cada repintado y le da
// `AlCambiar` para que la repinte cuando llega algo, que es como se pintan las pantallas
// de las apps de la suite.
type ImportarDatos struct {
	tr       *i18n.Translator
	svc      *api.Integraciones
	dominios []string

	// AlCambiar repinta a quien la monta. AlTerminar se llama cuando una importación
	// que estaba en curso acaba, para que la vista se recargue. AlPolitica abre la
	// política de privacidad del `DataFlow`.
	AlCambiar, AlTerminar, AlPolitica func()
	// Sondeo es cada cuánto se vuelve a preguntar mientras hay trabajo: los 5 s de la web.
	Sondeo time.Duration

	// plegado deja el bloque en su línea de título: abierto, la ayuda, el `DataFlow` y
	// los botones se comen media barra lateral. Se recuerda (`PreferenciaPlegado`).
	plegado bool

	origenes    []api.OrigenImportacion
	trabajos    []api.TrabajoImportacion
	cargada     bool
	enviando    bool
	fallo       bool
	ctx         context.Context
	cancelar    context.CancelFunc
	ciclo       uint64
	cargando    bool
	pararSondeo func()
	reloj       uint64
	esperar     func(time.Duration, func()) func()
	// aparte lanza el trabajo de red; nil es `go f()`. El guardián lo hace en el sitio.
	aparte func(func())
}

// NuevoImportarDatos crea la pieza para el contenido de `dominios` («drive»,
// «contacts», «mail», «calendar»).
func NuevoImportarDatos(tr *i18n.Translator, svc *api.Integraciones, dominios ...string) *ImportarDatos {
	d := &ImportarDatos{tr: tr, svc: svc, dominios: dominios, Sondeo: 5 * time.Second}
	if a := fyne.CurrentApp(); a != nil {
		d.plegado = a.Preferences().Bool(PreferenciaPlegado)
	}
	return d
}

// PreferenciaPlegado es la clave que recuerda el bloque plegado: la misma para todas
// las apps, como `sytue.import.plegado` en la web. Quien ya decidió no traer nada no
// quiere el bloque abierto en cada producto.
const PreferenciaPlegado = "import.plegado"

// Plegado dice si el bloque está en su línea de título.
func (d *ImportarDatos) Plegado() bool { return d.plegado }

// Alternar pliega o despliega el bloque, lo recuerda y lo repinta.
func (d *ImportarDatos) Alternar() {
	d.plegado = !d.plegado
	if a := fyne.CurrentApp(); a != nil {
		a.Preferences().SetBool(PreferenciaPlegado, d.plegado)
	}
	d.cambiar()
}

func (d *ImportarDatos) lanzar(f func()) {
	if d.aparte != nil {
		d.aparte(f)
		return
	}
	go f()
}

func (d *ImportarDatos) cambiar() {
	if d.AlCambiar != nil {
		d.AlCambiar()
	}
}

// enCurso dice si algo sigue trabajando.
func enCurso(ts []api.TrabajoImportacion) bool {
	for _, t := range ts {
		if t.EnCurso() {
			return true
		}
	}
	return false
}

// Cargar pregunta qué orígenes hay y cómo van las importaciones, y vuelve a preguntar
// mientras haya alguna en curso. Un fallo se queda en «no hay orígenes»: sin conexión con
// la pasarela no hay nada que ofrecer, y un error aquí solo alarmaría sobre algo que no
// se pidió.
func (d *ImportarDatos) Cargar() {
	if d.ctx == nil || d.ctx.Err() != nil {
		d.CargarCon(context.Background())
		return
	}
	d.cargar(d.ctx, d.ciclo)
}

// CargarCon liga las lecturas, importaciones explícitas y el sondeo a una vida de
// sesión. Una nueva vida retira el contenido anterior y descarta respuestas tardías.
func (d *ImportarDatos) CargarCon(ctx context.Context) {
	d.Parar()
	vida, cancelar := context.WithCancel(ctx)
	d.ctx, d.cancelar = vida, cancelar
	d.cargar(vida, d.ciclo)
}

// Parar retira el estado de la cuenta y detiene el sondeo. No deshace una
// importación que el servidor haya aceptado ni vuelve a enviarla automáticamente.
func (d *ImportarDatos) Parar() {
	d.ciclo++
	if d.cancelar != nil {
		d.cancelar()
	}
	d.pararReloj()
	d.origenes, d.trabajos = nil, nil
	d.cargada, d.enviando, d.fallo, d.cargando = false, false, false, false
	d.cambiar()
}

func (d *ImportarDatos) vigente(ctx context.Context, ciclo uint64) bool {
	return ctx.Err() == nil && ciclo == d.ciclo
}

func (d *ImportarDatos) cargar(parent context.Context, ciclo uint64) {
	if d.svc == nil {
		return
	}
	if parent == nil || !d.vigente(parent, ciclo) || d.cargando {
		return
	}
	d.pararReloj()
	d.cargando = true
	d.lanzar(func() {
		if parent.Err() != nil {
			return
		}
		ctx, cancelar := context.WithTimeout(parent, 30*time.Second)
		defer cancelar()
		origenes, err1 := d.svc.OrigenesImportacion(ctx)
		ts, err2 := d.svc.MisImportaciones(ctx, d.dominios)
		fyne.Do(func() {
			if !d.vigente(parent, ciclo) {
				return
			}
			d.cargando = false
			antes := enCurso(d.trabajos)
			if err1 != nil || err2 != nil {
				origenes, ts = nil, nil
			}
			d.origenes, d.trabajos, d.cargada = origenes, ts, true
			if antes && !enCurso(ts) && d.AlTerminar != nil {
				d.AlTerminar()
			}
			d.seguir(parent, ciclo)
			d.cambiar()
		})
	})
}

// seguir programa la siguiente pregunta si hay trabajo en curso.
func (d *ImportarDatos) seguir(ctx context.Context, ciclo uint64) {
	d.pararReloj()
	if enCurso(d.trabajos) && d.Sondeo > 0 {
		reloj := d.reloj
		if ctx == nil || !d.vigente(ctx, ciclo) {
			return
		}
		esperar := d.esperar
		if esperar == nil {
			esperar = func(plazo time.Duration, f func()) func() {
				t := time.AfterFunc(plazo, func() { fyne.Do(f) })
				return func() { t.Stop() }
			}
		}
		d.pararSondeo = esperar(d.Sondeo, func() {
			if d.vigente(ctx, ciclo) && reloj == d.reloj {
				d.cargar(ctx, ciclo)
			}
		})
	}
}

func (d *ImportarDatos) pararReloj() {
	d.reloj++
	if d.pararSondeo != nil {
		d.pararSondeo()
		d.pararSondeo = nil
	}
}

// traer lanza la importación desde un origen.
func (d *ImportarDatos) traer(o api.OrigenImportacion) {
	parent, ciclo := d.ctx, d.ciclo
	if d.enviando || d.svc == nil || parent == nil || !d.vigente(parent, ciclo) {
		return
	}
	d.enviando, d.fallo = true, false
	d.cambiar()
	d.lanzar(func() {
		if parent.Err() != nil {
			return
		}
		ctx, cancelar := context.WithTimeout(parent, 30*time.Second)
		defer cancelar()
		nuevos, err := d.svc.Importar(ctx, o, d.dominios)
		fyne.Do(func() {
			if !d.vigente(parent, ciclo) {
				return
			}
			d.enviando = false
			if err != nil {
				d.fallo = true
			} else {
				d.trabajos = append(nuevos, d.trabajos...)
				// La web no vuelve a preguntar tras lanzarla y se queda en «trayendo»
				// hasta que se recarga la página; aquí se sigue el progreso.
				d.seguir(parent, ciclo)
			}
			d.cambiar()
		})
	})
}

// Objeto es la sección a `ancho`, o nil si no hay nada que ofrecer.
//
// `border-border bg-surface rounded-md border p-3`: el título en `text-sm font-semibold`;
// mientras trae, la rueda de 16 con el recuento; si no, la ayuda, el `DataFlow` compacto y
// un botón por origen (`flex flex-wrap gap-2`). Debajo, el recuento de lo traído —SIEMPRE,
// también lo que se quedó fuera— y los fallos en `danger-text`.
func (d *ImportarDatos) Objeto(ancho float32, v tema.Variante) fyne.CanvasObject {
	if !d.cargada || d.ctx == nil || d.ctx.Err() != nil || (len(d.origenes) == 0 && len(d.trabajos) == 0) {
		return nil
	}
	t := func(k string, vars ...i18n.Vars) string {
		var x i18n.Vars
		if len(vars) > 0 {
			x = vars[0]
		}
		return d.tr.T(k, x)
	}
	relleno := tema.Borde + tema.Espacio(3)
	col := max(ancho-2*relleno, 0)
	var objs []fyne.CanvasObject
	y := relleno
	poner := func(o fyne.CanvasObject, alto, antes float32) {
		y += antes
		o.Move(fyne.NewPos(relleno, y))
		o.Resize(fyne.NewSize(col, alto))
		objs = append(objs, o)
		y += alto
	}
	parrafo := func(texto string, e tema.Escalon, p Peso, tinta string, antes float32) {
		x := NuevoParrafo(texto, e, p, tinta, v)
		poner(x, AltoParrafo(x, col), antes)
	}
	trabajando, hechos, fallidos := repartirTrabajos(d.trabajos)

	fila, altoFila := d.filaTitulo(col, len(trabajando) > 0, v)
	poner(fila, altoFila, 0)

	if d.plegado {
		return d.caja(ancho, y+relleno, objs, v)
	}
	if len(trabajando) > 0 {
		rueda := NuevaRueda(16, "text", v)
		x := NuevoParrafo(t("import.running", i18n.Vars{"count": contarTraidos(trabajando)}), tema.TextoXS, Normal, "text-muted", v)
		hueco := tema.Espacio(2)
		alto := max(16, AltoParrafo(x, col-16-hueco))
		fila := container.NewWithoutLayout(rueda, x)
		rueda.Move(fyne.NewPos(0, (alto-16)/2))
		rueda.Resize(fyne.NewSize(16, 16))
		x.Move(fyne.NewPos(16+hueco, (alto-AltoParrafo(x, col-16-hueco))/2))
		x.Resize(fyne.NewSize(col-16-hueco, AltoParrafo(x, col-16-hueco)))
		poner(fila, alto, tema.Espacio(2))
	} else {
		parrafo(t("import.help"), tema.TextoXS, Normal, "text-muted", 0.5*0.25*tema.Rem)
		entra := make([]string, 0, len(d.dominios))
		for _, dom := range d.dominios {
			entra = append(entra, categoriaImportacion[dom])
		}
		flujo := FlujoDatos(d.tr, "", entra, nil, true, d.AlPolitica, col)
		poner(flujo, flujo.MinSize().Height, tema.Espacio(2))
		var piezas []PiezaFlujo
		for _, o := range d.origenes {
			clave := "import.fromMicrosoft"
			if o.Source == "google" {
				clave = "import.fromGoogle"
			}
			origen := o
			b := NuevoBoton(t(clave), func() { d.traer(origen) }).Con(Secundario, TallaSM)
			b.Inerte = d.enviando
			m := b.MinSize()
			piezas = append(piezas, PiezaFlujo{O: b, Ancho: m.Width, Alto: m.Height, Sep: tema.Espacio(2)})
		}
		if len(piezas) > 0 {
			botones, alto := Fluir(piezas, col, piezas[0].Alto, tema.Espacio(2))
			poner(botones, alto, tema.Espacio(2))
		}
	}
	if len(hechos) > 0 {
		omitidos := 0
		for _, j := range hechos {
			omitidos += j.Counts.Skipped
		}
		parrafo(t("import.done", i18n.Vars{"count": contarTraidos(hechos), "skipped": omitidos}), tema.TextoXS, Normal, "text-subtle", tema.Espacio(2))
	}
	if fallidos {
		parrafo(t("import.someFailed"), tema.TextoXS, Normal, "danger-text", tema.Espacio(2))
	}
	if d.fallo {
		parrafo(t("import.failed"), tema.TextoXS, Normal, "danger-text", tema.Espacio(2))
	}
	return d.caja(ancho, y+relleno, objs, v)
}

// filaTitulo es la línea de título: el rótulo y, a la derecha, el galón que pliega
// (`flex items-center justify-between gap-2`). Plegado y trabajando, la rueda de 14
// sigue diciendo que algo pasa.
func (d *ImportarDatos) filaTitulo(col float32, trabajando bool, v tema.Variante) (fyne.CanvasObject, float32) {
	clave := "import.collapse"
	galon := iconos.ChevronAbajo
	if d.plegado {
		clave, galon = "import.expand", iconos.ChevronDerecha
	}
	b := NuevoBoton("", d.Alternar).Con(Fantasma, TallaXS)
	b.Icono, b.TamIcono, b.SoloIcono = galon, 14, true
	b.Rotulo = d.tr.T(clave, nil) + ": " + d.tr.T("import.title", nil)
	mb := b.MinSize()
	hueco := tema.Espacio(2)
	rueda := float32(0)
	if d.plegado && trabajando {
		rueda = 14 + hueco
	}
	ancho := max(col-mb.Width-hueco-rueda, 0)
	x := NuevoParrafo(d.tr.T("import.title", nil), tema.TextoSM, SemiNegrita, "text", v)
	ax := AltoParrafo(x, ancho)
	alto := max(ax, mb.Height)
	objsFila := []fyne.CanvasObject{x, b}
	x.Move(fyne.NewPos(0, (alto-ax)/2))
	x.Resize(fyne.NewSize(ancho, ax))
	if rueda > 0 {
		r := NuevaRueda(14, "text", v)
		r.Move(fyne.NewPos(min(Medir(d.tr.T("import.title", nil), tema.TextoSM, SemiNegrita).Width, ancho)+hueco, (alto-14)/2))
		r.Resize(fyne.NewSize(14, 14))
		objsFila = append(objsFila, r)
	}
	b.Move(fyne.NewPos(col-mb.Width, (alto-mb.Height)/2))
	b.Resize(mb)
	return container.NewWithoutLayout(objsFila...), alto
}

// repartirTrabajos separa los trabajos en curso, los terminados y si alguno falló.
func repartirTrabajos(ts []api.TrabajoImportacion) (trabajando, hechos []api.TrabajoImportacion, fallidos bool) {
	for _, j := range ts {
		switch {
		case j.EnCurso():
			trabajando = append(trabajando, j)
		case j.Status == "done":
			hechos = append(hechos, j)
		case j.Status == "error":
			fallidos = true
		}
	}
	return trabajando, hechos, fallidos
}

// caja envuelve lo pintado en el `border-border bg-surface rounded-md`.
func (d *ImportarDatos) caja(ancho, alto float32, objs []fyne.CanvasObject, v tema.Variante) fyne.CanvasObject {
	caja := NuevaCaja(tema.Color("surface", v), tema.Color("border", v), tema.RadioMD)
	caja.Redimensionar(fyne.NewSize(ancho, alto))
	return container.New(tamFijo{fyne.NewSize(ancho, alto)}, append(caja.Objetos(), objs...)...)
}

// categoriaImportacion es el `CATEGORIA` de la web: qué nombre tiene en el `DataFlow` lo
// que trae cada dominio.
var categoriaImportacion = map[string]string{
	"mail": "mail", "drive": "files", "calendar": "events", "contacts": "contacts",
}

// contarTraidos es el total de elementos traídos por una tanda de trabajos.
func contarTraidos(ts []api.TrabajoImportacion) int {
	n := 0
	for _, t := range ts {
		n += t.Counts.Migrated
	}
	return n
}
