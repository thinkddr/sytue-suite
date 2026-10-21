// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
)

// App es Sytue Calcus entero: la portada con las hojas de cálculo y, al abrir una, el
// editor —el cromo compartido con el lienzo de la hoja dentro—. Las dos son de ESTA app:
// en la web son `/calcus/` y `/calcus/edit?id=` del mismo subdominio.
//
// A diferencia de Docus, cuyo folio todavía es un visor, aquí se EDITA: lo que el lienzo
// entiende entero y el permiso deja escribir se guarda en el servicio con el
// autoguardado de la web (1,5 s tras el último cambio, `useDocument.ts`).
type App struct {
	T     func(string, i18n.Vars) string
	Svc   *api.Docs
	Red   *offline.Monitor
	Plazo time.Duration

	portada *editor.Portada
	avisos  *ui.PilaAvisos
	raiz    *fyne.Container
	abierto *abierto
	// lanzar corre una petición fuera del hilo de la interfaz; las pruebas la cambian
	// por una llamada directa.
	lanzar func(func())
	// abrirArchivo es «Abrir un archivo» del menú Archivo y de la portada (PonerAbrir).
	abrirArchivo *editor.AbrirArchivo
	// EnlaceDoc y EnlacePublico escriben las direcciones de la web que copia «Compartir»:
	// las pone el binario, que sabe el dominio de la organización. nil: no sale.
	EnlaceDoc     func(id string) string
	EnlacePublico func(token string) string
	// Directorio resuelve a quién se comparte (ver editor.Compartir.Dir).
	Directorio *api.Directorio
	// carpeta es dónde se dejan las descargas; las pruebas la cambian por una temporal.
	carpeta func() (string, error)
	// AlTitulo avisa del título de la ventana: el del documento abierto, o vacío en la
	// portada.
	AlTitulo func(string)
	// IA es la consulta a Sytue AI del relleno con IA. nil: el botón no sale.
	IA ConsultaIA
}

// abierto es el documento que hay en el editor.
type abierto struct {
	id    string
	ctx   *editor.Contexto
	lz    *Lienzo
	marco *editor.Marco
	auto  *editor.Autoguardado

	mu       sync.Mutex
	base     api.DocContent
	actor    string
	contador uint64
	// cambios cuenta los cambios; guardados, los que había al empezar el último guardado.
	// Un guardado que acaba con cambios nuevos detrás deja el chip en «sin guardar»,
	// como la web.
	cambios, guardados uint64
	pausaTit           *time.Timer
}

// PausaTitulo es el `RENAME_MS` de `useDocument.ts`.
const PausaTitulo = 800 * time.Millisecond

// NuevaApp monta Calcus. `svc` puede ser nil: la portada se pinta y no pide nada.
func NuevaApp(t func(string, i18n.Vars) string, loc i18n.Locale, svc *api.Docs) *App {
	a := &App{
		T: t, Svc: svc, Plazo: 30 * time.Second, avisos: ui.NuevaPilaAvisos(),
		lanzar: func(f func()) { go f() },
	}
	a.portada = editor.NuevaPortada(editor.Hoja, t, loc, svc)
	a.portada.Avisos = a.avisos
	a.portada.AlAbrir = func(d api.Document) { a.AbrirID(d.ID, d.Title) }
	a.raiz = container.NewStack(a.portada.Objeto(), a.avisos)
	return a
}

// Objeto es lo que se monta en la ventana.
func (a *App) Objeto() fyne.CanvasObject { return a.raiz }

// Portada es la portada, para quien necesite tocarla (la conectividad, el inicio de
// sesión).
func (a *App) Portada() *editor.Portada { return a.portada }

// Abierto es el lienzo del documento abierto, o nil en la portada.
func (a *App) Abierto() *Lienzo {
	if a.abierto == nil {
		return nil
	}
	return a.abierto.lz
}

// Arrancar pide la primera página de la portada.
func (a *App) Arrancar() {
	editor.EngancharAtras(a.raiz, a.portada, func() bool { return a.abierto != nil }, a.Volver)
	a.portada.Cargar()
}

func (a *App) t(clave string) string {
	if a.T == nil {
		return clave
	}
	return a.T(clave, nil)
}

func (a *App) mostrar(o fyne.CanvasObject) {
	a.raiz.Objects[0] = o
	a.raiz.Refresh()
}

func (a *App) titulo(s string) {
	if a.AlTitulo != nil {
		a.AlTitulo(s)
	}
}

