// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/apps/drive"
	"git.sytue.eu/sytue/one/apps/fyne/local"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/local/archivos"
)

// Los textos PROPIOS de esta app, que no están en el catálogo de Drive (PLAN §5.14):
// español e inglés aquí, los otros veinticuatro idiomas en equipo_idiomas.go.
var tabla = map[string][2]string{ // clave: {es, en}
	"copiarA":        {"Copiar a…", "Copy to…"},
	"mostrarCarpeta": {"Mostrar en la carpeta", "Show in folder"},
	"abrirCon":       {"Abrir con su app", "Open in app"},
	"noSeCopia":      {"No se pudo copiar", "Could not copy"},
	"noSeAbre":       {"No se pudo abrir", "Could not open"},
	"nuevoFallo":     {"No se pudo crear el documento", "Could not create the document"},
	"elegirDestino":  {"Elige dónde dejar la copia", "Choose where to put the copy"},
	"documento":      {"Documento sin título", "Untitled document"},
	"hoja":           {"Hoja sin título", "Untitled spreadsheet"},
	"presentacion":   {"Presentación sin título", "Untitled presentation"},
	"etiquetas":      {"Etiquetas", "Tags"},
	"etiquetaNueva":  {"Nueva etiqueta", "New tag"},
	"etiquetaAnadir": {"Añadir", "Add"},
	"etiquetasVacio": {"Todavía no has puesto ninguna etiqueta. Escribe una abajo para empezar.", "You haven't added any tags yet. Type one below to start."},
	"etiquetasFallo": {"No se pudieron guardar las etiquetas", "Could not save the tags"},
	"papeleraAyuda": {
		"Restaurar devuelve el elemento a la carpeta donde estaba. Mientras siga en la papelera, sigue ocupando espacio en el disco: para liberarlo hay que eliminarlo definitivamente.",
		"Restoring puts the item back in the folder it was in. While it stays in the trash, it still takes up disk space: to free it, delete it permanently.",
	},
	"papeleraCuenta": {
		"Debajo de cada nombre verás cuánto le queda antes de borrarse solo de este equipo.",
		"Under each name you will see how long it has left before it is deleted from this device on its own.",
	},
	"editarFoto":        {"Editar en Photos", "Edit in Photos"},
	"nubeSubido":        {"Guardado en la nube", "Saved to the cloud"},
	"nubeFallo":         {"No se pudo subir a la nube; se reintentará", "Could not upload to the cloud; will retry"},
	"nubeConflicto":     {"Alguien cambió el fichero en la nube", "Someone changed the file in the cloud"},
	"nubeConflictoComo": {"Tus cambios se han guardado al lado, como «{name}».", "Your changes were saved next to it, as “{name}”."},
	"purga": {
		"Se borrará «{name}» de este equipo, con su historial de versiones. No se puede deshacer.",
		"“{name}” will be deleted from this device, with its version history. This can't be undone.",
	},
	// La carpeta de Drive elegida con el selector del sistema en Android (DR-12, carpeta.go).
	"carpetaTitulo": {"Carpeta de Drive", "Drive folder"},
	"carpetaAyuda": {
		"Elige una carpeta del teléfono o de la tarjeta (en Documentos, por ejemplo) para tener ahí tus ficheros de Drive, a la vista de las demás apps. Drive recuerda el permiso hasta que lo quites aquí o en los ajustes del sistema.",
		"Choose a folder on your phone or memory card (in Documents, for example) to keep your Drive files there, where other apps can see them. Drive remembers the permission until you remove it here or in the system settings.",
	},
	"carpetaActual":      {"Ahora: «{name}»", "Now: “{name}”"},
	"carpetaApp":         {"Ahora: la carpeta de la app, que solo ve Drive", "Now: the app's own folder, which only Drive can see"},
	"carpetaElegir":      {"Elegir carpeta de Drive…", "Choose Drive folder…"},
	"carpetaVolver":      {"Usar la carpeta de la app", "Use the app's folder"},
	"carpetaSoloLectura": {"Esa carpeta no deja guardar cambios. Elige otra.", "That folder doesn't allow saving changes. Choose another one."},
	"carpetaTemporal": {
		"Esa carpeta no deja que Drive recuerde el permiso: la usará hasta que cierres la app.",
		"That folder doesn't let Drive remember the permission: Drive will use it until you close the app.",
	},
	"carpetaPerdida": {
		"Drive no puede abrir «{name}» ahora, así que usa la carpeta de la app. Si ya no está o le quitaste el permiso, vuelve a elegirla en Ajustes.",
		"Drive can't open “{name}” right now, so it is using the app's folder. If it's gone or you removed its permission, choose it again in Settings.",
	},
	"carpetaFallo": {"No se pudo usar esa carpeta", "Could not use that folder"},
	// «Subir carpeta» en Android, con el mismo selector (arbol.go).
	"noSeSube": {"No se pudo leer esa carpeta", "Could not read that folder"},
	// Mover entre unidades (el disco y las nubes): la hoja de progreso, el aviso del final
	// y por qué no se movió algo (mover.go).
	"moviendo":           {"Moviendo a «{name}»", "Moving to “{name}”"},
	"moviendoPaso":       {"{current} de {total}: {name}", "{current} of {total}: {name}"},
	"moverParcial":       {"Se movieron {moved} de {total}; lo demás sigue donde estaba", "{moved} of {total} moved; the rest is still where it was"},
	"moverCancelado":     {"Cancelado: se movieron {moved} de {total}; lo demás sigue donde estaba", "Cancelled: {moved} of {total} moved; the rest is still where it was"},
	"moverNoMovidos":     {"Lo que no se ha movido", "What wasn't moved"},
	"motivoExiste":       {"ya hay algo con ese nombre en el destino", "there is already something with that name at the destination"},
	"motivoComprobacion": {"la copia no llegó entera y se ha quitado", "the copy didn't arrive intact and was removed"},
	"motivoGoogle": {
		"es un documento de Google, sin fichero que mover; tráelo con «Copiar a…»",
		"it's a Google document with no file to move; bring it over with “Copy to…”",
	},
	"motivoOrigen": {
		"se copió, pero no se pudo quitar del origen: está en los dos sitios",
		"it was copied but couldn't be removed from where it was: it's in both places",
	},
	"motivoTamano": {"pasa de los {size} que admite la nube", "it's over the {size} the cloud accepts"},
	"motivoCopia":  {"no se pudo copiar", "couldn't be copied"},
	// Subir o soltar con un nombre que ya está en una unidad de la nube (homonimos.go).
	"homonimoTitulo":  {"Ya hay un «{name}» aquí", "There's already a “{name}” here"},
	"homonimosTitulo": {"Ya hay varios «{name}» aquí", "There are already several “{name}” here"},
	"homonimoTexto": {
		"Puedes sustituirlo por el que subes o guardar los dos; el nuevo se llamaría «{free}».",
		"You can replace it with the one you're uploading or keep both; the new one would be called “{free}”.",
	},
	"homonimosTexto": {
		"Elige cuál sustituir por el que subes, o guarda los dos; el nuevo se llamaría «{free}».",
		"Choose which one to replace with the one you're uploading, or keep both; the new one would be called “{free}”.",
	},
	"sustituir":    {"Sustituir", "Replace"},
	"guardarAmbos": {"Guardar los dos", "Keep both"},
	"noSubir":      {"No subirlo", "Don't upload"},
	// La estrella de lo que está en la nube se guarda en este equipo, no en la cuenta.
	"favoritoNube":       {"Añadir a favoritos de este equipo", "Add to favorites on this device"},
	"favoritoNubeQuitar": {"Quitar de favoritos de este equipo", "Remove from favorites on this device"},
}

