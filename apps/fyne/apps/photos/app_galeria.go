// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"context"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Lo que la galería hace además de enseñar fotos: la selección múltiple y sus acciones
// en lote, la subida, los recuerdos y los atajos a «parecidas».

// ---- La selección ----

// entrarEnSeleccion es «Seleccionar». La lista de álbumes del selector se pide aquí y
// no al pulsar el selector: `ui.Selector` no avisa de que se va a abrir.
func (a *App) entrarEnSeleccion() {
	a.g.seleccion = map[string]bool{}
	a.pintar()
	a.conAlbumes(func([]api.Album) { a.pintar() })
}

func (a *App) salirDeSeleccion() {
	a.g.seleccion = nil
	a.pintar()
}

func (a *App) alternar(f api.Photo) {
	if a.g.seleccion == nil {
		return
	}
	if a.g.seleccion[f.ID] {
		delete(a.g.seleccion, f.ID)
	} else {
		a.g.seleccion[f.ID] = true
	}
	a.pintar()
}

// alternarMes marca las fotos de un mes o, si ya estaban todas, las desmarca.
func (a *App) alternarMes(fotos []api.Photo) {
	todas := true
	for _, f := range fotos {
		todas = todas && a.g.seleccion[f.ID]
	}
	for _, f := range fotos {
		if todas {
			delete(a.g.seleccion, f.ID)
		} else {
			a.g.seleccion[f.ID] = true
		}
	}
	a.pintar()
}

// seleccionadas son las fotos marcadas, en el orden de la línea de tiempo.
func (a *App) seleccionadas() []api.Photo {
	var out []api.Photo
	for _, f := range a.g.fotos {
		if a.g.seleccion[f.ID] {
			out = append(out, f)
		}
	}
	return out
}

// albumDeLaSeleccion crea un álbum con lo marcado.
func (a *App) albumDeLaSeleccion() {
	fotos := a.seleccionadas()
	if len(fotos) == 0 {
		return
	}
	a.preguntar(a.v.Traduce("gallery.newAlbumTitle"), a.v.T.T("gallery.newAlbumPrompt", i18n.Vars{"count": len(fotos)}),
		a.v.Traduce("gallery.newAlbumCreate"), TituloSugerido(fotos), func(titulo string) {
			a.crearCon(titulo, idsDe(fotos), func(al api.Album, err error) {
				if err != nil {
					a.g.error = a.textoDeFallo(err)
					return
				}
				a.g.aviso = a.v.T.T("gallery.addedToAlbum", i18n.Vars{"count": len(fotos), "title": al.Title})
				a.g.seleccion = nil
			})
		})
}

// anyadirSeleccionA mete lo marcado en un álbum que ya existe.
func (a *App) anyadirSeleccionA(al api.Album) {
	ids := idsDe(a.seleccionadas())
	if len(ids) == 0 {
		return
	}
	pedir(a, func(x context.Context, c *api.Photos) (int, error) { return c.AddToAlbum(x, al.ID, ids) }, func(_ int, err error) {
		if err != nil {
			a.g.error = a.textoDeFallo(err)
			return
		}
		a.g.aviso = a.v.T.T("gallery.addedToAlbum", i18n.Vars{"count": len(ids), "title": al.Title})
		a.g.seleccion = nil
	})
}

// parecidasALaSeleccion es «Más como estas».
func (a *App) parecidasALaSeleccion() { a.AbrirParecidas(idsDe(a.seleccionadas())) }

// parecidasALosResultados es «Y las que se parecen»: los aciertos de una búsqueda como
// punto de partida para llegar a las que nadie describió.
func (a *App) parecidasALosResultados() { a.AbrirParecidas(idsDe(a.g.fotos)) }

// descargarSeleccion exporta en JPEG lo marcado, una a una.
func (a *App) descargarSeleccion() {
	fotos := a.seleccionadas()
	if len(fotos) == 0 || a.cli == nil || a.Guardar == nil {
		return
	}
	a.g.aviso = a.v.T.T("gallery.downloading", i18n.Vars{"count": len(fotos)})
	a.pintar()
	gen := a.gen
	a.enSesion(func(vida context.Context) {
		fallidas := 0
		for _, f := range fotos {
			if vida.Err() != nil {
				return
			}
			if a.bajar(vida, f, "jpeg") != nil {
				fallidas++
			}
		}
		if fallidas > 0 {
			//nolint:contextcheck // Sólo reconstruye UI; los eventos futuros capturan la vida de cuenta en enSesion.
			a.hilo(gen, func() {
				a.g.error = a.v.Traduce("error.connection")
				a.pintar()
			})
		}
	})
}

