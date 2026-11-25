// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pdf

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// LA SESIÓN DEL DISPOSITIVO (ADR 0086, «Firmar en el dispositivo», tercera y cuarta
// entregas).
//
// Es lo que `cmd/pdf-wasm` expone al estudio de CertiPDF para los PDF de la bóveda:
// los documentos que el host ya descifró, abiertos UNA vez y referidos por un
// identificador opaco, para que dibujar veinte lotes de páginas no vuelva a leer el
// fichero veinte veces. Cada operación es la MISMA del servicio de Docs
// (`RenderView`, `Summary`, `ApplyAnnotationOperation`, `ApplyFormOperation`,
// `ApplyPageOperation`, `ApplyLayerOperation`, `ApplyToolOperation`,
// `ApplyProtectionOperation`, `ApplyEditOperation`, `ApplyCompareOperation`,
// `TextSpans`, `VerificarFirmas`, `ExtraerAdjunto`): ni un segundo motor ni atajos.
// No hay red ni almacenamiento: devuelve bytes, y cifrarlos y guardarlos es cosa del
// host.
//
// Vive aquí, sin la etiqueta js/wasm, para probarla con `go test` como el resto del
// motor; el wasm solo traduce valores de JavaScript.

// Límites de la sesión: cuántos documentos a la vez (el abierto y los que se combinan
// con él) y cuántos bytes en total. Cada uno, además, con el MaxPDFBytes de siempre.
const (
	MaxDeviceDocuments = 21
	MaxDeviceBytes     = 2 * MaxPDFBytes
	// deviceSummaryBudget es el presupuesto de extractos de `Summary`, como el
	// `presupuestoExtractos` del servicio: la primera página siempre se lee.
	deviceSummaryBudget = 2 * time.Second
)

// ErrDeviceDocument se devuelve con un identificador que no está abierto.
var ErrDeviceDocument = errors.New("pdf: the document is not open on this device")

// DeviceFailure es el error que el estudio sabe enseñar: el código estable del motor
// (`pdf_password_required`, `pdf_invalidates_signatures`…) y su mensaje.
type DeviceFailure struct {
	Code    string `json:"codigo"`
	Message string `json:"mensaje"`
}

// DescribeDeviceError da el código de un error de lectura o de operación.
func DescribeDeviceError(err error) DeviceFailure {
	switch {
	case errors.Is(err, ErrDeviceDocument):
		return DeviceFailure{Code: "pdf_not_open", Message: err.Error()}
	case errors.Is(err, ErrPDFClaveIncorrecta), errors.Is(err, ErrPDFCifrado), errors.Is(err, ErrPDFNoEsPDF), errors.Is(err, ErrPDFNoSoportado):
		return DeviceFailure(DescribeReadError(err, "PDF"))
	}
	failure := DescribeOperationError(err)
	return DeviceFailure{Code: failure.Code, Message: failure.Message}
}

// DeviceEdit es el resultado de una operación que cambia el documento (o que lee su
// formulario): los bytes, su tipo y lo que el estudio avisa después.
type DeviceEdit struct {
	Data        []byte
	ContentType string
	// Archival es «perdido» o «rebajado» si la edición tocó la marca PDF/A.
	Archival       string
	ImportedFields int
	Compression    *ResultadoCompresion
	// UnchangedXMP avisa de que el documento lleva XMP y el cambio de propiedades
	// puede no verse en otro lector (el `X-Sytue-Metadatos-Xmp` del servicio).
	UnchangedXMP bool
	// Replacements es cuántas coincidencias se sustituyeron al buscar y reemplazar.
	Replacements *int
}

type deviceDocument struct {
	doc      *PDFDoc
	data     []byte
	password string
}

// DeviceSession guarda los documentos abiertos en este dispositivo. Las operaciones
// se serializan: un PDFDoc cachea objetos al leerlos y no es seguro a la vez.
type DeviceSession struct {
	mu    sync.Mutex
	next  uint64
	bytes int
	docs  map[string]*deviceDocument
}

