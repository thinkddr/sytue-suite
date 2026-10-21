// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"cmp"
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/peticiones"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
	"git.sytue.eu/sytue/one/services/docs/internal/drive"
	"git.sytue.eu/sytue/one/services/docs/internal/store"
)

// # Pedir firma (ADR 0051 §7)
//
// Desde CertiPDF, una persona pide a otra —de su organización o, con el ADR 0057, de
// otra— que firme un PDF de Drive. El PDF no pasa por aquí: vive en la unidad de la organización y la petición
// guarda su id y su ruta. Quien lo recibe tiene un aviso del Hub y un correo que abren
// el PDF; lo firma como firme siempre (certificado del sistema, .p12 o rúbrica), el
// firmado queda en Drive como copia, y al decir «ya está» el servicio lo comprueba:
// tiene que tener MÁS firmas que el original y la última tiene que cubrir el fichero
// entero. Entonces quien pidió recibe el aviso con el enlace al firmado.
//
// Lo que esto NO dice es QUIÉN firmó: eso lo dice el certificado dentro del PDF, y
// CertiPDF lo enseña al abrirlo. «Firmada» aquí es «la persona a quien se pidió
// entregó un documento con una firma nueva que lo cubre entero».
//
// A alguien de OTRA organización (ADR 0057): pdffirmaexterna.go.

// peticionesFirmaStore es lo que esto necesita de la 0011. Lo cumple *store.Store.
type peticionesFirmaStore interface {
	PuedePedirFirma(ctx context.Context, p store.PeticionFirma) error
	CrearPeticionFirma(ctx context.Context, p store.PeticionFirma) (store.PeticionFirma, error)
	PeticionesFirma(ctx context.Context, sub string) (recibidas, pedidas []store.PeticionFirma, err error)
	PeticionFirmaPendiente(ctx context.Context, id, destinatario string) (store.PeticionFirma, error)
	ResolverPeticionFirma(ctx context.Context, id, quien, estado, firmado, firmadoRuta string, enlace []byte, firmadoHuella string) (store.PeticionFirma, error)
	AnfitrionPeticionFirma(ctx context.Context, id, sub string) (string, error)
	OcuparPeticionFirma(ctx context.Context, id, sub string, recibida bool) (store.PeticionFirma, error)
	SoltarPeticionFirma(ctx context.Context, id, sub string) error
	GuardarCopiaFirma(ctx context.Context, id, destinatario, copia, copiaRuta string) (store.PeticionFirma, error)
	GuardarFirmadoTraido(ctx context.Context, id, solicitante, firmado, firmadoRuta string) (store.PeticionFirma, error)
	OlvidarEnlaceFirma(ctx context.Context, id, solicitante string) error
	BloquearFirma(ctx context.Context, quien, bloqueado, nombre, tenant string) error
	PeticionDeFueraRecibida(ctx context.Context, id, destinatario string) (store.PeticionFirma, error)
	DesbloquearFirma(ctx context.Context, quien, bloqueado string) error
	BloqueosFirma(ctx context.Context, quien string) ([]store.BloqueoFirma, error)
	FirmaBloqueada(ctx context.Context, destinatario, solicitante string) (bool, error)
	FirmaAdmitida(ctx context.Context, orgs []string, origen string) (bool, error)
	PoliticaDeFirma(ctx context.Context) (store.PoliticaFirma, error)
	GuardarPoliticaDeFirma(ctx context.Context, p store.PoliticaFirma, quien string) (store.PoliticaFirma, error)
}

// ficherosDrive es lo que esto necesita de Drive. Lo cumple *drive.Cliente.
type ficherosDrive interface {
	Metadatos(ctx context.Context, token, id string) (drive.Fichero, error)
	Descargar(ctx context.Context, token, id string, tope int64) ([]byte, error)
	CompartirEnlace(ctx context.Context, token, id string, caduca time.Time) (enlaceID, tokenEnlace string, err error)
	QuitarEnlace(ctx context.Context, token, enlaceID string) error
	DescargarEnlace(ctx context.Context, tokenEnlace string, tope int64) ([]byte, string, error)
	GuardarPDF(ctx context.Context, token, nombre string, datos []byte) (drive.Fichero, error)
	BorrarFichero(ctx context.Context, token, id string) error
}

