// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// # El BFF de CertiPDF, en el cliente
//
// La web tiene dos puentes: `pages/api/pdf.ts` para LEER el documento y
// `pages/api/pdf-aplicar.ts` para CAMBIARLO. Los dos hacen lo mismo por debajo —traer el
// fichero de Drive, pasarlo por el motor de Docs y, al cambiar, devolver el resultado a
// Drive— y la app nativa no tiene quién lo haga por ella: lo hace este fichero, con el
// token de la persona.
//
// Lo de leer es UNA llamada genérica ([PDF.Motor]) y no una por operación: son trece
// operaciones que se distinguen por dos campos del formulario, y trece métodos que solo
// cambian una cadena serían trece sitios donde equivocarse en esa cadena. Quien lee
// —`apps/fyne/apps/certipdf`— decodifica la respuesta en sus propios tipos.
//
// Lo de cambiar sí lleva su tabla ([pdfOperaciones]), porque ahí está la decisión
// importante de `pdf-aplicar.ts`: qué se sobrescribe y qué deja SIEMPRE copia.

// PDFParte es un fichero del formulario que no es el documento: la imagen de una firma,
// el XFDF que se importa, el `.p12` con que se firma.
type PDFParte struct {
	// Campo es el nombre del campo multipart que espera el motor (`imagen`, `datos`…).
	Campo  string
	Nombre string
	Datos  []byte
}

// PDFRespuesta es lo que contesta el motor, tal cual: el cuerpo y las cabeceras, que es
// donde comprimir y reconocer dejan su resumen (`X-Sytue-Ahorro`, `X-Sytue-Ocr-Paginas`…).
type PDFRespuesta struct {
	Datos    []byte
	Cabecera http.Header
}

