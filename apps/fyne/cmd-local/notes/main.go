// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command notes es Sytue Notepads de Sytue Office: los mismos cuadernos y el mismo
// editor de la suite, con las notas en este equipo y en claro, sin cuenta y sin servidor
// (apps/fyne/PLAN-EDICION-LOCAL.md §6.5).
//
// Comparado con `cmd/notes`: no hay `sesion.Abrir` (ni OIDC ni llavero de sesión), ni
// `cifrado.NewPrivateApplication` (ni Matrix ni dispositivo verificado). El repositorio
// de dominio es el MISMO (`apps/nucleo/notes`), abierto sobre el almacén en claro de
// `local/almacen`, y la pantalla es `notes.NuevaPantalla` con la fachada privada: sin
// `LocalWork` del cifrado, así que el asistente de IA sí se ofrece (N-01, N-09).
//
// No se montan: cuadernos cifrados (D-12), compartir, enlace público, «Compartido
// conmigo», menciones, Rooms, servicios vinculados ni la vista previa de Docs. Sus
// piezas quedan a nil o la pantalla las oculta (`SinCompartir`). El diario, las tareas,
// las etiquetas con estados, los recordatorios, mover, archivar y purgar tienen camino
// local en `notes.Repository` (N-03); los avisos del sistema a su hora son §5.12.
//
// Importar: un `.onex`, Markdown (`.md`, `.zip`) o Evernote (`.enex`) entra como
// cuaderno nuevo por `notes.Repository.ImportNotebook`, en el equipo y sin HTTP, tanto
// por el menú «Importar notas…» de la pantalla como por «Abrir con», los argumentos o
// soltándolo en la ventana.
//
// Ficheros del equipo (N-06, N-07): «Añadir una imagen del equipo…» o soltar una imagen
// sobre el cuaderno crea una nota de imagen con la imagen DENTRO (se ve sin red); soltar
// otro fichero sobre una nota le añade su enlace `file://`, y sobre el cuaderno crea una
// nota con él. El repositorio se abre con `notes.NewLocal`, que guarda las dos cosas. Las
// notas de imagen leen lo mismo que Photos (`local/imagenes`): además de PNG, JPEG, GIF y
// WebP, AVIF, TIFF y BMP (este, guardado en PNG), y HEIC convertida a JPEG con el sistema.
//
// La nota de imagen por URL (imagen_url.go): su imagen se baja solo cuando la persona lo
// pide («Cargar la imagen», o al crearla con la dirección que acaba de escribir), por el
// permiso puntual del portero, y queda dentro de la nota; «Volver a cargar la imagen», en
// el «…» de la nota, la baja otra vez por si ha cambiado en su sitio.
//
// Lo «borrado del todo» se va del disco en el acto: tras purgar, el almacén se compacta
// en segundo plano (`almacen.Recursos.CompactarYa`), y si no puede, al cerrar.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/apps/notes"
	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/local"

	// AVIF, TIFF y BMP registrados, y HEIC con el sistema: lo que lee Photos.
	"git.sytue.eu/sytue/one/apps/fyne/local/imagenes"
	"git.sytue.eu/sytue/one/apps/fyne/soltar"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
	"git.sytue.eu/sytue/one/apps/nucleo/local/espacio"
	"git.sytue.eu/sytue/one/apps/nucleo/local/exportar"
	"git.sytue.eu/sytue/one/apps/nucleo/local/indice"
	privateNotes "git.sytue.eu/sytue/one/apps/nucleo/notes"
	"git.sytue.eu/sytue/one/apps/nucleo/notes/model"
)

// Las mismas medidas que la edición conectada.
const (
	anchoInicial = 1280
	altoInicial  = 800
)