// Topes de lo que llega en una petición de firma.
const (
	maxMensajeFirma = 1000
	maxRutaFirma    = 4096
	maxIDFichero    = 128
)

type pedirFirmaJSON struct {
	FileID  string `json:"file_id"`
	Path    string `json:"path"`
	To      string `json:"to"`
	Email   string `json:"email"`
	Message string `json:"message"`
	// Confirm es la confirmación de quien pide de que el PDF salga por un enlace de
	// Drive, cuando quien firma es de otra organización: el `confirm` que trajo el 409,
	// que es a QUIÉN se confirmó. Sin ella, 409 y nada creado.
	Confirm string `json:"confirm"`
}

// pedirFirma — POST /api/v1/pdf/signature-requests.
func (h *Handler) pedirFirma(w http.ResponseWriter, r *http.Request) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	if h.peticionesFirma == nil || h.ficheros == nil || h.idBaseURL == "" {
		httpx.Fail(w, r, httpx.NewError(http.StatusServiceUnavailable, "pdf_signature_requests_unavailable",
			"Signature requests are not available on this installation."))
		return
	}
	var in pedirFirmaJSON
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := in.validar(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	token := authn.Token(r)
	// Quien pide tiene que poder ver el fichero, y tiene que ser un PDF: si no, el
	// aviso llevaría a otra persona a un documento que no existe o que no es suyo.
	// Y la ruta tiene que ser la de ESE fichero: el aviso enseña el nombre y abre la
	// ruta, así que una ruta de otro llevaría a firmar un documento distinto del pedido.
	// Antes que buscar a nadie: sin un PDF suyo, no se pregunta por personas.
	f, err := h.ficheros.Metadatos(r.Context(), token, in.FileID)
	if err != nil || f.Name != nombreDeRuta(in.Path) {
		httpx.Fail(w, r, httpx.ErrNotFound.WithMessage("The PDF is not in Drive, or you cannot open it."))
		return
	}
	destinatario, correo, fuera, err := h.destinatarioDeFirma(r.Context(), token, in)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if destinatario == subj.Sub {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("pdf_signature_request_self").
			WithMessage("You cannot ask yourself to sign."))
		return
	}
	externo := fuera != nil
	if err := h.puedeSalir(fuera, destinatario, in.Confirm); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	claims, _ := authnClaims(r)
	quien := nombreDeQuien(claims, externo)
	fila := store.PeticionFirma{
		Solicitante: subj.Sub, SolicitanteNombre: quien, Destinatario: destinatario, DestinatarioCorreo: correo,
		Fichero: in.FileID, Ruta: in.Path, Mensaje: in.Message, Externo: externo,
	}
	// Los topes, antes de crear en Drive un enlace que habría que quitar.
	if err := h.peticionesFirma.PuedePedirFirma(r.Context(), fila); err != nil {
		httpx.Fail(w, r, errorAlPedir(err))
		return
	}
	if externo {
		// Si no va a llegar —bloqueo o política—, todo lo demás es igual: quien pide
		// no tiene que poder distinguirla (ADR 0059).
		if fila.Silenciada, err = h.noLlega(r.Context(), destinatario, subj.Sub); err != nil {
			httpx.Fail(w, r, err)
			return
		}
		if fila.Huella, fila.EnlaceShare, fila.Enlace, err = h.prepararSalida(r.Context(), token, in.FileID, destinatario); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	p, err := h.peticionesFirma.CrearPeticionFirma(r.Context(), fila)
	if err != nil {
		h.quitarEnlace(r.Context(), token, fila.EnlaceShare) // la petición no existe: el enlace, tampoco
		httpx.Fail(w, r, errorAlPedir(err))
		return
	}
	nombre := nombreDeRuta(in.Path)
	cuerpo := quien + " is asking you to sign “" + nombre + "”."
	if in.Message != "" {
		cuerpo += "\n\n" + in.Message
	}
	enlace := h.enlacePDF(r, in.Path)
	if externo {
		// Su PDF no está en esa ruta, sino en la unidad de la otra organización: el
		// aviso le lleva a CertiPDF en la SUYA, donde está la petición para traérselo.
		// Y sin correo: el suyo no lo da Sytue ID (ADR 0040), y mandar desde avisos@ a la
		// dirección que teclee cualquiera sería prestar el remitente.
		enlace, token, cuerpo = hubnotify.EnlacePDF(h.publicURL, "", ""), "", ""
	}
	if !p.Silenciada {
		h.avisoDeFirma(r, p.ID, destinatario, "docs.signature.requested", quien+" asks you to sign “"+nombre+"”",
			enlace, map[string]string{"actor": quien, "name": nombre}, token, conPie(cuerpo, "Open the PDF to sign it:"))
	}
	httpx.JSON(w, r, http.StatusCreated, p.ParaQuienPide())
}

