// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"cmp"
	"context"
	"errors"
	"slices"
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
// Calendar): lo que cambia es QUÉ trae, y eso es `dominios`. Un botón por CUENTA, no por
// conexión —la misma cuenta conectada varias veces salía como varios «Traerlo de Google»
// idénticos—, con la cuenta en el rótulo cuando hay más de una del mismo proveedor, una
// línea por contenido con lo traído y cuándo terminó, y el motivo de un fallo con su
// «Reintentar». Las decisiones sin pintar llevan los nombres de la web
// (`packages/shell/src/importacion.ts`). No aparece si la
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

	origenes []api.OrigenImportacion
	trabajos []api.TrabajoImportacion
	cargada  bool
	enviando bool
	// fallo es el último lanzamiento rechazado, con el código de la pasarela; nil si no
	// hubo. Una conexión caducada no se arregla «intentándolo dentro de un momento».
	fallo       *falloAlLanzar
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

// falloAlLanzar es un lanzamiento que la pasarela rechazó.
type falloAlLanzar struct{ code, source string }

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
	d.cargada, d.enviando, d.fallo, d.cargando = false, false, nil, false
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

// lanzamiento es lo que se pide a la pasarela de una vez: un origen y sus contenidos.
type lanzamiento struct {
	origen   api.OrigenImportacion
	dominios []string
}

// traer lanza la importación desde un origen.
func (d *ImportarDatos) traer(o api.OrigenImportacion) {
	d.lanzarImportaciones([]lanzamiento{{origen: o, dominios: d.dominios}})
}

// lanzarImportaciones pide a la pasarela cada lanzamiento, uno tras otro, en una sola
// vuelta. La pasarela es idempotente: si esa cuenta y contenido ya tenían un trabajo,
// devuelve ese —en marcha, o reanudado donde se quedó— y aquí SUSTITUYE al que había en
// vez de sumarse a él.
//
//nolint:contextcheck // el importador usa la vida que guardó al cargarse; lo lanza un clic, no el flujo que lo pinta.
func (d *ImportarDatos) lanzarImportaciones(ls []lanzamiento) {
	parent, ciclo := d.ctx, d.ciclo
	if len(ls) == 0 || d.enviando || d.svc == nil || parent == nil || !d.vigente(parent, ciclo) {
		return
	}
	d.enviando, d.fallo = true, nil
	d.cambiar()
	d.lanzar(func() {
		if parent.Err() != nil {
			return
		}
		ctx, cancelar := context.WithTimeout(parent, 30*time.Second)
		defer cancelar()
		var nuevos []api.TrabajoImportacion
		var fallo *falloAlLanzar
		for _, l := range ls {
			ts, err := d.svc.Importar(ctx, l.origen, l.dominios)
			if err != nil {
				code := ""
				var apiErr *api.Error
				if errors.As(err, &apiErr) {
					code = apiErr.Code()
				}
				fallo = &falloAlLanzar{code: code, source: l.origen.Source}
				break
			}
			nuevos = append(nuevos, ts...)
		}
		fyne.Do(func() {
			if !d.vigente(parent, ciclo) {
				return
			}
			d.enviando, d.fallo = false, fallo
			if len(nuevos) > 0 {
				d.trabajos = api.UltimasImportaciones(append(nuevos, sinLosDe(d.trabajos, nuevos)...))
				// La web no vuelve a preguntar tras lanzarla y se queda en «trayendo»
				// hasta que se recarga la página; aquí se sigue el progreso.
				d.seguir(parent, ciclo)
			}
			d.cambiar()
		})
	})
}

// sinLosDe quita de `ts` los trabajos que vuelven en `nuevos`: un relanzamiento devuelve
// el MISMO trabajo, y no debe contarse dos veces.
func sinLosDe(ts, nuevos []api.TrabajoImportacion) []api.TrabajoImportacion {
	var out []api.TrabajoImportacion
	for _, t := range ts {
		if !slices.ContainsFunc(nuevos, func(n api.TrabajoImportacion) bool { return n.ID == t.ID }) {
			out = append(out, t)
		}
	}
	return out
}

