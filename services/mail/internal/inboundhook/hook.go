// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package inboundhook recibe correo por HTTPS cuando el 25 no es alcanzable.
//
// POR QUÉ EXISTE. Recibir correo exige un MX público en el puerto 25, y hay
// despliegues donde eso no se puede tener: el proveedor de acceso bloquea el 25
// entrante —medido en el piloto el 2026-09-06: ni una conexión llega, con el
// puerto abierto en el router y el MX publicado—. La salida ya se resolvió con
// un relé; la entrada necesitaba lo simétrico.
//
// Entonces el MX lo pone un tercero (Brevo Inbound Parsing, un Email Worker de
// Cloudflare, lo que sea) y nos entrega el mensaje por HTTP, que sí entra por el
// túnel. Este paquete es SOLO ese adaptador: no parsea MIME, no verifica nada y
// no toca la base de datos. Todo eso ya lo hace `delivery.Inbound`, que es el
// mismo procesador que usa el MX de verdad, y compartirlo es el punto: una sola
// implementación de «qué es recibir un correo», con dos transportes delante.
//
// DELIBERADAMENTE AGNÓSTICO DEL PROVEEDOR. El cuerpo es el mensaje RFC 822 tal
// cual y el sobre viaja en cabeceras. Un formato propio de un proveedor
// concreto ataría la recepción de correo a ese proveedor, y cambiarlo obligaría
// a tocar Go; así, adaptarlo es un puñado de líneas en el lado del que reenvía.
package inboundhook

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/services/mail/internal/delivery"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
	"git.sytue.eu/sytue/one/services/mail/internal/store"
)

// Ruta pública del webhook. Bajo /hooks/ como el resto de entradas de terceros
// de la suite (ver services/rooms), para que un vistazo al Ingress baste para
// saber qué superficie recibe llamadas de fuera.
const Ruta = "/hooks/inbound"

// Cabeceras del sobre. Van aparte del cuerpo porque el sobre SMTP no viaja
// dentro del mensaje: `MAIL FROM` y `RCPT TO` son del transporte, y confundirlos
// con las cabeceras `From:`/`To:` del MIME es exactamente el fallo que permite
// que un correo diga ser de quien no es.
const (
	CabeceraDe   = "X-Sytue-Mail-From"
	CabeceraPara = "X-Sytue-Rcpt-To" // repetible, o separada por comas
)

// Procesador es lo único que este paquete usa de `delivery.Inbound`. Se declara
// como interfaz para poder doblarlo en las pruebas —el tipo concreto arrastra
// PostgreSQL y S3— y de paso deja escrito, en dos líneas, todo lo que el webhook
// puede hacerle al correo: resolver un destinatario y entregar. Nada más.
type Procesador interface {
	Resolve(ctx context.Context, address string) (store.ResolvedAddress, error)
	Deliver(ctx context.Context, msg delivery.InboundMessage) error
}

// Deps es lo que el webhook necesita para funcionar.
type Deps struct {
	// Inbound DEBE construirse con VerifySPF=false. El SPF comprueba que la IP
	// que conecta esté autorizada por el dominio del remitente, y aquí la IP que
	// conecta es la del reenviador, no la del remitente: verificarlo rechazaría
	// todo el correo legítimo. Se apaga a sabiendas y se compensa con DKIM, que
	// sí sobrevive al reenvío porque va firmado dentro del mensaje.
	Inbound Procesador
	// Token compartido con el reenviador. Vacío ⇒ el webhook NO se monta: una
	// entrada de correo sin autenticar es un relé abierto con otro nombre.
	Token  string
	Logger *slog.Logger
}

type handler struct {
	in     Procesador
	token  string
	logger *slog.Logger
}

// Mount registra el webhook. Sin token no monta nada y lo dice: un despliegue
// que creía tener recepción por HTTP y no la tiene debe enterarse al arrancar,
// no la primera vez que alguien le escriba.
func Mount(r chi.Router, d Deps) {
	if d.Token == "" || d.Inbound == nil {
		if d.Logger != nil {
			d.Logger.Info("recepción por webhook desactivada (sin token o sin procesador)")
		}
		return
	}
	h := &handler{in: d.Inbound, token: d.Token, logger: d.Logger}
	r.Post(Ruta, h.recibir)
	if d.Logger != nil {
		d.Logger.Info("recepción por webhook activa", "ruta", Ruta)
	}
}