// Motor manda una operación al motor (`/api/v1<ruta>`, p. ej. `/pdf/form`) y devuelve
// la respuesta entera. Un rechazo llega como [*Error], con su código en el cuerpo
// ([PDFPideClave], [PDFMensaje]).
func (p *PDF) Motor(ctx context.Context, ruta string, campos map[string]string, partes []PDFParte, docs ...PDFFichero) (PDFRespuesta, error) {
	r, err := formularioPDFCon(ruta, campos, partes, docs...)
	if err != nil {
		return PDFRespuesta{}, err
	}
	resp, err := p.motor.Do(ctx, r)
	if err != nil {
		return PDFRespuesta{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	// Cuatro veces el tope de entrada: combinar veinte documentos da uno más grande que
	// cada uno, y un PNG a ×4 de una página pesa más que el PDF entero.
	datos, err := io.ReadAll(io.LimitReader(resp.Body, 4*PDFMaxBytes))
	return PDFRespuesta{Datos: datos, Cabecera: resp.Header}, err
}

// PDFMaxPNGPorLote es el tope de páginas por lote en PNG del servicio
// (`maxPaginasPNGPorLote`): una A4 a ×2 son 8 MiB de mapa de bits mientras se dibuja.
const PDFMaxPNGPorLote = 8

// RenderPNG dibuja un LOTE de páginas («1-4») RASTERIZADAS. Devuelve número de página →
// PNG; una página que el motor no supo dibujar no está en el mapa, y el lote sigue.
//
// Es la salida para quien no tiene un motor de SVG con texto detrás: el SVG de
// [PDF.Render] lleva el texto como `<text>`, y el pintor de Fyne no lo dibuja. `escala`
// 1 son 72 ppp, o sea un píxel por punto: para pintar a píxel de dispositivo se pide el
// zoom por la escala de la pantalla.
func (p *PDF) RenderPNG(ctx context.Context, doc PDFFichero, paginas string, escala float64, clave string) (map[int][]byte, error) {
	r, err := p.Motor(ctx, "/pdf/render", map[string]string{
		"paginas": paginas, "escala": strconv.FormatFloat(escala, 'f', -1, 64), "formato": "png", "clave": clave,
	}, nil, doc)
	if err != nil {
		return nil, err
	}
	var lote struct {
		Paginas map[string]string `json:"paginas"`
	}
	if err := json.Unmarshal(r.Datos, &lote); err != nil {
		return nil, fmt.Errorf("api: el lote de páginas no es JSON: %w", err)
	}
	out := make(map[int][]byte, len(lote.Paginas))
	for n, b64 := range lote.Paginas {
		i, err := strconv.Atoi(n)
		if err != nil {
			continue
		}
		png, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			continue
		}
		out[i] = png
	}
	return out, nil
}

// PorRuta resuelve la RUTA de Drive con la que se abre CertiPDF («/PDF/contrato.pdf»)
// en su fichero. Es el `resolveEntry` del cliente de Drive de la web, carpeta a carpeta:
// el servicio va por identificador y la página del PDF, por ruta.
//
// Un tramo que no existe es un 404 con la forma de los del servicio, así que quien lo
// recibe lo trata igual que un fichero borrado: la página «no encontrada».
func (p *PDF) PorRuta(ctx context.Context, ruta string) (File, error) {
	tramos := slices.DeleteFunc(strings.Split(ruta, "/"), func(s string) bool { return s == "" })
	if len(tramos) == 0 {
		return File{}, noEsta404(ruta)
	}
	raiz, err := p.drive.Root(ctx)
	if err != nil {
		return File{}, err
	}
	carpeta := raiz.ID
	for _, t := range tramos[:len(tramos)-1] {
		// Las carpetas llegan enteras en la primera página; lo paginado son los ficheros.
		hijos, err := p.drive.Children(ctx, carpeta, "")
		if err != nil {
			return File{}, err
		}
		i := slices.IndexFunc(hijos.Folders, func(f Folder) bool { return f.Name == t })
		if i < 0 {
			return File{}, noEsta404(ruta)
		}
		carpeta = hijos.Folders[i].ID
	}
	todos, err := p.ficheros(ctx, carpeta)
	if err != nil {
		return File{}, err
	}
	nombre := tramos[len(tramos)-1]
	if i := slices.IndexFunc(todos, func(f File) bool { return f.Name == nombre }); i >= 0 {
		return todos[i], nil
	}
	return File{}, noEsta404(ruta)
}

func noEsta404(ruta string) error {
	return &Error{Status: http.StatusNotFound, URL: ruta, Body: `{"code":"not_found","message":"The file does not exist."}`}
}

// pdfOperacion es una fila de `OPERACIONES` y de `CAMPO_DATOS` de `pdf-aplicar.ts`: a qué
// endpoint va, qué `op` espera el motor, en qué campo va el JSON de la operación y si
// deja SIEMPRE copia.
type pdfOperacion struct {
	ruta, motor, campoDatos string
	siempreCopia            bool
}

// pdfOperaciones es la tabla de `pdf-aplicar.ts`, fila a fila.
//
// SIEMPRE COPIA lo que no tiene vuelta atrás —censurar destruye contenido, aplanar
// quita los campos, firmar ata el fichero a un momento—: pisar el original ahí sería
// tirar algo que no se recupera. Anotar y rellenar sobrescriben, porque es el MISMO
// documento y Drive guarda la versión anterior en su historial.
var pdfOperaciones = map[string]pdfOperacion{
	"anotar":              {ruta: "/pdf/annotate", motor: "poner", campoDatos: "anotaciones"},
	"borrar-anotacion":    {ruta: "/pdf/annotate", motor: "borrar"},
	"aplanar-anotaciones": {ruta: "/pdf/annotate", motor: "aplanar", siempreCopia: true},
	// Responder NO deja copia: un hilo de revisión se sigue sobre el mismo documento.
	"responder":          {ruta: "/pdf/annotate", motor: "responder"},
	"marcar":             {ruta: "/pdf/annotate", motor: "marcar"},
	"crear-campos":       {ruta: "/pdf/form", motor: "crear", campoDatos: "campos", siempreCopia: true},
	"importar-datos":     {ruta: "/pdf/form", motor: "importar-datos", siempreCopia: true},
	"rellenar":           {ruta: "/pdf/form", motor: "rellenar", campoDatos: "valores"},
	"aplanar-formulario": {ruta: "/pdf/form", motor: "aplanar", siempreCopia: true},
	"proteger":           {ruta: "/pdf/protect", motor: "proteger", siempreCopia: true},
	"desproteger":        {ruta: "/pdf/protect", motor: "desproteger", siempreCopia: true},
	"sanear":             {ruta: "/pdf/protect", motor: "sanear", siempreCopia: true},
	"metadatos":          {ruta: "/pdf/protect", motor: "metadatos", campoDatos: "metadatos"},
	"redactar":           {ruta: "/pdf/protect", motor: "redactar", campoDatos: "areas", siempreCopia: true},
	"firmar":             {ruta: "/pdf/sign", motor: "dibujada", campoDatos: "firma", siempreCopia: true},
	"firmar-certificado": {ruta: "/pdf/sign", motor: "certificado", campoDatos: "firma", siempreCopia: true},
	"paginas":            {ruta: "/pdf/pages", siempreCopia: true},
	"comprimir":          {ruta: "/pdf/tools", motor: "comprimir", siempreCopia: true},
	"recortar":           {ruta: "/pdf/tools", motor: "recortar", siempreCopia: true},
	"archivo":            {ruta: "/pdf/tools", motor: "archivo", siempreCopia: true},
	"editar-texto":       {ruta: "/pdf/edit", motor: "texto", campoDatos: "cambios", siempreCopia: true},
	"reemplazar-texto":   {ruta: "/pdf/edit", motor: "buscar-reemplazar", siempreCopia: true},
	"editar-imagen":      {ruta: "/pdf/edit", motor: "imagen", siempreCopia: true},
	"capas":              {ruta: "/pdf/edit", motor: "capas"},
	"indice":             {ruta: "/pdf/edit", motor: "indice", campoDatos: "marcadores", siempreCopia: true},
	"capa-de-texto":      {ruta: "/pdf/ocr", motor: "capa", campoDatos: "textos", siempreCopia: true},
	// Reconocer SIEMPRE deja copia: el original escaneado es la prueba.
	"reconocer-texto": {ruta: "/pdf/ocr", motor: "reconocer", siempreCopia: true},
}

// PDFSiempreCopia dice si esa operación deja SIEMPRE un fichero nuevo, pida quien pida
// lo que pida. La interfaz lo avisa ANTES; el cliente no se fía de que lo haya hecho.
func PDFSiempreCopia(op string) bool { return pdfOperaciones[op].siempreCopia }

// PDFCambio es lo que se le pide a [PDF.Aplicar]: el cuerpo de `/api/pdf-aplicar`.
type PDFCambio struct {
	// Op es la operación, con el nombre de la web (`anotar`, `redactar`…).
	Op string
	// Datos es el JSON de la operación; va en el campo que fija la tabla, no en uno que
	// elija quien llama. Nil no manda nada.
	Datos any
	// Campos son los sueltos del formulario (páginas, contraseñas, permisos).
	Campos map[string]string
	// Partes son los ficheros sueltos: la imagen de la firma, el XFDF, el `.p12`.
	Partes []PDFParte
	// Sufijo es la palabra de la copia —«contrato (censurado).pdf»—, ya traducida. Vacía
	// usa el nombre de la operación, como la web.
	Sufijo string
	// Copia pide copia aunque la operación sobrescriba (`destino: 'copia'`).
	Copia bool
	// Extras son los documentos que van DETRÁS del actual al combinar, en su orden.
	Extras []PDFFichero
	// Rutas son los ficheros de Drive que se combinan detrás, en su orden (`rutas`):
	// Aplicar los descarga, convierte las imágenes en página y los pone tras Extras.
	Rutas []string
}

// PDFAplicado es lo que queda tras aplicar: el fichero de Drive donde quedó el resultado
// —el mismo, sobrescrito, o la copia—, sus bytes y las cabeceras del motor.
type PDFAplicado struct {
	Fichero  File
	Copia    bool
	Datos    []byte
	Cabecera http.Header
}

// Aplicar cambia el documento con el motor y deja el resultado en Drive: sobre el
// original o en una copia a su lado, según la tabla. Es `pdf-aplicar.ts` entero.
func (p *PDF) Aplicar(ctx context.Context, original File, doc PDFFichero, clave string, c PDFCambio) (PDFAplicado, error) {
	op, ok := pdfOperaciones[c.Op]
	if !ok {
		return PDFAplicado{}, fmt.Errorf("api: operación de PDF desconocida %q", c.Op)
	}
	campos := maps.Clone(c.Campos)
	if campos == nil {
		campos = map[string]string{}
	}
	if op.motor != "" {
		campos["op"] = op.motor
	}
	campos["clave"] = clave
	if op.campoDatos != "" && c.Datos != nil {
		j, err := json.Marshal(c.Datos)
		if err != nil {
			return PDFAplicado{}, err
		}
		campos[op.campoDatos] = string(j)
	}
	extras, err := p.extrasDeRutas(ctx, original, len(doc.Datos), c.Rutas)
	if err != nil {
		return PDFAplicado{}, err
	}
	docs := append(append([]PDFFichero{doc}, c.Extras...), extras...)
	r, err := p.Motor(ctx, op.ruta, campos, c.Partes, docs...)
	if err != nil {
		return PDFAplicado{}, err
	}
	out := PDFAplicado{Copia: op.siempreCopia || c.Copia, Datos: r.Datos, Cabecera: r.Cabecera}
	if out.Copia {
		sufijo := c.Sufijo
		if sufijo == "" {
			sufijo = c.Op
		}
		out.Fichero, err = p.GuardarCopia(ctx, original, sufijo, r.Datos)
		return out, err
	}
	// Subir con el MISMO nombre a la misma carpeta sustituye el fichero, y Drive guarda
	// la versión anterior en su historial.
	out.Fichero, err = p.drive.Upload(ctx, original.FolderID, original.Name, "application/pdf",
		bytes.NewReader(r.Datos), int64(len(r.Datos)))
	return out, err
}

// PDFMaxCombinar es `MAX_COMBINAR`: cada fichero es una descarga entera en memoria, y sin
// tope una combinación podría pedir el Drive completo.
const PDFMaxCombinar = 20

var (
	// ErrPDFDemasiados: una combinación de más de PDFMaxCombinar ficheros.
	ErrPDFDemasiados = errors.New("api: demasiados ficheros para combinar")
	// ErrPDFDescarga: uno de los ficheros de una combinación no se ha podido traer. El
	// error dice CUÁL: con cinco, «no se pudo» a secas obliga a probarlos de uno en uno.
	ErrPDFDescarga = errors.New("api: no se ha podido traer el fichero")
	// ErrPDFImagen: una imagen de la combinación no se ha podido convertir en página.
	ErrPDFImagen = errors.New("api: no se ha podido convertir la imagen")
)

// imagenCombinable es `IMAGENES`: lo que el motor sabe convertir en página.
var imagenCombinable = regexp.MustCompile(`(?i)\.(jpe?g|png|gif|webp|bmp|tiff?)$`)

// extrasDeRutas es el bucle de la combinación de `pdf-aplicar.ts`: cada ruta, en orden,
// descargada de Drive; una imagen, convertida antes en PDF (`op=imagenes-a-pdf`, una
// llamada por imagen para no sacarla de su sitio). El documento actual no se repite, y
// el tamaño se suma sobre la marcha, porque el tope del motor es del cuerpo ENTERO.
func (p *PDF) extrasDeRutas(ctx context.Context, original File, total int, rutas []string) ([]PDFFichero, error) {
	var limpias []string
	for _, r := range rutas {
		if r = strings.TrimSpace(r); r != "" {
			limpias = append(limpias, r)
		}
	}
	if len(limpias) > PDFMaxCombinar {
		return nil, ErrPDFDemasiados
	}
	extras := make([]PDFFichero, 0, len(limpias))
	for _, ruta := range limpias {
		f, err := p.PorRuta(ctx, ruta)
		if err != nil {
			return nil, fmt.Errorf("%w: %s: %w", ErrPDFDescarga, ruta, err)
		}
		if f.ID == original.ID {
			continue
		}
		datos, err := p.Descargar(ctx, f.ID)
		if err != nil {
			return nil, fmt.Errorf("%w: %s: %w", ErrPDFDescarga, ruta, err)
		}
		if total += len(datos); total > PDFMaxBytes {
			return nil, fmt.Errorf("%w: %s", ErrPDFGrande, ruta)
		}
		if imagenCombinable.MatchString(f.Name) {
			r, err := p.Motor(ctx, "/pdf/tools", map[string]string{"op": "imagenes-a-pdf"}, nil, PDFFichero{Nombre: f.Name, Datos: datos})
			if err != nil {
				return nil, fmt.Errorf("%w: %s: %w", ErrPDFImagen, ruta, err)
			}
			extras = append(extras, PDFFichero{Nombre: f.Name + ".pdf", Datos: r.Datos})
			continue
		}
		extras = append(extras, PDFFichero{Nombre: f.Name, Datos: datos})
	}
	return extras, nil
}

// PDFMaxHojasEscaneo es el tope de fotos de un escaneo (`MAX_HOJAS` de `pdf-escanear.ts`).
const PDFMaxHojasEscaneo = 50

// Escanear convierte unas fotos en UN PDF de varias páginas con el motor
// (`op=imagenes-a-pdf`) y lo guarda en la carpeta «PDF» de Drive: es
// `pdf-escanear.ts`. Las fotos van en UNA petición porque un escaneo de diez hojas es UN
// documento; subirlas de una en una dejaría diez PDF sueltos.
func (p *PDF) Escanear(ctx context.Context, fotos []PDFFichero, nombre string) (File, error) {
	if len(fotos) == 0 || len(fotos) > PDFMaxHojasEscaneo {
		return File{}, fmt.Errorf("api: un escaneo lleva de 1 a %d fotos", PDFMaxHojasEscaneo)
	}
	hojas := make([]PDFFichero, len(fotos))
	for i, f := range fotos {
		hojas[i] = PDFFichero{Nombre: "hoja-" + strconv.Itoa(i+1), Datos: f.Datos}
	}
	r, err := p.Motor(ctx, "/pdf/tools", map[string]string{"op": "imagenes-a-pdf"}, nil, hojas...)
	if err != nil {
		return File{}, err
	}
	return p.Subir(ctx, nombre, bytes.NewReader(r.Datos), int64(len(r.Datos)))
}
