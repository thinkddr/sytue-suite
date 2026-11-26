// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package carrete es la galería de fotos y vídeos DEL SISTEMA: la fototeca de la app Fotos en
// iOS y macOS (PhotoKit), la galería de Android (MediaStore) y la carpeta de imágenes
// del perfil en Windows y Linux, que es la que leen la app Fotos de Windows, GNOME Fotos
// o Shotwell.
//
// Sirve para tres cosas, y ninguna más:
//
//   - **Leer** lo que hay (`Listar`, `Abrir`): la copia de seguridad de Sytue Photos sube
//     desde aquí las fotos del dispositivo.
//   - **Guardar** una foto en ella (`Guardar`), en el álbum o la carpeta «Sytue Photos»:
//     una foto bajada de la nube se ve después en la galería del teléfono, como en
//     cualquier otra app de fotos.
//   - **Pedir permiso** (`EstadoPermiso`, `PedirPermiso`): en el móvil y en macOS la
//     fototeca es privada y la persona tiene que concederlo.
//
// Y dos ayudas que solo el sistema sabe dar: `Convertir` pasa una foto HEIC, WebP o AVIF
// a JPEG CON sus metadatos (fecha, lugar, orientación, cámara), que es lo que hace el
// propio iPhone al pasar fotos a un ordenador; y `RedMedida` dice si se está usando una
// red de pago (datos móviles, el teléfono compartiendo conexión), para no gastarla.
//
// Lo que el paquete NO hace es decidir qué se sube, ni cuándo, ni recordar qué se subió:
// eso es de quien lo usa (`apps/fyne/apps/photos`, la copia de seguridad). Así el núcleo
// no sabe nada de Sytue Drive ni de cuentas, y se puede probar sin red.
//
// Lo guardado por Sytue NO se vuelve a listar: la carpeta o el álbum «Sytue Photos» se
// salta al leer, o la copia de seguridad subiría otra vez lo que acaba de bajar.
package carrete