func (h *handler) recibir(w http.ResponseWriter, r *http.Request) {
	if !h.autorizado(r) {
		// 401 y no 403: al reenviador le sirve para distinguir «no me conoces»
		// de «te conozco y no puedes», y a nosotros para verlo en las métricas.
		http.Error(w, "no autorizado", http.StatusUnauthorized)
		return
	}

	de := strings.TrimSpace(r.Header.Get(CabeceraDe))
	para := destinatarios(r.Header.Values(CabeceraPara))
	if de == "" || len(para) == 0 {
		// 400 y no 500: el reenviador no debe reintentar algo que siempre
		// fallará igual. Los reintentos de un webhook mal formado son un bucle.
		http.Error(w, "faltan "+CabeceraDe+" o "+CabeceraPara, http.StatusBadRequest)
		return
	}

	// Se resuelven los destinatarios ANTES de entregar, igual que hace la fase
	// RCPT del MX. Sin esto, un mensaje para una dirección que no existe se
	// procesaría entero —parseo, subida del .eml— para descubrirlo al final.
	locales := make([]string, 0, len(para))
	for _, rcpt := range para {
		if _, err := h.in.Resolve(r.Context(), rcpt); err != nil {
			if errors.Is(err, domain.ErrNoSuchUser) {
				continue
			}
			h.fallo(w, "resolver destinatario", err)
			return
		}
		locales = append(locales, rcpt)
	}
	if len(locales) == 0 {
		// 200 y no error: el destinatario no existe aquí, y eso es una respuesta
		// definitiva. Devolver 5xx haría que el reenviador lo intentara durante
		// días contra una dirección que nunca va a existir.
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("sin destinatarios locales\n"))
		return
	}

	msg := delivery.InboundMessage{
		// La IP del reenviador, no la del remitente. Se pasa igualmente porque
		// el procesador la usa para su límite de tasa, y ahí sí es la correcta:
		// lo que se quiere acotar es cuánto nos mete QUIEN NOS LO ENTREGA.
		RemoteIP:   ipDe(r),
		MailFrom:   de,
		Recipients: locales,
		Body:       http.MaxBytesReader(w, r.Body, maxCuerpo),
	}
	if err := h.in.Deliver(r.Context(), msg); err != nil {
		h.fallo(w, "entregar", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// maxCuerpo es un tope de transporte, no la política de tamaño: la de verdad la
// aplica el procesador con MaxBytes y devuelve ErrMessageTooLarge. Este existe
// para que un cuerpo absurdo no se lea entero antes de llegar a esa política.
const maxCuerpo = 64 << 20

func (h *handler) autorizado(r *http.Request) bool {
	dado := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	// Comparación en tiempo constante: comparar tokens con == filtra su longitud
	// y su prefijo por el tiempo de respuesta.
	return subtle.ConstantTimeCompare([]byte(strings.TrimSpace(dado)), []byte(h.token)) == 1
}

// fallo traduce el error del procesador a un código que le diga al reenviador
// si merece la pena reintentar. Es la misma decisión que toma el MX con sus
// códigos SMTP, y equivocarse aquí cuesta lo mismo: un 5xx sobre algo definitivo
// convierte un mensaje rechazado en un bucle de reintentos de días.
func (h *handler) fallo(w http.ResponseWriter, que string, err error) {
	switch {
	case errors.Is(err, domain.ErrMessageTooLarge):
		http.Error(w, "mensaje demasiado grande", http.StatusRequestEntityTooLarge)
	case errors.Is(err, domain.ErrQuotaExceeded):
		// 507 y no 4xx: la cuota se libera sola, así que reintentar SÍ tiene
		// sentido. Es el equivalente al 452 del SMTP.
		http.Error(w, "cuota superada", http.StatusInsufficientStorage)
	case errors.Is(err, delivery.ErrRejectedPolicy):
		http.Error(w, "rechazado por la política DMARC del remitente", http.StatusUnprocessableEntity)
	default:
		if h.logger != nil {
			h.logger.Error("webhook de entrada", "que", que, "err", err)
		}
		http.Error(w, fmt.Sprintf("no se pudo %s", que), http.StatusInternalServerError)
	}
}

// destinatarios acepta las dos formas —cabecera repetida o lista con comas—
// porque no todos los reenviadores dejan repetir una cabecera.
func destinatarios(valores []string) []string {
	var out []string
	for _, v := range valores {
		for _, parte := range strings.Split(v, ",") {
			if p := strings.TrimSpace(parte); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}

func ipDe(r *http.Request) net.IP {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return net.ParseIP(host)
}
