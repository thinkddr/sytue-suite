// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets"
)

// # El despertar de iOS: el Hub despierta la app para su widget
//
// Con la app cerrada, cuando cambia algo de su producto, el Hub le manda un aviso VACÍO
// de APNs (ADR 0039 y 0078, enmiendas del 2026-11-06), la app lee lo suyo con la misma
// entrada que el refresco periódico (RefrescarWidgetEnFondo) y repinta el widget. Aquí
// se decide cuándo registrarse en el Hub y cuándo olvidarse:
//
//   - se registra SOLO con «Actualizar los widgets con la app cerrada» encendido
//     por quien está dentro (ajustes.DespertarWidgets y DespertarDe, apagado por
//     defecto), un widget suyo puesto
//     (widgets.Instalados), sesión y sin el bloqueo de la suite, y con los datos
//     protegidos a mano. Cada arranque vuelve a registrarse: el token puede cambiar, y es
//     lo que mantiene viva la fila (el Hub purga a los 30 días);
//   - se olvida (`forget` con su token) cuando deja de cumplirse algo de eso, y la
//     condición es de esta app (quitar su último widget);
//   - lo que es de las quince —salir, poner el bloqueo, apagar el interruptor, cambiar
//     de organización— lo hace
//     la app en que ocurre, con TODOS los tokens de este aparato (olvidarDespertares):
//     las demás pueden no estar corriendo, y sin sesión ya no podrían. Va antes de que
//     la sesión deje de valer, y hasta que el estado cambie ninguna vuelta en curso
//     vuelve a registrar (despertarSuite.vetado).
//
// El token de cada app se guarda en el almacén seguro, en `despertar-<bundle>` y en el
// grupo del llavero de TODA la suite (auth.PrefijoDespertar): para saber qué olvidar al
// salir, y si hace falta volver a registrarse. Al salir se recorre la lista cerrada
// bundlesDespertar, y no el índice de cuentas del almacén, que en Apple es de cada app. La API es la opcional del driver móvil del fork
// (`mobile.RemoteNotificationDriver`, parche apps/fyne/upstream/0017), pedida por
// aserción como la de fondo.go: sin ella, nada de esto existe.

// conductorAvisos es mobile.RemoteNotificationDriver del fork, con sus mismos métodos.
type conductorAvisos interface {
	RemoteNotificationsAvailable() bool
	RegisterForRemoteNotifications(f func(token string, err error))
	OnRemoteNotification(f func(ctx context.Context) (newData, ok bool))
}

const (
	// prefijoDespertar es la cuenta del almacén seguro con el token APNs de cada app.
	prefijoDespertar = auth.PrefijoDespertar
	// plazoToken es lo que se espera a que el sistema dé el token.
	plazoToken = 15 * time.Second
	// plazoHubDespertar es lo que tiene cada petición al Hub de registrar u olvidar.
	plazoHubDespertar = 10 * time.Second
	// PlazoOlvidarAlSalir acota lo que salir espera a que el Hub olvide los aparatos: sin
	// red se sale igual, y lo que quede lo purga el Hub al mes.
	PlazoOlvidarAlSalir = 4 * time.Second
)

// plazoTurnoOlvido es lo que el olvido espera a la vuelta que esté registrando: una
// variable solo para que las pruebas no esperen los cuatro segundos.
var plazoTurnoOlvido = PlazoOlvidarAlSalir

// bundlePorProducto es, por producto (el esquema sin «sytue-»), el paquete de su app:
// el `ID` de su FyneApp.toml y la lista cerrada de services/hub/internal/despertar.
// TestLosBundlesDelDespertarSonLosDeLasApps la ata a las apps. La usan el olvido del
// despertar de iOS (bundlesDespertar) y el relevo de Android (relevo.go).
var bundlePorProducto = map[string]string{
	"bases": "eu.sytue.bases", "sheets": "eu.sytue.calcus", "calendar": "eu.sytue.calendar",
	"cast": "eu.sytue.cast", "pdf": "eu.sytue.certipdf", "contacts": "eu.sytue.contacts",
	"slides": "eu.sytue.diapos", "docs": "eu.sytue.docus", "drive": "eu.sytue.drive",
	"erp": "eu.sytue.erp", "mail": "eu.sytue.mail", "notes": "eu.sytue.notes", "one": bundleOne,
	"photos": "eu.sytue.photos", "rooms": "eu.sytue.rooms",
}