// conPie añade `pie` al cuerpo del correo; sin cuerpo no hay correo.
func conPie(cuerpo, pie string) string {
	if cuerpo == "" {
		return ""
	}
	return cuerpo + "\n\n" + pie
}

// validar deja la petición recortada y dice qué le falta o le sobra.
func (in *pedirFirmaJSON) validar() error {
	in.Message = strings.TrimSpace(in.Message)
	in.To, in.Email = strings.TrimSpace(in.To), strings.TrimSpace(in.Email)
	switch {
	case in.FileID == "" || len(in.FileID) > maxIDFichero:
		return httpx.ErrBadRequest.WithMessage(`Send the Drive file in "file_id".`)
	case !rutaDePDF(in.Path):
		return httpx.ErrBadRequest.WithMessage(`"path" must be the Drive path of a PDF.`)
	case (in.To == "") == (in.Email == ""):
		return httpx.ErrBadRequest.WithMessage(`Say who signs: "to" (their id) or "email", not both.`)
	case utf8.RuneCountInString(in.Message) > maxMensajeFirma:
		return httpx.ErrBadRequest.WithMessage("The message is too long.")
	}
	return nil
}

// errorAlPedir traduce lo que dice el store al crear la petición.
func errorAlPedir(err error) error {
	switch {
	case errors.Is(err, store.ErrDemasiadasPeticionesFirma):
		return httpx.NewError(http.StatusTooManyRequests, "pdf_signature_request_too_many",
			"You have asked for too many signatures in the last hour. Try again later.")
	case errors.Is(err, store.ErrPeticionFirmaRepetida):
		return httpx.NewError(http.StatusConflict, "pdf_signature_request_exists",
			"That person already has a pending request to sign this PDF.")
	case errors.Is(err, store.ErrDemasiadasParaEsaPersona):
		return httpx.NewError(http.StatusTooManyRequests, "pdf_signature_request_recipient_busy",
			"That person already has too many pending signature requests.")
	}
	return err
}

// listarPeticionesFirma — GET /api/v1/pdf/signature-requests: las recibidas y las
// pedidas de quien llama.
func (h *Handler) listarPeticionesFirma(w http.ResponseWriter, r *http.Request) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	if h.peticionesFirma == nil {
		httpx.JSON(w, r, http.StatusOK, map[string]any{
			"received": []store.PeticionFirma{}, "sent": []store.PeticionFirma{}, "blocked": []store.BloqueoFirma{},
		})
		return
	}
	recibidas, pedidas, err := h.peticionesFirma.PeticionesFirma(r.Context(), subj.Sub)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// Vacías y no nulas: la interfaz recorre las dos sin comprobar.
	if recibidas == nil {
		recibidas = []store.PeticionFirma{}
	}
	if pedidas == nil {
		pedidas = []store.PeticionFirma{}
	}
	for i := range recibidas {
		recibidas[i] = recibidas[i].ParaQuienFirma()
	}
	bloqueados, err := h.peticionesFirma.BloqueosFirma(r.Context(), subj.Sub)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if bloqueados == nil {
		bloqueados = []store.BloqueoFirma{}
	}
	for i := range pedidas {
		h.revocarSiSobra(r.Context(), authn.Token(r), pedidas[i])
		pedidas[i] = pedidas[i].ParaQuienPide()
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"received": recibidas, "sent": pedidas, "blocked": bloqueados})
}

