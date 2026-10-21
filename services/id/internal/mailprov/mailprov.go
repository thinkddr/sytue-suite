// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package mailprov crea en Sytue Mail las cuentas de los buzones que Sytue ID ha
// asignado al dar de alta a alguien.
//
// POR QUÉ HACE FALTA EMPUJAR NADA: las direcciones se asignan en la base de Sytue
// ID (migración 0048) pero quien resuelve el destinatario de un correo entrante es
// Sytue Mail contra la SUYA. Mientras la cuenta no exista allí, el MX contesta 550
// «No such user here» y el correo dirigido a esa persona se pierde con un rebote.
// Antes eso pasaba con todo el mundo hasta que abría el webmail por primera vez;
// un alta no puede depender de que la persona se conecte para poder recibir.
//
// POR QUÉ NO HAY UN ENDPOINT NUEVO EN SYTUE MAIL: Mail ya sabe hacer las dos cosas
// que hacen falta —crear la cuenta al abrir la sesión JMAP y registrar un alias por
// su REST del dueño— así que aquí se recorren esos MISMOS caminos en lugar de abrir
// una segunda puerta de aprovisionamiento que habría que mantener, autorizar y
// auditar en paralelo. Sytue ID emite un token de vida muy corta a nombre del
// usuario, con scope de correo y su dirección ya asignada, y llama: Mail crea la
// cuenta y sus carpetas exactamente igual que si la persona acabara de entrar, y
// acepta el alias como si lo hubiera pedido ella. La diferencia es quién llama.
//
// POR QUÉ UN RECONCILIADOR Y NO UNA LLAMADA EN EL ALTA: las dos bases no comparten
// transacción. Empujar dentro del alta obligaría a decidir qué hacer si Mail está
// caído —¿se cae el alta entera?— y a repetir esa decisión en cada una de las vías
// que crean membresías. Con una cola en la propia tabla, el alta solo escribe en su
// base, y crear la cuenta se reintenta solo hasta que salga.
package mailprov

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"

	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// lote acota cuántos buzones se crean por barrido. Un alta masiva por SCIM puede
// dejar miles pendientes de golpe y no tiene sentido dispararle a Sytue Mail miles
// de peticiones seguidas: se van drenando barrido a barrido.
const lote = 50

// Espera entre reintentos de una fila que Sytue Mail rechazó de forma PERMANENTE
// (un 4xx: la petición es inválida y repetirla igual no la va a validar). Sin
// esto, una sola fila mal formada se reintenta al ritmo del ticker para siempre:
// en producción llegó a generar 4 peticiones por minuto y otras tantas trazas de
// error, indefinidamente, ahogando en ruido cualquier fallo de verdad.
//
// Se reintenta igualmente, espaciando: el rechazo puede depender de algo externo
// que sí cambia (la cuenta que aún no existía, un dominio recién dado de alta), y
// abandonar la fila dejaría a alguien sin su dirección sin que nadie lo mirase.
const (
	esperaInicial = 2 * time.Minute
	esperaMaxima  = time.Hour
)

// errHTTP es una respuesta no esperada de Sytue Mail. Se tipa para poder
// distinguir el 4xx (permanente) del 5xx o el fallo de red (transitorio).
type errHTTP struct {
	metodo, ruta string
	status       int
}

func (e *errHTTP) Error() string {
	return fmt.Sprintf("mailprov: %s %s respondió %d", e.metodo, e.ruta, e.status)
}

// permanente indica que repetir la misma petición no puede cambiar el resultado.
func permanente(err error) bool {
	var e *errHTTP
	return errors.As(err, &e) && e.status >= 400 && e.status < 500
}

// MintToken emite un access token de Sytue Mail a nombre de un usuario. Lo inyecta
// quien construye el Provisioner porque el emisor de tokens vive en el arranque del
// servicio, no aquí.
type MintToken func(subject, tenant, address, name string) (string, error)