// bundleOne es el de One, el Hub: la única app que mantiene el SSE con la app cerrada.
const bundleOne = "eu.sytue.one"

// bundlesDespertar son las quince apps que el Hub sabe despertar, ordenadas: las
// cuentas `despertar-<bundle>` que olvida la app que sale.
var bundlesDespertar = slices.Sorted(maps.Values(bundlePorProducto))

// despertarSuite es el estado del despertar de esta app.
type despertarSuite struct {
	mu        sync.Mutex
	corriendo bool
	otraVez   bool
	token     string // el último que dio el sistema en este proceso
	registro  string // la época con que este proceso registró el token en el Hub
	visto     string // lo último que se miró: cambiar de sesión o de ajustes vuelve a mirar
	// vetado es la claveDespertar con que se olvidaron los de las quince: mientras siga
	// siendo esa, no se registra (una vuelta en curso no deshace el olvido). Cambiar de
	// sesión, de ajustes o de bloqueo lo levanta.
	vetado string
	// olvidos sube en cada olvidarDespertares: una vuelta que lo ve cambiar mientras
	// registraba no guarda lo registrado y lo olvida ella misma.
	olvidos uint64
	// cancelarVuelta y enVuelo son el registro que está en camino al Hub: el olvido que
	// no consigue el turno a tiempo lo corta y olvida también ese token.
	cancelarVuelta context.CancelFunc
	enVuelo        string
	hecho          chan struct{}

	// turno pone en fila lo que registra y lo que olvida los de las quince: lo que una
	// vuelta registre antes de un olvido, el olvido lo encuentra guardado.
	turnoUna sync.Once
	turnoCh  chan struct{}
}

func (d *despertarSuite) turno() chan struct{} {
	d.turnoUna.Do(func() { d.turnoCh = make(chan struct{}, 1) })
	return d.turnoCh
}

// verClave apunta `clave` como lo último mirado y levanta el veto si el estado cambió.
func (d *despertarSuite) verClave(clave string) {
	d.mu.Lock()
	d.visto = clave
	if d.vetado != clave {
		d.vetado = ""
	}
	d.mu.Unlock()
}

func (s *Suite) conductorAvisos() conductorAvisos {
	c, _ := s.conductor().(conductorAvisos)
	return c
}

// DespertarDisponible dice si esta compilación sabe despertar la app por su widget: el
// driver del fork con el modo de fondo `remote-notification` (SYTUE_DESPERTAR_APPLE =
// YES) y un widget que publicar. Es lo que enseña el interruptor.
func (s *Suite) DespertarDisponible() bool {
	c := s.conductorAvisos()
	return c != nil && s.publicadorWidget() != nil && c.RemoteNotificationsAvailable()
}

// PonerDespertar enciende o apaga «Actualizar los widgets con la app cerrada» para las
// quince apps. Apagarlo olvida antes, en el Hub, todos los aparatos de las apps de aquí
// (como mucho PlazoOlvidarAlSalir). Lo lanza un clic: no hay contexto que heredar.
//
// Encenderlo lo ata a quien está dentro (ajustes.DespertarDe): con otra persona cuenta
// como apagado, aunque la sesión de antes se acabara sin pasar por salir.
func (s *Suite) PonerDespertar(encender bool) error {
	de := ""
	if encender {
		if de = huellaDespertar(s.Estado().Sujeto); de == "" {
			return errors.New("despertar: sin nadie dentro no hay a quién atar el interruptor")
		}
	} else {
		s.olvidarDespertares(context.Background(), true)
	}
	if err := s.Poner(func(a *ajustes.Ajustes) { a.DespertarWidgets, a.DespertarDe = encender, de }); err != nil {
		if !encender {
			s.levantarVetoDespertar() // se olvidó y sigue encendido: se vuelve a registrar
		}
		return err
	}
	if encender {
		s.levantarVetoDespertar()
	}
	s.sincronizarDespertar()
	return nil
}

