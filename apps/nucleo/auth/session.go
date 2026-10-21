// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package auth mantiene la sesión contra Sytue ID.
//
// Hay UNA sesión y UN margen de caducidad. En el escritorio Fyne eran tres copias con
// tres márgenes (30 s en la web, 1 min el vigilante del Hub, 2 min el cliente de API),
// que es como una app acaba mandando tokens ya caducados sin que nadie lo vea.
//
// Esto es la sesión de las apps NATIVAS: Authorization Code + PKCE y refresh token
// (oidc.go), contra un cliente público. El camino de vuelta del navegador es lo único
// que cambia entre plataformas, y RFC 8252 ya lo separa: en el escritorio, un puerto del
// propio equipo (§7.3, `sytue-desktop`); en el móvil, el esquema propio de cada app
// (§7.1, `sytue-mobile`).
//
// La web no tiene sesión aquí, y es a propósito: allí el token no baja al navegador. Lo
// guarda el BFF tras una cookie httpOnly y lo pone él en cada petición que pasa por su
// proxy, así que ningún script de la página lo alcanza.
package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
)

// ErrLoginRequired: no queda nada que renovar; hay que volver a pasar por Sytue ID.
var ErrLoginRequired = errors.New("auth: hace falta iniciar sesión")

// guardado es lo ÚNICO que llega al almacén seguro del sistema —el llavero en el
// escritorio, el Keystore en Android, el Keychain en iOS—: el refresh token y de quién
// es. El access token se vuelve a pedir al arrancar: dura minutos, y con él dentro la
// entrada no cabría en Windows, donde una credencial admite 2560 bytes.
type guardado struct {
	Refresh string `json:"refresh"`
	Issuer  string `json:"issuer,omitempty"`
	Cliente string `json:"client,omitempty"`
	Subject string `json:"subject,omitempty"`
	// Org es la organización activa al guardar: sin red al arrancar no hay access token,
	// y lo que se guarda sin conexión va por persona Y organización.
	Org string `json:"org,omitempty"`
	// OrgColor y OrgForma son la apariencia por defecto de esa organización (ver
	// Tokens): se guardan para pintar bien desde el primer fotograma, sin red.
	OrgColor string `json:"org_color,omitempty"`
	OrgForma string `json:"org_forma,omitempty"`
	// Guardado es cuándo se escribió, en nanosegundos Unix: en un Linux que ha tenido la
	// sesión a ratos en el llavero y a ratos en el fichero cifrado, manda la más nueva.
	Guardado int64 `json:"guardado,omitempty"`
}

// serializar y recuperar son ese formato, UNO para las tres plataformas: lo que cambia
// entre ellas es dónde se deja, no qué.
func serializar(t Tokens) ([]byte, error) {
	return json.Marshal(guardado{
		Refresh: t.Refresh, Issuer: t.Issuer, Cliente: t.Cliente, Subject: t.Subject, Org: organizacionDe(t),
		OrgColor: t.OrgColor, OrgForma: t.OrgForma, Guardado: time.Now().UnixNano(),
	})
}

func recuperar(crudo []byte) (Tokens, error) {
	t, _, err := recuperarConInstante(crudo)
	return t, err
}

// recuperarConInstante es recuperar con el instante en que se guardó (0 si es de antes de
// que se apuntara).
func recuperarConInstante(crudo []byte) (Tokens, int64, error) {
	var g guardado
	if err := json.Unmarshal(crudo, &g); err != nil {
		return Tokens{}, 0, err
	}
	return Tokens{
		Refresh: g.Refresh, Issuer: g.Issuer, Cliente: g.Cliente, Subject: g.Subject, Organizacion: g.Org,
		OrgColor: nombreDeApariencia(g.OrgColor), OrgForma: nombreDeApariencia(g.OrgForma),
	}, g.Guardado, nil
}