// confirmarTirarSeleccion pregunta antes de mandar lo marcado a la papelera.
func (a *App) confirmarTirarSeleccion() {
	n := len(a.g.seleccion)
	if n == 0 {
		return
	}
	a.confirmar(a.v.Traduce("viewer.delete"), a.v.T.T("gallery.trashSelectedConfirm", i18n.Vars{"count": n}),
		a.v.Traduce("viewer.delete"), a.tirarSeleccion)
}

func (a *App) tirarSeleccion() {
	ids := idsDe(a.seleccionadas())
	pedir(a, func(x context.Context, c *api.Photos) ([]string, error) {
		var hechas []string
		for _, id := range ids {
			if c.Trash(x, id) == nil {
				hechas = append(hechas, id)
			}
		}
		return hechas, nil
	}, func(hechas []string, _ error) {
		for _, id := range hechas {
			a.g.fotos = quitarFoto(a.g.fotos, id)
		}
		a.g.seleccion = nil
		a.g.aviso = a.v.T.T("gallery.trashed", i18n.Vars{"count": len(hechas)})
		if len(hechas) < len(ids) {
			a.g.error = a.v.T.T("gallery.trashFailed", i18n.Vars{"count": len(ids) - len(hechas)})
		}
	})
}

// ---- La subida ----

// CarpetaFotos es `CARPETA_FOTOS`: los originales van a «/Fotos» del Drive de la
// persona, que es donde viven (decisión de almacén de Sytue Fotos).
const CarpetaFotos = "Fotos"

// elegirFicheros es «Añadir fotos»: el selector de ficheros DEL SISTEMA, con varios a
// la vez, como el `<input type="file" multiple accept="image/*">` de la web.
func (a *App) elegirFicheros() {
	if a.Elegir == nil || a.g.subida != nil {
		return
	}
	gen := a.gen
	go func() {
		rutas, err := a.Elegir()
		if err == nil && len(rutas) > 0 {
			a.hilo(gen, func() { a.subir(rutas) })
		}
	}()
}

// Arrastrando es el `dragover`/`dragleave` de la zona de soltar de la web: con ficheros
// arrastrándose por encima de la ventana (true) la galería dice «Suelta para añadir», y
// deja de decirlo al salir, al cancelar o al soltar. Lo conecta el `main` con
// `soltar.AlArrastrar`. Solo se ve en «Fotos» y «Favoritas», donde se puede soltar.
func (a *App) Arrastrando(encima bool) {
	if a.arrastrando == encima {
		return
	}
	a.arrastrando = encima
	if a.ruta == RutaFotos || a.ruta == RutaFavoritas {
		a.pintar()
	}
}

// soltado recibe los ficheros soltados sobre la ventana: la zona de soltar de la web.
func (a *App) soltado(_ fyne.Position, uris []fyne.URI) {
	if a.ruta != RutaFotos && a.ruta != RutaFavoritas {
		return
	}
	rutas := make([]string, 0, len(uris))
	for _, u := range uris {
		if u.Scheme() == "file" {
			rutas = append(rutas, u.Path())
		}
	}
	a.subir(rutas)
}

// esImagen es `f.type.startsWith('image/')`, con el tipo sacado de la extensión.
func esImagen(ruta string) bool { return strings.HasPrefix(tipoDe(ruta), "image/") }

// tipoDe es el tipo de una foto por su extensión. `mime` no sabe de HEIC ni de HEIF
// donde el sistema no trae su tabla —Android e iOS, que es justo de donde salen, y la
// copia que deja el selector del teléfono conserva el nombre—, y el navegador sí.
func tipoDe(ruta string) string {
	switch ext := strings.ToLower(filepath.Ext(ruta)); ext {
	case ".heic", ".heif", ".avif":
		return "image/" + ext[1:]
	default:
		return mime.TypeByExtension(ext)
	}
}