// Volver cierra el documento y enseña la portada, con la lista al día.
func (a *App) Volver() {
	if ab := a.abierto; ab != nil {
		switch {
		case ab.auto == nil:
		case ab.ctx.Guardado == editor.Sucio:
			// Salir no es perder lo último: el `flush` de useDocument.ts al irse de la
			// página, y lo que ya hacen Docus y Diapos. Con `Parar` a secas, lo escrito en
			// el último segundo y medio se perdía al volver a la portada.
			a.lanzar(ab.auto.Ahora)
		default:
			ab.auto.Parar()
		}
		ab.mu.Lock()
		if ab.pausaTit != nil {
			ab.pausaTit.Stop()
		}
		ab.mu.Unlock()
	}
	a.abierto = nil
	editor.DejarDeEscucharRol()
	a.mostrar(a.portada.Objeto())
	a.titulo("")
	a.portada.Cargar()
}

// editorDe monta el cromo con un lienzo nuevo, en «cargando».
func (a *App) editorDe(id, titulo string) *abierto {
	ctx := &editor.Contexto{Tipo: editor.Hoja, Titulo: titulo, Estado: editor.Cargando, Zoom: 100, T: a.T}
	libro := NuevoLibroCon([]string{a.T0("sheet.defaultName", i18n.Vars{"n": 1})}, 0)
	lz := NuevoLienzo(libro)
	lz.T, lz.IA = a.T, a.IA
	lz.AlAviso = func(tono ui.TonoAviso, t, d string) { a.avisos.Notificar(ui.NuevoAviso(tono, t, d)) }
	ctx.Lienzo = lz
	ctx.Paneles = panelesDe(lz, a.T) // «Buscar» en las celdas de la hoja
	// «Nuevo hoja de cálculo» es el alta en blanco de la portada, y «Abrir un archivo»,
	// el del binario: los dos del menú Archivo, también con una hoja convertida delante.
	if a.Svc != nil {
		ctx.Nuevo = a.portada.Crear
	}
	if a.abrirArchivo != nil {
		ctx.Abrir = a.abrirArchivo.Abrir
	}
	marco := editor.NuevoMarco(ctx, Registro(lz), a.Volver)
	cromo := ctx.Refrescar
	// El cromo repinta la cinta y la barra de estado cuando algo cambia; el lienzo,
	// además, sigue el zoom de la barra de estado, que en la web es el `zoom` de CSS de
	// la rejilla.
	ctx.Refrescar = func() {
		cromo()
		if z := float32(ctx.Zoom) / 100; lz.Rejilla.Zoom != z {
			lz.Rejilla.Zoom = z
			lz.Rejilla.Refresh()
		}
	}
	lz.Rejilla.Contexto = ctx
	lz.AlSeleccionar = marco.Recomponer
	lz.AlCambiar = marco.Recomponer // lo que no se guarda también cambia la barra de estado
	ctx.BloqueoPropio = func() (string, func()) { return lz.bloqueoPropio(ctx) }
	return &abierto{id: id, ctx: ctx, lz: lz, marco: marco, actor: actorNuevo()}
}

// PonerAbrir engancha «Abrir un archivo» —el del menú Archivo y el de la portada— con lo
// que solo el binario tiene: el selector del sistema, el `cargar` que ya recibe «Abrir
// con» y el de pasarle al sistema lo que es de otra app. El `.csv` no es de ninguna app
// de la suite (`api.AppQueAbre`), pero la portada de la web lo importa como hoja.
func (a *App) PonerAbrir(elegir func(func([]string)), cargar func(context.Context, api.Apertura) error, fuera func(string) error) {
	a.abrirArchivo = &editor.AbrirArchivo{
		Tipo: editor.Hoja, Elegir: elegir, Cargar: cargar, Fuera: fuera, Importa: []string{"csv"},
		AlFallar: func(string, error) { a.Avisar(a.t("import.error.title"), a.t("import.error.description")) },
	}
	a.portada.AlAbrirArchivo = a.abrirArchivo.Abrir
}

// ImportarAlAbrir hace de «Abrir un archivo» lo de la portada de la web: la hoja elegida
// se convierte (`importar`, el de la sesión), se trae como una hoja NUEVA y se abre para
// escribir, en vez de enseñarse convertida en solo lectura.
func (a *App) ImportarAlAbrir(importar func(context.Context, api.Apertura) (api.Importado, error)) {
	if a.abrirArchivo != nil {
		a.abrirArchivo.Importar, a.abrirArchivo.Svc, a.abrirArchivo.AbrirID = importar, a.Svc, a.AbrirID
	}
}