// El SOBRE es el formato del fichero de Android (store_android.go), donde lo guardado
// va cifrado y hay que llevar al lado el vector de inicialización que eligió GCM —no
// es secreto, pero sin él no se descifra—:
//
//	[1 byte con la longitud del IV][IV][criptograma]
//
// Vive aquí, sin etiqueta de plataforma, porque es lo único de ese almacén que se puede
// probar en una máquina que no sea un teléfono. El llavero del escritorio y el Keychain
// de iOS no lo necesitan: guardan el valor entero y lo devuelven entero.
func empaquetar(iv, cifrado []byte) ([]byte, error) {
	// El IV de AES/GCM son 12 bytes; el byte de longitud da margen por si un proveedor
	// eligiera otro tamaño, y 255 es todo lo que cabe ahí.
	if len(iv) == 0 || len(iv) > 255 || len(cifrado) == 0 {
		return nil, fmt.Errorf("auth: no se puede guardar un sobre de IV %d y criptograma %d", len(iv), len(cifrado))
	}
	sobre := make([]byte, 0, 1+len(iv)+len(cifrado))
	sobre = append(sobre, byte(len(iv))) //nolint:gosec // G115: len(iv) está acotado a 255 justo arriba
	sobre = append(sobre, iv...)
	return append(sobre, cifrado...), nil
}

func desempaquetar(sobre []byte) (iv, cifrado []byte, err error) {
	if len(sobre) == 0 || 1+int(sobre[0]) >= len(sobre) {
		return nil, nil, errors.New("auth: la sesión guardada está truncada")
	}
	return sobre[1 : 1+int(sobre[0])], sobre[1+int(sobre[0]):], nil
}

// Margen es con cuánta vida por delante se da un token por bueno. Es el de
// `getAccessToken` en @sytue/auth/client.
const Margen = 30 * time.Second

// Tokens es la sesión tal y como se guarda.
type Tokens struct {
	// Issuer y Cliente atan el refresh al emisor y cliente que lo emitieron. Las
	// apps comparten una sesión sólo cuando ambos coinciden.
	Issuer  string    `json:"issuer,omitempty"`
	Cliente string    `json:"client,omitempty"`
	Access  string    `json:"access"`
	Refresh string    `json:"refresh,omitempty"`
	Subject string    `json:"subject,omitempty"`
	Expiry  time.Time `json:"expiry"`
	// Organizacion es la última organización conocida, la que se guarda al lado del
	// `sub`: sin red, al arrancar no hay access token del que leerla.
	Organizacion string `json:"organizacion,omitempty"`
	// OrgColor y OrgForma son el color y la forma por defecto de la organización, que fija
	// su administrador; vacíos si son los de la casa. Sytue ID los manda SOLO en el ID
	// token (reclamos `org_color` y `org_forma`), y son lo que pinta quien no ha elegido
	// los suyos. La web los recibe igual y su BFF los deja en una cookie.
	OrgColor string `json:"org_color,omitempty"`
	OrgForma string `json:"org_forma,omitempty"`
	// conIDToken dice que estos tokens vienen con su ID token: sin él, la apariencia de
	// la organización no se sabe y se conserva la que había (ver conservar).
	conIDToken bool
	// persistenciaAtendida solo acompaña el resultado de Compartida: el almacén ya
	// se ha atendido bajo su cerrojo, incluso si guardar falló. Se consume antes de
	// poner los tokens en memoria; no se serializa ni se hereda en otra renovación.
	persistenciaAtendida bool
}

// nombreDeApariencia deja pasar solo un nombre de color o de forma: letras minúsculas, o
// nada. Es la misma regla con la que la web lo lleva a una cookie.
func nombreDeApariencia(v string) string {
	if v == "" || len(v) > 16 {
		return ""
	}
	for _, c := range v {
		if c < 'a' || c > 'z' {
			return ""
		}
	}
	return v
}

// Renewer consigue tokens nuevos a partir de los que hay. Devuelve ErrLoginRequired
// cuando el IdP dice que esa sesión ya no vale; cualquier otro error es pasajero.
type Renewer func(ctx context.Context, actuales Tokens) (Tokens, error)

// Session entrega siempre un access token vigente. Es segura entre goroutines.
type Session struct {
	mu      sync.Mutex
	tokens  Tokens
	renovar Renewer
	guardar func(Tokens) error
	ahora   func() time.Time
}

// NewSession arranca con los tokens que hubiera guardados (o ninguno). `guardar` recibe
// cada juego nuevo —y uno vacío al cerrar sesión—; en la web es nil.
func NewSession(guardados Tokens, renovar Renewer, guardar func(Tokens) error) *Session {
	guardados.persistenciaAtendida = false
	return &Session{tokens: guardados, renovar: renovar, guardar: guardar, ahora: time.Now}
}

