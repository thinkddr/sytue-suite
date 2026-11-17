// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/office/model"
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
	// Reclamos dice quién ha entrado: su `sub` desbloquea las celdas de sus rangos
	// editables en una hoja protegida (`fijarYo` de la web). nil, o sin sesión: nadie, y
	// ningún rango concede nada.
	Reclamos func(context.Context) (auth.Reclamos, error)
}

// abierto es el documento que hay en el editor.
type abierto struct {
	id    string
	ctx   *editor.Contexto
	lz    *Lienzo
	marco *editor.Marco
	auto  *editor.Autoguardado
	// comentarios es el panel, una vez abierto.
	comentarios *editor.Comentarios

	mu       sync.Mutex
	base     api.DocContent
	actor    string
	contador uint64
	// cambios cuenta los cambios; guardados, los que había al empezar el último guardado.
	// Un guardado que acaba con cambios nuevos detrás deja el chip en «sin guardar»,
	// como la web.
	cambios, guardados uint64
	pausaTit           *time.Timer
	// cola son fotos del libro que van cada una en SU guardado, en orden y antes que lo
	// que haya después: las de un cambio de estructura y lo pendiente de antes de él
	// (`aparte`). ultimo es lo último que salió o entró en la cola, para no repetirlo.
	cola   []string
	ultimo string
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
	a.portada.Carpeta = func() (string, error) {
		if a.carpeta == nil {
			return ficheros.Carpeta()
		}
		return a.carpeta()
	}
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
	// La tesela comparte con el mismo panel que la cabecera; los enlaces los pone quien
	// monta la app después de NuevaApp, así que se engancha aquí y no allí.
	a.portada.Compartir = nil
	if a.EnlaceDoc != nil {
		a.portada.Compartir = func(d api.Document, desde fyne.CanvasObject) {
			a.panelCompartir(d.ID, d.Title).Mostrar(desde)
		}
	}
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
	delLienzo := panelesDe(lz, a.T) // «Buscar» en las celdas de la hoja
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
	// La hoja de atajos del editor acaba con los de la rejilla (`propiosDe` de la web).
	ctx.AtajosPropios = func() []ui.GrupoDeAtajos {
		return AtajosPropios(func(k string) string { return ctx.Traducir(k) })
	}
	lz.AlSeleccionar = marco.Recomponer
	lz.AlCambiar = marco.Recomponer // lo que no se guarda también cambia la barra de estado
	ctx.BloqueoPropio = func() (string, func()) { return lz.bloqueoPropio(ctx) }
	ab := &abierto{id: id, ctx: ctx, lz: lz, marco: marco, actor: actorNuevo()}
	// «Comentarios» es el panel de Docus (`CommentsPanel` en los dos editores de la web).
	ctx.Paneles = func(p editor.Panel) fyne.CanvasObject {
		if p == editor.PanelComentarios && a.Svc != nil {
			return a.panelComentarios(ab)
		}
		return delLienzo(p)
	}
	return ab
}

// panelComentarios monta, la primera vez, los hilos del libro. El de una celda dice dónde
// está hoy y lleva a ella (CALC-11 de la web), y al pasar por él se enmarca su celda.
func (a *App) panelComentarios(ab *abierto) fyne.CanvasObject {
	if ab.comentarios == nil {
		lz := ab.lz
		ab.comentarios = editor.NuevosComentarios(editor.FuenteComentarios{
			T: a.t, Loc: a.portada.Loc, Svc: a.Svc, ID: ab.id, Lanzar: a.lanzar, Plazo: a.Plazo,
			Avisos: a.avisos, SinRed: func() bool { return ab.ctx.SinRed },
			Celda: func(x api.AnclaCelda) string {
				ref := lz.Libro.RefDeAncla(x)
				if ref == "" || a.T == nil {
					return ref
				}
				return a.T("comments.cell", i18n.Vars{"ref": ref})
			},
			IrACelda:      lz.IrAComentario,
			ResaltarCelda: lz.Rejilla.ResaltarComentario,
			AlCargar:      lz.Rejilla.PonerComentarios,
		})
	}
	return ab.comentarios.Objeto()
}