// Provisioner crea en Sytue Mail las cuentas pendientes.
type Provisioner struct {
	// Hub, si lo hay, borra los avisos de las direcciones que se retiran (ADR 0053).
	Hub *hubnotify.Client

	st      *store.Store
	baseURL string
	mint    MintToken
	log     *slog.Logger
	cl      *http.Client
	// espera acota los reintentos de las filas rechazadas de forma permanente.
	// Solo lo toca Run, que corre en serie desde un único ticker, así que no
	// necesita candado.
	// ponytail: en memoria; un reinicio reintenta antes de tiempo, que es
	// inofensivo. Si hiciera falta que sobreviva, va como columna de la fila.
	espera map[string]reintento
}

// reintento es hasta cuándo no volver a intentar una fila, y con qué tramo.
type reintento struct {
	hasta time.Time
	tramo time.Duration
}

// New construye el Provisioner. baseURL vacía lo deja inactivo: sin coordenadas de
// Sytue Mail no hay a quién empujar, y las filas se quedan en la cola —que es la
// situación correcta— en lugar de fallar en bucle.
func New(st *store.Store, baseURL string, mint MintToken, logger *slog.Logger) *Provisioner {
	return &Provisioner{
		st:      st,
		baseURL: strings.TrimRight(baseURL, "/"),
		mint:    mint,
		log:     logger,
		espera:  map[string]reintento{},
		// Timeout corto: crear una cuenta es una escritura pequeña. Si Sytue Mail
		// tarda más, es que no está bien, y conviene soltar el barrido y reintentar
		// en el siguiente antes que dejar peticiones colgando.
		cl: &http.Client{Timeout: 10 * time.Second},
	}
}

// Enabled indica si hay coordenadas de Sytue Mail configuradas.
func (p *Provisioner) Enabled() bool { return p.baseURL != "" && p.mint != nil }

// Run drena la cola de buzones pendientes. Devuelve cuántas cuentas quedaron
// creadas. Un fallo con una persona no detiene a las demás: se anota y se
// reintentará en el siguiente barrido, porque su fila sigue sin sello.
func (p *Provisioner) Run(ctx context.Context) (int, error) {
	if !p.Enabled() {
		return 0, nil
	}
	pendientes, err := p.st.PendingMailboxes(ctx, lote)
	if err != nil {
		return 0, err
	}
	hechos := 0
	ahora := time.Now()
	vigentes := make(map[string]struct{}, len(pendientes))
	for _, m := range pendientes {
		if ctx.Err() != nil {
			// Se devuelve la cancelación en vez de nil: el barrido quedó a medias y
			// decirle al que llama que fue bien es mentirle. Su bucle ya distingue
			// el apagado (`ctx.Err() == nil`) y no registra ruido al parar.
			return hechos, ctx.Err()
		}
		vigentes[m.ID] = struct{}{}
		if r, ok := p.espera[m.ID]; ok && ahora.Before(r.hasta) {
			continue
		}
		if err := p.provision(ctx, m); err != nil {
			// Sin dirección en el registro: es un dato personal y el identificador
			// de la fila basta para seguirle la pista (RGPD art. 25).
			campos := []any{"error", err, "buzon", m.ID, "org", m.OrgSlug}
			if hasta := p.posponer(m.ID, err, ahora); !hasta.IsZero() {
				campos = append(campos, "proximo_intento", hasta)
			}
			p.log.WarnContext(ctx, "no se pudo crear el buzón en Sytue Mail", campos...)
			continue
		}
		delete(p.espera, m.ID)
		if err := p.st.MarkMailboxProvisioned(ctx, m.ID); err != nil {
			// La cuenta ya existe en Mail pero no se pudo sellar aquí. El siguiente
			// barrido lo repetirá; crear la cuenta es idempotente, así que repetirlo
			// no rompe nada.
			p.log.WarnContext(ctx, "buzón creado pero no se pudo marcar", "error", err, "buzon", m.ID)
			continue
		}
		hechos++
	}
	// Lo que ya no está en la cola tampoco tiene que ocupar sitio en el mapa.
	for id := range p.espera {
		if _, sigue := vigentes[id]; !sigue {
			delete(p.espera, id)
		}
	}
	return hechos + p.retirar(ctx), nil
}

