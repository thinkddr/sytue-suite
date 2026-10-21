// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/editor/docus"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// PausaTitulo es el `RENAME_MS` de `useDocument.ts`: el título se manda 800 ms después
// de la última tecla, no en cada una.
const PausaTitulo = 800 * time.Millisecond

// App es Sytue Diapos entero: la portada con las presentaciones (`DocList.tsx` con
// `?kind=presentation`, la del cromo compartido) y, al abrir una, el editor. En la web
// son `/diapos/` y `/diapos/edit?id=` del mismo subdominio, así que abrir no lanza
// otra ventana: cambia la pantalla de esta.
type App struct {
	T      func(string, i18n.Vars) string
	Idioma i18n.Locale
	// Slides abre y guarda las presentaciones; nil, la app solo enseña lo que le den.
	Slides *api.Slides
	Plazo  time.Duration
	// ElegirImagen y ElegirMedio son los selectores del sistema que se le pasan al
	// editor: el de «Imagen» y el de «Subir fichero…» de Multimedia.
	ElegirImagen func(listo func(datos []byte, jpeg bool))
	ElegirMedio  func(listo func(nombre string, datos []byte, grande bool))
	// AlTitulo avisa del título del documento abierto (la ventana lo pone).
	AlTitulo func(string)
	// AbrirArchivo es «Abrir un archivo» del menú Archivo y de la portada; lo pone el
	// binario, que es quien sabe abrir el selector y convertir.
	AbrirArchivo *editor.AbrirArchivo
	// Carpeta es dónde se dejan las descargas; nil, la del sistema.
	Carpeta func() (string, error)
	// EnlaceDoc y EnlacePublico escriben las direcciones de la web que copia «Compartir»,
	// y Directorio resuelve a quién se comparte: los mismos que Docus y Calcus. Sin
	// EnlaceDoc no hay «Compartir».
	EnlaceDoc     func(id string) string
	EnlacePublico func(token string) string
	Directorio    *api.Directorio
	// Asistir es Sytue AI (`api.IA.Asistir`), con el token de la persona; nil, sin panel
	// «Asistente».
	Asistir func(context.Context, api.PeticionIA) (string, error)
	// Cast es «Presentar con Cast»: el cliente de Sytue Cast y cómo se abre su página de
	// emisión (el `DocID` lo pone la app al abrir). Nil, sin esa orden en la cinta.
	Cast *Cast

	portada *editor.Portada
	avisos  *ui.PilaAvisos
	raiz    *fyne.Container
	abierta *abierta
	// lanzar corre una petición fuera del hilo de la interfaz; las pruebas la cambian
	// por una llamada directa.
	lanzar func(func())

	mu       sync.Mutex
	pausaTit *time.Timer
}

// abierta es la presentación del editor, con lo que hace falta para guardarla.
type abierta struct {
	ctx   *editor.Contexto
	ed    *Editor
	marco *editor.Marco
	doc   *Documento // nil si no vive en el servicio (un fichero, una conversión)
	auto  *Autoguardado
	// cambios cuenta los cambios. Solo lo toca el hilo de la interfaz: un guardado que
	// termina con cambios nuevos detrás deja el chip en «sin guardar», como la web («si
	// se tecleó durante el vuelo, sigue dirty»).
	cambios uint64
}

// NuevaApp monta Diapos. `docs` y `slides` pueden ser nil: la portada se pinta y no pide
// nada, que es lo que quieren los guardianes.
func NuevaApp(t func(string, i18n.Vars) string, loc i18n.Locale, docs *api.Docs, slides *api.Slides) *App {
	a := &App{
		T: t, Idioma: loc, Slides: slides, Plazo: 30 * time.Second, avisos: ui.NuevaPilaAvisos(),
		lanzar: func(f func()) { go f() },
	}
	a.portada = editor.NuevaPortada(editor.Presentacion, t, loc, docs)
	a.portada.Avisos = a.avisos
	a.portada.AlAbrir = func(d api.Document) { a.AbrirID(d.ID, d.Title) }
	a.raiz = container.NewStack(a.portada.Objeto(), a.avisos)
	return a
}

// Objeto es lo que se monta en la ventana.
func (a *App) Objeto() fyne.CanvasObject { return a.raiz }

// Portada es la portada, para quien la tenga que tocar (el inicio de sesión).
func (a *App) Portada() *editor.Portada { return a.portada }

// Editor es el lienzo de la presentación abierta, o nil en la portada.
func (a *App) Editor() *Editor {
	if a.abierta == nil {
		return nil
	}
	return a.abierta.ed
}

// Contexto es el del editor abierto, o nil en la portada.
func (a *App) Contexto() *editor.Contexto {
	if a.abierta == nil {
		return nil
	}
	return a.abierta.ctx
}