// levantarVetoDespertar deja registrar otra vez con el estado de ahora: lo que vetaba
// olvidarDespertares no llegó a cambiar (no se pudo guardar el bloqueo o el
// interruptor), o la persona acaba de encenderlo.
func (s *Suite) levantarVetoDespertar() {
	s.despertar.mu.Lock()
	s.despertar.vetado = ""
	s.despertar.mu.Unlock()
	s.sincronizarDespertar()
}

// iniciarDespertar engancha el despertar: el manejador del aviso silencioso, que va en
// el arranque para que un lanzamiento por un aviso lo encuentre, y la sincronización con
// el Hub al entrar, al cambiar de sesión o de ajustes y al volver a primer plano (es
// donde se nota que se quitó un widget).
func (s *Suite) iniciarDespertar() {
	c := s.conductorAvisos()
	if c == nil || s.publicadorWidget() == nil {
		return
	}
	c.OnRemoteNotification(s.avisoSilencioso)
	if !c.RemoteNotificationsAvailable() {
		return
	}
	s.alCambiarSinEscena(func(Estado) { s.sincronizarDespertarSiCambio() })
	s.AlVolver(s.sincronizarDespertar)
	s.sincronizarDespertar()
}

// avisoSilencioso es lo que la app contesta a un aviso del Hub: la misma entrada que el
// refresco periódico, sin presencia ni SSE.
func (s *Suite) avisoSilencioso(ctx context.Context) (newData, ok bool) {
	switch s.RefrescarWidgetEnFondo(ctx) {
	case Nuevos:
		return true, true
	case Fallo:
		return false, false
	default:
		return false, true
	}
}

// claveDespertar es lo que, si cambia, obliga a volver a mirar.
func (s *Suite) claveDespertar() string {
	e := s.Estado()
	a := ajustes.Leer()
	return strings.Join([]string{e.Sujeto, e.EpocaSesion, boolS(e.Dentro()), boolS(despertarEncendido(a, e)), boolS(a.Bloqueo)}, "\x00")
}

// huellaDespertar es lo que se guarda de quien encendió el interruptor: no el sujeto,
// que no tiene por qué estar en claro en los ajustes. "" sin nadie dentro.
func huellaDespertar(sujeto string) string {
	if sujeto == "" {
		return ""
	}
	suma := sha256.Sum256([]byte("sytue-despertar\x00" + sujeto))
	return hex.EncodeToString(suma[:16])
}

// despertarEncendido dice si el interruptor está encendido PARA quien está dentro: lo
// encendió esta persona. Otra que entre en el mismo aparato, aunque la sesión de antes
// caducase sin salir, lo encuentra apagado y nadie la registra sin pedirlo.
func despertarEncendido(a ajustes.Estado, e Estado) bool {
	return a.DespertarWidgets && e.Sujeto != "" && a.DespertarDe == huellaDespertar(e.Sujeto)
}

// DespertarEncendido es lo que enseña la casilla del interruptor.
func (s *Suite) DespertarEncendido() bool {
	return despertarEncendido(ajustes.Leer(), s.Estado())
}