// AccessToken devuelve el token vigente, renovándolo antes si le queda menos del Margen.
func (s *Session) AccessToken(ctx context.Context) (string, error) {
	// ponytail: el cerrojo se mantiene durante la renovación. Eso YA es el «una sola
	// petición en vuelo» de la web: quien llega mientras tanto espera y se encuentra
	// el token nuevo. A cambio, esa espera no atiende a su propio ctx; si algún día
	// molesta, singleflight.
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tokens.Access != "" && s.tokens.Expiry.Sub(s.ahora()) > Margen {
		return s.tokens.Access, nil
	}
	nuevos, err := s.renovar(ctx, s.tokens)
	switch {
	case errors.Is(err, ErrLoginRequired):
		// Descartar todos los datos parciales del renovador, conservando solo que
		// Compartida ya ha limpiado el almacén antes de soltar su cerrojo.
		s.poner(Tokens{persistenciaAtendida: nuevos.persistenciaAtendida})
		return "", err
	case err != nil:
		// Un fallo de red no cierra la sesión: si el token aún no ha caducado del
		// todo, se sigue con él y ya se renovará en la siguiente.
		if s.tokens.Access != "" && s.tokens.Expiry.After(s.ahora()) {
			return s.tokens.Access, nil
		}
		return "", err
	}
	s.poner(conservar(nuevos, s.tokens))
	return nuevos.Access, nil
}

// Invalidate marca como gastado el token que un servicio acaba de rechazar con 401,
// para que la siguiente llamada renueve. Solo si SIGUE siendo el actual: dos peticiones
// rechazadas a la vez no deben provocar dos renovaciones.
func (s *Session) Invalidate(rechazado string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tokens.Access == rechazado {
		s.tokens.Expiry = time.Time{}
	}
}

// Adoptar mete en la sesión los tokens recién traídos del IdP y los guarda, de forma que
// las otras apps de la suite los encuentren. Es lo que se llama al volver del navegador.
//
// Existe para no tener que CAMBIAR el *Session después de entrar: los clientes de `api`
// se quedan con el puntero que se les dio al construirlos, así que sustituirlo los
// dejaba hablando con la sesión vacía de antes del login.
func (s *Session) Adoptar(t Tokens) {
	if err := s.AdoptarCon(context.Background(), t); err != nil {
		slog.Warn("auth: no se pudo bloquear el inicio de sesión", "error", err)
	}
}

// AdoptarCon es Adoptar con el plazo del login para esperar una renovación de otra
// app. Un login cancelado no escribe después de que su contexto haya terminado.
func (s *Session) AdoptarCon(ctx context.Context, t Tokens) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	soltar, err := ajustes.Cerrojo(ctx)
	if err != nil {
		return err
	}
	defer soltar()
	// Adoptar siempre guarda, incluso si le reenvían un resultado de Compartida.
	t.persistenciaAtendida = false
	s.poner(t)
	return nil
}

// PonerApariencia apunta la apariencia de la organización que OTRA app de la suite ha
// dejado guardada con la sesión al entrar: esta todavía no ha renovado, y sin esto
// seguiría pintando la de antes hasta que lo hiciera.
func (s *Session) PonerApariencia(colorSuite, forma string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens.OrgColor, s.tokens.OrgForma = nombreDeApariencia(colorSuite), nombreDeApariencia(forma)
}

// Apariencia son el color y la forma por defecto de la organización de la sesión, o
// vacíos si son los de la casa o no hay sesión.
func (s *Session) Apariencia() (colorSuite, forma string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tokens.OrgColor, s.tokens.OrgForma
}

// Subject es el `sub` de la persona, o "" sin sesión. Es el dueño de lo que se guarda
// sin conexión.
func (s *Session) Subject() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tokens.Subject
}