type firmadaJSON struct {
	SignedFileID string `json:"signed_file_id"`
	SignedPath   string `json:"signed_path"`
}

// firmarPeticion — POST /api/v1/pdf/signature-requests/{id}/sign: quien recibió la
// petición entrega el PDF firmado.
func (h *Handler) firmarPeticion(w http.ResponseWriter, r *http.Request) {
	subj, id, ok := h.peticionDeLaRuta(w, r)
	if !ok {
		return
	}
	var in firmadaJSON
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if in.SignedFileID == "" || len(in.SignedFileID) > maxIDFichero || !rutaDePDF(in.SignedPath) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(`Send the signed PDF in "signed_file_id" and "signed_path".`))
		return
	}
	if !h.admiteDesdeAqui(w, r) {
		return
	}
	p, err := h.peticionesFirma.PeticionFirmaPendiente(r.Context(), id, subj.Sub)
	if err != nil {
		h.failPeticionFirma(w, r, err)
		return
	}
	token := authn.Token(r)
	// De fuera, el original es SU copia: la que abrió y firmó.
	original, firmadoRuta := p.Fichero, in.SignedPath
	if p.Externo {
		if p.Copia == "" {
			httpx.Fail(w, r, errFirmadoNoVale)
			return
		}
		original = p.Copia
		// Su ruta es de su organización y no le dice nada a quien pidió.
		firmadoRuta = ""
	}
	antes, despues, err := h.comprobarFirmado(r.Context(), token, original, in.SignedFileID, nombreDeRuta(in.SignedPath))
	// De fuera, su copia tiene que ser el original que se pidió, sin tocar: si no,
	// firmaría otro documento. Se dice eso antes que cualquier otra cosa.
	if p.Externo && antes != nil && huellaDe(antes) != p.Huella {
		err = errCopiaCambiada
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var enlace []byte
	var enlaceID, huella string
	if p.Externo {
		huella = huellaDe(despues)
		if enlaceID, enlace, err = h.enlaceSellado(r.Context(), token, in.SignedFileID, caducaEnlaceFirmado, paraFirmado(id)); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	p, err = h.peticionesFirma.ResolverPeticionFirma(r.Context(), id, subj.Sub, store.FirmaFirmada, in.SignedFileID, firmadoRuta, enlace, huella)
	if err != nil {
		h.quitarEnlace(r.Context(), token, enlaceID)
		h.failPeticionFirma(w, r, err)
		return
	}
	actor, cuerpo := h.actorDeFirma(r, p), ""
	nombre := nombreDeRuta(p.Ruta)
	if !p.Externo {
		// De fuera, sin correo: la sesión de quien firma no resuelve a nadie de la otra
		// organización, y ella tampoco recibe correo (ADR 0057 §7).
		cuerpo = actor + " signed “" + nombre + "”. Open the signed PDF:"
	} else {
		token = ""
	}
	h.avisoDeFirma(r, p.ID, p.Solicitante, "docs.signature.completed", actor+" signed “"+nombre+"”",
		h.enlacePDF(r, firmadoRuta), map[string]string{"actor": actor, "name": nombre}, token, cuerpo)
	httpx.JSON(w, r, http.StatusOK, p.ParaQuienFirma())
}

// rechazarPeticionFirma — POST /api/v1/pdf/signature-requests/{id}/decline.
func (h *Handler) rechazarPeticionFirma(w http.ResponseWriter, r *http.Request) {
	subj, id, ok := h.peticionDeLaRuta(w, r)
	if !ok {
		return
	}
	p, err := h.peticionesFirma.ResolverPeticionFirma(r.Context(), id, subj.Sub, store.FirmaRechazada, "", "", nil, "")
	if err != nil {
		h.failPeticionFirma(w, r, err)
		return
	}
	actor := h.actorDeFirma(r, p)
	nombre := nombreDeRuta(p.Ruta)
	h.avisoDeFirma(r, p.ID, p.Solicitante, "docs.signature.declined", actor+" declined to sign “"+nombre+"”",
		h.enlacePDF(r, p.Ruta), map[string]string{"actor": actor, "name": nombre}, "", "")
	httpx.JSON(w, r, http.StatusOK, p.ParaQuienFirma())
}

// cancelarPeticionFirma — DELETE /api/v1/pdf/signature-requests/{id}: solo quien la pidió.
func (h *Handler) cancelarPeticionFirma(w http.ResponseWriter, r *http.Request) {
	subj, id, ok := h.peticionDeLaRuta(w, r)
	if !ok {
		return
	}
	p, err := h.peticionesFirma.ResolverPeticionFirma(r.Context(), id, subj.Sub, store.FirmaCancelada, "", "", nil, "")
	if err != nil {
		h.failPeticionFirma(w, r, err)
		return
	}
	h.revocarSiSobra(r.Context(), authn.Token(r), p)
	httpx.NoContent(w)
}

// peticionDeLaRuta es quien llama y el id de la ruta, ya validado como UUID.
func (h *Handler) peticionDeLaRuta(w http.ResponseWriter, r *http.Request) (domain.Subject, string, bool) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return subj, "", false
	}
	if h.peticionesFirma == nil {
		httpx.Fail(w, r, errPeticionFirmaNoEsta)
		return subj, "", false
	}
	id := chi.URLParam(r, "id")
	if _, err := uuid.Parse(id); err != nil {
		httpx.Fail(w, r, errPeticionFirmaNoEsta)
		return subj, "", false
	}
	return subj, id, true
}

