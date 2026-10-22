// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"errors"
	"net/http"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
	"git.sytue.eu/sytue/one/services/docs/internal/drive"
)

// EL HOGAR DE UN DOCUMENTO ESTÁ EN LA UNIDAD DE SU DUEÑO.
//
// Un documento nacido en Sytue Docs vivía SOLO en Docs: fila en `documents`, log
// CRDT y snapshots en su propio bucket. No aparecía en Sytue Drive, no se
// descargaba desde donde la gente busca sus cosas y no gastaba ni un byte del
// espacio que su unidad decía tener libre. Al mismo tiempo, la migración 0006 ya
// había construido la mitad del puente en el sentido contrario —`source_ref`, el
// ancla «drive:<uuid>» de un documento a un fichero— y la interfaz ya tenía el
// botón «Guardar en Sytue Drive», visible SOLO para los documentos que tuvieran
// ancla. O sea: existía el camino de vuelta, y casi ningún documento podía tomarlo.
//
// Lo que se hace aquí es la otra mitad: al crear un documento se crea TAMBIÉN su
// fichero en `Sytue Docs/` de la unidad, y el documento nace anclado a él. Con eso
// el documento existe en la unidad desde el primer segundo, y el botón de guardar
// —que ya estaba escrito— pasa a estar disponible para todos.
//
// SE ESCRIBE EL DOCUMENTO VACÍO DE VERDAD, no un fichero de cero bytes. Un .odt de
// cero bytes es un fichero roto: quien lo descargue antes de haber escrito nada se
// encontraría con que su documento «no se puede abrir». Cuesta unos kilobytes y se
// ahorra ese susto.
//
// QUÉ *NO* HACE, dicho aquí para que no se busque: el contenido del fichero NO se
// mantiene sincronizado solo. Se escribe al crear y se reescribe cuando la persona
// guarda en Drive (la acción del editor, que ahora ve todos los documentos). La
// fuente de verdad de la edición colaborativa sigue siendo el CRDT: sincronizar en
// cada pulsación exigiría serializar el documento entero por operación, y el
// fichero no es un formato en el que se pueda fusionar nada. Ver docs/architecture.md#docs-en-drive.

// formatoNativo es el formato con el que un documento aterriza en la unidad.
//
// ODF Y NO OOXML: es el estándar abierto (OASIS ODF 1.3), es lo que corresponde a
// una suite soberana, y es un formato que esta misma casa sabe LEER además de
// escribir —el catálogo WOPI anuncia odt/ods/odp— así que el fichero que se deja
// en la unidad se puede volver a abrir desde Drive sin salir de Sytue. Un .docx
// habría sido igual de válido técnicamente y peor como decisión por defecto.
const formatoNativo = domain.FormatODF

// extensionNativa es la extensión del fichero según el tipo de documento. Se fija
// aquí y no se deduce del export porque el NOMBRE se decide antes de serializar.
var extensionNativa = map[domain.Kind]string{
	domain.KindDocument:     "odt",
	domain.KindSpreadsheet:  "ods",
	domain.KindPresentation: "odp",
}

// tokenPortador devuelve el JWT crudo con el que llegó la petición.
//
// Se lee de la cabecera y no del contexto a propósito: lo que hay en el contexto
// son los claims YA verificados (authn.Claims), y para hablar con Drive en nombre
// de esta persona hace falta el token tal cual, que es lo único que Drive sabe
// validar. No se reacuña nada: se reenvía la credencial que la persona presentó.
func tokenPortador(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "Bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

// crearFicheroEnDrive materializa el documento en la unidad —vacío, o con el
// contenido de la plantilla de la que nace (`estado`)— y devuelve su ancla
// («drive:<uuid>»).
//
// Se llama ANTES de crear el documento, y ese orden es la decisión que evita el
// peor estado posible. Al revés —documento primero, fichero después— un fallo de
// Drive dejaría un documento SIN ancla y para siempre: nadie volvería a pasar por
// aquí para ese documento, no habría botón de guardar y quedaría invisible en la
// unidad sin que nada lo delatara. Fallando primero, lo que queda es que la
// creación no se hace y la persona lo ve y lo reintenta.
//
// El precio de este orden es un fichero huérfano en la unidad si la creación del
// documento falla DESPUÉS. Es un fichero vacío, con su nombre, en su carpeta y
// visible: la persona lo borra. Un documento fantasma no se puede ni ver.
func (h *Handler) crearFicheroEnDrive(r *http.Request, kind domain.Kind, titulo string, estado *domain.State) (string, string, error) {
	ext, ok := extensionNativa[kind]
	if !ok {
		return "", "", domain.ErrInvalidKind
	}
	token := tokenPortador(r)
	if token == "" {
		// Sin la credencial de la persona no se puede escribir en su unidad, y
		// escribir con otra identidad no es una alternativa: es un privilegio.
		return "", "", httpx.ErrUnauthorized
	}
	datos, contentType, _, err := domain.Export(kind, titulo, estado, formatoNativo)
	if err != nil {
		return "", "", err
	}
	f, err := h.drive.CrearDocumento(r.Context(), token, titulo, ext, contentType, datos)
	if err != nil {
		return "", "", err
	}
	// EL «(2)» DEL FICHERO VIAJA AL TÍTULO. Drive ya desambiguó el nombre para no
	// pisar el documento anterior; sin traerlo aquí, la unidad quedaba con tres
	// ficheros distinguibles y la lista de Sytue Docs con tres «Documento sin
	// título» idénticos, que es donde la gente los busca.
	return "drive:" + f.ID, titulo + drive.SufijoDesambiguador(titulo, ext, f.Name), nil
}

// errorDeDrive traduce el fallo al hablar con Drive en la respuesta que le toca a
// quien intentaba crear el documento.
//
// LA CUOTA SE PROPAGA TAL CUAL (507), y es lo importante de esta función: desde
// que el documento vive en la unidad, crear uno CONSUME cuota del tenant —poco,
// unos kilobytes, pero la consume— y el contador que la mide es el de Drive, el
// mismo que ya cuenta las subidas. Si la unidad está llena, el documento no se
// crea y se dice por qué. Callarlo y crear el documento igual habría sido peor:
// un documento sin fichero es justo el estado que este cambio viene a eliminar.
func errorDeDrive(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, drive.ErrCuota):
		httpx.Fail(w, r, httpx.NewError(http.StatusInsufficientStorage, "quota_exceeded",
			"There is not enough space left in the drive to create this document."))
	case errors.Is(err, drive.ErrNoAutorizado):
		httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("Sytue Drive rejected this request."))
	case errors.Is(err, domain.ErrInvalidKind):
		fail(w, r, err)
	default:
		var apiErr *httpx.Error
		if errors.As(err, &apiErr) {
			httpx.Fail(w, r, apiErr)
			return
		}
		// EL ERROR ORIGINAL SE REGISTRA ANTES DE APLANARLO. «Drive is unavailable»
		// es lo que le toca leer a quien crea el documento, pero para quien tenga
		// que arreglarlo esconde CUÁL de las cuatro llamadas falló y con qué código:
		// `pedir` ya trae el status dentro («drive: respuesta 409»), y aquí se
		// perdía. Diagnosticar «no se crea el segundo documento» acababa siendo leer
		// el código y adivinar, en vez de leer una línea de log.
		log.From(r.Context()).Error("crear el fichero del documento en Drive",
			"error", err.Error())
		httpx.Fail(w, r, httpx.ErrUnavailable.WithMessage("Sytue Drive is unavailable; the document was not created."))
	}
}