// NewDeviceSession crea una sesión vacía.
func NewDeviceSession() *DeviceSession {
	return &DeviceSession{docs: map[string]*deviceDocument{}}
}

// Open lee un PDF (la sesión se queda con `data`: no se debe tocar después) y
// devuelve su identificador y el resumen de `Summary` en JSON.
func (s *DeviceSession) Open(data []byte, password string) (string, map[string]any, error) {
	if len(data) == 0 {
		return "", nil, ErrPDFNoEsPDF
	}
	if len(data) > MaxPDFBytes {
		return "", nil, &OperationFailure{Status: 413, Code: "payload_too_large", Message: "The file is larger than the maximum allowed size."}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.docs) >= MaxDeviceDocuments || s.bytes+len(data) > MaxDeviceBytes {
		return "", nil, &OperationFailure{Status: 413, Code: "pdf_device_full", Message: "Too many documents are open on this device."}
	}
	doc, err := LeerPDFConClave(data, password)
	if err != nil {
		return "", nil, err
	}
	s.next++
	id := "d" + strconv.FormatUint(s.next, 10)
	s.docs[id] = &deviceDocument{doc: doc, data: data, password: password}
	s.bytes += len(data)
	return id, Summary(doc, deviceSummaryBudget), nil
}

// Close olvida un documento. Sus bytes los libera el recolector: el host ya borró
// los suyos, y el wasm entero muere con el worker al salir del estudio.
func (s *DeviceSession) Close(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.docs[id]
	if !ok {
		return false
	}
	delete(s.docs, id)
	s.bytes -= len(entry.data)
	return true
}

func (s *DeviceSession) lookup(id string) (*PDFDoc, error) {
	entry, ok := s.docs[id]
	if !ok {
		return nil, ErrDeviceDocument
	}
	return entry.doc, nil
}

// target es el documento sobre el que se edita. Con `invalidar_firmas` (lo que el
// estudio manda solo después de que la persona lo confirme, como el servicio) se lee
// OTRA vez de sus bytes y solo esa copia queda autorizada: `PermitirInvalidarFirmas`
// no tiene vuelta atrás, y el documento abierto no puede quedarse con el permiso.
func (s *DeviceSession) target(id string, fields map[string]string) (*PDFDoc, error) {
	entry, ok := s.docs[id]
	if !ok {
		return nil, ErrDeviceDocument
	}
	if v := fields["invalidar_firmas"]; v != "1" && v != "true" {
		return entry.doc, nil
	}
	doc, err := LeerPDFConClave(entry.data, entry.password)
	if err != nil {
		return nil, err
	}
	doc.PermitirInvalidarFirmas()
	return doc, nil
}

// Render dibuja páginas con `RenderView` (los mismos campos que `/pdf/render`:
// `paginas`, `pagina`, `escala`, `formato`, `sin_anotaciones`).
func (s *DeviceSession) Render(ctx context.Context, id string, fields map[string]string) (ViewResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.lookup(id)
	if err != nil {
		return ViewResult{}, err
	}
	return RenderView(ctx, doc, onlyFields(fields, "paginas", "pagina", "escala", "formato", "sin_anotaciones"))
}

// Annotate aplica `ApplyAnnotationOperation`. `author` lo pone el host (la persona
// con la sesión abierta), nunca un campo del formulario.
func (s *DeviceSession) Annotate(id string, fields map[string]string, author string) (DeviceEdit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.target(id, fields)
	if err != nil {
		return DeviceEdit{}, err
	}
	out, err := ApplyAnnotationOperation(doc, AnnotationOperationInput{
		Fields: onlyFields(fields, "op", "anotaciones", "indices", "indice", "texto", "estado"),
		Author: author,
	})
	if err != nil {
		return DeviceEdit{}, err
	}
	return DeviceEdit{Data: out, ContentType: "application/pdf", Archival: ArchivalChange(doc, out)}, nil
}

