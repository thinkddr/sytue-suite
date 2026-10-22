// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package domain define el modelo y las reglas de Sytue Photos: el activo (una foto
// o un vídeo), su orientación EXIF y el direccionamiento de las derivadas.
// Independiente de transporte (HTTP), de almacenamiento y de Drive.
//
// DÓNDE VIVE CADA COSA, que es la decisión que ordena todo lo demás:
//
//   - El ORIGINAL vive en Sytue Drive. Sytue Photos no lo copia ni lo custodia: ya
//     hay subida, cuotas, papelera, compartición y copia de seguridad allí, y
//     duplicarlas sería mantener dos veces lo mismo. Aquí solo se guarda con qué
//     identificarlo.
//   - Las DERIVADAS —miniaturas, previsualizaciones, exportaciones— viven en el
//     almacén propio de Sytue Photos. Son REGENERABLES: si se pierden, se vuelven a
//     calcular del original y la receta. Por eso pueden caducar, comprimirse o
//     borrarse sin pedir permiso a nadie.
//
// Clean-room: modelo propio. Las ideas funcionales —línea de tiempo por fecha de
// captura, álbumes, edición no destructiva por receta— no tienen copyright; no se
// ha consultado ni portado código de terceros (CONVENTIONS §12).
package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// Errores de dominio. La capa HTTP los traduce a *httpx.Error.
var (
	ErrNotFound     = errors.New("syphotos: recurso no encontrado")
	ErrInvalidInput = errors.New("syphotos: entrada inválida")
	ErrForbidden    = errors.New("syphotos: no autorizado para esta acción")
)

// Orientacion es el valor EXIF (1..8) que dice cómo hay que girar y voltear los
// píxeles guardados para verlos como se tomó la foto.
//
// NO ES UN DETALLE MENOR. La cámara casi nunca rota los píxeles: escribe la
// etiqueta y deja el sensor como está, así que una foto vertical llega como un
// bloque horizontal con un «6» al lado. Aplicarlo en un sitio y olvidarlo en
// otro es el fallo clásico de toda galería: la miniatura sale tumbada y el
// visor derecha, o al revés. Aquí se traduce UNA vez, a la transformación que
// significa, y todo lo demás consume eso.
type Orientacion uint8

// Transformacion es lo que hay que hacerle a los píxeles guardados.
type Transformacion struct {
	// Grados en sentido horario: 0, 90, 180 o 270.
	Grados int
	// EspejoH invierte de izquierda a derecha, ANTES de girar.
	EspejoH bool
}

// IntercambiaLados dice si la transformación cambia ancho por alto. Es lo que
// hay que preguntar antes de reservar el lienzo o calcular una proporción: con
// 90 o 270 grados, una foto de 4000×3000 se ve 3000×4000.
func (t Transformacion) IntercambiaLados() bool { return t.Grados == 90 || t.Grados == 270 }

// transformaciones es la tabla del estándar EXIF (TIFF 6.0, etiqueta 0x0112).
// Se escribe entera y no se calcula: son ocho casos fijos desde 1992 y una
// fórmula «ingeniosa» aquí solo consigue que nadie pueda comprobarla de un
// vistazo contra la especificación.
var transformaciones = map[Orientacion]Transformacion{
	1: {Grados: 0},                  // como está
	2: {Grados: 0, EspejoH: true},   // espejo horizontal
	3: {Grados: 180},                // media vuelta
	4: {Grados: 180, EspejoH: true}, // espejo vertical
	// 5 y 7 son las TRANSPUESTAS, y es donde se equivoca todo el mundo. El
	// estándar las define por dónde acaba la esquina (0,0): la 5 es «left-top»
	// —transponer por la diagonal principal— y la 7 «right-bottom», por la
	// contraria. Con el espejo aplicado ANTES, eso es 270 y 90 respectivamente,
	// no al revés. Aquí estuvieron cambiadas hasta que un test sobre PÍXELES lo
	// enseñó: el test de esta tabla repetía la misma suposición que la tabla.
	5: {Grados: 270, EspejoH: true}, // transpuesta (left-top)
	6: {Grados: 90},                 // un cuarto horario
	7: {Grados: 90, EspejoH: true},  // transversa (right-bottom)
	8: {Grados: 270},                // tres cuartos horarios
}

// Transformacion traduce la etiqueta EXIF.
//
// ANTE LA DUDA, NO TOCAR. Un valor fuera de 1..8 —o ausente, que llega como 0—
// se trata como «sin rotación» en vez de rechazarse: hay cámaras y programas de
// edición que escriben basura ahí, y perder la foto entera por una etiqueta mal
// puesta es peor que enseñarla como está guardada.
func (o Orientacion) Transformacion() Transformacion {
	if t, ok := transformaciones[o]; ok {
		return t
	}
	return Transformacion{}
}