func main() {
	a := local.NuevaApp("eu.sytue.office.notes")
	l, err := local.Abrir(a, "notes")
	if err != nil {
		slog.Error("notes", "error", err)
		os.Exit(1)
	}
	tema.ElegirCromo(tema.CromoPapel)
	tr, err := l.Traductor(catalogos.Notes())
	if err != nil {
		slog.Error("no se pudo cargar el catálogo de idiomas", "error", err)
		os.Exit(1)
	}

	// La vida del almacén es la de la app: se cierra con la ventana (l.AlCerrar).
	ctx := context.Background()
	repo, rec, err := abrirRepositorio(ctx, l)
	if err != nil {
		slog.Error("notes: no se pudo abrir el almacén local", "error", err)
		os.Exit(1)
	}
	svc, err := api.NewNotes("", nil, nil).WithPrivateData(notasLocales{repo, rec})
	if err != nil {
		slog.Error("notes", "error", err)
		os.Exit(1)
	}

	w := a.NewWindow(a.Metadata().Name)
	if a.Metadata().Name == "" {
		w.SetTitle("Sytue Notepads")
	}
	w.SetPadded(false)
	w.Resize(fyne.NewSize(anchoInicial, altoInicial))

	// Sin monitor de red: no hay servicio al que no llegar, y `hayRed` es siempre cierto.
	p := notes.NuevaPantalla(tr, l.Idioma(), svc, nil, w)
	// El diario sellado local (D-08) guarda la cola de capturas y los borradores del
	// editor, como hacía la edición conectada antes del cifrado de extremo a extremo:
	// un cierre a mitad de escribir no pierde lo escrito.
	sujeto := rec.Config.Scope.Subject
	p.Almacen = rec.Config.Journal
	p.Yo = func() string { return sujeto }
	p.Org = func() string { return rec.Config.Scope.Tenant }
	p.Asistir = l.Asistir()
	l.AlCambiarIA(func() { p.Asistir = l.Asistir() })
	// Sin cuenta no hay «Compartido conmigo» ni cuadernos cifrados (N-05); la copia de
	// uno cifrado se rechaza diciendo por qué.
	p.SinCompartir = true
	p.VolcadoCifrado = texto(l, "volcado.cifrado")
	// Un enlace a un fichero de este equipo (`file://…`) se abre con su app de Sytue
	// Office, o con la del sistema (N-07).
	p.AbrirLocal = func(ruta string) {
		producto, _ := indice.ProductoDeFichero(filepath.Base(ruta))
		if err := l.AbrirEn(producto, ruta); err != nil {
			l.Fallo(w, err)
		}
	}
	// La nota de imagen con la imagen dentro y el enlace al soltar un fichero (N-06, N-07);
	// la de una dirección, a petición y por el portero (imagen_url.go).
	p.FicherosLocales = &notes.FicherosLocales{
		Elegir:      func() ([]string, error) { return ficheros.ElegirImagenes(texto(l, "imagen.elegir")) },
		Rotulo:      texto(l, "imagen.anadir"),
		NoEsImagen:  texto(l, "imagen.noLeible"),
		HEIC:        imagenes.AJPEG,
		BajarImagen: bajarImagen(l.Red),
	}
	// «Añadir al calendario» abre Calendar de este equipo con el formulario relleno (N-08).
	p.CrearEventoFuera = func(titulo, descripcion string) {
		q := url.Values{"action": {"create"}, "title": {titulo}}
		if descripcion != "" && descripcion != titulo {
			q.Set("description", descripcion)
		}
		if err := l.AbrirEnlace(espacio.Esquema("calendar") + ":/events/new?" + q.Encode()); err != nil {
			local.Info(w, titulo, texto(l, "sin.calendar"))
		}
	}

	// «Exportar en formatos abiertos»: cada cuaderno en Markdown (los cifrados no hay).
	l.AlExportarTodo(func(ctx context.Context, carpeta string) error { return exportar.Cuadernos(ctx, repo, carpeta) })
	// Ficheros que llegan: un volcado de cuaderno se importa como cuaderno nuevo.
	l.AlAbrirFichero(func(ctx context.Context, ap api.Apertura) error {
		return importar(ctx, repo, p, w, tr, ap)
	}, func(err error) {
		mensaje := tr.T("import.failed", nil)
		if errors.Is(err, model.ErrEncryptedMigrationRequired) {
			mensaje = texto(l, "volcado.cifrado")
		}
		local.Info(w, tr.T("import.action", nil), mensaje)
	})
	w.SetOnDropped(alSoltarFicheros(l, p))
	// Soltar texto o un enlace sobre el cuaderno crea la nota, como en la edición
	// conectada; los ficheros soltados van por SetOnDropped.
	soltar.AlSoltar(w, func(x soltar.Soltado) { p.Soltar(x.Texto, x.URLs, x.Posicion) })

	w.SetContent(l.Montar(w, iconos.Notes, p.Objeto()))
	// El editor abierto se guarda antes de cerrar, y después se suelta el almacén.
	w.SetCloseIntercept(func() {
		if !p.PrepararRetirada() {
			return
		}
		p.Cerrar()
		w.SetCloseIntercept(nil)
		w.Close()
	})
	p.Arrancar()
	// Recordatorios en el sistema (§5.12).
	recordar(ctx, l, repo)
	// La nota de un aviso pulsado o del Hub (`?clase=nota&id=…`), en su cuaderno; y los
	// enlaces de la suite a Notepads (`?notebook=…&note=…`, `?link=…`).
	l.AlAbrirElemento(func(clase, id string) {
		if clase == "nota" {
			go abrirNota(ctx, repo, p, id)
		}
	})
	l.AlAbrirEnlace(func(u *url.URL) { p.AtenderEnlace(u) })
	w.ShowAndRun()
}