// AbrirApertura abre un fichero que llega de fuera (Finder, Explorador, Drive) como hoja
// EDITABLE; ver [editor.AbrirArchivo.AbrirApertura]. Sin PonerAbrir no hay con qué.
func (a *App) AbrirApertura(ctx context.Context, ap api.Apertura) error {
	if a.abrirArchivo == nil {
		return editor.ErrNoEsDeLaSuite
	}
	return a.abrirArchivo.AbrirApertura(ctx, ap)
}

// archivo engancha lo que el menú Archivo y Colaboración piden al servicio: descargar
// (con cualquier papel, como la web), guardar y restaurar versiones (quien escribe), el
// historial y compartir.
func (a *App) archivo(ab *abierto, escribe bool) {
	ctx := ab.ctx
	srv := a.servicio()
	ctx.Exportar = func(f string) { a.exportar(ab, f) }
	ctx.ExportarPlantilla = func(f string) { a.exportarCon(ab, f, true) }
	ctx.Imprimir = func() { a.exportar(ab, editor.FormatoImprimir) }
	ctx.Versiones = func(listo func([]api.DocVersion)) { srv.Versiones(ab.id, listo) }
	if escribe {
		ctx.GuardarVersion = func(etiqueta string) { srv.GuardarVersion(ab.id, etiqueta) }
		ctx.RestaurarVersion = func(v api.DocVersion) {
			// Lo que hubiera sin guardar se descarta a propósito: se ha pedido volver a
			// otra versión, y guardarlo después la pisaría.
			if ab.auto != nil {
				ab.auto.Parar()
			}
			srv.RestaurarVersion(ab.id, v, func() { a.lanzar(func() { a.cargar(ab) }) })
		}
	}
	if a.EnlaceDoc != nil {
		ctx.Compartir = func(desde fyne.CanvasObject) {
			(&editor.Compartir{
				T: a.T, Svc: a.Svc, ID: ab.id, Titulo: ctx.Titulo,
				Enlace: a.EnlaceDoc(ab.id), EnlacePublico: a.EnlacePublico,
				Avisos: a.avisos, Lanzar: a.lanzar, Plazo: a.Plazo, Dir: a.Directorio,
			}).Mostrar(desde)
		}
	}
}

// servicio son las acciones compartidas de los editores, con la pila de avisos de aquí.
func (a *App) servicio() editor.Servicio {
	return editor.Servicio{
		Svc: a.Svc, Plazo: a.Plazo, T: a.T, Lanzar: a.lanzar, Carpeta: a.carpeta,
		Avisar: func(tono ui.TonoAviso, titulo string) { a.avisos.Notificar(ui.NuevoAviso(tono, titulo, "")) },
	}
}

// exportar es «Descargar como…»: guarda antes lo que falte y apaga las descargas del
// menú mientras se genera.
func (a *App) exportar(ab *abierto, formato string) {
	a.exportarCon(ab, formato, false)
}

func (a *App) exportarCon(ab *abierto, formato string, plantilla bool) {
	if a.Svc == nil || ab.ctx.Exportando != "" {
		return
	}
	ab.ctx.Exportando = formato
	ab.marco.Recomponer()
	var antes func() error
	if ab.auto != nil {
		ab.mu.Lock()
		pendiente := ab.cambios > ab.guardados
		ab.mu.Unlock()
		if pendiente {
			ab.auto.Parar()
			antes = func() error {
				err := a.guardar(ab)
				ab.auto.AlTerminar(err) // el chip, como tras un autoguardado
				return err
			}
		}
	}
	descargar := a.servicio().Descargar
	if plantilla {
		descargar = a.servicio().DescargarPlantilla
	}
	descargar(ab.id, ab.ctx.Titulo, editor.Hoja, formato, antes, func() {
		ab.ctx.Exportando = ""
		ab.marco.Recomponer()
	})
}

// Avisar enseña un aviso de peligro en la pila de la ventana.
func (a *App) Avisar(titulo, texto string) {
	a.avisos.Notificar(ui.NuevoAviso(ui.AvisoPeligro, titulo, texto))
}

// T0 es T con el hueco tapado.
func (a *App) T0(clave string, v i18n.Vars) string {
	if a.T == nil {
		return clave
	}
	return a.T(clave, v)
}