func boolS(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

func (s *Suite) sincronizarDespertarSiCambio() {
	clave := s.claveDespertar()
	s.despertar.mu.Lock()
	igual := s.despertar.visto == clave
	if s.despertar.vetado != clave {
		s.despertar.vetado = ""
	}
	s.despertar.mu.Unlock()
	if !igual {
		s.sincronizarDespertar()
	}
}

// sincronizarDespertar mira, fuera del hilo de la interfaz, si esta app debe estar
// registrada en el Hub y lo arregla. Una vuelta a la vez; lo que llegue durante una
// vuelta hace otra al acabar.
func (s *Suite) sincronizarDespertar() {
	d := &s.despertar
	d.mu.Lock()
	if d.corriendo {
		d.otraVez = true
		d.mu.Unlock()
		return
	}
	d.corriendo = true
	d.mu.Unlock()
	go func() {
		for {
			s.unaVueltaDespertar()
			d.mu.Lock()
			if !d.otraVez {
				d.corriendo = false
				hecho := d.hecho
				d.hecho = nil
				d.mu.Unlock()
				if hecho != nil {
					close(hecho)
				}
				return
			}
			d.otraVez = false
			d.mu.Unlock()
		}
	}()
}

func (s *Suite) unaVueltaDespertar() {
	c := s.conductorAvisos()
	if c == nil || s.app == nil || s.Hub == nil || !c.RemoteNotificationsAvailable() || s.publicadorWidget() == nil {
		return
	}
	// Con el teléfono bloqueado no se lee ni si la suite está bloqueada ni el llavero:
	// nada de lo que se decidiera aquí sería cierto. Se mira al desbloquear.
	if !s.conductor().ProtectedDataAvailable() {
		return
	}
	clave := s.claveDespertar()
	s.despertar.verClave(clave)

	bundle := s.app.Metadata().ID
	cuenta := prefijoDespertar + bundle
	e := s.Estado()
	quiere, conSesion, sabido := s.debeDespertar(e)
	if !sabido {
		return // WidgetKit no ha contestado: lo que hay se queda
	}
	guardado, _ := auth.LeerSecreto(cuenta)
	if !quiere {
		// Lo de esta app (su último widget quitado): se olvida su token. Sin sesión no se
		// puede, y no hace falta: salir y el bloqueo ya olvidaron los de las quince.
		if len(guardado) > 0 && conSesion {
			s.olvidarToken(cuenta, string(guardado))
		}
		return
	}
	token := s.tokenDespertar(c)
	if token == "" {
		return
	}
	s.despertar.mu.Lock()
	hecho := string(guardado) == token && s.despertar.registro == e.EpocaSesion
	s.despertar.mu.Unlock()
	if hecho {
		return
	}
	soltar, sigue, olvidos := s.turnoParaRegistrar(clave)
	defer soltar()
	if !sigue {
		return
	}
	ctx, parar := context.WithTimeout(context.Background(), plazoHubDespertar)
	defer parar()
	s.despertar.mu.Lock()
	s.despertar.cancelarVuelta, s.despertar.enVuelo = parar, token
	s.despertar.mu.Unlock()
	err := s.Hub.RegistrarAparatoApple(ctx, bundle, token)
	s.despertar.mu.Lock()
	s.despertar.cancelarVuelta, s.despertar.enVuelo = nil, ""
	olvidado := s.despertar.olvidos != olvidos
	s.despertar.mu.Unlock()
	if err != nil {
		slog.Info("despertar: el Hub no ha registrado el aparato; se reintenta al volver", "error", err)
		return
	}
	if olvidado {
		// Se olvidó todo mientras el Hub contestaba: lo recién registrado se olvida
		// también, y no se guarda.
		s.olvidarToken("", token)
		return
	}
	if len(guardado) > 0 && string(guardado) != token {
		s.olvidarToken("", string(guardado)) // el token viejo que el sistema cambió
	}
	if err := auth.GuardarSecreto(cuenta, []byte(token)); err != nil {
		slog.Warn("despertar: no se ha podido guardar el token; al salir no se olvidará", "error", err)
	}
	s.despertar.mu.Lock()
	s.despertar.registro = e.EpocaSesion
	s.despertar.mu.Unlock()
}

// turnoParaRegistrar pone la vuelta en fila con olvidarDespertares y mira otra vez: si
// se olvidó todo mientras se pedía el token, no se registra; si cambió el estado desde
// `clave`, decide otra vuelta. soltar devuelve el turno; olvidos, los que llevaba
// olvidarDespertares al darlo.
func (s *Suite) turnoParaRegistrar(clave string) (soltar func(), sigue bool, olvidos uint64) {
	turno := s.despertar.turno()
	turno <- struct{}{}
	soltar = func() { <-turno }
	ahora := s.claveDespertar()
	d := &s.despertar
	d.mu.Lock()
	defer d.mu.Unlock()
	vetada := d.vetado == ahora
	if ahora != clave && !vetada {
		d.otraVez = true
	}
	return soltar, ahora == clave && !vetada, d.olvidos
}

// debeDespertar dice si esta app tiene que estar registrada en el Hub: el interruptor
// encendido, sesión propia sin el bloqueo de la suite y un widget suyo puesto. conSesion
// es si se puede hablar con el Hub; sabido, falso si WidgetKit no ha contestado.
func (s *Suite) debeDespertar(e Estado) (quiere, conSesion, sabido bool) {
	a := ajustes.Leer()
	conSesion = e.Dentro() && !bloqueoPuesto(a) && !s.sinSesionPropia()
	if !despertarEncendido(a, e) || !conSesion {
		return false, conSesion, true
	}
	n, sabido := widgets.Instalados()
	return n > 0, conSesion, sabido
}

// tokenDespertar pide el token al sistema la primera vez en este proceso.
func (s *Suite) tokenDespertar(c conductorAvisos) string {
	s.despertar.mu.Lock()
	tok := s.despertar.token
	s.despertar.mu.Unlock()
	if tok != "" {
		return tok
	}
	llega := make(chan string, 1)
	c.RegisterForRemoteNotifications(func(token string, err error) {
		if err != nil {
			slog.Info("despertar: el sistema no da token", "error", err)
		}
		llega <- strings.ToLower(strings.TrimSpace(token))
	})
	select {
	case tok = <-llega:
	case <-time.After(plazoToken):
	}
	if tok != "" {
		s.despertar.mu.Lock()
		s.despertar.token = tok
		s.despertar.mu.Unlock()
	}
	return tok
}

// olvidarToken pide al Hub que olvide `token` y, si lo hace (o dice que no lo entiende,
// que es lo mismo para quien olvida), borra la cuenta del almacén.
func (s *Suite) olvidarToken(cuenta, token string) {
	ctx, parar := context.WithTimeout(context.Background(), plazoHubDespertar)
	defer parar()
	err := s.Hub.OlvidarAparatoApple(ctx, token)
	var he *api.Error
	if err != nil && (!errors.As(err, &he) || he.Status != http.StatusBadRequest) {
		slog.Info("despertar: el Hub no ha olvidado el aparato; se reintenta al volver", "error", err)
		return
	}
	if cuenta != "" {
		_ = auth.BorrarSecreto(cuenta)
	}
	s.despertar.mu.Lock()
	s.despertar.registro = ""
	s.despertar.mu.Unlock()
}

// olvidarDespertares pide al Hub, con la sesión que aún vale, que olvide los tokens de
// TODAS las apps de este aparato (la lista cerrada bundlesDespertar, en el grupo de la
// suite), y los borra del almacén. Lo llaman salir, poner el bloqueo y apagar el
// interruptor: es de las quince y las demás pueden no estar corriendo. Espera como mucho
// PlazoOlvidarAlSalir al Hub, y otro tanto a una vuelta que esté registrando: pasado,
// corta ese registro y olvida también su token.
//
// En escritorio, web y Android (sin el conductor de avisos, o sin el modo de fondo) no
// se registra nada, así que no hay nada que olvidar ni que leer del almacén seguro.
//
// conservarFallidos es para quien sigue con sesión (apagar el interruptor, poner el
// bloqueo): el token que el Hub no ha olvidado se queda en el almacén y la vuelta de su
// app lo reintenta (olvidarToken) al volver. Al salir no habrá sesión con que
// reintentarlo: se borra todo y lo que quede lo purga el Hub al mes; la app que despierte
// sin sesión no lee nada.
func (s *Suite) olvidarDespertares(ctx context.Context, conservarFallidos bool) {
	if c := s.conductorAvisos(); s.Hub == nil || c == nil || !c.RemoteNotificationsAvailable() {
		return
	}
	d := &s.despertar
	clave := s.claveDespertar()
	d.mu.Lock()
	d.vetado = clave
	d.olvidos++
	d.mu.Unlock()
	soltar, enVuelo := d.turnoParaOlvidar(ctx)
	defer soltar()
	ctx, parar := context.WithTimeout(ctx, PlazoOlvidarAlSalir)
	defer parar()
	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		borrar []string
	)
	for _, b := range bundlesDespertar {
		cuenta := prefijoDespertar + b
		tok, err := auth.LeerSecreto(cuenta)
		if err != nil || len(tok) == 0 {
			continue
		}
		if string(tok) == enVuelo {
			enVuelo = "" // ya va con lo guardado
		}
		wg.Go(func() {
			err := s.Hub.OlvidarAparatoApple(ctx, string(tok))
			var he *api.Error
			olvidado := err == nil || (errors.As(err, &he) && he.Status == http.StatusBadRequest)
			mu.Lock()
			defer mu.Unlock()
			if olvidado || !conservarFallidos {
				borrar = append(borrar, cuenta)
			}
			if !olvidado {
				slog.Info("despertar: el Hub no ha olvidado un aparato", "reintento", conservarFallidos, "error", err)
			}
		})
	}
	if enVuelo != "" {
		wg.Go(func() {
			if err := s.Hub.OlvidarAparatoApple(ctx, enVuelo); err != nil {
				slog.Info("despertar: el Hub no ha olvidado el registro cortado", "error", err)
			}
		})
	}
	wg.Wait()
	for _, c := range borrar {
		if err := auth.BorrarSecreto(c); err != nil {
			slog.Warn("despertar: no se ha podido borrar un token del almacén", "error", err)
		}
	}
	// Lo de esta app se vuelve a registrar entero si vuelve a hacer falta: el Hub puede
	// haberla olvidado aunque otra fallase.
	d.mu.Lock()
	d.registro = ""
	d.mu.Unlock()
}

