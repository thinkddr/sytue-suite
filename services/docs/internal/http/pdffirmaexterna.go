// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/peticiones"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
	"git.sytue.eu/sytue/one/services/docs/internal/drive"
	"git.sytue.eu/sytue/one/services/docs/internal/store"
)

// # Pedir firma a alguien de OTRA organización (ADR 0057)
//
// El PDF sale por lo que Drive ya tiene para sacar algo fuera: un enlace de solo
// lectura, que se crea aquí con la sesión de quien pide —su permiso y la verja de
// etiquetas de Drive— y solo después de que lo confirme. El token del enlace no lo ve
// nadie: se guarda cifrado y atado a su petición, y con él este servicio deja una
// copia en la unidad de quien firma cuando la abre. Esa copia tiene que ser el
// original que se pidió (su huella), es la que firma, y lo entregado se comprueba
// contra ella. Al entregar, lo mismo al revés: un enlace a la copia firmada, que quien
// pidió se trae a SU unidad, y tiene que ser exactamente lo entregado. La petición vive
// en la organización de quien pide, y las rutas de una petición pasan a ella para su
// destinatario de fuera (peticionEnAnfitrion) sin nada de su token salvo el `sub`.

// Lo que vive cada enlace. Su token no lo conoce nadie fuera de este servicio, así que
// esto es la red por debajo: un enlace que nadie usó no se queda abierto para siempre.
const (
	caducaEnlaceFirma   = 30 * 24 * time.Hour
	caducaEnlaceFirmado = 7 * 24 * time.Hour
)

// limiteFirmaFuera acota «abrir» y «traer» por persona: cada vuelta baja y sube hasta
// 25 MiB. Uno cada cinco segundos, con ráfaga de cinco.
//
// ponytail: por réplica, como execLimiter; a pg.RateLimiter si hiciera falta compartido.
var limiteFirmaFuera = httpx.NewTokenBucket(0.2, 5)

var (
	// errEnlaceCompartir: Drive no deja crear el enlace (una etiqueta que no deja
	// compartir fuera, o un permiso que no alcanza).
	errEnlaceCompartir = httpx.ErrForbidden.WithCode("pdf_signature_share_blocked").
				WithMessage("Drive does not allow this PDF to be shared outside the organisation.")
	// errEnlaceNoVale: el enlace ya no sirve (revocado, caducado o sin clave).
	errEnlaceNoVale = httpx.NewError(http.StatusGone, "pdf_signature_request_link_gone",
		"The Drive link for this PDF no longer works. Ask the person to send the request again.")
	// errCopiaCambiada: lo que hay no es el PDF que se pidió firmar.
	errCopiaCambiada = httpx.NewError(http.StatusConflict, "pdf_signature_request_changed",
		"This is no longer the PDF that was sent for signing.")
	errOcupada = httpx.NewError(http.StatusConflict, "pdf_signature_request_busy",
		"This request is already being opened. Try again in a moment.")
)

// deFuera es quien firma cuando es de otra organización: cómo se le enseña.
type deFuera struct{ nombre string }

// destinatarioDeFirma resuelve a quien firma. Primero SIEMPRE contra la organización de
// quien pide (`directory:resolve`, con su sesión). Por correo o por @usuario, y si no
// es de ella, entre todas (`people:resolve`, ADR 0040): entonces `fuera` no es nil, y
// el PDF tendrá que salir por un enlace. Por `sub`, solo de la organización.
func (h *Handler) destinatarioDeFirma(ctx context.Context, token string, in pedirFirmaJSON) (sub, correo string, fuera *deFuera, err error) {
	noEsta := httpx.ErrNotFound.WithCode("pdf_signature_recipient_not_found").
		WithMessage("Nobody on Sytue One uses that address or username.")
	if in.Email == "" {
		if _, err := uuid.Parse(in.To); err != nil {
			return "", "", nil, noEsta
		}
		correo, _, err = peticiones.Persona(ctx, h.idBaseURL, token, in.To)
		if errors.Is(err, peticiones.ErrSinPersona) {
			return "", "", nil, noEsta
		}
		return in.To, correo, nil, err
	}
	if !strings.HasPrefix(in.Email, "@") {
		a, err := mail.ParseAddress(in.Email)
		if err != nil || a.Address != in.Email {
			return "", "", nil, httpx.ErrBadRequest.WithMessage("Enter a valid email address or a @username.")
		}
		sub, err := peticiones.PorCorreo(ctx, h.idBaseURL, token, in.Email)
		if err == nil || !errors.Is(err, peticiones.ErrSinPersona) {
			return sub, strings.ToLower(in.Email), nil, err
		}
	}
	sub, nombre, err := peticiones.DeFuera(ctx, h.idBaseURL, token, in.Email)
	if errors.Is(err, peticiones.ErrSinPersona) {
		return "", "", nil, noEsta
	}
	if err != nil {
		return "", "", nil, err
	}
	// Un @usuario —o una dirección que no es la de entrar— puede ser de alguien de la
	// organización: entonces es de dentro, y su PDF no sale por ningún enlace.
	if correo, _, err := peticiones.Persona(ctx, h.idBaseURL, token, sub); err == nil {
		return sub, correo, nil, nil
	}
	return sub, strings.ToLower(in.Email), &deFuera{nombre: cmp.Or(nombre, in.Email)}, nil
}