// AbrirID abre por identificador, con el título que ya se sepa.
func (a *App) AbrirID(id, titulo string) {
	ab := a.editorDe(id, titulo)
	a.abierto = ab
	a.mostrar(ab.marco)
	a.titulo(titulo)
	if a.Svc == nil {
		return
	}
	a.lanzar(func() { a.cargar(ab) })
}

// AbrirConvertido enseña un fichero de fuera ya convertido —de Drive o de la
// máquina—: sin id y en SOLO LECTURA, porque no es un documento de Docs y no hay dónde
// guardarlo. Devuelve si el lienzo lo entendió; si no, no se enseña nada y quien llama
// lo manda a la web, donde se ve con todo su formato.
func (a *App) AbrirConvertido(titulo, contenido string) bool {
	ab := a.editorDe("", titulo)
	if !ab.lz.Cargar(contenido) {
		return false
	}
	ab.ctx.Estado, ab.ctx.SoloLectura = editor.Listo, true
	ab.lz.SoloLectura(true)
	a.abierto = ab
	a.mostrar(ab.marco)
	a.titulo(titulo)
	ab.marco.Recomponer()
	return true
}

// cargar pide la ficha (título y papel) y el contenido. Corre en su propia rutina.
func (a *App) cargar(ab *abierto) {
	c, cancelar := context.WithTimeout(context.Background(), a.Plazo)
	defer cancelar()
	doc, rol, err := a.Svc.Get(c, ab.id)
	var cont api.DocContent
	if err == nil {
		cont, err = a.Svc.Content(c, ab.id, ab.ctx.Tipo.ClaveContenido())
	}
	hayRed := a.Red == nil || a.Red.State() != offline.Offline
	fyne.Do(func() {
		if a.abierto != ab {
			return // se cerró mientras llegaba
		}
		if err != nil {
			ab.ctx.Estado = editor.EstadoDe(err, hayRed, 1)
			a.mostrar(a.pantallaFallo(ab))
			return
		}
		ab.ctx.SinRed = !hayRed
		a.poner(ab, doc.Title, rol, cont)
	})
}

// poner enseña el documento y engancha el guardado. Lo que el lienzo no entiende entero
// se abre CERRADO —y la franja lo dice—; lo que el permiso no deja escribir, también.
func (a *App) poner(ab *abierto, titulo, rol string, cont api.DocContent) {
	ctx := ab.ctx
	ab.base = cont
	ctx.Titulo = titulo
	a.titulo(titulo)
	entendido := ab.lz.Cargar(cont.Value)
	escribe := api.CanWrite(rol)
	ctx.SoloLectura = !entendido || !escribe
	ab.lz.SoloLectura(ctx.SoloLectura)
	// La salida de la franja: pedir acceso, o concederlo si soy el dueño. Calcus no
	// coedita: el canal se abre solo para oír que el rol ha cambiado.
	editor.EngancharPeticionEdicion(ctx, a.Svc, ab.id, rol, true, ab.marco.Recomponer, func() {
		if a.abierto == ab {
			a.lanzar(func() { a.cargar(ab) })
		}
	})
	if escribe {
		ctx.AlRenombrar = func(s string) { a.renombrar(ab, s) }
	}
	a.archivo(ab, escribe)
	if escribe && !ctx.SoloLectura {
		ctx.Bloques = &editor.BloquesDoc{
			Svc: a.Svc, Lanzar: a.lanzar, Idioma: string(a.portada.Loc),
			Colocar: func(contenido string) { ctx.Mandar(CmdBloque, contenido) },
		}
		ctx.Temas = &editor.TemasDoc{
			Svc: a.Svc, Lanzar: a.lanzar, DocID: ab.id,
			Guardar: func() {
				if ab.auto != nil {
					ab.auto.Ahora()
				}
			},
			Recargar: func() { a.lanzar(func() { a.cargar(ab) }) },
		}
	}
	if rol == "owner" {
		ctx.Actividad = func(listo func([]api.DocActividad, error)) { a.servicio().Actividad(ab.id, listo) }
	}
	if !ctx.SoloLectura {
		a.engancharGuardado(ab)
	}
	ctx.Estado, ctx.Guardado = editor.Listo, editor.Limpio
	ab.marco.Recomponer()
}