// reintentar vuelve a lanzar lo que falló, con sus contenidos: cada trabajo sigue donde
// se quedó y vuelve a intentar SOLO lo que no entró. Un lanzamiento por cuenta.
func (d *ImportarDatos) reintentar(fallidos []api.TrabajoImportacion) {
	var ls []lanzamiento
	for _, j := range fallidos {
		o := api.OrigenImportacion{InstanceID: j.InstanceID, InstanceIDs: []string{j.InstanceID}, Source: j.Source}
		for _, x := range d.origenes {
			if esDeOrigen(j, x) {
				o = x
				break
			}
		}
		i := slices.IndexFunc(ls, func(l lanzamiento) bool {
			return l.origen.Source == o.Source && l.origen.InstanceID == o.InstanceID
		})
		if i < 0 {
			ls = append(ls, lanzamiento{origen: o})
			i = len(ls) - 1
		}
		if !slices.Contains(ls[i].dominios, j.Domain) {
			ls[i].dominios = append(ls[i].dominios, j.Domain)
		}
	}
	d.lanzarImportaciones(ls)
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
	r := repartirImportaciones(d.origenes, d.trabajos, d.dominios)

	fila, altoFila := d.filaTitulo(col, len(r.trabajando) > 0, v)
	poner(fila, altoFila, 0)

	if d.plegado {
		return d.caja(ancho, y+relleno, objs, v)
	}
	if len(r.trabajando) > 0 {
		rueda := NuevaRueda(16, "text", v)
		x := NuevoParrafo(t("import.running", i18n.Vars{"count": contarTraidos(r.trabajando)}), tema.TextoXS, Normal, "text-muted", v)
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
	}
	piezas, apagados := d.botonesDeOrigenes(r.libres)
	if len(piezas) > 0 {
		botones, alto := Fluir(piezas, col, piezas[0].Alto, tema.Espacio(2))
		poner(botones, alto, tema.Espacio(2))
	}
	for _, m := range apagados {
		parrafo(m, tema.TextoXS, Normal, "text-muted", tema.Espacio(2))
	}
	// Una línea por contenido (y cuenta) con lo acumulado y cuándo terminó: «traído» a
	// secas no dice si llegó todo, y lo que se quedó fuera es la mitad honesta.
	for _, j := range r.hechos {
		parrafo(d.lineaHecha(j, r.variasCuentas), tema.TextoXS, Normal, "text-subtle", tema.Espacio(2))
	}
	if len(r.fallidos) > 0 {
		for _, m := range d.motivosDeFallo(r.fallidos, apagados) {
			parrafo(m, tema.TextoXS, Normal, "danger-text", tema.Espacio(2))
		}
		fallidos := r.fallidos
		b := NuevoBoton(t("action.retry"), func() { d.reintentar(fallidos) }).Con(Fantasma, TallaXS)
		b.Inerte = d.enviando || len(r.trabajando) > 0
		poner(b, b.MinSize().Height, tema.Espacio(2))
	}
	if d.fallo != nil {
		texto := t("import.failed")
		if claveDeFallo(d.fallo.code) == "import.unavailable" {
			texto = t("import.unavailable", i18n.Vars{"source": marcas[d.fallo.source]})
		}
		parrafo(texto, tema.TextoXS, Normal, "danger-text", tema.Espacio(2))
	}
	return d.caja(ancho, y+relleno, objs, v)
}

// botonesDeOrigenes es un botón por origen libre y, de los que no pueden leer, el aviso
// con el motivo. Las cuentas con una importación en marcha ya vienen fuera (`libres`):
// la pasarela devolvería la misma, y un botón que no hace nada confunde.
func (d *ImportarDatos) botonesDeOrigenes(libres []api.OrigenImportacion) (piezas []PiezaFlujo, apagados []string) {
	repetidos := proveedoresRepetidos(d.origenes)
	for _, o := range libres {
		origen := o
		b := NuevoBoton(rotuloOrigen(d.tr, o, repetidos), func() { d.traer(origen) }).Con(Secundario, TallaSM)
		b.Inerte = d.enviando || !o.Disponible()
		m := b.MinSize()
		piezas = append(piezas, PiezaFlujo{O: b, Ancho: m.Width, Alto: m.Height, Sep: tema.Espacio(2)})
		if !o.Disponible() {
			apagados = append(apagados, d.tr.T("import.unavailable", i18n.Vars{"source": marcaDeOrigen(o, repetidos)}))
		}
	}
	return piezas, apagados
}

// lineaHecha es la línea del resumen de una importación terminada: qué trajo, cuánto
// —acumulado de todas sus pasadas, también lo que se quedó fuera— y cuándo terminó.
func (d *ImportarDatos) lineaHecha(j api.TrabajoImportacion, variasCuentas bool) string {
	cuando := j.CreatedAt
	if j.FinishedAt != nil {
		cuando = *j.FinishedAt
	}
	return d.tr.T("import.done", i18n.Vars{
		"what": d.queTrajo(j, variasCuentas), "count": j.Counts.Migrated, "skipped": j.Counts.Skipped,
		"date": i18n.FormatDate(d.tr.Locale(), cuando.Local()),
	})
}