// puedeSalir dice si se puede pedir ya a alguien de fuera (`fuera` no nil): hace
// falta la confirmación de quien pide para ESA persona —el `confirm` del 409, que es a
// quién se le enseñó— y la clave con que cifrar el token.
func (h *Handler) puedeSalir(fuera *deFuera, destinatario, confirmado string) error {
	switch {
	case fuera == nil:
		return nil
	case confirmado != destinatario:
		// La interfaz lo explica y pregunta: el PDF saldrá por un enlace de Drive.
		return httpx.NewError(http.StatusConflict, "pdf_signature_recipient_external",
			"That person is not in your organisation: the PDF has to be shared with them through a read-only Drive link first.").
			WithDetails(map[string]string{"name": fuera.nombre, "confirm": destinatario})
	case !h.sealer.Enabled() || h.idSecreto == "":
		return httpx.NewError(http.StatusServiceUnavailable, "pdf_signature_requests_unavailable",
			"Asking someone from another organisation to sign is not available on this installation.")
	}
	return nil
}

// nombreDeQuien es cómo se nombra a quien pide en la petición. A alguien de fuera,
// nunca por su correo (ADR 0040): su nombre, o nada.
func nombreDeQuien(c authn.Claims, externo bool) string {
	if !externo {
		return peticiones.De(c, "").Quien()
	}
	return cmp.Or(strings.TrimSpace(c.Name), "Sytue One")
}

// Para qué es cada token cifrado: sin el mismo `para`, no se abre. Un blob no vale
// en otra petición ni para otra cosa.
func paraOriginal(fichero, destinatario string) []byte {
	return []byte("pdf-firma:original:" + fichero + ":" + destinatario)
}

func paraFirmado(id string) []byte { return []byte("pdf-firma:firmado:" + id) }

func huellaDe(datos []byte) string {
	h := sha256.Sum256(datos)
	return hex.EncodeToString(h[:])
}

// prepararSalida es lo que hace falta para pedir fuera: la huella del original (que
// quien pide ve) y el enlace de Drive, con su token ya cifrado.
func (h *Handler) prepararSalida(ctx context.Context, token, fichero, destinatario string) (huella, enlaceID string, sellado []byte, err error) {
	datos, err := h.ficheros.Descargar(ctx, token, fichero, domain.MaxPDFBytes)
	if err != nil {
		return "", "", nil, httpx.ErrNotFound.WithMessage("The PDF is not in Drive, or you cannot open it.")
	}
	if _, err := domain.LeerPDF(datos); err != nil {
		return "", "", nil, httpx.ErrBadRequest.WithMessage("That file is not a PDF.")
	}
	enlaceID, sellado, err = h.enlaceSellado(ctx, token, fichero, caducaEnlaceFirma, paraOriginal(fichero, destinatario))
	return huellaDe(datos), enlaceID, sellado, err
}

// enlaceSellado crea el enlace de solo lectura al fichero `id` con la sesión `token` y
// devuelve su id y su token ya cifrado para `para`: en claro no se guarda nunca.
func (h *Handler) enlaceSellado(ctx context.Context, token, id string, caduca time.Duration, para []byte) (string, []byte, error) {
	enlaceID, tok, err := h.ficheros.CompartirEnlace(ctx, token, id, time.Now().Add(caduca))
	switch {
	case errors.Is(err, drive.ErrProhibido):
		return "", nil, errEnlaceCompartir
	case errors.Is(err, drive.ErrNoAutorizado):
		return "", nil, httpx.ErrUnauthorized
	case err != nil:
		return "", nil, err
	}
	sellado, err := h.sealer.SealPara([]byte(tok), para)
	if err != nil {
		h.quitarEnlace(ctx, token, enlaceID)
		return "", nil, err
	}
	return enlaceID, sellado, nil
}

