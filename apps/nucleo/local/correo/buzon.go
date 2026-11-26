// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package correo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"net/mail"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-message/charset"

	"git.sytue.eu/sytue/one/apps/nucleo/eml"
	"git.sytue.eu/sytue/one/apps/nucleo/htmltext"
	"git.sytue.eu/sytue/one/apps/nucleo/local/biblioteca"
)

// Medidas de la sincronización.
const (
	// Ventana: los mensajes más recientes de cada carpeta que se bajan la primera vez.
	// Lo anterior se queda en el servidor (fase posterior: «tener sin conexión los
	// últimos N días», M-03).
	Ventana = 500
	// CuerposPorVuelta: los cuerpos que se bajan por carpeta en cada vuelta, del más
	// nuevo al más viejo, para leer sin conexión. El resto se baja al abrirlo.
	CuerposPorVuelta = 100
	// TopeCuerpo: un mensaje más grande no se baja solo; se baja al abrirlo.
	TopeCuerpo = 10 << 20
)

// Buzon es el correo local: las cuentas, el índice y el sincronizador. Cumple la
// interfaz `mail.Servicio` de la pantalla de Mail (servicio.go).
type Buzon struct {
	raiz     string
	x        *indice
	reg      *registro
	secretos Secretos
	red      Red

	// Intervalo entre sincronizaciones automáticas con la app abierta.
	Intervalo time.Duration
	// AlFallar avisa de lo que la persona tiene que saber aunque no esté mirando: un
	// envío rechazado, una cuenta que pide volver a entrar. Puede llegar desde
	// cualquier goroutine.
	AlFallar func(Cuenta, error)
	// AlLlegar recibe el correo nuevo que se queda en la entrada sin leer, para el aviso
	// del sistema (M-07). Puede llegar desde cualquier goroutine. Nil: nadie avisa.
	AlLlegar func([]Llegada)

	mu        sync.Mutex
	oyentes   map[int]func()
	siguiente int
	despertar chan struct{}
	// despertarUna y porCuenta son la vuelta que pide el vigía de una cuenta (IDLE);
	// vigias, sinIdle y enIdle, los vigías que corren, las cuentas cuyo servidor no
	// ofrece IDLE y las que están esperando en él (vigia.go).
	despertarUna chan struct{}
	porCuenta    map[string]bool
	vigias       map[string]context.CancelFunc
	sinIdle      map[string]bool
	enIdle       map[string]bool
	vigiando     sync.WaitGroup
	// imagenes son las remotas ya bajadas en esta sesión (fuera.go).
	imagenes map[string][]byte
	parar    context.CancelFunc
	hecho    chan struct{}
	// sincro serializa las vueltas de sincronización (una conexión por cuenta cada
	// vez); salida, el vaciado de la bandeja de salida.
	sincro, salida sync.Mutex
}

// Abrir abre el correo en raiz (`<Datos>/Correo`). No habla con la red: eso empieza
// con Arrancar.
func Abrir(raiz string, s Secretos, red Red) (*Buzon, error) {
	if err := os.MkdirAll(filepath.Join(raiz, "salida"), 0o700); err != nil {
		return nil, err
	}
	reg, err := abrirRegistro(raiz)
	if err != nil {
		return nil, err
	}
	x, err := abrirIndice(raiz)
	if err != nil {
		return nil, err
	}
	// Un envío que se quedó a medias al cerrarse la app pudo salir: no se repite solo.
	_, _ = x.db.ExecContext(context.Background(), "UPDATE salida SET estado = ?, error = ? WHERE estado = 'enviando'", salidaError, ErrEnvioIncierto.Error())
	return &Buzon{
		raiz: raiz, x: x, reg: reg, secretos: s, red: red, Intervalo: 3 * time.Minute,
		oyentes: map[int]func(){}, despertar: make(chan struct{}, 1), despertarUna: make(chan struct{}, 1),
		porCuenta: map[string]bool{}, vigias: map[string]context.CancelFunc{}, sinIdle: map[string]bool{}, enIdle: map[string]bool{},
		imagenes: map[string][]byte{},
	}, nil
}

// Arrancar empieza a sincronizar en segundo plano: ya, cada Intervalo, cuando se pide
// con SincronizarYa y, por cuenta, en cuanto su entrada avisa en IDLE (vigia.go). La
// bandeja de salida y lo pospuesto que vence se miran cada pocos segundos.
func (b *Buzon) Arrancar() {
	b.mu.Lock()
	if b.parar != nil {
		b.mu.Unlock()
		return
	}
	ctx, parar := context.WithCancel(context.Background())
	b.parar, b.hecho = parar, make(chan struct{})
	b.mu.Unlock()
	go func() {
		defer close(b.hecho)
		t := time.NewTicker(3 * time.Second)
		defer t.Stop()
		ultima := time.Time{}
		for {
			if time.Since(ultima) >= b.Intervalo {
				ultima = time.Now()
				_ = b.sincronizarTodo(ctx)
			} else if solo := b.cuentasPedidas(); solo != nil {
				_ = b.sincronizarCuentas(ctx, solo)
			}
			b.ajustarVigias(ctx)
			b.despertarPospuestos(ctx)
			// Lo que otra app (el ERP) envió por una cuenta sin copia en el servidor
			// (enviados.go): a «Enviados», y con APPEND en la próxima vuelta de esa cuenta.
			b.recogerEnviados(ctx)
			b.VaciarSalida(ctx)
			select {
			case <-ctx.Done():
				return
			case <-b.despertar:
				ultima = time.Time{}
			case <-b.despertarUna:
			case <-t.C:
			}
		}
	}()
}

