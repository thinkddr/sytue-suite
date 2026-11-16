// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"context"
	"errors"
	"mime"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Lo que la galería hace además de enseñar fotos: la selección múltiple y sus acciones
// en lote, la subida, los recuerdos y los atajos a «parecidas».

// ---- La selección ----

// entrarEnSeleccion es «Seleccionar». La lista de álbumes del selector se pide aquí y
// no al pulsar el selector: `ui.Selector` no avisa de que se va a abrir.
func (a *App) entrarEnSeleccion() {
	a.g.seleccion, a.g.ancla = map[string]bool{}, ""
	a.pintar()
	a.conAlbumes(func([]api.Album) { a.pintar() })
}

// empezarSeleccion es mantener pulsada una tesela (en el escritorio, el clic derecho):
// entra en el modo selección con esa foto ya marcada y de ancla, sin abrir el visor.
// Es el gesto de Google Photos en el móvil, donde «Seleccionar» queda lejos del dedo.
func (a *App) empezarSeleccion(f api.Photo) {
	if a.g.seleccion != nil {
		a.alternar(f)
		return
	}
	a.g.seleccion, a.g.ancla = map[string]bool{f.ID: true}, f.ID
	a.pintar()
	a.conAlbumes(func([]api.Album) { a.pintar() })
}

func (a *App) salirDeSeleccion() {
	a.g.seleccion, a.g.ancla = nil, ""
	a.pintar()
}

// mayusPulsada es `ui.Mayus`, en variable para que las pruebas pulsen Mayús sin un
// driver de escritorio.
var mayusPulsada = ui.Mayus

// alternar marca o desmarca una foto; con Mayús, añade el tramo desde el ancla. El tramo
// se cuenta sobre `fotos`, el orden en que se ven (los meses la parten, no la reordenan).
func (a *App) alternar(f api.Photo) {
	if a.g.seleccion == nil {
		return
	}
	desde := -1
	if a.g.ancla != "" {
		desde = a.posicionDe(a.g.ancla)
	}
	if i := ui.MarcarConTramo(a.g.seleccion, a.g.fotos, idDeFoto, a.posicionDe(f.ID), desde, mayusPulsada()); i >= 0 {
		a.g.ancla = a.g.fotos[i].ID
	}
	a.pintar()
}

func idDeFoto(f api.Photo) string { return f.ID }

// posicionDe es el índice de la foto en la línea de tiempo cargada; -1 si no está.
func (a *App) posicionDe(id string) int {
	for i, f := range a.g.fotos {
		if f.ID == id {
			return i
		}
	}
	return -1
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
				a.avisar(ui.AvisoExito, a.v.T.T("gallery.addedToAlbum", i18n.Vars{"count": len(fotos), "title": al.Title}), "")
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
	pedirAlbum(a, func(x context.Context, c *api.Photos) (int, error) { return c.AddToAlbum(x, al.ID, ids) }, func(_ int, err error) {
		if err != nil {
			a.g.error = a.textoDeFallo(err)
			return
		}
		a.avisar(ui.AvisoExito, a.v.T.T("gallery.addedToAlbum", i18n.Vars{"count": len(ids), "title": al.Title}), "")
		a.g.seleccion = nil
	})
}

// parecidasALaSeleccion es «Más como estas».
func (a *App) parecidasALaSeleccion() { a.AbrirParecidas(idsDe(a.seleccionadas())) }

// parecidasALosResultados es «Y las que se parecen»: los aciertos de una búsqueda como
// punto de partida para llegar a las que nadie describió.
func (a *App) parecidasALosResultados() { a.AbrirParecidas(idsDe(a.g.fotos)) }

// guardarSeleccionEnSistema deja lo marcado en la galería del sistema (guardarEnSistema).
func (a *App) guardarSeleccionEnSistema() { a.guardarEnSistema(a.seleccionadas()) }