// Grupos son los grupos de Sytue ID de la persona, del reclamo `groups` del access
// token, SIN el pseudo-grupo de la organización (`org/<slug>`), que no es un grupo con el
// que se pueda compartir. Es lo que hace el BFF de Drive en `pages/api/groups.ts`.
//
// Se lee sin verificar la firma, como el `sub` de `subDe`: solo sirve para OFRECER los
// grupos en un selector. Quien autoriza es el servicio, que verifica el token.
func (s *Session) Grupos() []string {
	s.mu.Lock()
	access := s.tokens.Access
	s.mu.Unlock()
	var reclamos struct {
		Groups []string `json:"groups"`
	}
	if !reclamosDe(access, &reclamos) {
		return nil
	}
	out := make([]string, 0, len(reclamos.Groups))
	for _, g := range reclamos.Groups {
		if !strings.HasPrefix(strings.TrimPrefix(g, "/"), "org/") {
			out = append(out, g)
		}
	}
	return out
}

// Organizacion es la organización activa de la persona, del reclamo `tenant` del
// access token o, antes del primer token, la que se guardó con la sesión (lo guardado
// trae el refresh, no el access); "" sin sesión. Es la que llevan los enlaces entre apps como segmento del path
// (ADR 0056): nombra el destino, no autoriza nada —eso lo sigue decidiendo el token
// que cada petición lleva—.
//
// Se lee sin verificar la firma, como el `sub` de [Subject] y los grupos de [Grupos]:
// es un dato para COMPOSER enlaces, y no hay enlace cuyo valor dé un permiso.
func (s *Session) Organizacion() string {
	s.mu.Lock()
	t := s.tokens
	s.mu.Unlock()
	return organizacionDe(t)
}

func organizacionDe(t Tokens) string {
	var reclamos struct {
		Tenant string `json:"tenant"`
	}
	if reclamosDe(t.Access, &reclamos) && reclamos.Tenant != "" {
		return reclamos.Tenant
	}
	return t.Organizacion
}

// reclamosDe desarma la carga de un JWT en `en`. Dice si pudo.
func reclamosDe(jwt string, en any) bool {
	partes := strings.Split(jwt, ".")
	if len(partes) != 3 {
		return false
	}
	crudo, err := base64.RawURLEncoding.DecodeString(partes[1])
	return err == nil && json.Unmarshal(crudo, en) == nil
}

// Sincronizar adopta lo que otra app dejó en el almacén sin volver a guardarlo. Un
// cambio de cuenta invalida inmediatamente el access token anterior. Un cierre no
// genera otra marca: evitaría que las apps se notificaran el mismo cierre sin fin.
func (s *Session) Sincronizar(t Tokens) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t.Refresh == "" || t.Subject != s.tokens.Subject || t.Organizacion != organizacionDe(s.tokens) {
		t.persistenciaAtendida = false
		s.tokens = t
		return
	}
	s.tokens.Refresh = t.Refresh
	s.tokens.Issuer, s.tokens.Cliente = t.Issuer, t.Cliente
	s.tokens.Organizacion = t.Organizacion
	s.tokens.OrgColor, s.tokens.OrgForma = t.OrgColor, t.OrgForma
}

// Logout borra la sesión bajo el mismo cerrojo que su renovación. El orden de
// bloqueos es el de AccessToken: primero esta Session, después el de los procesos.
// Se espera al propietario vivo; salir con un plazo y escribir sin exclusión
// permitiría que su renovación guardara después y resucitara la sesión.
func (s *Session) Logout() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if soltar, err := ajustes.Cerrojo(context.Background()); err == nil {
		defer soltar()
	} else {
		slog.Warn("auth: no se pudo bloquear el cierre de sesión", "error", err)
	}
	s.poner(Tokens{})
}

func (s *Session) poner(t Tokens) {
	atendida := t.persistenciaAtendida
	t.persistenciaAtendida = false
	s.tokens = t
	if atendida {
		return
	}
	guardarTokens(t, s.guardar)
}

func guardarTokens(t Tokens, guardar func(Tokens) error) {
	if guardar == nil {
		return
	}
	// No poder guardar no invalida el token: la sesión vale para esta ejecución y en
	// la siguiente se pedirá login. Se deja dicho, no se esconde.
	if err := guardar(t); err != nil {
		slog.Warn("auth: no se pudo guardar la sesión", "error", err)
	}
}