// Form aplica `ApplyFormOperation`; `data` es el XFDF de `importar-datos`, si lo hay.
func (s *DeviceSession) Form(id string, fields map[string]string, filename string, data []byte) (DeviceEdit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.target(id, fields)
	if err != nil {
		return DeviceEdit{}, err
	}
	result, err := ApplyFormOperation(doc, FormOperationInput{
		Fields:       onlyFields(fields, "op", "pagina", "campos", "valores"),
		Filename:     filename,
		Data:         data,
		DataProvided: data != nil,
	})
	if err != nil {
		return DeviceEdit{}, err
	}
	edit := DeviceEdit{Data: result.Data, ContentType: result.ContentType, ImportedFields: result.ImportedFields}
	if result.ContentType == "application/pdf" {
		edit.Archival = ArchivalChange(doc, result.Data)
	}
	return edit, nil
}

// Pages aplica `ApplyPageOperation` (`combinar`, `extraer` —que, con otro orden, es
// reordenar—, `borrar`, `rotar`) con los campos `op`, `paginas`, `grados` e
// `invalidar_firmas`. Combinar recibe los documentos en su orden. Las capas de
// `/pdf/pages` (`marca-agua`, `fondo`, `numerar`, `bates`, `encabezado`, `pie`, con
// `texto`, `posicion`, `desde` y `digitos`) van por `ApplyLayerOperation`, sobre un
// único documento: el diálogo «Sellos y marcas» del estudio.
func (s *DeviceSession) Pages(ids []string, fields map[string]string) (DeviceEdit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(ids) == 0 || len(ids) > MaxDeviceDocuments {
		return DeviceEdit{}, inputError(fmt.Sprintf("Give between 1 and %d documents.", MaxDeviceDocuments))
	}
	docs := make([]*PDFDoc, 0, len(ids))
	for _, id := range ids {
		doc, err := s.target(id, fields)
		if err != nil {
			return DeviceEdit{}, err
		}
		docs = append(docs, doc)
	}
	op := strings.TrimSpace(fields["op"])
	if IsLayerOperation(op) {
		if len(docs) != 1 {
			return DeviceEdit{}, inputError(fmt.Sprintf("“%s” works on a single file.", op))
		}
		out, err := ApplyLayerOperation(op, docs[0], onlyFields(fields, "texto", "posicion", "desde", "digitos"))
		if err != nil {
			return DeviceEdit{}, err
		}
		return DeviceEdit{Data: out, ContentType: "application/pdf", Archival: ArchivalChange(docs[0], out)}, nil
	}
	if !slices.Contains([]string{"combinar", "extraer", "borrar", "rotar"}, op) {
		return DeviceEdit{}, inputError(fmt.Sprintf("Unknown op: %q.", op))
	}
	out, err := ApplyPageOperation(op, fields["paginas"], fields["grados"], docs...)
	if err != nil {
		return DeviceEdit{}, err
	}
	return DeviceEdit{Data: out, ContentType: "application/pdf", Archival: ArchivalChange(docs[0], out)}, nil
}

// Protect aplica `ApplyProtectionOperation`: `proteger` (AES-256, revisión 6),
// `desproteger`, `metadatos`, `sanear` y `redactar`. Las contraseñas y las zonas
// llegan del host y no salen de aquí.
func (s *DeviceSession) Protect(id string, fields map[string]string) (DeviceEdit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.target(id, fields)
	if err != nil {
		return DeviceEdit{}, err
	}
	result, err := ApplyProtectionOperation(doc, onlyFields(fields, "op", "usuario", "dueno", "permisos", "metadatos", "areas"))
	if err != nil {
		clear(result.Data)
		return DeviceEdit{}, err
	}
	return DeviceEdit{
		Data: result.Data, ContentType: "application/pdf",
		Archival: ArchivalChange(doc, result.Data), UnchangedXMP: result.UnchangedXMP,
	}, nil
}

