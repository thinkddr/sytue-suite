// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package correo

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// El correo nuevo, en cuanto llega (M-07): con la app abierta, cada cuenta mantiene una
// conexión aparte con su entrada en IDLE (RFC 2177). Cuando el servidor cuenta algo
// (EXISTS, EXPUNGE o un cambio de marcas), se hace una vuelta de ESA cuenta, sin esperar
// al sondeo, y la pantalla se entera por AlCambiar. El resto de carpetas siguen con el
// sondeo de Intervalo. Un servidor sin IDLE se queda solo con el sondeo.
//
// La conexión de vigía no escribe nada: abre la entrada en solo lectura (EXAMINE).

// Las esperas del vigía tras un corte: crecen hasta topeVigia.
const (
	esperaVigia = 5 * time.Second
	topeVigia   = 5 * time.Minute
	// renovarIdle es cada cuánto se rehace el IDLE (RFC 2177 pide menos de 29 minutos;
	// el cliente ya lo hace a los 28, esto además comprueba que la conexión vive).
	renovarIdle = 20 * time.Minute
)

// errSinIdle: el servidor no ofrece IDLE; esa cuenta se queda con el sondeo.
var errSinIdle = errors.New("correo: el servidor no ofrece IDLE")

// Llegada es un correo nuevo en la entrada, para el aviso del sistema.
type Llegada struct {
	Cuenta  Cuenta
	Mensaje api.MailMessage
}

// despertarCuenta pide una vuelta de una sola cuenta, sin esperar a que acabe.
func (b *Buzon) despertarCuenta(id string) {
	b.mu.Lock()
	b.porCuenta[id] = true
	b.mu.Unlock()
	select {
	case b.despertarUna <- struct{}{}:
	default:
	}
}

// cuentasPedidas son las cuentas que pidió el vigía desde la última vez.
func (b *Buzon) cuentasPedidas() map[string]bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.porCuenta) == 0 {
		return nil
	}
	out := b.porCuenta
	b.porCuenta = map[string]bool{}
	return out
}

// ajustarVigias arranca el vigía de cada cuenta que no lo tenga y para el de las que se
// quitaron o piden volver a entrar. Lo llama el bucle de Arrancar.
func (b *Buzon) ajustarVigias(ctx context.Context) {
	cuentas := b.reg.lista()
	b.mu.Lock()
	defer b.mu.Unlock()
	vivas := map[string]bool{}
	for _, c := range cuentas {
		if c.Estado == EstadoReconectar || b.sinIdle[c.ID] {
			continue
		}
		vivas[c.ID] = true
		if _, ya := b.vigias[c.ID]; ya {
			continue
		}
		vctx, parar := context.WithCancel(ctx)
		b.vigias[c.ID] = parar
		b.vigiando.Add(1)
		go func(id string) {
			defer b.vigiando.Done()
			b.vigilar(vctx, id)
		}(c.ID)
	}
	for id, parar := range b.vigias {
		if !vivas[id] {
			parar()
			delete(b.vigias, id)
		}
	}
}

// Vigilando dice si la cuenta tiene su entrada en IDLE ahora mismo.
func (b *Buzon) Vigilando(id string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.enIdle[id]
}

func (b *Buzon) ponerEnIdle(id string, si bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if si {
		b.enIdle[id] = true
	} else {
		delete(b.enIdle, id)
	}
}

// vigilar mantiene el IDLE de la cuenta id hasta que se corta ctx.
func (b *Buzon) vigilar(ctx context.Context, id string) {
	espera := esperaVigia
	for ctx.Err() == nil {
		c, ok := b.reg.cuenta(id)
		if !ok || c.Estado == EstadoReconectar {
			return
		}
		empezo := time.Now()
		err := b.idle(ctx, c)
		b.ponerEnIdle(id, false)
		switch {
		case ctx.Err() != nil:
			return
		case errors.Is(err, errSinIdle):
			b.mu.Lock()
			b.sinIdle[id] = true
			delete(b.vigias, id)
			b.mu.Unlock()
			return
		case errors.Is(err, ErrAcceso):
			return // la vuelta normal marcará la cuenta para volver a entrar
		}
		if time.Since(empezo) > time.Minute {
			espera = esperaVigia // estuvo viva un rato: el corte es nuevo
		}
		slog.Debug("correo: el vigía se cortó", "cuenta", c.Direccion, "error", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(espera):
		}
		espera = min(espera*2, topeVigia)
	}
}

// idle abre la conexión del vigía y espera en IDLE. Vuelve con el error que la cortó.
func (b *Buzon) idle(ctx context.Context, c Cuenta) error {
	avisos := make(chan struct{}, 1)
	avisar := func() {
		select {
		case avisos <- struct{}{}:
		default:
		}
	}
	h := &imapclient.UnilateralDataHandler{
		Mailbox: func(d *imapclient.UnilateralDataMailbox) {
			if d.NumMessages != nil {
				avisar()
			}
		},
		Expunge: func(uint32) { avisar() },
		Fetch: func(m *imapclient.FetchMessageData) {
			_, _ = m.Collect() // hay que leerlo entero para que el cliente siga
			avisar()
		},
	}
	cli, err := b.abrirIMAPCon(ctx, c, h)
	if err != nil {
		return err
	}
	defer func() { _ = cli.Close() }()
	caps := cli.Caps()
	if !caps.Has(imap.CapIdle) && !caps.Has(imap.CapIMAP4rev2) {
		_ = cli.Logout().Wait()
		return errSinIdle
	}
	if _, err := cli.Select(nombreEntrada(ctx, b, c), &imap.SelectOptions{ReadOnly: true}).Wait(); err != nil {
		return err
	}
	for {
		cmd, err := cli.Idle()
		if err != nil {
			return err
		}
		b.ponerEnIdle(c.ID, true)
		var fin error
		select {
		case <-avisos:
			b.despertarCuenta(c.ID)
		case <-time.After(renovarIdle):
		case <-cli.Closed():
			fin = errors.New("correo: el servidor cerró la conexión")
		case <-ctx.Done():
			fin = ctx.Err()
		}
		if err := cmd.Close(); err != nil && fin == nil {
			fin = err
		}
		if err := cmd.Wait(); err != nil && fin == nil {
			fin = err
		}
		if fin != nil {
			return fin
		}
	}
}

// nombreEntrada es la carpeta de rol inbox de la cuenta; INBOX si aún no se conoce.
func nombreEntrada(ctx context.Context, b *Buzon, c Cuenta) string {
	cs, err := b.x.carpetas(ctx, c.ID)
	if err == nil {
		for _, f := range cs {
			if f.rol == "inbox" && !f.local {
				return f.nombre
			}
		}
	}
	return "INBOX"
}

// anunciarLlegadas pasa a AlLlegar lo que acaba de llegar a la entrada y sigue en ella
// sin leer (las reglas y el silencio ya han hecho lo suyo).
func (b *Buzon) anunciarLlegadas(ctx context.Context, c Cuenta, ids []int64) {
	if b.AlLlegar == nil {
		return
	}
	var ls []Llegada
	for _, n := range ids {
		s, err := b.x.sobre(ctx, n)
		if err != nil || s.rol != "inbox" || s.pospuesto.After(time.Time{}) || tieneMarca(s.marcas, api.MailFlagSeen) {
			continue
		}
		ms, err := b.x.mensajes(ctx, []sobre{s})
		if err == nil && len(ms) == 1 {
			ls = append(ls, Llegada{Cuenta: c, Mensaje: ms[0]})
		}
	}
	if len(ls) > 0 {
		b.AlLlegar(ls)
	}
}