// posponer espacia el siguiente intento de una fila que Sytue Mail rechazó de
// forma permanente y devuelve cuándo será. Para un fallo transitorio (5xx, red,
// Mail caído) devuelve el cero: esos sí se reintentan al ritmo del barrido,
// porque lo que falla es el otro extremo y puede volver en cualquier momento.
func (p *Provisioner) posponer(id string, err error, ahora time.Time) time.Time {
	if !permanente(err) {
		return time.Time{}
	}
	r := p.espera[id]
	r.tramo = min(max(2*r.tramo, esperaInicial), esperaMaxima)
	r.hasta = ahora.Add(r.tramo)
	p.espera[id] = r
	return r.hasta
}

// retirar quita de Sytue Mail los alias que se han eliminado en la consola y borra
// entonces su fila. Sin esto, «eliminar» en la consola dejaría la dirección
// entregando en Sytue Mail: quien la quitó creería haber terminado y el correo
// seguiría llegando, que es la clase de sorpresa que no se descubre hasta tarde.
func (p *Provisioner) retirar(ctx context.Context) int {
	pendientes, err := p.st.PendingMailboxRemovals(ctx, lote)
	if err != nil {
		p.log.WarnContext(ctx, "no se pudieron leer las direcciones a retirar", "error", err)
		return 0
	}
	hechos := 0
	for _, m := range pendientes {
		if ctx.Err() != nil {
			return hechos
		}
		if err := p.retirarUna(ctx, m); err != nil {
			p.log.WarnContext(ctx, "no se pudo retirar la dirección de Sytue Mail",
				"error", err, "buzon", m.ID, "org", m.OrgSlug)
			continue
		}
		// Sus avisos, fuera de la campana de todas las organizaciones ANTES de dejar la
		// dirección libre: quien la reciba no debe heredarlos (ADR 0053). Si el Hub no
		// contesta, la fila se queda y el siguiente barrido lo reintenta: liberar la
		// dirección sin olvidar sus avisos se los daría al próximo titular.
		if p.Hub != nil && m.Address != "" {
			if err := p.Hub.OlvidarBuzon(ctx, m.Address); err != nil {
				p.log.WarnContext(ctx, "el Hub no olvidó los avisos de la dirección; se reintenta", "error", err, "buzon", m.ID)
				continue
			}
		}
		if err := p.st.PurgeMailbox(ctx, m.ID); err != nil {
			p.log.WarnContext(ctx, "dirección retirada pero no se pudo purgar", "error", err, "buzon", m.ID)
			continue
		}
		hechos++
	}
	return hechos
}

// retirarUna localiza el alias en Sytue Mail por su dirección y lo borra. Hay que
// listar antes porque Sytue Mail identifica sus alias por un id propio que aquí no
// se guarda: duplicar ese identificador obligaría a mantener sincronizadas dos
// claves entre dos bases, y basta con preguntar.
func (p *Provisioner) retirarUna(ctx context.Context, m store.MailboxPendiente) error {
	if m.PrimaryAddress == "" {
		// Su dueño ya no tiene buzón (se le quitó entero): no hay a quién pedirle la
		// baja del alias, y la cuenta que lo sostenía tampoco está.
		return nil
	}
	token, err := p.mint(m.UserID, m.OrgSlug, m.PrimaryAddress, m.FullName)
	if err != nil {
		return fmt.Errorf("mailprov: emitir token: %w", err)
	}
	var lista struct {
		Aliases []struct {
			ID      string `json:"id"`
			Address string `json:"address"`
		} `json:"aliases"`
	}
	if err := p.leer(ctx, "/api/v1/aliases", token, &lista); err != nil {
		return err
	}
	for _, a := range lista.Aliases {
		if strings.EqualFold(a.Address, m.Address) {
			return p.llamar(ctx, http.MethodDelete, "/api/v1/aliases/"+url.PathEscape(a.ID), token, nil)
		}
	}
	// No está: o nunca llegó a crearse o ya se borró. En los dos casos el estado
	// deseado se cumple, así que la fila puede purgarse.
	return nil
}