// textos devuelve la función de rótulos en el idioma de la app: español e inglés de
// `tabla`, y los otros veinticuatro de `traducciones` (equipo_idiomas.go).
func textos(l *local.App) func(string) string {
	p, idioma := local.Propios{Base: tabla, Traducciones: traducciones}, l.Idioma()
	return func(clave string) string { return p.En(idioma, clave) }
}

// tituloDe es el nombre con el que nace un documento nuevo.
var tituloDe = map[api.DocKind]string{api.DocText: "documento", api.DocSheet: "hoja", api.DocSlides: "presentacion"}

// enEquipo es lo que la pantalla de Drive necesita para trabajar sobre el disco. carpeta
// es la elegida con el selector en Android (nil fuera): lo de ahí se enseña con la app del
// sistema, que no puede leer su ruta.
func enEquipo(a fyne.App, disco *archivos.Archivos, rem *archivos.Remotas, t func(string) string, carpeta *carpetaDrive, idioma i18n.Locale) *drive.EnEquipo {
	ruta := func(el drive.Elemento) (string, error) {
		ctx, cancelar := ctxCorto()
		defer cancelar()
		if el.Carpeta {
			c, err := rem.Folder(ctx, el.ID)
			return c.Path, err
		}
		// En una unidad de la nube, File lo baja a la caché y da esa ruta.
		f, err := rem.File(ctx, el.ID)
		return f.Path, err
	}
	return &drive.EnEquipo{
		Texto: t,
		Abrir: func(el drive.Elemento) error {
			r, err := ruta(el)
			if err != nil {
				return err
			}
			if ver, ok := carpeta.enArbol(r); ok {
				return ver(true)
			}
			return abrirEnSuApp(a, r)
		},
		CrearDocumento: func(ctx context.Context, carpeta string, k api.DocKind) (api.File, error) {
			return disco.CrearDocumento(ctx, carpeta, k, t(tituloDe[k]))
		},
		CopiarA: func(el drive.Elemento, listo func(string, error)) {
			if carpeta != nil {
				// En Android, el destino es una carpeta del teléfono elegida con el selector
				// del sistema (arbol.go); cancelar no dice nada.
				go func() {
					ctx, cancelar := ctxCorto()
					defer cancelar()
					destino, elegida, err := carpeta.copiarA(ctx, el.ID, rem.CopiarFueraEn)
					if elegida {
						listo(destino, err)
					}
				}()
				return
			}
			local.ElegirCarpeta(t("elegirDestino"), func(destino string) {
				go func() {
					ctx, cancelar := ctxCorto()
					defer cancelar()
					listo(rem.CopiarFuera(ctx, el.ID, destino))
				}()
			})
		},
		// Las del disco y las de las nubes, que se guardan en este equipo (nube.go).
		Etiquetas: &drive.EtiquetasEnEquipo{De: rem.Etiquetas, Todas: rem.TodasLasEtiquetas, Poner: rem.PonerEtiquetas},
		Unidades:  unidadesDe(rem),
		UnidadDe:  archivos.UnidadDe,
		// Entre el disco y las nubes: copiar, comprobar y quitar el origen (nube.go).
		MoverEntreUnidades: moverEntreUnidades(rem, t, idioma),
		Homonimos:          homonimosDe(rem),
		// Una imagen, directamente en el editor de Photos de Sytue Office (FO-06); sin
		// Photos instalada, con la app del sistema.
		EditarFoto: func(el drive.Elemento) error {
			r, err := ruta(el)
			if err != nil {
				return err
			}
			if ver, ok := carpeta.enArbol(r); ok {
				return ver(true)
			}
			return local.AbrirEn(a, "photos", r)
		},
		EsRemoto: archivos.EsRemoto,
		MostrarEnCarpeta: func(el drive.Elemento) error {
			r, err := ruta(el)
			if err != nil {
				return err
			}
			if r == "" {
				return archivos.ErrNoDisponible
			}
			if ver, ok := carpeta.enArbol(r); ok {
				return ver(false)
			}
			return ficheros.Revelar(r)
		},
	}
}

// productoDe es la app de Sytue Office que abre un fichero, o "" si no hay ninguna: la
// tabla de la suite (`api.AppQueAbre`) sin las fotos —aquí se ven en la vista previa, y
// Photos las importaría a su fototeca en vez de enseñarlas— y con el vídeo a Cast (§5.11).
func productoDe(nombre string) string {
	if p, ok := api.AppQueAbre(nombre); ok && p != "photos" {
		return p
	}
	if drive.EsMultimedia(drive.Elemento{ID: "-", Nombre: nombre}) && esVideo(nombre) {
		return "cast"
	}
	return ""
}

var extensionesVideo = map[string]bool{"mp4": true, "m4v": true, "webm": true, "mov": true, "ogv": true}

func esVideo(nombre string) bool { return extensionesVideo[drive.Extension(nombre)] }

// abrirEnSuApp abre r en su app de Sytue Office por su esquema y, si no está instalada o
// no hay app para ese formato, con la app predeterminada del sistema, directamente sobre
// el fichero (DR-07). Nunca cae a la web: es `local.AbrirEn` (§5.11).
func abrirEnSuApp(a fyne.App, r string) error { return local.AbrirEn(a, productoDe(r), r) }