// OrientacionDe traduce un valor leído de FUERA —el EXIF de la foto o la
// columna de la base de datos— a una orientación.
//
// EXISTE POR EL TRUNCADO, que es un fallo silencioso y no teórico. La etiqueta
// llega como `uint16` desde el EXIF y como `int16` desde Postgres, y esto es un
// `uint8`: convertir a pelo un 260 da 4, que ES una de las ocho válidas y
// rotaría la foto noventa grados sin que nadie hubiera pedido nada. Un valor
// corrupto tiene que quedarse en «no sé», no convertirse en una instrucción
// creíble.
//
// Fuera de rango devuelve 0, que no está en la tabla: no rota, y `Valida()` lo
// delata para poder avisar al ingerir.
func OrientacionDe(v int) Orientacion {
	if v < 1 || v > 8 {
		return 0
	}
	return Orientacion(v)
}

// Valida dice si la etiqueta es una de las ocho del estándar. Sirve para AVISAR
// al ingerir —«esta foto trae orientación 9»— sin que eso impida guardarla.
func (o Orientacion) Valida() bool {
	_, ok := transformaciones[o]
	return ok
}

// Activo es una foto o un vídeo indexado por Sytue Photos.
//
// El original NO está aquí: `DriveID` es la referencia, y los bytes se piden a
// Drive cuando hacen falta. Lo que sí vive aquí es todo lo que hace falta para
// PINTAR LA LÍNEA DE TIEMPO sin abrir un solo fichero: fecha, tamaño y
// proporción. Una galería que tiene que leer el original para saber dónde
// colocar un hueco no puede desplazarse con fluidez.
type Activo struct {
	ID      uuid.UUID
	Tenant  string // slug de la organización
	Duenyo  string // `sub` de OIDC
	DriveID string // el fichero en Sytue Drive
	Nombre  string
	Tipo    string // media type del original
	Bytes   int64

	// Dimensiones DEL SENSOR, tal como están guardadas: sin aplicar la
	// orientación. Se guardan crudas a propósito —es lo que dice el fichero— y
	// quien pinte usa `Vistas()`.
	Ancho, Alto int
	Orientacion Orientacion

	// TomadaEn es la fecha de captura del EXIF. Nula cuando el fichero no la
	// trae: entonces la línea de tiempo cae a `SubidaEn`, y eso se DICE en la
	// interfaz en vez de inventar una fecha que parezca de captura.
	TomadaEn *time.Time
	SubidaEn time.Time

	// Lat y Lon son dónde se hizo, en grados decimales, sacados del GPS del
	// EXIF. Las dos o ninguna: media coordenada no sitúa nada. Nulas es lo
	// corriente —muchas cámaras no llevan GPS y los móviles lo quitan al
	// compartir—, así que la interfaz no puede darlas por hechas.
	Lat *float64
	Lon *float64

	// Descripcion y Etiquetas son QUÉ SALE en la foto, escrito por Sytue AI a
	// petición de su dueña. La descripción es además el texto alternativo: el
	// nombre del fichero no le dice nada a un lector de pantalla.
	Descripcion string
	Etiquetas   []string

	// Favorita: marcada a mano por su dueña. Es un atributo de la foto y no una
	// relación aparte porque una foto tiene un solo dueño.
	Favorita bool

	// Cámara y objetivo, para filtrar. Texto libre porque el EXIF lo es: hay
	// fabricantes que escriben «NIKON CORPORATION» y otros «Nikon», y
	// normalizarlo aquí perdería lo que puso el fichero.
	Camara   string
	Objetivo string
}

// Vistas devuelve las dimensiones ya orientadas, que son las que ve quien mira.
func (a Activo) Vistas() (ancho, alto int) {
	if a.Orientacion.Transformacion().IntercambiaLados() {
		return a.Alto, a.Ancho
	}
	return a.Ancho, a.Alto
}

// FechaDeGaleria es la que ordena la línea de tiempo: la de captura si la hay,
// y si no la de subida. `deCaptura` distingue las dos, porque la interfaz tiene
// que poder decir de cuál está hablando.
func (a Activo) FechaDeGaleria() (cuando time.Time, deCaptura bool) {
	if a.TomadaEn != nil && !a.TomadaEn.IsZero() {
		return *a.TomadaEn, true
	}
	return a.SubidaEn, false
}