// leer hace un GET autenticado a Sytue Mail y decodifica la respuesta.
func (p *Provisioner) leer(ctx context.Context, ruta, token string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+ruta, http.NoBody)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := p.cl.Do(req)
	if err != nil {
		return fmt.Errorf("mailprov: GET %s: %w", ruta, err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 1<<20))
		return &errHTTP{metodo: http.MethodGet, ruta: ruta, status: res.StatusCode}
	}
	return json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(dest)
}

// provision deja existiendo en Sytue Mail una dirección ya asignada.
//
// El token se emite SIEMPRE con la dirección PRINCIPAL de su dueño, también cuando
// lo que se registra es un alias: en Sytue Mail el alias pertenece a la cuenta del
// llamante, así que hay que presentarse como el dueño para que se cuelgue de la
// cuenta correcta. Presentarse con la dirección del alias abriría una cuenta nueva.
func (p *Provisioner) provision(ctx context.Context, m store.MailboxPendiente) error {
	principal := m.Address
	if !m.Primary {
		principal = m.PrimaryAddress
	}
	if principal == "" {
		// Alias sin dirección principal: su dueño aún no tiene buzón. No es un fallo
		// que reintentar a ciegas, es que todavía no toca.
		return fmt.Errorf("mailprov: el alias %s no tiene buzón principal todavía", m.ID)
	}
	token, err := p.mint(m.UserID, m.OrgSlug, principal, m.FullName)
	if err != nil {
		return fmt.Errorf("mailprov: emitir token: %w", err)
	}
	// La sesión crea la cuenta si no existe (idempotente). Se pide también para los
	// alias porque es la garantía barata de que la cuenta a la que colgarlos está.
	if err := p.llamar(ctx, http.MethodGet, "/jmap/session", token, nil); err != nil {
		return err
	}
	if m.Primary {
		return nil
	}
	cuerpo, err := json.Marshal(map[string]string{"address": m.Address})
	if err != nil {
		return err
	}
	return p.llamar(ctx, http.MethodPost, "/api/v1/aliases", token, cuerpo)
}

// llamar hace una petición autenticada a Sytue Mail y traduce el resultado.
//
// El 409 cuenta como éxito: significa que la dirección ya está registrada, que es
// justo el estado al que se quería llegar. Tratarlo como fallo dejaría reintentando
// para siempre una fila que en realidad ya está hecha — el caso de haber empujado
// bien y no haber podido sellarlo aquí.
func (p *Provisioner) llamar(ctx context.Context, metodo, ruta, token string, cuerpo []byte) error {
	var body io.Reader
	if cuerpo != nil {
		body = bytes.NewReader(cuerpo)
	}
	req, err := http.NewRequestWithContext(ctx, metodo, p.baseURL+ruta, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if cuerpo != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := p.cl.Do(req)
	if err != nil {
		return fmt.Errorf("mailprov: %s %s: %w", metodo, ruta, err)
	}
	defer func() { _ = res.Body.Close() }()
	// El cuerpo se descarta pero se drena: sin leerlo, la conexión no vuelve al
	// pool y cada alta abriría una nueva.
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 1<<20))
	if res.StatusCode >= 300 && res.StatusCode != http.StatusConflict {
		return &errHTTP{metodo: metodo, ruta: ruta, status: res.StatusCode}
	}
	return nil
}