// Arrancar pide la primera página de la portada.
func (a *App) Arrancar() {
	editor.EngancharAtras(a.raiz, a.portada, func() bool { return a.abierta != nil }, a.Volver)
	a.portada.Cargar()
}

func (a *App) mostrar(o fyne.CanvasObject) {
	a.raiz.Objects[0] = o
	a.raiz.Refresh()
}

func (a *App) titulo(t string) {
	if a.AlTitulo != nil {
		a.AlTitulo(t)
	}
}

// Volver cierra la presentación —guardando lo que quedara— y enseña la portada, con la
// lista al día.
func (a *App) Volver() {
	a.soltar()
	a.mu.Lock()
	if a.pausaTit != nil {
		a.pausaTit.Stop()
		a.pausaTit = nil
	}
	a.mu.Unlock()
	a.abierta = nil
	editor.DejarDeEscucharRol()
	a.titulo("")
	a.mostrar(a.portada.Objeto())
	a.portada.Cargar()
}

// soltar termina lo que se escribía en la presentación abierta y adelanta su guardado:
// lo último no se pierde por cerrarla —o cambiarla por otra— antes del reloj.
func (a *App) soltar() {
	if ab := a.abierta; ab != nil {
		ab.ed.TerminarEscritura()
		if ab.auto != nil && ab.ctx.Guardado == editor.Sucio {
			a.lanzar(ab.auto.Ahora)
		}
	}
}

// Avisar enseña un aviso de peligro en la pila de la ventana.
func (a *App) Avisar(titulo, texto string) {
	a.avisos.Notificar(ui.NuevoAviso(ui.AvisoPeligro, titulo, texto))
}

// nueva monta el cromo del editor con el lienzo de Diapos, «cargando».
func (a *App) nueva(titulo string) *abierta {
	a.soltar() // lo que hubiera abierto se guarda antes de cambiarlo por otra
	ctx := &editor.Contexto{Tipo: editor.Presentacion, Titulo: titulo, Estado: editor.Cargando, Zoom: 100, T: a.T}
	// El menú Archivo en nativo: «Nuevo» es el mismo alta que «En blanco» de la portada
	// (una presentación vacía, con el tema por defecto de la web) y «Abrir…» el del
	// binario.
	if a.portada.Svc != nil {
		ctx.Nuevo = a.portada.Crear
	}
	if a.AbrirArchivo != nil {
		ctx.Abrir = a.AbrirArchivo.Abrir
	}
	ed := NuevoEditor(ctx)
	ed.Idioma = a.Idioma
	ed.ElegirImagen, ed.ElegirMedio = a.ElegirImagen, a.ElegirMedio
	ctx.Lienzo = ed
	ctx.Notificar = func(tono ui.TonoAviso, titulo string) { ed.Avisos().Notificar(ui.NuevoAviso(tono, titulo, "")) }
	// El «Asistente» del carril: el panel de los editores sobre la presentación, si hay
	// Sytue AI detrás (sin ella la web no lo ofrece).
	if a.Asistir != nil {
		ctx.IA = true
		panel := docus.PanelIA{Ctx: ctx, Asistir: a.Asistir, Idioma: string(a.Idioma), Lanzar: a.lanzar}
		ctx.Paneles = func(p editor.Panel) fyne.CanvasObject {
			if p == editor.PanelIA {
				return panel.Objeto(ed)
			}
			return nil
		}
	}
	marco := editor.NuevoMarco(ctx, Registro(ed), a.Volver)
	cromo := ctx.Refrescar
	ctx.Refrescar = func() { cromo(); ed.Refrescar() }
	ab := &abierta{ctx: ctx, ed: ed, marco: marco}
	a.abierta = ab
	a.mostrar(container.NewStack(marco, ed.Avisos()))
	return ab
}