// motivosDeFallo es un aviso por motivo distinto: el correo y los contactos de la misma
// cuenta caducada son UN problema, y si ya lo dice el origen apagado, sobra.
func (d *ImportarDatos) motivosDeFallo(fallidos []api.TrabajoImportacion, apagados []string) []string {
	var motivos []string
	for _, j := range fallidos {
		m := d.tr.T(claveDeFallo(j.ErrorCode), i18n.Vars{"source": marcas[j.Source]})
		if !slices.Contains(motivos, m) && !slices.Contains(apagados, m) {
			motivos = append(motivos, m)
		}
	}
	return motivos
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

// marcas son los nombres de los proveedores. Marcas comerciales: no se traducen.
var marcas = map[string]string{"google": "Google", "m365": "Microsoft 365"}

// esDeOrigen dice si un trabajo es de un origen: por la cuenta si se sabe de las dos
// partes; si no, por la conexión, mirando TODAS las de la cuenta.
func esDeOrigen(j api.TrabajoImportacion, o api.OrigenImportacion) bool {
	if j.Source != o.Source {
		return false
	}
	if j.AccountKey != "" && o.AccountKey != "" {
		return j.AccountKey == o.AccountKey
	}
	return slices.Contains(o.InstanceIDs, j.InstanceID)
}

// proveedoresRepetidos son los proveedores con más de un origen: solo ahí el botón lleva
// la cuenta («Traerlo de Google · acme.com»).
func proveedoresRepetidos(os []api.OrigenImportacion) map[string]bool {
	vistos, repetidos := map[string]bool{}, map[string]bool{}
	for _, o := range os {
		if vistos[o.Source] {
			repetidos[o.Source] = true
		}
		vistos[o.Source] = true
	}
	return repetidos
}

// cuentaDeOrigen es cómo se nombra la cuenta: su dominio o, sin él, el nombre de la conexión.
func cuentaDeOrigen(o api.OrigenImportacion) string {
	if o.AccountLabel != "" {
		return o.AccountLabel
	}
	return o.Name
}

// rotuloOrigen es el texto del botón: «Traerlo de Google», con la cuenta detrás si hay
// más de una del mismo proveedor.
func rotuloOrigen(tr *i18n.Translator, o api.OrigenImportacion, repetidos map[string]bool) string {
	clave := "import.fromMicrosoft"
	if o.Source == "google" {
		clave = "import.fromGoogle"
	}
	if repetidos[o.Source] {
		return tr.T(clave, nil) + " · " + cuentaDeOrigen(o)
	}
	return tr.T(clave, nil)
}

// marcaDeOrigen es el proveedor tal y como se nombra en un aviso, con la cuenta si hace falta.
func marcaDeOrigen(o api.OrigenImportacion, repetidos map[string]bool) string {
	if repetidos[o.Source] {
		return marcas[o.Source] + " · " + cuentaDeOrigen(o)
	}
	return marcas[o.Source]
}

// queTrajo es el contenido de una línea del resumen: «Correo», y la cuenta detrás si el
// resumen mezcla cuentas.
func (d *ImportarDatos) queTrajo(j api.TrabajoImportacion, variasCuentas bool) string {
	tipo := d.tr.T("dataflow.item."+categoriaImportacion[j.Domain], nil)
	if variasCuentas && j.AccountLabel != "" {
		return tipo + " · " + j.AccountLabel
	}
	return tipo
}

// repartoImportaciones es lo que hay que pintar, ya decidido.
type repartoImportaciones struct {
	trabajando, hechos, fallidos []api.TrabajoImportacion
	// libres son los orígenes que se pueden ofrecer: los que no tienen ya una
	// importación en marcha de este contenido.
	libres        []api.OrigenImportacion
	variasCuentas bool
}

// repartirImportaciones es el `repartir` de la web: separa los trabajos de este
// contenido en curso, terminados y fallidos, y deja fuera de los botones las cuentas que
// ya están trayendo. Los trabajos llegan reducidos al vigente de cada cuenta y contenido
// (`api.UltimasImportaciones`): no se suma nada dos veces.
func repartirImportaciones(os []api.OrigenImportacion, ts []api.TrabajoImportacion, dominios []string) repartoImportaciones {
	var r repartoImportaciones
	cuentas := map[string]bool{}
	for _, j := range ts {
		if !slices.Contains(dominios, j.Domain) {
			continue
		}
		cuentas[cmp.Or(j.AccountKey, "instance:"+j.InstanceID)] = true
		switch {
		case j.EnCurso():
			r.trabajando = append(r.trabajando, j)
		case j.Status == "done":
			r.hechos = append(r.hechos, j)
		case j.Status == "error":
			r.fallidos = append(r.fallidos, j)
		}
	}
	for _, o := range os {
		if !slices.ContainsFunc(r.trabajando, func(j api.TrabajoImportacion) bool { return esDeOrigen(j, o) }) {
			r.libres = append(r.libres, o)
		}
	}
	r.variasCuentas = len(cuentas) > 1
	return r
}

// claveDeFallo es qué decir de un fallo: el motivo con arreglo, si lo tiene.
func claveDeFallo(code string) string {
	switch code {
	case "connection_expired", "import_connection_unavailable":
		return "import.unavailable"
	case "needs_admin":
		return "import.needsAdmin"
	default:
		return "import.someFailed"
	}
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