// traerDeEnlace baja el PDF de un enlace sellado para `para`.
func (h *Handler) traerDeEnlace(ctx context.Context, sellado, para []byte) ([]byte, error) {
	tok, err := h.sealer.OpenPara(sellado, para)
	if err != nil {
		return nil, errEnlaceNoVale
	}
	datos, _, err := h.ficheros.DescargarEnlace(ctx, string(tok), domain.MaxPDFBytes)
	if err != nil {
		return nil, errEnlaceNoVale
	}
	return datos, nil
}

// guardarEnDrive deja el PDF en la unidad de quien llama.
func (h *Handler) guardarEnDrive(ctx context.Context, token, nombre string, datos []byte) (drive.Fichero, error) {
	f, err := h.ficheros.GuardarPDF(ctx, token, nombre, datos)
	if errors.Is(err, drive.ErrCuota) {
		return f, httpx.NewError(http.StatusInsufficientStorage, "quota_exceeded", "Your Drive is full.")
	}
	return f, err
}

// borrarCopia quita de la unidad de quien llama una copia que se acaba de dejar y ya
// no pertenece a ninguna petición.
func (h *Handler) borrarCopia(ctx context.Context, token, id string) {
	if err := h.ficheros.BorrarFichero(ctx, token, id); err != nil {
		log.From(ctx).Warn("docs: borrar una copia huérfana de una petición de firma", "error", err)
	}
}

// quitarEnlace revoca un enlace sin parar por ello lo que se estaba haciendo: su token
// no lo tiene nadie, y además caduca solo.
func (h *Handler) quitarEnlace(ctx context.Context, token, enlaceID string) {
	if enlaceID == "" {
		return
	}
	if err := h.ficheros.QuitarEnlace(ctx, token, enlaceID); err != nil {
		log.From(ctx).Warn("docs: revocar el enlace de una petición de firma", "error", err)
	}
}

// revocarSiSobra revoca, con la sesión de quien pidió, el enlace del original de una
// petición de fuera que ya no está pendiente (firmada, rechazada o cancelada), y lo
// apunta. Lo hace quien pidió porque solo su sesión puede: al rechazar, la otra
// persona no podría.
func (h *Handler) revocarSiSobra(ctx context.Context, token string, p store.PeticionFirma) {
	if !p.Externo || p.EnlaceShare == "" || p.Estado == store.FirmaPendiente {
		return
	}
	if err := h.ficheros.QuitarEnlace(ctx, token, p.EnlaceShare); err != nil && !errors.Is(err, drive.ErrProhibido) {
		log.From(ctx).Warn("docs: revocar el enlace de una petición de firma", "error", err)
		return
	}
	if err := h.peticionesFirma.OlvidarEnlaceFirma(ctx, p.ID, p.Solicitante); err != nil {
		log.From(ctx).Warn("docs: apuntar el enlace revocado", "error", err)
	}
}

// ocupar reserva la petición para una vuelta de «abrir» o «traer», con el tope por
// persona. Devuelve la fila y cómo soltarla si la vuelta falla.
func (h *Handler) ocupar(w http.ResponseWriter, r *http.Request, id, sub string, recibida bool) (store.PeticionFirma, func(context.Context), bool) {
	if res, _ := limiteFirmaFuera.Allow(r.Context(), sub); !res.Allowed {
		httpx.Fail(w, r, httpx.NewError(http.StatusTooManyRequests, "rate_limited", "Too many requests. Try again in a moment."))
		return store.PeticionFirma{}, nil, false
	}
	p, err := h.peticionesFirma.OcuparPeticionFirma(r.Context(), id, sub, recibida)
	if errors.Is(err, store.ErrPeticionFirmaOcupada) {
		httpx.Fail(w, r, errOcupada)
		return p, nil, false
	}
	if err != nil {
		h.failPeticionFirma(w, r, err)
		return p, nil, false
	}
	soltar := func(ctx context.Context) {
		if err := h.peticionesFirma.SoltarPeticionFirma(ctx, id, sub); err != nil {
			log.From(ctx).Warn("docs: soltar la petición de firma", "error", err)
		}
	}
	return p, soltar, true
}