// engancharGuardado cuenta cada cambio del libro y guarda tras la pausa de la web.
func (a *App) engancharGuardado(ab *abierto) {
	ab.auto = &editor.Autoguardado{
		Espera:  editor.AutoguardadoMS * time.Millisecond,
		Guardar: func() error { return a.guardar(ab) },
		AlTerminar: func(err error) {
			fyne.Do(func() {
				ab.mu.Lock()
				sucio := ab.cambios > ab.guardados
				ab.mu.Unlock()
				switch {
				case err != nil:
					ab.ctx.Guardado = editor.FalloAlGuardar
				case sucio:
					ab.ctx.Guardado = editor.Sucio
				default:
					ab.ctx.Guardado = editor.Limpio
				}
				ab.marco.Recomponer()
			})
		},
	}
	ab.lz.AlCambiar = func() {
		ab.mu.Lock()
		ab.cambios++
		ab.mu.Unlock()
		if ab.ctx.Guardado != editor.Guardando {
			ab.ctx.Guardado = editor.Sucio
		}
		ab.marco.Recomponer()
		ab.auto.Cambio()
	}
}

// guardar escribe el libro entero encima de lo último que se leyó o se escribió. Lee el
// modelo en el hilo de la interfaz, que es el único que lo toca; la red va aquí.
func (a *App) guardar(ab *abierto) error {
	var modelo string
	fyne.DoAndWait(func() {
		modelo = ab.lz.Serializar()
		ab.ctx.Guardado = editor.Guardando
		ab.marco.Recomponer()
	})
	ab.mu.Lock()
	ab.guardados = ab.cambios
	ab.contador++
	dot := api.DocDot{Actor: ab.actor, Counter: ab.contador}
	base := ab.base
	ab.mu.Unlock()
	c, cancelar := context.WithTimeout(context.Background(), a.Plazo)
	defer cancelar()
	nuevo, err := a.Svc.SaveChecked(c, ab.id, ab.ctx.Tipo.ClaveContenido(), modelo, base, dot, false)
	if err != nil {
		return err
	}
	ab.mu.Lock()
	ab.base = nuevo
	ab.mu.Unlock()
	return nil
}

// renombrar programa el cambio de título tras la pausa de la web. Un título en blanco
// no se manda: el documento se queda con el que tenía.
func (a *App) renombrar(ab *abierto, titulo string) {
	titulo = strings.TrimSpace(titulo)
	if titulo == "" {
		return
	}
	ab.ctx.Titulo = titulo
	ab.mu.Lock()
	defer ab.mu.Unlock()
	if ab.pausaTit != nil {
		ab.pausaTit.Stop()
	}
	ab.pausaTit = time.AfterFunc(PausaTitulo, func() {
		c, cancelar := context.WithTimeout(context.Background(), a.Plazo)
		defer cancelar()
		err := a.Svc.Rename(c, ab.id, titulo)
		fyne.Do(func() {
			if a.abierto != ab {
				return
			}
			if err != nil {
				a.avisos.Notificar(ui.NuevoAviso(ui.AvisoPeligro, a.t("rename.error"), ""))
				return
			}
			a.titulo(titulo)
		})
	})
}

// pantallaFallo es lo que la web pinta cuando el documento no se puede abrir: el editor
// entero cede el sitio a un EmptyState con el aviso en `danger`, «Reintentar» y
// «Volver» (`status === 'error'`).
func (a *App) pantallaFallo(ab *abierto) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	e := ui.NuevoVacio(a.t("editor.error.title"), a.t("editor.error.description"))
	e.Ilustracion = iconos.Trazo(iconos.Aviso, 40, "danger", v).Objeto()
	reintentar := ui.NuevoBoton(a.t("editor.error.retry"), func() { a.AbrirID(ab.id, ab.ctx.Titulo) }).
		Con(ui.Primario, ui.TallaSM)
	volver := ui.NuevoBoton(a.t("editor.error.back"), a.Volver).Con(ui.Secundario, ui.TallaSM)
	e.Accion = container.New(ui.Fila{Hueco: tema.Espacio(2)}, reintentar, volver)
	return container.NewStack(canvas.NewRectangle(tema.Color("surface", v)), e)
}

// IDDeEnlace saca el identificador de un enlace de edición de una hoja: el esquema
// propio (`sytue-sheets:/edit?id=…`) y la ruta de la web con su prefijo
// (`sytue-sheets:/calcus/edit?id=…`). Vacío si el enlace no es de editar.
func IDDeEnlace(u *url.URL) string { return api.IDDeEdicion("sheets", u) }

// actorNuevo es el `actor` del reloj de los guardados: uno por apertura, como la pestaña
// de la web.
func actorNuevo() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "calcus"
	}
	return hex.EncodeToString(b)
}