import (
	"context"
	"errors"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// AlbumPropio es el álbum (iOS, macOS) o la carpeta (Android, Windows, Linux) donde
// Sytue deja lo que guarda. Es una marca, no se traduce: es el nombre del producto.
const AlbumPropio = "Sytue Photos"

// ProfundidadMaxima es hasta dónde se baja en cada carpeta (la galería del escritorio y
// las carpetas de Photos de Sytue Office): lo bastante para «Imágenes/2024/Vacaciones/Día
// 3» y no tanto como para recorrer un disco entero enlazado ahí por error. Aquí y no en
// carpeta.go, que no se compila en Android ni en Apple con cgo y quien la usa sí.
const ProfundidadMaxima = 8

// ErrNoDisponible es que ESTE sistema no tiene galería a la que llegar: el navegador, o
// un binario compilado sin el puente nativo. Quien llama no ofrece la función; no es un
// fallo que enseñar.
var ErrNoDisponible = errors.New("carrete: este sistema no tiene galería de fotos")

// ErrSinPermiso es que la persona no ha dado (o ha quitado) el acceso a sus fotos.
var ErrSinPermiso = errors.New("carrete: sin permiso para leer las fotos")

// Permiso es lo que la persona ha decidido sobre el acceso a su galería.
type Permiso uint8

// Los cinco.
const (
	// SinPreguntar: todavía no se le ha preguntado. Pedirlo enseña el diálogo del sistema.
	SinPreguntar Permiso = iota
	// Concedido: se ven todas las fotos.
	Concedido
	// Limitado: solo las que la persona eligió (iOS 14+, Android 14+). La copia de
	// seguridad sube esas y avisa de que no son todas.
	Limitado
	// Denegado: dijo que no, o lo quitó en los ajustes. Volver a pedirlo no enseña nada:
	// solo se cambia desde los ajustes del sistema.
	Denegado
	// NoDisponible: este sistema no tiene galería ([ErrNoDisponible]).
	NoDisponible
)

// Lee dice si con este permiso se puede listar algo.
func (p Permiso) Lee() bool { return p == Concedido || p == Limitado }

// Elemento es una foto de la galería del sistema.
type Elemento struct {
	// ID la identifica en ESTE dispositivo, estable entre arranques: el
	// `localIdentifier` de PhotoKit, el `_ID` de MediaStore o la ruta relativa a la
	// carpeta de imágenes. No vale en otro dispositivo.
	ID string
	// Nombre es el del fichero original («IMG_0001.HEIC»), con su extensión.
	Nombre string
	// Tipo es el MIME del original («image/heic»). Vacío si el sistema no lo dice.
	Tipo string
	// Tomada es cuándo se hizo; si el sistema no lo sabe, cuándo llegó a la galería.
	Tomada time.Time
	// Tamano en bytes del original; -1 si no se sabe sin leerlo (PhotoKit).
	Tamano int64
	// Video: es un vídeo y no una foto.
	Video bool
	// Duracion, Ancho y Alto son los de un vídeo si el sistema los dice al listar
	// (MediaStore, PhotoKit); cero si no. Quien lo importa los mide del fichero y estos
	// le sirven de respaldo.
	Duracion    time.Duration
	Ancho, Alto int
}

// Disponible dice si este sistema tiene galería a la que llegar. No pregunta nada a la
// persona ni enseña ningún diálogo.
func Disponible() bool { return disponible() }

// EstadoPermiso dice lo que la persona ha decidido, SIN preguntárselo.
func EstadoPermiso() Permiso {
	if !disponible() {
		return NoDisponible
	}
	return estadoPermiso()
}

// PedirPermiso enseña el diálogo del sistema si hace falta y ESPERA la respuesta (o a que
// `ctx` se acabe). Con el permiso ya decidido no enseña nada y devuelve lo que hay.
// BLOQUEA: no se llama desde el hilo de la interfaz.
func PedirPermiso(ctx context.Context) (Permiso, error) {
	if !disponible() {
		return NoDisponible, ErrNoDisponible
	}
	if p := estadoPermiso(); p != SinPreguntar {
		return p, nil
	}
	return pedirPermiso(ctx)
}

// Listar devuelve las fotos de la galería, de la más antigua a la más reciente, sin las
// que guardó Sytue ([AlbumPropio]). Sin permiso, [ErrSinPermiso].
func Listar(ctx context.Context) ([]Elemento, error) {
	if !disponible() {
		return nil, ErrNoDisponible
	}
	if !estadoPermiso().Lee() {
		return nil, ErrSinPermiso
	}
	l, err := listar(ctx)
	if err != nil {
		return nil, err
	}
	return ordenar(l), nil
}

// Abrir deja el ORIGINAL de `e` en un fichero local y dice dónde. `limpiar` borra la
// copia temporal, si la hubo (en el móvil la hay siempre: el original no es un fichero
// que se pueda abrir), y hay que llamarla también cuando hay error.
func Abrir(ctx context.Context, e Elemento) (ruta string, limpiar func(), err error) {
	if !disponible() {
		return "", func() {}, ErrNoDisponible
	}
	return abrir(ctx, e)
}

// Guardar deja `ruta` (un fichero local, que no se mueve ni se borra) en la galería del
// sistema, en [AlbumPropio], con `nombre`, y devuelve el ID del elemento nuevo. Con el
// permiso de solo añadir basta (iOS). Si este sistema no sabe guardar,
// [ErrNoDisponible], y quien llama lo deja en la carpeta de descargas.
func Guardar(ctx context.Context, ruta, nombre string) (string, error) {
	if !disponible() {
		return "", ErrNoDisponible
	}
	return guardar(ctx, ruta, limpiarNombre(nombre))
}

// Convertir deja en un fichero temporal la foto de `ruta` en JPEG, con sus metadatos
// (EXIF y GPS, orientación incluida), y dice dónde. `limpiar` la borra, y hay que
// llamarla también con error. Para lo que no se puede abrir en todas partes —HEIC, WebP,
// AVIF—; donde el sistema no sabe hacerlo (el escritorio, un Android anterior al 9 con
// HEIC), [ErrNoDisponible].
func Convertir(ctx context.Context, ruta string) (jpeg string, limpiar func(), err error) {
	if !disponible() {
		return "", func() {}, ErrNoDisponible
	}
	return convertir(ctx, ruta)
}

// RedMedida dice si el dispositivo está usando ahora una red de pago: datos móviles, o
// un punto de acceso de otro teléfono (iOS y macOS lo marcan «caro»). Donde el sistema
// no lo dice (Windows y Linux), false: ahí no hay datos móviles que cuidar.
func RedMedida() bool { return disponible() && redMedida() }

// ordenar deja la lista de la más antigua a la más reciente, con el ID para desempatar:
// la copia sube por orden y una ronda cortada a medias deja hechas las primeras.
func ordenar(l []Elemento) []Elemento {
	sort.SliceStable(l, func(i, j int) bool {
		if !l[i].Tomada.Equal(l[j].Tomada) {
			return l[i].Tomada.Before(l[j].Tomada)
		}
		return l[i].ID < l[j].ID
	})
	return l
}

// limpiarNombre se queda con la última parte de un nombre y le quita lo que ningún
// sistema de ficheros acepta. Vacío, «foto.jpg».
func limpiarNombre(nombre string) string {
	nombre = filepath.Base(strings.ReplaceAll(nombre, "\\", "/"))
	nombre = strings.Map(func(r rune) rune {
		if r < 0x20 || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, nombre)
	if nombre == "." || nombre == ".." || nombre == "/" || strings.TrimSpace(nombre) == "" {
		return "foto.jpg"
	}
	return nombre
}

// TipoDe es el MIME de una foto o un vídeo por su extensión, también los que la tabla
// del sistema no conoce en el móvil (HEIC, HEIF, AVIF, DNG). Vacío si no es ni una
// imagen ni un vídeo conocidos.
func TipoDe(nombre string) string {
	switch strings.ToLower(filepath.Ext(nombre)) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".heic":
		return "image/heic"
	case ".heif":
		return "image/heif"
	case ".avif":
		return "image/avif"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	case ".tif", ".tiff":
		return "image/tiff"
	case ".dng":
		return "image/x-adobe-dng"
	case ".mov":
		return "video/quicktime"
	case ".mp4":
		return "video/mp4"
	case ".m4v":
		return "video/x-m4v"
	case ".3gp":
		return "video/3gpp"
	case ".webm":
		return "video/webm"
	case ".mkv":
		return "video/x-matroska"
	case ".avi":
		return "video/x-msvideo"
	}
	return ""
}

// EsVideo dice si un tipo MIME es de vídeo.
func EsVideo(tipo string) bool { return strings.HasPrefix(tipo, "video/") }

// deLineas lee «id\tnombre\ttipo\tms\tbytes[\tv[\tduración\tancho\talto]]» por línea
// —la «v» marca un vídeo, y detrás pueden ir su duración en milisegundos y sus lados—:
// lo que devuelven el puente de Android (Carrete.listar) y el de PhotoKit. Una línea mal
// formada se salta; un número que falta o no se lee es cero.
func deLineas(ctx context.Context, texto string) ([]Elemento, error) { //nolint:unused // solo en los puentes de Android y PhotoKit
	var l []Elemento
	for linea := range strings.SplitSeq(texto, "\n") {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		c := strings.Split(linea, "\t")
		if len(c) < 5 || len(c) > 9 || c[0] == "" {
			continue
		}
		ms, _ := strconv.ParseInt(c[3], 10, 64)
		tam, err := strconv.ParseInt(c[4], 10, 64)
		if err != nil {
			tam = -1
		}
		tipo := c[2]
		if tipo == "" {
			tipo = TipoDe(c[1])
		}
		video := len(c) >= 6 && c[5] == "v"
		e := Elemento{ID: c[0], Nombre: c[1], Tipo: tipo, Tomada: time.UnixMilli(ms), Tamano: tam, Video: video || EsVideo(tipo)}
		if video {
			e.Duracion = time.Duration(numero(c, 6)) * time.Millisecond
			e.Ancho, e.Alto = int(numero(c, 7)), int(numero(c, 8))
		}
		l = append(l, e)
	}
	return l, nil
}

// numero es la columna i de una línea como número positivo; cero si no está o no se lee.
func numero(c []string, i int) int64 { //nolint:unused // solo en los puentes de Android y PhotoKit
	if i >= len(c) {
		return 0
	}
	n, err := strconv.ParseInt(c[i], 10, 64)
	if err != nil || n < 0 || n > 1<<40 {
		return 0
	}
	return n
}