// SincronizarYa pide una vuelta de sincronización sin esperar a que acabe.
func (b *Buzon) SincronizarYa() {
	select {
	case b.despertar <- struct{}{}:
	default:
	}
}

// Cerrar para la sincronización y cierra el índice.
func (b *Buzon) Cerrar() {
	b.mu.Lock()
	parar, hecho := b.parar, b.hecho
	b.mu.Unlock()
	if parar != nil {
		parar()
		<-hecho
	}
	b.vigiando.Wait()
	_ = b.x.cerrar()
}

// AlCambiar registra f para cuando cambia algo del índice que la pantalla enseña
// (llega correo, termina una sincronización). Devuelve cómo dejar de escuchar.
func (b *Buzon) AlCambiar(f func()) (quitar func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.siguiente++
	n := b.siguiente
	b.oyentes[n] = f
	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		delete(b.oyentes, n)
	}
}

func (b *Buzon) avisar() {
	b.mu.Lock()
	fs := make([]func(), 0, len(b.oyentes))
	for _, f := range b.oyentes {
		fs = append(fs, f)
	}
	b.mu.Unlock()
	for _, f := range fs {
		f()
	}
}

func (b *Buzon) fallo(c Cuenta, err error) {
	slog.Warn("correo: fallo", "cuenta", c.Direccion, "error", err)
	if b.AlFallar != nil {
		b.AlFallar(c, err)
	}
}

// ---- Cuentas ----

// Cuentas son las cuentas conectadas, en el orden en que se añadieron.
func (b *Buzon) Cuentas() []Cuenta { return b.reg.lista() }

// Anadir da de alta una cuenta (o la actualiza, con el mismo ID) con su secreto: la
// contraseña, o el token de refresco de OAuth. No la prueba: eso es Probar.
func (b *Buzon) Anadir(c Cuenta, secreto string) (Cuenta, error) {
	if err := validar(&c); err != nil {
		return Cuenta{}, err
	}
	if otra, ok := b.reg.porDireccion(c.Direccion); ok && otra.ID != c.ID {
		return Cuenta{}, fmt.Errorf("correo: %s ya está conectada", c.Direccion)
	}
	if secreto != "" {
		clave := claveContrasena(c.ID)
		if c.Acceso == AccesoMicrosoft {
			clave = claveRefresco(c.ID)
		}
		if err := b.secretos.Guardar(clave, secreto); err != nil {
			return Cuenta{}, fmt.Errorf("correo: no se pudo guardar la contraseña en el llavero: %w", err)
		}
	}
	c.Estado, c.Error = EstadoOK, ""
	if err := b.reg.poner(c); err != nil {
		return Cuenta{}, err
	}
	b.mu.Lock()
	delete(b.sinIdle, c.ID) // con otros datos, el servidor puede ser otro
	if parar, ok := b.vigias[c.ID]; ok {
		parar()
		delete(b.vigias, c.ID)
	}
	b.mu.Unlock()
	b.SincronizarYa()
	return c, nil
}

// ConSecreto guarda el secreto de una cuenta aún no dada de alta, para Probar.
func (b *Buzon) ConSecreto(c *Cuenta, secreto string) error {
	if err := validar(c); err != nil {
		return err
	}
	clave := claveContrasena(c.ID)
	if c.Acceso == AccesoMicrosoft {
		clave = claveRefresco(c.ID)
	}
	return b.secretos.Guardar(clave, secreto)
}

// Quitar desconecta una cuenta: borra sus secretos, su índice y sus .eml de este
// equipo. En el servidor no se toca nada.
func (b *Buzon) Quitar(id string) error {
	b.sincro.Lock()
	defer b.sincro.Unlock()
	if err := b.reg.quitar(id); err != nil {
		return err
	}
	_ = b.secretos.Borrar(claveContrasena(id))
	_ = b.secretos.Borrar(claveRefresco(id))
	ctx := context.Background()
	tx, err := b.x.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, "INSERT INTO cambios (mensaje) SELECT id FROM mensajes WHERE cuenta = ?", id); err != nil {
		return err
	}
	for _, q := range []string{
		"DELETE FROM mensaje_etiquetas WHERE mensaje IN (SELECT id FROM mensajes WHERE cuenta = ?)",
		"DELETE FROM mensajes WHERE cuenta = ?", "DELETE FROM carpetas WHERE cuenta = ?",
		"DELETE FROM pendientes WHERE cuenta = ?", "DELETE FROM salida WHERE cuenta = ?",
	} {
		if _, err := tx.ExecContext(ctx, q, id); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	_ = os.RemoveAll(filepath.Join(b.raiz, filepath.Base(id)))
	_ = os.RemoveAll(rutaColaEnviados(b.raiz, id))
	b.avisar()
	return nil
}