// turnoParaOlvidar espera el turno a la vuelta que esté registrando, como mucho
// plazoTurnoOlvido. Si no llega, la vuelta lleva ese plazo registrando: se corta, y
// enVuelo es su token, que el olvido olvida con lo guardado (si el Hub llegó a
// contestar, la vuelta lo olvida ella misma al ver que olvidos ha cambiado). soltar
// devuelve el turno si se consiguió.
func (d *despertarSuite) turnoParaOlvidar(ctx context.Context) (soltar func(), enVuelo string) {
	turno := d.turno()
	espera := time.NewTimer(plazoTurnoOlvido)
	defer espera.Stop()
	select {
	case turno <- struct{}{}:
		return func() { <-turno }, ""
	case <-espera.C:
		d.mu.Lock()
		defer d.mu.Unlock()
		if d.cancelarVuelta != nil {
			d.cancelarVuelta()
		}
		return func() {}, d.enVuelo
	case <-ctx.Done():
		return func() {}, ""
	}
}

// esperarDespertar es SOLO para pruebas: espera a que acabe la vuelta en curso.
func (s *Suite) esperarDespertar(ctx context.Context) {
	d := &s.despertar
	d.mu.Lock()
	if !d.corriendo {
		d.mu.Unlock()
		return
	}
	if d.hecho == nil {
		d.hecho = make(chan struct{})
	}
	hecho := d.hecho
	d.mu.Unlock()
	select {
	case <-hecho:
	case <-ctx.Done():
	}
}