// Edit aplica `ApplyEditOperation`: `analizar` y buscar sin aplicar devuelven JSON;
// `texto`, `buscar-reemplazar` con `aplicar=1`, `imagen` (con `image`, o sin ella
// para quitarla), `capas` e `indice` devuelven el PDF.
func (s *DeviceSession) Edit(id string, fields map[string]string, image []byte) (DeviceEdit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.target(id, fields)
	if err != nil {
		return DeviceEdit{}, err
	}
	result, err := ApplyEditOperation(doc, EditOperationInput{
		Fields: onlyFields(fields, "op", "pagina", "cambios", "busqueda", "aplicar", "elegidas", "id", "visibles", "marcadores"),
		Image:  image,
	})
	if err != nil {
		clear(result.Data)
		return DeviceEdit{}, err
	}
	edit := DeviceEdit{Data: result.Data, ContentType: result.ContentType, Replacements: result.Replacements}
	if result.ContentType == "application/pdf" {
		edit.Archival = ArchivalChange(doc, result.Data)
	}
	return edit, nil
}

// Compare compara dos documentos abiertos con `ApplyCompareOperation` (`modo`,
// `pagina`, `escala`, `tolerancia`, `vecindad`) y devuelve su JSON.
func (s *DeviceSession) Compare(ctx context.Context, a, b string, fields map[string]string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	first, err := s.lookup(a)
	if err != nil {
		return nil, err
	}
	second, err := s.lookup(b)
	if err != nil {
		return nil, err
	}
	return ApplyCompareOperation(ctx, first, second, onlyFields(fields, "modo", "pagina", "escala", "tolerancia", "vecindad"))
}

// Text devuelve el texto de cada página con su posición (`/pdf/text`): lo que leen
// buscar y leer en voz alta.
func (s *DeviceSession) Text(ctx context.Context, id string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.lookup(id)
	if err != nil {
		return nil, err
	}
	pages, err := TextSpans(ctx, doc)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"paginas": pages})
}

// Verify es el informe de cada firma (`/pdf/verify`) SIN anclas de confianza: el
// dispositivo no tiene en quién confiar, y el panel lo dice (`anclas: false`).
// Integridad, cambios posteriores y sellos sí se comprueban.
func (s *DeviceSession) Verify(id string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.lookup(id)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"firmas": doc.VerificarFirmas(OpcionesVerificacion{}), "anclas": false})
}

// Attachment saca un fichero incrustado por su nombre (`/pdf/attachment`).
func (s *DeviceSession) Attachment(id, name string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.lookup(id)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(name) == "" {
		return nil, inputError("Say which attachment to extract.")
	}
	out, err := doc.ExtraerAdjunto(name)
	if err != nil {
		return nil, &OperationFailure{Status: 404, Code: "not_found", Message: "This document has no such attachment."}
	}
	return out, nil
}

// Tool aplica `ApplyToolOperation` (`comprimir`, `recortar`, `archivo`,
// `comprobar-archivo`).
func (s *DeviceSession) Tool(id string, fields map[string]string) (DeviceEdit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.target(id, fields)
	if err != nil {
		return DeviceEdit{}, err
	}
	result, err := ApplyToolOperation(doc, onlyFields(fields, "op", "nivel", "margenes", "idioma", "titulo", "autor"))
	if err != nil {
		return DeviceEdit{}, err
	}
	edit := DeviceEdit{Data: result.Data, ContentType: result.ContentType, Compression: result.Compression}
	if result.ContentType == "application/pdf" {
		edit.Archival = ArchivalChange(doc, result.Data)
	}
	return edit, nil
}

// onlyFields deja pasar solo los campos que la operación conoce, como el servicio
// (que lee `r.FormValue` de una lista fija).
func onlyFields(fields map[string]string, keys ...string) map[string]string {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := fields[key]; ok {
			out[key] = value
		}
	}
	return out
}