// alSoltarFicheros atiende los ficheros soltados en la ventana: un volcado de cuaderno se
// importa, como al abrirlo; los demás, sobre una nota o el cuaderno, son su enlace o su
// nota (N-06, N-07). Lo que la pantalla no toma (fuera del cuaderno) va por donde iba:
// importarlo, que dice qué ficheros se importan.
func alSoltarFicheros(l *local.App, p *notes.Pantalla) func(fyne.Position, []fyne.URI) {
	return func(pos fyne.Position, uris []fyne.URI) {
		var otros []string
		for _, u := range uris {
			if u == nil || u.Scheme() != "file" {
				continue
			}
			if esVolcado(u.Path()) {
				l.AbrirLocal(u.Path())
			} else {
				otros = append(otros, u.Path())
			}
		}
		if len(otros) > 0 && !p.SoltarFicheros(otros, pos) {
			for _, r := range otros {
				l.AbrirLocal(r)
			}
		}
	}
}

// abrirRepositorio abre el repositorio de Notepads de la edición conectada, sin tocarlo,
// sobre el almacén en claro de este equipo.
func abrirRepositorio(ctx context.Context, l *local.App) (*privateNotes.Repository, *almacen.Recursos, error) {
	rec, err := l.Almacen(ctx, "notes")
	if err != nil {
		return nil, nil, err
	}
	// NewLocal: el de la edición conectada, que además guarda las notas de imagen con la
	// imagen dentro y los enlaces a ficheros del equipo (N-06, N-07).
	repo, err := privateNotes.NewLocal(rec.Config)
	if err != nil {
		return nil, nil, err
	}
	// Se cierra ANTES que el almacén: AlCerrar va del último al primero. Y antes, se
	// espera a la compactación en el acto, que lo refresca.
	l.AlCerrar(repo.Close)
	l.AlCerrar(rec.PararCompactar)
	if err := almacen.Preparar(ctx, repo); err != nil {
		return nil, nil, err
	}
	return repo, rec, nil
}

// notasLocales es el repositorio de Notepads con lo que Sytue Office hace al «borrar del
// todo» (§5.6): compactar el almacén en el acto, en segundo plano, para que lo purgado se
// vaya del disco sin esperar a cerrar la app. Lo demás es el repositorio tal cual (los
// métodos suben por la incrustación, también las interfaces opcionales que mira la
// fachada).
type notasLocales struct {
	*privateNotes.Repository
	rec *almacen.Recursos
}

// PurgeNote borra la nota para siempre y pide compactar (vaciar la papelera son muchas
// seguidas: se juntan en una).
func (n notasLocales) PurgeNote(ctx context.Context, id string) error {
	err := n.Repository.PurgeNote(ctx, id)
	if err == nil {
		n.rec.CompactarYa(n.RefreshPrivateData) //nolint:contextcheck // compacta en segundo plano y vive más que el borrado que la pidió
	}
	return err
}