var errPeticionFirmaNoEsta = httpx.ErrNotFound.WithCode("pdf_signature_request_not_found").
	WithMessage("This signature request is no longer pending for you.")

func (h *Handler) failPeticionFirma(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, store.ErrPeticionFirmaNoEsta) {
		httpx.Fail(w, r, errPeticionFirmaNoEsta)
		return
	}
	httpx.Fail(w, r, err)
}

// errFirmadoNoVale es lo que se contesta cuando lo entregado no es el PDF firmado.
var errFirmadoNoVale = httpx.NewError(http.StatusUnprocessableEntity, "pdf_signature_request_not_signed",
	"That PDF does not carry a new signature covering the whole document.")

var errOriginalNoEsta = httpx.ErrNotFound.WithMessage("The original PDF is not in Drive, or you cannot open it.")

// comprobarFirmado baja el original y el firmado CON LA SESIÓN DE QUIEN FIRMA y
// exige lo de [firmaNueva]. Sin esto, «firmada» sería lo que dijera el cliente.
// Devuelve los dos, para quien quiera sus huellas.
func (h *Handler) comprobarFirmado(ctx context.Context, token, original, firmado, nombre string) (antes, despues []byte, err error) {
	// El aviso de «firmado» abrirá la ruta que se entrega: tiene que ser la del fichero.
	if f, err := h.ficheros.Metadatos(ctx, token, firmado); err != nil || f.Name != nombre {
		return nil, nil, errFirmadoNoVale
	}
	if antes, err = h.ficheros.Descargar(ctx, token, original, domain.MaxPDFBytes); err != nil {
		return nil, nil, errOriginalNoEsta
	}
	if despues, err = h.ficheros.Descargar(ctx, token, firmado, domain.MaxPDFBytes); err != nil {
		return nil, nil, errFirmadoNoVale
	}
	return antes, despues, firmaNueva(antes, despues)
}

