// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"context"
	"errors"
	"net/http"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/docs/internal/store"
)

// # Pedir firma a varias personas, en orden, con recordatorios y caducidad (mig. 0034)
//
// Una cadena son varias peticiones de la 0011 con el mismo `circuito`: la primera
// persona recibe el aviso y las demás esperan sin saber nada. Cuando una entrega su
// firmado, la siguiente recibe el aviso con ESE fichero, y así cada una firma sobre
// las firmas de las de antes; el servicio comprueba en cada paso lo de siempre (una
// firma nueva que cubre el documento entero, el mismo texto). Rechazar o cancelar
// cierra la cadena entera: firmar a medias un circuito no tiene sentido.
//
// En una cadena solo firma gente de la organización. A alguien de fuera el PDF le
// llega por un enlace y su firmado vuelve por otro (ADR 0057); encadenar eso pide
// mover copias entre organizaciones en cada turno, y se deja para cuando haga falta.
// Una persona de fuera sigue pudiendo firmar sola, con caducidad y recordatorios.

// caducaEn es la fecha de caducidad a `dias` de ahora; nil con 0, que es «nunca».
func caducaEn(dias int) *time.Time {
	if dias <= 0 {
		return nil
	}
	t := time.Now().Add(time.Duration(dias) * 24 * time.Hour)
	return &t
}

// pedirFirmaEnCadena es POST /api/v1/pdf/signature-requests con «signers»: ya está
// validado el cuerpo y comprobado que quien pide ve el PDF.
func (h *Handler) pedirFirmaEnCadena(w http.ResponseWriter, r *http.Request, sub string, in pedirFirmaJSON) {
	token := authn.Token(r)
	claims, _ := authnClaims(r)
	quien := nombreDeQuien(claims, false)
	filas := make([]store.PeticionFirma, 0, len(in.Signers))
	vistos := map[string]bool{sub: true}
	for _, s := range in.Signers {
		destinatario, correo, fuera, err := h.destinatarioDeFirma(r.Context(), token, pedirFirmaJSON{To: s.To, Email: s.Email})
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		switch {
		case destinatario == sub:
			httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("pdf_signature_request_self").
				WithMessage("You cannot ask yourself to sign."))
			return
		case fuera != nil:
			httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("pdf_signature_chain_external").
				WithMessage("Everyone in a signing order has to be in your organisation.").
				WithDetails(map[string]string{"name": fuera.nombre}))
			return
		case vistos[destinatario]:
			httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("pdf_signature_chain_repeated").
				WithMessage("The same person appears twice in the signing order."))
			return
		}
		vistos[destinatario] = true
		filas = append(filas, store.PeticionFirma{
			Solicitante: sub, SolicitanteNombre: quien, Destinatario: destinatario, DestinatarioCorreo: correo,
			Fichero: in.FileID, Ruta: in.Path, Mensaje: in.Message,
			Caduca: caducaEn(in.ExpiresInDays), RecordarDias: in.RemindEveryDays,
		})
	}
	cadena, err := h.peticionesFirma.CrearCadenaFirma(r.Context(), filas)
	if err != nil {
		httpx.Fail(w, r, errorAlPedir(err))
		return
	}
	h.avisarDeTurno(r, cadena[0], token)
	httpx.JSON(w, r, http.StatusCreated, map[string]any{"chain": cadena[0].Circuito, "requests": cadena})
}

// avisarDeTurno avisa a quien le toca firmar `p` en una cadena, con aviso y correo
// como al pedir; `token` es el de quien llama, para buscar su correo.
func (h *Handler) avisarDeTurno(r *http.Request, p store.PeticionFirma, token string) {
	nombre := nombreDeRuta(p.Ruta)
	cuerpo := p.SolicitanteNombre + " is asking you to sign “" + nombre + "”."
	if p.Mensaje != "" {
		cuerpo += "\n\n" + p.Mensaje
	}
	h.avisoDeFirma(r, p.ID, p.Destinatario, "docs.signature.requested", p.SolicitanteNombre+" asks you to sign “"+nombre+"”",
		h.enlacePDF(r, p.Ruta), map[string]string{"actor": p.SolicitanteNombre, "name": nombre}, token,
		conPie(cuerpo, "Open the PDF to sign it:"))
}