// comentar crea un hilo nuevo con ese cuerpo, pegado a la celda del ancla, y abre el panel
// para verlo.
func (a *App) comentar(ab *abierto, cuerpo string, celda api.AnclaCelda) {
	a.lanzar(func() {
		c, cancelar := context.WithTimeout(context.Background(), a.Plazo)
		defer cancelar()
		_, err := a.Svc.CreateCellComment(c, ab.id, cuerpo, celda)
		fyne.Do(func() {
			if err != nil {
				a.avisos.Notificar(ui.NuevoAviso(ui.AvisoPeligro, a.t("comments.addError"), ""))
				return
			}
			a.avisos.Notificar(ui.NuevoAviso(ui.AvisoExito, a.t("comments.added"), ""))
			ab.ctx.Panel = editor.PanelComentarios
			ab.marco.Recomponer()
			if ab.comentarios != nil {
				ab.comentarios.Recargar() // y con la lista, la marca de la celda
			}
		})
	})
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
	// «Hacer una copia», con cualquier papel, como la web: guarda lo pendiente y la abre.
	ctx.HacerCopia = func(listo func()) {
		srv.HacerCopia(api.Document{ID: ab.id, Kind: api.DocSheet, Title: ctx.Titulo}, a.guardarAntes(ab),
			func(d api.Document) { a.AbrirID(d.ID, d.Title) }, listo)
	}
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
		ctx.Compartir = func(desde fyne.CanvasObject) { a.panelCompartir(ab.id, ctx.Titulo).Mostrar(desde) }
	}
}