// abrirNota busca el cuaderno de la nota id y la abre en él, como el enlace de la suite
// `/notebooks/<cuaderno>?note=<nota>`. Una nota que ya no está no abre nada.
func abrirNota(ctx context.Context, repo *privateNotes.Repository, p *notes.Pantalla, id string) {
	cuadernos, err := repo.ListNotebooks(ctx)
	if err != nil {
		return
	}
	for _, c := range cuadernos {
		notas, err := repo.ListNotes(ctx, c.ID)
		if err != nil {
			continue
		}
		for _, n := range notas {
			if n.ID == id {
				u := &url.URL{Path: "/notebooks/" + c.ID, RawQuery: url.Values{"note": {id}}.Encode()}
				fyne.Do(func() { p.AtenderEnlace(u) }) //nolint:contextcheck // la nota abierta vive lo que la pantalla, no la búsqueda que la encontró
				return
			}
		}
	}
}

// extensionesVolcado son las que entiende `model.LeerImportable`.
var extensionesVolcado = []string{".onex", ".md", ".markdown", ".zip", ".enex"}

// importar lee el fichero del equipo y lo da de alta como cuaderno nuevo.
func importar(ctx context.Context, repo *privateNotes.Repository, p *notes.Pantalla, w fyne.Window, tr *i18n.Translator, ap api.Apertura) error {
	nombre, datos, err := local.LeerLocal(ap.Local)
	if err != nil {
		return err
	}
	if !esVolcado(nombre) {
		return errNoEsVolcado
	}
	r, err := repo.ImportNotebook(ctx, datos, nombre)
	if err != nil {
		return err
	}
	//nolint:contextcheck // volver a pedir los cuadernos es de la pantalla, no de la importación
	fyne.Do(func() {
		texto := tr.T("import.done", i18n.Vars{"count": r.Importadas})
		if r.Descartadas > 0 {
			texto = tr.T("import.someSkipped", i18n.Vars{"count": r.Importadas, "skipped": r.Descartadas})
		}
		// Se vuelven a pedir los cuadernos: el nuevo aparece en el árbol.
		p.Arrancar()
		local.Info(w, r.Notebook.Title, texto)
	})
	return nil
}

var errNoEsVolcado = errors.New("notes: el fichero no es un volcado de cuaderno")

func esVolcado(nombre string) bool {
	ext := strings.ToLower(path.Ext(strings.ReplaceAll(nombre, `\`, "/")))
	for _, e := range extensionesVolcado {
		if ext == e {
			return true
		}
	}
	return false
}

// Textos propios de esta edición (§5.14): español e inglés aquí; los otros veinticuatro
// idiomas, en textos_idiomas.go.
var textos = map[string][2]string{
	"imagen.anadir":   {"Añadir una imagen de este equipo…", "Add an image from this device…"},
	"imagen.elegir":   {"Elige una imagen para la nota", "Choose an image for the note"},
	"imagen.noLeible": {"No se puede leer como imagen o es demasiado grande para una nota. Usa PNG, JPEG, GIF, WebP, AVIF, TIFF o HEIC.", "It can't be read as an image, or it's too big for a note. Use PNG, JPEG, GIF, WebP, AVIF, TIFF or HEIC."},
	"sin.calendar":    {"Sytue Calendar de Sytue Office no está instalada en este equipo.", "Sytue Calendar from Sytue Office is not installed on this device."},
	"volcado.cifrado": {"Esta copia es de un cuaderno cifrado y Sytue Office no abre cuadernos cifrados. Descífralo en la app donde lo creaste y vuelve a exportarlo.", "This backup is from an encrypted notebook, and Sytue Office does not open encrypted notebooks. Decrypt it in the app where you created it and export it again."},
}

var tabla = local.Propios{Base: textos, Traducciones: traducciones}

func texto(l *local.App, clave string) string { return tabla.En(l.Idioma(), clave) }