// subir deja cada foto en «/Fotos» de Drive y la indexa, DE UNA EN UNA: así el progreso
// es de verdad y una que falla no se lleva a las demás.
func (a *App) subir(rutas []string) {
	var fotos []string
	for _, r := range rutas {
		if esImagen(r) {
			fotos = append(fotos, r)
		}
	}
	if len(fotos) == 0 {
		a.g.aviso = a.v.Traduce("gallery.onlyImages")
		a.pintar()
		return
	}
	if a.cli == nil || a.Drive == nil || a.g.subida != nil {
		return
	}
	a.g.aviso, a.g.subida = "", &[2]int{0, len(fotos)}
	a.pintar()
	gen := a.gen
	a.enSesion(func(vida context.Context) {
		carpeta, err := a.carpetaFotos(vida)
		var fallos []string
		for i, r := range fotos {
			if vida.Err() != nil {
				return
			}
			if err != nil || a.subirUna(vida, carpeta, r) != nil {
				fallos = append(fallos, filepath.Base(r))
			}
			//nolint:contextcheck // Sólo reconstruye UI; los eventos futuros capturan la vida de cuenta en enSesion.
			a.hilo(gen, func() {
				a.g.subida = &[2]int{i + 1, len(fotos)}
				a.pintar()
			})
		}
		//nolint:contextcheck // Sólo reconstruye UI; los eventos futuros capturan la vida de cuenta en enSesion.
		a.hilo(gen, func() {
			a.g.subida = nil
			hechas := len(fotos) - len(fallos)
			if len(fallos) > 0 {
				a.g.aviso = a.v.T.T("gallery.uploadFailed", i18n.Vars{"name": strings.Join(fallos, ", ")})
			} else {
				a.g.aviso = a.v.T.T("gallery.uploaded", i18n.Vars{"count": hechas})
			}
			if hechas > 0 {
				// La línea de tiempo se vuelve a pedir desde el principio: las nuevas
				// caen donde les toque por fecha, no arriba por haber llegado las
				// últimas.
				aviso := a.g.aviso
				a.gen++
				a.g = estadoGaleria{texto: a.g.texto, aviso: aviso}
				a.cerrarVisor()
				a.cargarMas()
			}
			a.pintar()
		})
	})
}

// carpetaFotos es el id de «/Fotos» en la raíz del Drive, creándola si no está.
func (a *App) carpetaFotos(vida context.Context) (string, error) {
	c, fin := ctx(vida)
	defer fin()
	raiz, err := a.Drive.Root(c)
	if err != nil {
		return "", err
	}
	for cursor := ""; ; {
		hijos, err := a.Drive.Children(c, raiz.ID, cursor)
		if err != nil {
			return "", err
		}
		for _, f := range hijos.Folders {
			if f.Name == CarpetaFotos {
				return f.ID, nil
			}
		}
		if cursor = hijos.NextCursor; cursor == "" {
			break
		}
	}
	f, err := a.Drive.CreateFolder(c, raiz.ID, CarpetaFotos)
	return f.ID, err
}

func (a *App) subirUna(vida context.Context, carpeta, ruta string) error {
	f, err := os.Open(ruta) //nolint:gosec // es la ruta que la persona ha elegido o soltado
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	c, fin := context.WithTimeout(vida, 10*time.Minute)
	defer fin()
	fichero, err := a.Drive.Upload(c, carpeta, filepath.Base(ruta), tipoDe(ruta), f, info.Size())
	if err != nil {
		return err
	}
	_, err = a.cli.Indexar(c, fichero.ID)
	return err
}

// ---- Los recuerdos ----

// prefRecuerdosOcultos es `claveDeHoy()`: ocultarlos vale para HOY, y mañana hay otros.
func prefRecuerdosOcultos(hoy time.Time) string {
	return "sytue-fotos-recuerdos-ocultos:" + hoy.UTC().Format("2006-01-02")
}

// pedirRecuerdos los pide una vez por sesión, salvo que hoy estén ocultos.
func (a *App) pedirRecuerdos() {
	if a.rec.pedidos || a.cli == nil {
		return
	}
	if p := a.prefs(); p != nil && p.Bool(prefRecuerdosOcultos(time.Now())) {
		return
	}
	a.rec.pedidos = true
	gen := a.gen
	a.enSesion(func(vida context.Context) {
		c, fin := ctx(vida)
		defer fin()
		lista, err := a.cli.Memories(c)
		fyne.Do(func() {
			if err != nil || vida.Err() != nil {
				return
			}
			a.rec.lista = lista
			if gen == a.gen {
				//nolint:contextcheck // Sólo reconstruye UI; los eventos futuros capturan la vida de cuenta en enSesion.
				a.pintar()
			}
		})
	})
}

func (a *App) ocultarRecuerdos() {
	if p := a.prefs(); p != nil {
		p.SetBool(prefRecuerdosOcultos(time.Now()), true)
	}
	a.rec.lista = nil
	a.pintar()
}

// abrirRecuerdo abre la foto si ya está en la página cargada; si no —un recuerdo de
// 2019 puede estar tres mil filas más abajo—, va a su pantalla de parecidas, que la
// enseña sin arrastrar la línea de tiempo entera.
func (a *App) abrirRecuerdo(id string) {
	if i := posicion(a.g.fotos, id); i >= 0 {
		a.abrirVisor(desdeGaleria, i)
		return
	}
	a.AbrirParecidas([]string{id})
}