// abrirPeticionFirma — POST /api/v1/pdf/signature-requests/{id}/open: quien la recibió
// desde otra organización se trae el PDF a SU unidad, y desde ahí lo abre y lo firma.
// Si ya la tiene, la misma copia; si la borró, otra.
func (h *Handler) abrirPeticionFirma(w http.ResponseWriter, r *http.Request) {
	subj, id, ok := h.peticionDeLaRuta(w, r)
	if !ok {
		return
	}
	// La organización desde la que se abre tiene que admitir las peticiones de la
	// organización donde vive esta (ADR 0059): la copia acabaría en SU unidad.
	if !h.admiteDesdeAqui(w, r) {
		return
	}
	p, soltar, ok := h.ocupar(w, r, id, subj.Sub, true)
	if !ok {
		return
	}
	token := authn.Token(r)
	if p.Copia != "" {
		if _, err := h.ficheros.Metadatos(r.Context(), token, p.Copia); err == nil {
			soltar(r.Context())
			httpx.JSON(w, r, http.StatusOK, p.ParaQuienFirma())
			return
		}
	}
	datos, err := h.traerDeEnlace(r.Context(), p.Enlace, paraOriginal(p.Fichero, subj.Sub))
	if err == nil && huellaDe(datos) != p.Huella {
		// El enlace sirve lo que HOY es el fichero: si cambió, no es lo que se pidió.
		err = errCopiaCambiada
	}
	var f drive.Fichero
	if err == nil {
		f, err = h.guardarEnDrive(r.Context(), token, nombreDeRuta(p.Ruta), datos)
	}
	if err == nil {
		p, err = h.peticionesFirma.GuardarCopiaFirma(r.Context(), id, subj.Sub, f.ID, f.Path)
		if err != nil {
			// La petición dejó de estar (la bloqueó, o se canceló) mientras se traía:
			// la copia no le sirve a nadie.
			h.borrarCopia(r.Context(), token, f.ID)
		}
	}
	if err != nil {
		soltar(r.Context())
		h.failPeticionFirma(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, p.ParaQuienFirma())
}

// traerFirmado — POST /api/v1/pdf/signature-requests/{id}/fetch-signed: quien pidió
// se trae a SU unidad la copia firmada por alguien de otra organización.
func (h *Handler) traerFirmado(w http.ResponseWriter, r *http.Request) {
	subj, id, ok := h.peticionDeLaRuta(w, r)
	if !ok {
		return
	}
	p, soltar, ok := h.ocupar(w, r, id, subj.Sub, false)
	if !ok {
		return
	}
	token := authn.Token(r)
	datos, err := h.traerDeEnlace(r.Context(), p.Enlace, paraFirmado(id))
	if err == nil && huellaDe(datos) != p.FirmadoHuella {
		// Tiene que ser exactamente lo que se entregó y se comprobó: el fichero de la
		// otra organización pudo cambiar después.
		err = errFirmadoNoVale
	}
	var f drive.Fichero
	if err == nil {
		// El nombre lo pone esta organización, no el fichero de la otra.
		f, err = h.guardarEnDrive(r.Context(), token, strings.TrimSuffix(nombreDeRuta(p.Ruta), ".pdf")+" (firmado).pdf", datos)
	}
	if err == nil {
		p, err = h.peticionesFirma.GuardarFirmadoTraido(r.Context(), id, subj.Sub, f.ID, f.Path)
		if err != nil {
			h.borrarCopia(r.Context(), token, f.ID)
		}
	}
	if err != nil {
		soltar(r.Context())
		h.failPeticionFirma(w, r, err)
		return
	}
	h.revocarSiSobra(r.Context(), token, p)
	httpx.JSON(w, r, http.StatusOK, p.ParaQuienPide())
}

// peticionEnAnfitrion es el middleware de `/signature-requests/{id}`: si quien llama es
// el destinatario de FUERA de esa petición, la petición sigue en la organización donde
// vive, y `subjectFrom` la reduce a su `sub`. Si no, nada cambia.
func (h *Handler) peticionEnAnfitrion(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := authn.FromContext(r.Context())
		id := chi.URLParam(r, "id")
		if h.peticionesFirma == nil || !ok || c.Subject == "" {
			next.ServeHTTP(w, r)
			return
		}
		if _, err := uuid.Parse(id); err != nil {
			next.ServeHTTP(w, r) // la ruta contesta 404
			return
		}
		anfitriona, err := h.peticionesFirma.AnfitrionPeticionFirma(r.Context(), id, c.Subject)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		if anfitriona == "" || anfitriona == c.Tenant {
			next.ServeHTTP(w, r)
			return
		}
		t, err := tenancy.New(anfitriona)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}
		ctx := context.WithValue(tenancy.Into(r.Context(), t), claveExterno{}, true)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