// panelCompartir es el panel de «Compartir» de un documento, el de la cabecera y el de
// la tesela de la portada.
func (a *App) panelCompartir(id, titulo string) *editor.Compartir {
	return &editor.Compartir{
		T: a.T, Svc: a.Svc, ID: id, Titulo: titulo,
		Enlace: a.EnlaceDoc(id), EnlacePublico: a.EnlacePublico,
		Avisos: a.avisos, Lanzar: a.lanzar, Plazo: a.Plazo, Dir: a.Directorio,
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

// guardarAntes es lo que hay que guardar antes de que el servicio lea el libro (exportar,
// hacer una copia): nil si no hay nada pendiente. Para el reloj del autoguardado aquí,
// en el hilo de la interfaz, y guarda en la rutina de quien llame.
func (a *App) guardarAntes(ab *abierto) func() error {
	if ab.auto == nil {
		return nil
	}
	ab.mu.Lock()
	pendiente := ab.cambios > ab.guardados || len(ab.cola) > 0
	ab.mu.Unlock()
	if !pendiente {
		return nil
	}
	ab.auto.Parar()
	return func() error {
		err := a.guardar(ab)
		ab.auto.AlTerminar(err) // el chip, como tras un autoguardado
		return err
	}
}

func (a *App) exportarCon(ab *abierto, formato string, plantilla bool) {
	if a.Svc == nil || ab.ctx.Exportando != "" {
		return
	}
	ab.ctx.Exportando = formato
	ab.marco.Recomponer()
	antes := a.guardarAntes(ab)
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
	// «Reintentar» del error vuelve a pedir el mismo documento, con el título que ya se sepa.
	ab.ctx.Reintentar = func() { a.AbrirID(id, ab.ctx.Titulo) }
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
	// Los hilos abiertos marcan sus celdas desde el principio, con el panel cerrado, como en
	// la web (`ctx.comments`). Sin ellos el libro se abre igual, sin marcas.
	var hilos []api.DocComment
	var yo string
	if err == nil {
		hilos, _ = a.Svc.Comments(c, ab.id, false)
		yo = a.yo(c)
	}
	hayRed := a.Red == nil || a.Red.State() != offline.Offline
	fyne.Do(func() {
		if a.abierto != ab {
			return // se cerró mientras llegaba
		}
		ab.lz.Libro.FijarYo(yo)
		if err != nil {
			ab.ctx.Estado = editor.EstadoDe(err, hayRed, 1)
			// Como en la web (`status === 'error'`), un fallo al abrir cede la
			// pantalla entera al error con «Reintentar» y «Volver». Antes se recompone el
			// marco, que así quita el esqueleto de carga y para su pulso.
			ab.ctx.Refrescar()
			a.mostrar(editor.PantallaFallo(ab.ctx))
			return
		}
		ab.ctx.SinRed = !hayRed
		a.poner(ab, doc.Title, rol, cont)
		ab.lz.Rejilla.PonerComentarios(hilos)
	})
}

// yo es el `sub` de quien ha entrado, o "" si no se sabe.
func (a *App) yo(c context.Context) string {
	if a.Reclamos == nil {
		return ""
	}
	r, err := a.Reclamos(c)
	if err != nil {
		return ""
	}
	return r.Sub
}

// poner enseña el documento y engancha el guardado. Lo que el lienzo no entiende entero
// se abre CERRADO —y la franja lo dice—; lo que el permiso no deja escribir, también.
func (a *App) poner(ab *abierto, titulo, rol string, cont api.DocContent) {
	ctx := ab.ctx
	ab.base = cont
	ctx.Titulo = titulo
	a.titulo(titulo)
	entendido := ab.lz.Cargar(cont.Value)
	ab.lz.Libro.FijarAjeno(rol != "owner") // `fijarPropietario` de la web
	ultimo := ab.lz.Serializar()
	ab.mu.Lock()
	ab.cola, ab.ultimo = nil, ultimo // lo de antes se basaba en otro libro
	ab.mu.Unlock()
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
	if api.CanComment(rol) && a.Svc != nil {
		// Como en Docus, el texto se pregunta en un diálogo; y como en Excel y en la web, el
		// comentario va pegado a la celda activa, que se toma ANTES de preguntar.
		ctx.NuevoComentario = func() {
			lienzo := fyne.CurrentApp().Driver().CanvasForObject(ab.marco)
			if lienzo == nil {
				return
			}
			celda := ab.lz.AnclaActiva()
			ui.Pregunta(a.t("comments.newPrompt"), "", a.t("ui.cancel"), a.t("ui.save"), a.t("ui.required"),
				func(cuerpo string) { a.comentar(ab, cuerpo, celda) }).Mostrar(lienzo)
		}
	}
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
	// El chip de la cabecera, en error: guarda ya, fuera del hilo de la interfaz.
	ab.ctx.GuardarYa = func() { a.lanzar(ab.auto.Ahora) }
	// Un cambio de estructura va en su propio guardado y enseguida, con lo pendiente de
	// antes en otro: en una hoja protegida el servidor solo reconoce UN desplazamiento
	// por guardado, y dos inserciones en el mismo autoguardado se rechazaban. Si llega
	// otro mientras se guarda el primero, espera en la cola sin parar la interfaz.
	ab.lz.Libro.AntesDeEstructura = func() { a.aparte(ab) }
	ab.lz.Libro.TrasEstructura = func() {
		a.aparte(ab)
		a.lanzar(ab.auto.Ahora)
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

// aparte pone en la cola el libro tal como está, para que vaya en su propio guardado
// (si no es ya lo último que sale). Corre en el hilo de la interfaz.
func (a *App) aparte(ab *abierto) {
	modelo := ab.lz.Serializar()
	ab.mu.Lock()
	defer ab.mu.Unlock()
	if modelo != ab.ultimo {
		ab.cola = append(ab.cola, modelo)
		ab.ultimo = modelo
	}
}

// guardar escribe el libro entero encima de lo último que se leyó o se escribió: primero
// la cola, de una en una (`aparte`), y luego lo de ahora. Lee el modelo en el hilo de
// la interfaz, que es el único que lo toca; la red va aquí. Si algo falla, lo que queda
// de la cola sigue ahí para el siguiente intento, en su orden.
func (a *App) guardar(ab *abierto) error {
	huboCola := false
	for {
		var modelo string
		var sobre model.Proteccion
		deCola := false
		fyne.DoAndWait(func() {
			sobre = ab.lz.Libro.Sobre()
			ab.mu.Lock()
			if len(ab.cola) > 0 {
				modelo, deCola = ab.cola[0], true
			}
			ab.mu.Unlock()
			if !deCola {
				// En el mismo hilo que `aparte`: lo que sale ya no vuelve a la cola.
				modelo = ab.lz.Serializar()
				ab.mu.Lock()
				ab.ultimo = modelo
				ab.mu.Unlock()
			}
			ab.ctx.Guardado = editor.Guardando
			ab.marco.Recomponer()
		})
		ab.mu.Lock()
		if !deCola {
			ab.guardados = ab.cambios
		}
		base := ab.base
		if !deCola && huboCola && modelo == base.Value {
			ab.mu.Unlock()
			return nil // tras la cola no queda nada nuevo
		}
		huboCola = huboCola || deCola
		ab.contador++
		dot := api.DocDot{Actor: ab.actor, Counter: ab.contador}
		ab.mu.Unlock()
		nuevo, err := a.guardarModelo(ab, modelo, base, dot, sobre)
		if codigo := rechazoProteccion(err); codigo != "" {
			a.adoptarServidor(ab, codigo)
			return nil
		}
		if err != nil {
			return err
		}
		fyne.DoAndWait(func() { ab.lz.Libro.OlvidarSobre(sobre) })
		ab.mu.Lock()
		ab.base = nuevo
		if deCola && len(ab.cola) > 0 && ab.cola[0] == modelo {
			ab.cola = ab.cola[1:]
		}
		ab.mu.Unlock()
		if !deCola {
			return nil
		}
	}
}

// guardarModelo es un guardado, con su plazo y las contraseñas de protección tecleadas
// (`sobre`), que viajan aparte del modelo.
func (a *App) guardarModelo(ab *abierto, modelo string, base api.DocContent, dot api.DocDot, sobre model.Proteccion) (api.DocContent, error) {
	c, cancelar := context.WithTimeout(context.Background(), a.Plazo)
	defer cancelar()
	c = model.ConProteccion(c, &sobre)
	return a.Svc.SaveChecked(c, ab.id, ab.ctx.Tipo.ClaveContenido(), modelo, base, dot, false)
}

// rechazoProteccion es el código del 403 con el que el servidor rechaza un cambio por la
// protección de hoja o libro («protected» o «protection_password»), o "" si no es eso.
func rechazoProteccion(err error) string {
	var e *api.Error
	if !errors.As(err, &e) || e.Status != http.StatusForbidden {
		return ""
	}
	if c := e.Code(); c == "protected" || c == "protection_password" {
		return c
	}
	return ""
}

// adoptarServidor es el de `useDocument` de la web: el servidor rechazó el cambio por la
// protección, así que lo que el lienzo enseña ya no es el documento. Se olvida lo
// tecleado y lo pendiente (se basaba en ello), se vuelve a lo que tiene el servidor y se
// dice por qué: con una contraseña que no es, «Contraseña incorrecta».
func (a *App) adoptarServidor(ab *abierto, codigo string) {
	c, cancelar := context.WithTimeout(context.Background(), a.Plazo)
	defer cancelar()
	cont, err := a.Svc.Content(c, ab.id, ab.ctx.Tipo.ClaveContenido())
	fyne.DoAndWait(func() {
		l := ab.lz.Libro
		l.OlvidarSobre(l.Sobre())
		ab.mu.Lock()
		ab.cola = nil
		ab.mu.Unlock()
		if err == nil && a.abierto == ab {
			// Cargar avisa de un cambio; no es del usuario y no se vuelve a guardar. Los
			// ganchos de estructura son de quien guarda: Cargar rehace el libro sin ellos.
			alCambiar, antes, tras := ab.lz.AlCambiar, l.AntesDeEstructura, l.TrasEstructura
			ab.lz.AlCambiar = nil
			ab.lz.Cargar(cont.Value)
			ab.lz.AlCambiar, l.AntesDeEstructura, l.TrasEstructura = alCambiar, antes, tras
			ab.mu.Lock()
			ab.base, ab.ultimo, ab.guardados = cont, ab.lz.Serializar(), ab.cambios
			ab.mu.Unlock()
			ab.marco.Recomponer()
		}
		titulo := "sheet.msg.protectedSheet"
		if codigo == "protection_password" {
			titulo = "sheet.msg.wrongPassword"
		}
		ab.lz.avisar2(ui.AvisoAdvertencia, titulo, "sheet.msg.protectionDenied")
	})
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