// descargarSeleccion exporta en JPEG lo marcado, una a una.
func (a *App) descargarSeleccion() {
	fotos := a.seleccionadas()
	if len(fotos) == 0 || a.cli == nil || a.Guardar == nil {
		return
	}
	a.g.aviso = a.v.T.T("gallery.downloadingNative", i18n.Vars{"count": len(fotos)})
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

// tirarSeleccion manda lo marcado a la papelera SIN preguntar: es reversible y lo
// deshace el aviso (ver deshacer.go). Lo que el servicio no acepta se dice aparte.
func (a *App) tirarSeleccion() {
	lista, gen, ruta := a.g.fotos, a.gen, a.ruta
	a.tirarSinRepetir(idsDe(a.seleccionadas()), func(nuevas, hechas []string, _ error, misma bool) {
		t := a.nuevaTirada(lista, hechas, gen, ruta)
		// Lo tirado sale de la lista que se vea; la selección y el error son de la
		// galería de cuando se tiró.
		for _, id := range hechas {
			a.g.fotos = quitarFoto(a.g.fotos, id)
		}
		if misma {
			a.g.seleccion = nil
			if len(hechas) < len(nuevas) {
				a.g.error = a.v.T.T("gallery.trashFailed", i18n.Vars{"count": len(nuevas) - len(hechas)})
			}
		}
		a.avisarTirada(t)
		a.pintar()
	})
}

// planFavoritas es qué hace el corazón de la barra con lo marcado (`planFavoritas` en
// favoritasEnLote.ts): si TODAS son ya favoritas, las quita; si falta alguna, las marca.
// Solo se piden las que cambian.
//
// Una marcada que no está en `fotos` tiene un corazón que no se conoce: entonces no se
// puede decir que TODAS lo sean, y se marca (no cambia nada si ya lo era). Aquí `buscar`
// vacía la selección y no llega a pasar; la regla es la de la web, donde sí.
func planFavoritas(fotos []api.Photo, sel map[string]bool) (valor bool, ids []string) {
	todas, alguna := true, false
	cargada := make(map[string]bool, len(sel))
	for _, f := range fotos {
		if sel[f.ID] {
			alguna = true
			todas = todas && f.Favorite
			cargada[f.ID] = true
		}
	}
	var fuera []string
	for id, si := range sel {
		if si && !cargada[id] {
			fuera = append(fuera, id)
		}
	}
	sort.Strings(fuera)
	valor = len(fuera) > 0 || !alguna || !todas
	for _, f := range fotos {
		if sel[f.ID] && f.Favorite != valor {
			ids = append(ids, f.ID)
		}
	}
	if valor {
		ids = append(ids, fuera...)
	}
	return valor, ids
}

// conFavoritas es la lista tras el lote con SOLO las que el servicio aceptó: en
// «Favoritas», desmarcar las saca; en las demás, cambia su corazón.
func conFavoritas(fotos []api.Photo, hechas []string, valor, soloFavoritas bool) []api.Photo {
	hecha := make(map[string]bool, len(hechas))
	for _, id := range hechas {
		hecha[id] = true
	}
	out := make([]api.Photo, 0, len(fotos))
	for _, f := range fotos {
		if hecha[f.ID] {
			if soloFavoritas && !valor {
				continue
			}
			f.Favorite = valor
		}
		out = append(out, f)
	}
	return out
}

// favoritasDeLaSeleccion es el corazón de la barra de selección: una llamada por foto
// que cambia (el servicio no tiene lote), y la lista cambia con las que volvieron bien.
// Mientras corre, otra pulsación no hace nada: con un doble clic el plan se calculaba
// dos veces y cada foto se pedía dos veces.
func (a *App) favoritasDeLaSeleccion() {
	if len(a.g.seleccion) == 0 || a.g.marcandoFavoritas || a.cli == nil {
		return
	}
	valor, ids := planFavoritas(a.g.fotos, a.g.seleccion)
	solo := a.soloFavoritas()
	a.g.marcandoFavoritas = true
	a.pintar()
	pedir(a, func(x context.Context, c *api.Photos) ([]string, error) {
		var hechas []string
		for _, id := range ids {
			if c.SetFavorite(x, id, valor) == nil {
				hechas = append(hechas, id)
			}
		}
		return hechas, nil
	}, func(hechas []string, _ error) {
		a.g.marcandoFavoritas = false
		a.g.fotos = conFavoritas(a.g.fotos, hechas, valor, solo)
		a.g.seleccion = nil
		clave := "gallery.unfavorited"
		if valor {
			clave = "gallery.favorited"
		}
		a.avisar(ui.AvisoExito, a.v.T.T(clave, i18n.Vars{"count": len(hechas)}), "")
		if len(hechas) < len(ids) {
			a.g.error = a.v.T.T("gallery.favoriteFailed", i18n.Vars{"count": len(ids) - len(hechas)})
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
	if a.cli == nil || (!a.cli.CanImportPrivatePhotos() && a.Drive == nil) || a.g.subida != nil || a.reanudandoImportaciones {
		return
	}
	a.g.aviso, a.g.subida = "", &[2]int{0, len(fotos)}
	a.pintar()
	gen := a.gen
	a.enSesion(func(vida context.Context) {
		d := nuevoDestinoDrive(a.Drive, a.cli)
		_, err := d.carpetaFotos(vida)
		var fallos, sinGaleria []string
		for i, r := range fotos {
			if vida.Err() != nil {
				return
			}
			e := err
			if e == nil {
				e = a.subirUna(vida, d, r)
			}
			switch {
			case errors.Is(e, ErrNoIndexable):
				sinGaleria = append(sinGaleria, filepath.Base(r))
			case e != nil:
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
			hechas := len(fotos) - len(fallos) - len(sinGaleria)
			// El progreso va en línea; cómo acabó, en el aviso. Lo que está en Drive pero la
			// galería no sabe enseñar NO es un fallo: volver a subirlo no lo arreglaría.
			switch {
			case len(fallos) > 0:
				a.avisar(ui.AvisoPeligro, a.v.T.T("gallery.uploadFailed", i18n.Vars{"name": strings.Join(fallos, ", ")}), "")
			case len(sinGaleria) > 0:
				a.avisar(ui.AvisoAdvertencia, a.v.T.T("gallery.uploadedNotShown", i18n.Vars{"name": strings.Join(sinGaleria, ", ")}), "")
			default:
				a.avisar(ui.AvisoExito, a.v.T.T("gallery.uploaded", i18n.Vars{"count": hechas}), "")
			}
			if hechas > 0 {
				// La línea de tiempo se vuelve a pedir desde el principio: las nuevas
				// caen donde les toque por fecha, no arriba por haber llegado las
				// últimas.
				a.gen++
				// En el año al que se había saltado: subir no tiene por qué mover la
				// vista. Y la barra se vuelve a pedir, que las subidas pueden ser de un
				// año que aún no tenía (como la web).
				a.g = estadoGaleria{texto: a.g.texto, anyo: a.g.anyo}
				a.cerrarVisor()
				a.cargarMas()
				a.pedirAnyos()
			}
			a.pintar()
		})
	})
}

// carpetaFotosEn es el id de «/Fotos» en la raíz del Drive, creándola si no está. Con un
// cliente de Drive cualquiera: también el de la copia con la app cerrada, sin App.
func carpetaFotosEn(vida context.Context, drive *api.Drive) (string, error) {
	c, fin := ctx(vida)
	defer fin()
	raiz, err := drive.Root(c)
	if err != nil {
		return "", err
	}
	for cursor := ""; ; {
		hijos, err := drive.Children(c, raiz.ID, cursor)
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
	f, err := drive.CreateFolder(c, raiz.ID, CarpetaFotos)
	return f.ID, err
}

// subirUna sube una foto elegida o soltada sin sustituir ninguna de «/Fotos» y la
// indexa. Lo que la galería no sabe enseñar (HEIC, WebP, AVIF) se pasa antes a JPEG con
// sus metadatos donde el sistema sabe hacerlo, como en la copia de seguridad; si no, sube
// tal cual y queda en Drive ([ErrNoIndexable]).
func (a *App) subirUna(vida context.Context, d *destinoDrive, ruta string) error {
	if d == nil {
		d = nuevoDestinoDrive(a.Drive, a.cli)
	}
	nombre, tipo := filepath.Base(ruta), tipoDe(ruta)
	if !SyphotosMide(tipo) && a.Galeria != nil && a.Galeria.Disponible() {
		jpeg, limpiar, err := a.Galeria.Convertir(vida, ruta)
		defer limpiar()
		if err == nil {
			ruta, tipo = jpeg, "image/jpeg"
			nombre = strings.TrimSuffix(nombre, filepath.Ext(nombre)) + ".jpg"
		}
	}
	s, err := d.Subir(vida, ruta, nombre, tipo)
	if err != nil {
		return err
	}
	return d.Indexar(vida, s.ID)
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