func (b *Buzon) marcarEstado(c Cuenta, err error) {
	b.reg.cambiar(c.ID, func(x *Cuenta) {
		switch {
		case err == nil:
			x.Estado, x.Error, x.Sincronizada = EstadoOK, "", time.Now().UTC()
		case errors.Is(err, ErrAcceso):
			x.Estado, x.Error = EstadoReconectar, err.Error()
		default:
			x.Error = err.Error()
		}
	})
}

// ---- Ficheros .eml ----

func (b *Buzon) rutaEML(cuenta string, id int64) string {
	return filepath.Join(b.raiz, filepath.Base(cuenta), strconv.FormatInt(id, 10)+".eml")
}

func (b *Buzon) rutaSalida(id string) string {
	return filepath.Join(b.raiz, "salida", filepath.Base(id)+".eml")
}

func (b *Buzon) escribirEML(cuenta string, id int64, crudo []byte) error {
	ruta := b.rutaEML(cuenta, id)
	if err := os.MkdirAll(filepath.Dir(ruta), 0o700); err != nil {
		return err
	}
	return biblioteca.EscribirAtomico(ruta, crudo)
}

// ---- Leer cabeceras y cuerpos ----

var decodificador = &mime.WordDecoder{CharsetReader: charset.Reader}

func decodificar(s string) string {
	if d, err := decodificador.DecodeHeader(s); err == nil {
		return d
	}
	return s
}

func direcciones(h mail.Header, clave string) []string {
	p := mail.AddressParser{WordDecoder: decodificador}
	l, err := p.ParseList(h.Get(clave))
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(l))
	for _, a := range l {
		out = append(out, a.Address)
	}
	return out
}

// idsMensaje son los Message-ID de una cabecera, sin ángulos.
func idsMensaje(v string) []string {
	var out []string
	for _, f := range strings.Fields(v) {
		if f = strings.Trim(f, "<>,"); f != "" {
			out = append(out, f)
		}
	}
	return out
}

// sobreDeCabecera rellena un sobre con las cabeceras de un mensaje (con o sin cuerpo).
// Devuelve también los candidatos a raíz del hilo.
func sobreDeCabecera(s *sobre, crudo []byte) (refs []string) {
	m, err := mail.ReadMessage(bytes.NewReader(crudo))
	if err != nil {
		return nil
	}
	h := m.Header
	s.asunto = decodificar(h.Get("Subject"))
	p := mail.AddressParser{WordDecoder: decodificador}
	if a, err := p.Parse(h.Get("From")); err == nil {
		s.de, s.deNombre = a.Address, a.Name
	}
	s.para, s.cc, s.responder = direcciones(h, "To"), direcciones(h, "Cc"), direcciones(h, "Reply-To")
	if ids := idsMensaje(h.Get("Message-ID")); len(ids) > 0 {
		s.messageID = ids[0]
	}
	if ids := idsMensaje(h.Get("In-Reply-To")); len(ids) > 0 {
		s.enRespuesta = ids[0]
	}
	if f, err := h.Date(); err == nil {
		s.fecha = f.UTC()
	}
	refs = idsMensaje(h.Get("References"))
	if s.enRespuesta != "" {
		refs = append(refs, s.enRespuesta)
	}
	return refs
}

// conCuerpo completa un sobre con lo que dice su .eml: el resumen, los adjuntos y el
// texto que se busca.
func conCuerpo(ctx context.Context, s *sobre, crudo []byte) {
	s.eml, s.tamano = true, max(s.tamano, int64(len(crudo)))
	m, err := eml.LeerConLimite(ctx, bytes.NewReader(crudo), int64(len(crudo))+1)
	if err != nil && m.Texto == "" && m.HTML == "" {
		s.buscable = textoBuscable(s, "")
		return
	}
	texto := m.Texto
	if strings.TrimSpace(texto) == "" && m.HTML != "" {
		texto = htmltext.Text(m.HTML)
	}
	s.vista = htmltext.Summary(texto, 200)
	s.adjuntos = false
	for _, a := range m.Adjuntos {
		if a.CID == "" {
			s.adjuntos = true
		}
	}
	s.buscable = textoBuscable(s, texto)
}

// hiloDe elige el hilo de un mensaje: el de un mensaje ya guardado al que referencia
// (el padre o la raíz) o, si no hay ninguno, el Message-ID de la raíz.
func (b *Buzon) hiloDe(ctx context.Context, tx execer, s *sobre, refs []string) {
	cand := append([]string{}, refs...)
	if s.messageID != "" {
		cand = append(cand, s.messageID)
	}
	for _, r := range cand {
		var h string
		if err := tx.QueryRowContext(ctx, "SELECT hilo FROM mensajes WHERE message_id = ? AND hilo != '' LIMIT 1", r).Scan(&h); err == nil && h != "" {
			s.hilo = h
			return
		}
	}
	if len(cand) > 0 {
		s.hilo = "t:" + cand[0]
	}
}
