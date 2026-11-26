// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	photorender "git.sytue.eu/sytue/one/apps/nucleo/photos/render"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// # Lo que comparten las dos fototecas de Sytue Office
//
// Sytue Office guarda Photos en SQLite (`local/fototeca`, el plan B de §5.6 del plan de
// la edición local) y la edición conectada lo sigue guardando en el motor de registros
// (este paquete). Las REGLAS son las mismas, y viven aquí una sola vez (R-NO-COPIAR):
// cómo se mide una foto o un vídeo al entrar, cómo se ve en la galería, el cursor y el
// orden de la línea de tiempo, qué casa con una búsqueda, los años, los recuerdos, las
// repetidas, «Por aquí», las derivadas que se generan y cómo se exportan los originales.
// Son funciones puras (o que solo leen el fichero que se les da): el repositorio de este
// paquete las usa igual que antes, y la fototeca de SQLite las llama sobre sus filas.

// FotoConHuella es una foto de la galería con lo que hace falta para agrupar repetidas
// y exportar su original: su vista, cuándo se importó y la huella SHA-256 (en
// hexadecimal, minúsculas) de su original, vacía si no la tiene.
type FotoConHuella struct {
	Vista  api.Photo
	Subida time.Time
	Huella string
}

// Vista es la foto de la galería que describe una fila (sin el estado de un original
// externo: ConEstado).
func Vista(a model.AssetRow) api.Photo { return photoView(a) }

// ConEstado añade a la vista de una foto con original externo si ese fichero sigue en
// su sitio (api.Photo.EstadoExterno).
func ConEstado(v api.Photo) api.Photo { return conEstado(v) }

// CopiaDeVista es v sin compartir nada (etiquetas, lugar, fechas) con quien la tenía.
func CopiaDeVista(v api.Photo) api.Photo { return copiaDeVista(v) }

// CompararEnLinea es el orden de la línea de tiempo: de la más reciente a la más antigua
// por la fecha de la galería, y por id (también de mayor a menor) a igual fecha.
func CompararEnLinea(a, b api.Photo) int { return compararEnLinea(a, b) }

// CursorDeLinea es el cursor de la página que sigue a la foto de esa fecha e id en la
// línea de tiempo de scope.
func CursorDeLinea(scope storage.Scope, fecha time.Time, id string) string {
	raw, _ := json.Marshal(timelineCursor{Scope: scope, TakenAt: fecha, ID: id})
	return base64.RawURLEncoding.EncodeToString(raw)
}

// LeerCursorDeLinea lee un cursor de CursorDeLinea (o de CursorDesde, si desdeFecha):
// la página es lo que va estrictamente detrás de (fecha, id) en la línea de tiempo.
func LeerCursorDeLinea(scope storage.Scope, cursor string, desdeFecha bool) (fecha time.Time, id string, err error) {
	c, err := decodificarCursor(scope, cursor, desdeFecha)
	return c.TakenAt, c.ID, err
}

// CursorDesde es el cursor de TimelineDesde: el de la primera foto del periodo `desde`
// (`2019`, `2019-07` o `2019-07-14`) y hacia atrás.
func CursorDesde(scope storage.Scope, desde string) (string, error) {
	limite, err := limiteDeFecha(desde)
	if err != nil {
		return "", err
	}
	return CursorDeLinea(scope, limite, ""), nil
}

// TextoDeBusqueda es lo que se busca en una foto, plegado (sin mayúsculas ni acentos):
// su nombre, su descripción y sus etiquetas.
func TextoDeBusqueda(nombre, descripcion string, etiquetas []string) string {
	return buscar.Plegar(nombre + " " + descripcion + " " + strings.Join(etiquetas, " "))
}

// DescripcionValida dice si cabe la descripción con sus etiquetas (SetDescription).
func DescripcionValida(descripcion string, etiquetas []string) error {
	if len(descripcion) > 500 || len(etiquetas) > 12 {
		return model.ErrInvalidInput
	}
	for _, tag := range etiquetas {
		if len(tag) > 60 {
			return model.ErrInvalidInput
		}
	}
	return nil
}

// fotosDeVistas envuelve vistas (ya en el orden de la línea) para los cálculos.
func fotosDeVistas(vistas []api.Photo) []fotoLocal {
	out := make([]fotoLocal, len(vistas))
	for i, v := range vistas {
		out[i] = fotoLocal{vista: v}
	}
	return out
}