// AbrirID abre la presentación `id` del servicio: el enlace `sytue-slides:/edit?id=…`
// y lo que se pulse en la portada.
func (a *App) AbrirID(id, titulo string) {
	ab := a.nueva(titulo)
	if a.Slides == nil {
		return
	}
	a.lanzar(func() {
		c, cancelar := context.WithTimeout(context.Background(), a.Plazo)
		defer cancelar()
		ap, err := Abrir(c, a.Slides, id)
		fyne.Do(func() {
			if a.abierta != ab {
				return // se cerró mientras llegaba
			}
			if err != nil {
				slog.Warn("diapos: no se pudo abrir la presentación", "id", id, "error", err)
				ab.ctx.Estado = EstadoDe(err)
				ab.ctx.Refrescar()
				return
			}
			ab.doc = ap.Doc
			if ap.Escribir {
				ab.ctx.Bloques = &editor.BloquesDoc{
					Svc: a.portada.Svc, Lanzar: a.lanzar, Idioma: string(a.Idioma), Colocar: ab.ed.InsertarBloque,
				}
				ab.ctx.Temas = &editor.TemasDoc{
					Svc: a.portada.Svc, Lanzar: a.lanzar, DocID: id,
					Guardar:  func() { a.guardarAhora(ab) },
					Recargar: func() { a.AbrirID(id, ap.Titulo) },
				}
			}
			if a.Cast != nil {
				ab.ed.Cast = &Cast{Cliente: a.Cast.Cliente, DocID: id, Abrir: a.Cast.Abrir, Lanzar: a.lanzar}
			}
			a.poner(ab, ap.Titulo, ap.Contenido, ap.Escribir, func(modelo string) error {
				c, cancelar := context.WithTimeout(context.Background(), a.Plazo)
				defer cancelar()
				return ap.Doc.Guardar(c, modelo)
			})
			if ap.Escribir {
				ab.ctx.AlRenombrar = func(s string) { a.renombrar(ab, s) }
			}
			a.archivo(ab, id, ap.Escribir)
			if ap.Rol == "owner" {
				ab.ctx.Actividad = func(listo func([]api.DocActividad, error)) { a.servicio().Actividad(id, listo) }
			}
			// La salida de la franja. Diapos no coedita: el canal se abre solo para oír
			// que el rol ha cambiado, y entonces se vuelve a abrir la presentación.
			editor.EngancharPeticionEdicion(ab.ctx, a.portada.Svc, id, ap.Rol, true, ab.ctx.Refrescar, func() {
				if a.abierta == ab {
					a.AbrirID(id, ap.Titulo)
				}
			})
		})
	})
}

// guardarAhora vuelca lo pendiente; corre fuera del hilo de la interfaz.
func (a *App) guardarAhora(ab *abierta) {
	if ab.auto != nil {
		ab.auto.Ahora()
	}
}

// archivo engancha lo que el menú Archivo y Colaboración piden al servicio, con la
// pieza común de los editores (`editor.Servicio`): descargar con cualquier papel, como
// la web; guardar y restaurar versiones quien escribe; y el historial.
func (a *App) archivo(ab *abierta, id string, escribe bool) {
	if a.portada.Svc == nil {
		return
	}
	ctx := ab.ctx
	srv := a.servicio()
	ctx.Exportar = func(f string) { a.exportar(ab, id, f) }
	ctx.ExportarPlantilla = func(f string) { a.exportarCon(ab, id, f, true) }
	ctx.Imprimir = func() { a.exportar(ab, id, editor.FormatoImprimir) }
	ctx.Versiones = func(listo func([]api.DocVersion)) { srv.Versiones(id, listo) }
	if escribe {
		ctx.GuardarVersion = func(etiqueta string) { srv.GuardarVersion(id, etiqueta) }
		ctx.RestaurarVersion = func(v api.DocVersion) {
			// Lo que hubiera sin guardar se descarta a propósito: se ha pedido volver a
			// otra versión, y guardarlo después la pisaría.
			if ab.auto != nil {
				ab.auto.Parar()
			}
			srv.RestaurarVersion(id, v, func() { a.AbrirID(id, ctx.Titulo) })
		}
	}
	if a.EnlaceDoc != nil {
		ctx.Compartir = func(desde fyne.CanvasObject) {
			(&editor.Compartir{
				T: a.T, Svc: a.portada.Svc, ID: id, Titulo: ctx.Titulo,
				Enlace: a.EnlaceDoc(id), EnlacePublico: a.EnlacePublico,
				Avisos: a.avisos, Lanzar: a.lanzar, Plazo: a.Plazo, Dir: a.Directorio,
			}).Mostrar(desde)
		}
	}
}

// servicio son las acciones compartidas de los editores, con la pila de avisos de aquí.
func (a *App) servicio() editor.Servicio {
	return editor.Servicio{
		Svc: a.portada.Svc, Plazo: a.Plazo, T: a.T, Lanzar: a.lanzar, Carpeta: a.Carpeta,
		Avisar: func(tono ui.TonoAviso, titulo string) { a.avisos.Notificar(ui.NuevoAviso(tono, titulo, "")) },
	}
}

// exportar es «Descargar como…»: guarda antes lo que falte —el servicio exporta desde
// SU estado— y apaga las descargas del menú mientras se genera.
func (a *App) exportar(ab *abierta, id, formato string) {
	a.exportarCon(ab, id, formato, false)
}

func (a *App) exportarCon(ab *abierta, id, formato string, plantilla bool) {
	if ab.ctx.Exportando != "" {
		return
	}
	ab.ctx.Exportando = formato
	ab.marco.Recomponer()
	var antes func() error
	if ab.auto != nil && ab.ctx.Guardado == editor.Sucio {
		antes = func() error { ab.auto.Ahora(); return nil }
	}
	descargar := a.servicio().Descargar
	if plantilla {
		descargar = a.servicio().DescargarPlantilla
	}
	descargar(id, ab.ctx.Titulo, editor.Presentacion, formato, antes, func() {
		ab.ctx.Exportando = ""
		ab.marco.Recomponer()
	})
}