// firmaNueva: `firmado` tiene más firmas que `original`, la última lo cubre entero, y
// es EL MISMO documento: las mismas páginas con el mismo texto. Firmar reescribe el
// fichero la primera vez (no se puede exigir que empiece por el original), pero no
// cambia lo que se lee; sin esto se podría entregar cualquier otro PDF firmado.
func firmaNueva(original, firmado []byte) error {
	a, err := domain.LeerPDF(original)
	if err != nil {
		return errOriginalNoEsta
	}
	b, err := domain.LeerPDF(firmado)
	if err != nil {
		return errFirmadoNoVale
	}
	antes, despues := a.FirmasDelDocumento(), b.VerificarFirmas(domain.OpcionesVerificacion{})
	// La última firma tiene que cubrirlo todo y ser ÍNTEGRA: un diccionario de
	// firma con basura en el hueco también «cubre el fichero entero».
	if len(despues) <= len(antes) || !despues[len(despues)-1].CubreTodo ||
		!despues[len(despues)-1].Integra || !mismoTexto(a, b) {
		return errFirmadoNoVale
	}
	return nil
}

// mismoTexto: el mismo número de páginas y el mismo texto en cada una.
func mismoTexto(a, b *domain.PDFDoc) bool {
	if a.NumPaginas() != b.NumPaginas() {
		return false
	}
	for i := range a.NumPaginas() {
		// Sin anotaciones: firmar añade el widget de la firma, con su dibujo.
		ta, errA := a.TextoDelContenido(i)
		tb, errB := b.TextoDelContenido(i)
		if (errA == nil) != (errB == nil) || ta != tb {
			return false
		}
	}
	return true
}

// enlacePDF es el enlace de un aviso a un PDF de la organización de la petición.
func (h *Handler) enlacePDF(r *http.Request, ruta string) string {
	t, _ := tenancy.From(r.Context())
	return hubnotify.EnlacePDF(h.publicURL, t.Slug, ruta)
}

// avisoDeFirma deja el aviso del Hub (origen `pdf`: lo abre CertiPDF) con `enlace` y,
// con `cuerpo`, el correo desde avisos@.
func (h *Handler) avisoDeFirma(r *http.Request, id, para, tipo, titulo, enlace string, params map[string]string, token, cuerpo string) {
	t, _ := tenancy.From(r.Context())
	asunto := ""
	if cuerpo != "" {
		asunto = titulo
	}
	h.avisador().Avisar(r.Context(), peticiones.Aviso{
		Tenant: t.Slug, Para: para, Source: "pdf", Type: tipo, Titulo: titulo,
		URL: enlace, Recurso: id, Params: params,
		Token: token, Asunto: asunto, Cuerpo: cuerpo,
	})
}

// actorDeFirma es cómo se nombra a quien firma o rechaza en el aviso a quien pidió.
// De fuera, nunca por su correo: su nombre o, si no lo tiene, lo que tecleó quien pidió.
func (h *Handler) actorDeFirma(r *http.Request, p store.PeticionFirma) string {
	c, _ := authnClaims(r)
	if p.Externo {
		return cmp.Or(strings.TrimSpace(c.Name), p.DestinatarioCorreo)
	}
	return peticiones.De(c, "").Quien()
}

// rutaDePDF: una ruta absoluta de Drive que acaba en .pdf, sin tramos vacíos ni «..».
func rutaDePDF(ruta string) bool {
	if !strings.HasPrefix(ruta, "/") || len(ruta) > maxRutaFirma || !strings.HasSuffix(strings.ToLower(ruta), ".pdf") ||
		strings.ContainsAny(ruta, "\x00\r\n") {
		return false
	}
	for _, t := range strings.Split(ruta[1:], "/") {
		if t == "" || t == "." || t == ".." {
			return false
		}
	}
	return true
}

func nombreDeRuta(ruta string) string { return ruta[strings.LastIndexByte(ruta, '/')+1:] }