// Barrido es lo que hizo una vuelta de [Handler.BarrerPeticionesFirma].
type Barrido struct {
	Caducadas, Recordadas, Errores int
}

// BarrerPeticionesFirma caduca lo que pasó su fecha y recuerda lo que toca, en todas
// las organizaciones. Lo llama cada hora el servicio; varias réplicas a la vez no
// avisan dos veces, porque solo la que consigue cambiar la fila avisa.
//
// Al caducar se avisa a quien pidió (una vez por petición o cadena: con la que tenía
// el turno); los recordatorios van solo por el Hub, sin correo: no hay sesión de nadie
// con la que buscar la dirección, y el aviso del Hub ya llega al móvil.
func (h *Handler) BarrerPeticionesFirma(ctx context.Context) (Barrido, error) {
	var b Barrido
	if h.peticionesFirma == nil {
		return b, nil
	}
	vencidas, recordar, err := h.peticionesFirma.PeticionesFirmaPorBarrer(ctx)
	if err != nil {
		return b, err
	}
	for _, v := range vencidas {
		tctx, ok := h.contextoDe(ctx, v.Tenant)
		if !ok {
			b.Errores++
			continue
		}
		p, err := h.peticionesFirma.CaducarPeticionFirma(tctx, v.Peticion.ID)
		if errors.Is(err, store.ErrPeticionFirmaNoEsta) {
			continue
		}
		if err != nil {
			log.From(ctx).Warn("docs: caducar una petición de firma", "error", err)
			b.Errores++
			continue
		}
		b.Caducadas++
		if v.Estado != store.FirmaPendiente {
			continue // esperaba turno: el aviso va con la de la cadena que lo tenía
		}
		nombre := nombreDeRuta(p.Ruta)
		h.avisarDeFirma(tctx, v.Tenant, p.ID, p.Solicitante, "docs.signature.expired",
			"The request to sign “"+nombre+"” expired", hubnotify.EnlacePDF(h.publicURL, v.Tenant, p.Ruta),
			map[string]string{"name": nombre}, "", "")
	}
	for _, v := range recordar {
		tctx, ok := h.contextoDe(ctx, v.Tenant)
		if !ok {
			b.Errores++
			continue
		}
		p, err := h.peticionesFirma.RecordarPeticionFirma(tctx, v.Peticion.ID)
		if errors.Is(err, store.ErrPeticionFirmaNoEsta) {
			continue
		}
		if err != nil {
			log.From(ctx).Warn("docs: recordar una petición de firma", "error", err)
			b.Errores++
			continue
		}
		b.Recordadas++
		nombre, enlace := nombreDeRuta(p.Ruta), hubnotify.EnlacePDF(h.publicURL, v.Tenant, p.Ruta)
		if p.Externo {
			// Su PDF no está en esa ruta: el aviso le lleva a CertiPDF en SU organización,
			// como el de «te piden firmar».
			enlace = hubnotify.EnlacePDF(h.publicURL, "", "")
		}
		h.avisarDeFirma(tctx, v.Tenant, p.ID, p.Destinatario, "docs.signature.reminder",
			p.SolicitanteNombre+" is still waiting for you to sign “"+nombre+"”", enlace,
			map[string]string{"actor": p.SolicitanteNombre, "name": nombre}, "", "")
	}
	return b, nil
}

// contextoDe es `ctx` con la organización `slug`, para cambiar una fila con su RLS.
func (h *Handler) contextoDe(ctx context.Context, slug string) (context.Context, bool) {
	t, err := tenancy.New(slug)
	if err != nil {
		log.From(ctx).Warn("docs: organización inválida en una petición de firma", "error", err)
		return ctx, false
	}
	return tenancy.Into(ctx, t), true
}