// AbrirImportado enseña la presentación que el servicio ha convertido de un fichero —un
// .odp o un .pptx pulsado en Drive, o uno de la máquina— en SOLO LECTURA: es una
// conversión y no un documento de Docs, y lo escrito encima no tendría dónde guardarse.
// Dice que no, sin tocar la pantalla, si lo convertido no es una presentación; quien
// llama cae entonces a la web.
func (a *App) AbrirImportado(imp api.Importado) error {
	if imp.Kind != api.DocSlides {
		return fmt.Errorf("diapos: %q no es una presentación (%s)", imp.Title, imp.Kind)
	}
	fyne.Do(func() { a.AbrirContenido(imp.Title, imp.Content, false, nil) })
	return nil
}

// AbrirContenido enseña una presentación que no viene del servicio: un fichero del
// disco (que se guarda con `guardar`) o una conversión de Drive (sin dónde guardarse:
// `guardar` nil y en solo lectura).
func (a *App) AbrirContenido(titulo, contenido string, escribir bool, guardar func(string) error) {
	ab := a.nueva(titulo)
	a.poner(ab, titulo, contenido, escribir, guardar)
}

// Fallo deja el editor en el estado de un fallo al abrir (sin permiso, sin conexión…).
func (a *App) Fallo(e editor.Estado) {
	ab := a.nueva("")
	ab.ctx.Estado = e
	ab.ctx.Refrescar()
}

// poner enseña el contenido y engancha el autoguardado. `escribir` es el permiso; el
// lienzo añade el suyo: lo que no entiende entero no se deja tocar.
func (a *App) poner(ab *abierta, titulo, contenido string, escribir bool, guardar func(string) error) {
	entendido := ab.ed.Cargar(contenido)
	ab.ctx.Titulo, ab.ctx.Estado, ab.ctx.Guardado = titulo, editor.Listo, editor.Limpio
	ab.ctx.SoloLectura = !entendido || !escribir || guardar == nil
	ab.ctx.MotivoSoloLectura = ""
	a.titulo(titulo)
	if ab.ctx.SoloLectura {
		// Lo que no se puede guardar no arma reloj ni cuenta cambios.
		ab.ed.AlCambiar, ab.auto = nil, nil
		ab.ctx.Refrescar()
		return
	}
	var guardados uint64
	ab.auto = &Autoguardado{
		Espera: AutoguardadoMS * time.Millisecond,
		Guardar: func() error {
			// El modelo se lee en el hilo de la interfaz, que es el único que lo toca;
			// la red va aquí, fuera de él.
			var modelo string
			fyne.DoAndWait(func() {
				modelo, guardados = ab.ed.Serializar(), ab.cambios
				ab.ctx.Guardado = editor.Guardando
				ab.ctx.Refrescar()
			})
			return guardar(modelo)
		},
		AlTerminar: func(err error) {
			fyne.Do(func() {
				switch {
				case err != nil:
					slog.Warn("diapos: no se pudo guardar", "error", err)
					ab.ctx.Guardado = editor.FalloAlGuardar
				case ab.cambios > guardados:
					ab.ctx.Guardado = editor.Sucio
				default:
					ab.ctx.Guardado = editor.Limpio
				}
				ab.ctx.Refrescar()
			})
		},
	}
	ab.ed.AlCambiar = func() {
		ab.cambios++
		ab.auto.Cambio()
	}
	ab.ctx.Refrescar()
}

// renombrar programa el cambio de título tras la pausa de la web. Un título en blanco
// no se manda (`setTitle` hace lo mismo): el documento se queda con el que tenía.
func (a *App) renombrar(ab *abierta, titulo string) {
	titulo = strings.TrimSpace(titulo)
	if titulo == "" || ab.doc == nil {
		return
	}
	ab.ctx.Titulo = titulo
	ab.ctx.Guardado = editor.Sucio
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.pausaTit != nil {
		a.pausaTit.Stop()
	}
	a.pausaTit = time.AfterFunc(PausaTitulo, func() {
		c, cancelar := context.WithTimeout(context.Background(), a.Plazo)
		defer cancelar()
		err := ab.doc.Renombrar(c, titulo)
		fyne.Do(func() {
			if a.abierta != ab {
				return
			}
			if err != nil {
				ab.ctx.Guardado = editor.FalloAlGuardar
				a.avisos.Notificar(ui.NuevoAviso(ui.AvisoPeligro, a.T("rename.error", nil), ""))
			} else if ab.ctx.Guardado == editor.Sucio {
				ab.ctx.Guardado = editor.Limpio
			}
			a.titulo(titulo)
			ab.ctx.Refrescar()
		})
	})
}