// Anyos son los años de vistas (en el orden de la línea de tiempo) con cuántas tiene
// cada uno, del más reciente al más antiguo (TimelineYears).
func Anyos(vistas []api.Photo) []api.TimelineYear { return anyosDe(fotosDeVistas(vistas)) }

// Recuerdos son los de hoy entre vistas, en el orden de la línea de tiempo (Memories).
func Recuerdos(vistas []api.Photo, hoy time.Time) []api.Memory {
	return recuerdosDe(fotosDeVistas(vistas), hoy)
}

// Repetidas son los grupos de fotos con la misma huella (Duplicates); fotos van en el
// orden de la línea de tiempo.
func Repetidas(fotos []FotoConHuella, limite int) [][]api.Photo {
	out := make([]fotoLocal, len(fotos))
	for i, f := range fotos {
		out[i] = fotoLocal{asset: model.AssetRow{UploadedAt: f.Subida}, vista: f.Vista, huella: f.Huella}
	}
	return repetidasDe(out, limite)
}

// Cerca son las de vistas (en el orden de la línea de tiempo) a menos de RadioDeCerca
// de la que tiene ese id (Nearby). ErrNotFound si no está entre ellas.
func Cerca(vistas []api.Photo, id string, limite int) ([]api.Photo, error) {
	return cercaDe(fotosDeVistas(vistas), id, limite)
}

// EsVideo dice si el tipo MIME es de vídeo.
func EsVideo(tipo string) bool { return esVideo(tipo) }

// TamanoMaximo es lo más que puede ocupar el original importado de ese tipo.
func TamanoMaximo(tipo string) int64 { return tamanoMaximo(tipo) }

// PosterNeutro es el póster de un vídeo del que no se sabe sacar ningún fotograma.
func PosterNeutro(ancho, alto int) []byte { return posterNeutro(ancho, alto) }

// HuellaDeFichero lee entero el fichero de ruta una vez: su información, su SHA-256 y
// su cabecera (para medirlo).
func HuellaDeFichero(ctx context.Context, ruta string) (os.FileInfo, string, []byte, error) {
	return huellaDeFichero(ctx, ruta)
}

// CopiarExterno copia a out el original externo de ruta comprobando su tamaño y su
// huella; si no es el mismo, ErrOriginalExterno.
func CopiarExterno(ctx context.Context, out io.Writer, ruta string, tamano int64, huella string) error {
	return copiarExterno(ctx, out, ruta, tamano, huella)
}

// LeerExterno lee entero un original externo de hasta tope bytes, comprobando que es el
// mismo.
func LeerExterno(ctx context.Context, ruta string, tamano int64, huella string, tope int64) ([]byte, error) {
	return leerExterno(ctx, ruta, tamano, huella, tope)
}

// VistaDeReceta es la receta del modelo como la ve la pantalla.
func VistaDeReceta(receta model.Receta) api.Receta { return recipeView(receta) }

// RecetaDeVista es la receta de la pantalla en el modelo.
func RecetaDeVista(receta api.Receta) model.Receta { return recipeModel(receta) }

// DerivarVariantes genera todas las derivadas (model.Variantes) de una foto con su
// orientación y su receta, del original que da cargar, y llama a emitir con cada una:
// su variante, su formato («jpg» o «png») y sus bytes. png pide la salida en PNG.
func DerivarVariantes(ctx context.Context, cargar func(context.Context) ([]byte, error), orientacion model.Orientacion, receta model.Receta, png bool, emitir func(model.Variante, string, []byte) error) error {
	sides, variants := renditionSides()
	return photorender.Derivar(ctx, cargar, orientacion, receta, png, sides, func(side int, format, _ string, encoded []byte) error {
		return emitir(variants[side], format, encoded)
	})
}

// PreviaSinEditar escribe en out la previa de la foto sin ninguna receta, calculada en
// el acto del original que da cargar (la que pide el editor para comparar cuando no hay
// una guardada). No guarda nada.
func PreviaSinEditar(ctx context.Context, out io.Writer, cargar func(context.Context) ([]byte, error), orientacion model.Orientacion, png bool) error {
	side, _ := model.VariantePrevia.LadoMayor()
	return photorender.Derivar(ctx, cargar, orientacion, model.RecetaVacia(), png, []int{side}, func(_ int, _, _ string, raw []byte) error {
		_, err := out.Write(raw)
		return err
	})
}