// Compartida envuelve una renovación para cuando VARIOS procesos comparten la sesión
// guardada: las apps de escritorio son trece binarios y una sola entrada del llavero.
//
// Sytue ID rota el refresh token en cada uso y, si le presentan uno ya gastado, revoca la
// familia entera. Con Mail y Calendar abiertas a la vez, la que renovara segunda con el
// token que tenía en memoria —ya rotado por la otra— las dejaba a las dos sin sesión. Así
// que antes de renovar se mira lo guardado: si hay otro refresh token, manda ese; si no
// hay ninguno, otra app ha cerrado la sesión y esta no la resucita.
//
// Y eso NO basta por sí solo. Dos apps que caduquen a la vez leen las dos el mismo token
// antes de que ninguna guarde el nuevo, así que las dos lo presentan y la segunda tumba a
// las dos. Por eso las tres cosas —leer lo guardado, pedir el token nuevo y guardarlo—
// van dentro de un cerrojo ENTRE PROCESOS (apps/nucleo/ajustes.Cerrojo); la que llega
// segunda espera, encuentra el token ya rotado y no llega a presentar el viejo. El
// resultado indica a Session que no vuelva a persistir fuera del cerrojo: otra app
// podría haber rotado otra vez mientras tanto. Lo prueba
// TestSessionCompartidaNoSobrescribeUnaRotacionPosterior.
//
// Si no puede obtenerse el cerrojo, se conserva la sesión y se devuelve el error.
// Renovar sin él podría reutilizar un refresh token rotado y revocar toda la familia.
// Con guardar=nil no se marca persistencia atendida: Session conserva su guardado normal.
func Compartida(renovar Renewer, cargar func() (Tokens, error), guardar func(Tokens) error) Renewer {
	persistir := func(t Tokens) Tokens {
		t.persistenciaAtendida = false
		if guardar != nil {
			guardarTokens(t, guardar)
			// También tras un fallo: reintentar fuera del cerrojo podría pisar una
			// sesión posterior. El error ya se ha avisado y el token sigue en memoria.
			t.persistenciaAtendida = true
		}
		return t
	}
	return func(ctx context.Context, actuales Tokens) (Tokens, error) {
		actuales.persistenciaAtendida = false
		soltar, err := ajustes.Cerrojo(ctx)
		if err != nil {
			return Tokens{}, err
		}
		defer soltar()
		guardados, err := cargar()
		switch {
		case err != nil:
			// Lo guardado no se deja leer: se sigue con lo que hay en memoria.
		case guardados.Refresh == "":
			// El almacén ya está vacío; no borrarlo de nuevo después del cerrojo.
			return Tokens{persistenciaAtendida: guardar != nil}, ErrLoginRequired
		default:
			// La cuenta puede haber cambiado en otra app. El refresh y su dueño
			// son una sola unidad; no se conserva el sub de la sesión anterior.
			if actuales.Subject != guardados.Subject {
				actuales = guardados
			}
			actuales.Refresh, actuales.Subject = guardados.Refresh, guardados.Subject
			actuales.Issuer, actuales.Cliente = guardados.Issuer, guardados.Cliente
			actuales.Organizacion = guardados.Organizacion
			actuales.OrgColor, actuales.OrgForma = guardados.OrgColor, guardados.OrgForma
		}
		nuevos, err := renovar(ctx, actuales)
		nuevos.persistenciaAtendida = false
		if errors.Is(err, ErrLoginRequired) {
			return persistir(Tokens{}), err
		}
		if err != nil {
			return nuevos, err
		}
		return persistir(conservar(nuevos, actuales)), nil
	}
}

// conservar rellena con lo viejo lo que el IdP no ha mandado: puede no rotar el refresh
// token, y el `sub` solo viene en el ID token del login.
func conservar(nuevos, viejos Tokens) Tokens {
	if nuevos.Refresh == "" {
		nuevos.Refresh = viejos.Refresh
	}
	if nuevos.Subject == "" {
		nuevos.Subject = viejos.Subject
	}
	if nuevos.Issuer == "" {
		nuevos.Issuer = viejos.Issuer
	}
	if nuevos.Cliente == "" {
		nuevos.Cliente = viejos.Cliente
	}
	// Con ID token, lo que diga —vacío es «la de la casa»—; sin él, lo que se sabía.
	if !nuevos.conIDToken {
		nuevos.OrgColor, nuevos.OrgForma = viejos.OrgColor, viejos.OrgForma
	}
	return nuevos
}
