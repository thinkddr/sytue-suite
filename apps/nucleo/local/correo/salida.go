// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package correo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/mail"
	"os"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/local/biblioteca"
)

// La bandeja de salida local (M-06). Enviar deja el mensaje aquí con su hora de
// salida —ahora más la ventana de deshacer, o la hora programada— y el bucle de
// Arrancar lo manda por SMTP en cuanto vence y hay red. Sin red se queda esperando; un
// rechazo del servidor no se reintenta solo y se avisa (AlFallar). Un envío programado
// solo sale con la app abierta, o en el siguiente arranque.

// encolarSalida guarda un mensaje compuesto para enviarlo a su hora.
func (b *Buzon) encolarSalida(ctx context.Context, de string, para []string, crudo []byte, sale time.Time, estado string) (string, error) {
	c, err := b.cuentaDe(de)
	if err != nil {
		return "", err
	}
	if len(para) == 0 {
		return "", errors.New("correo: el mensaje no tiene destinatarios")
	}
	id := uuid.NewString()
	if err := biblioteca.EscribirAtomico(b.rutaSalida(id), crudo); err != nil {
		return "", err
	}
	asunto := ""
	if m, err := mail.ReadMessage(bytes.NewReader(crudo)); err == nil {
		asunto = decodificar(m.Header.Get("Subject"))
	}
	_, err = b.x.db.ExecContext(ctx, "INSERT INTO salida (id, cuenta, de, destinatarios, asunto, sale, creado, estado) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		id, c.ID, de, jsonLista(para), asunto, sale.Unix(), time.Now().Unix(), estado)
	if err != nil {
		_ = os.Remove(b.rutaSalida(id))
		return "", err
	}
	return id, nil
}

// cuentaDe es la cuenta que envía desde una dirección; sin ninguna que encaje, la
// primera.
func (b *Buzon) cuentaDe(de string) (Cuenta, error) {
	if c, ok := b.reg.porDireccion(de); ok {
		return c, nil
	}
	cs := b.reg.lista()
	if len(cs) == 0 {
		return Cuenta{}, errors.New("correo: añade una cuenta de correo para enviar")
	}
	return cs[0], nil
}

type envio struct {
	id, cuenta, de string
	para           []string
	intentos       int
}

// VaciarSalida manda lo que ya ha vencido. Lo llama el bucle de Arrancar; las pruebas,
// a mano.
func (b *Buzon) VaciarSalida(ctx context.Context) {
	b.salida.Lock()
	defer b.salida.Unlock()
	es, err := b.vencidos(ctx)
	if err != nil {
		return
	}
	for _, e := range es {
		b.enviarUno(ctx, e)
	}
}

// vencidos son los envíos de la bandeja de salida a los que ya les toca salir. Una fila
// que no se deja leer se salta; una lectura que se corta a medias no manda nada (se
// vuelve a mirar en la próxima vuelta).
func (b *Buzon) vencidos(ctx context.Context) ([]envio, error) {
	filas, err := b.x.db.QueryContext(ctx, "SELECT id, cuenta, de, destinatarios, intentos FROM salida WHERE estado IN (?, ?) AND sale <= ? ORDER BY sale",
		salidaPendiente, salidaProgramada, time.Now().Unix())
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	var es []envio
	for filas.Next() {
		var e envio
		var para string
		if filas.Scan(&e.id, &e.cuenta, &e.de, &para, &e.intentos) == nil {
			e.para = deLista(para)
			es = append(es, e)
		}
	}
	return es, filas.Err()
}

func (b *Buzon) enviarUno(ctx context.Context, e envio) {
	// Se reclama la fila: deshacer (CancelOutbound) ya no la puede quitar a medio envío.
	r, err := b.x.db.ExecContext(ctx, "UPDATE salida SET estado = 'enviando' WHERE id = ? AND estado IN (?, ?)", e.id, salidaPendiente, salidaProgramada)
	if err != nil {
		return // r es nil: preguntarle por las filas sería un pánico
	}
	if n, _ := r.RowsAffected(); n == 0 {
		return
	}
	c, ok := b.reg.cuenta(e.cuenta)
	crudo, errLeer := os.ReadFile(b.rutaSalida(e.id))
	if !ok || errLeer != nil {
		_, _ = b.x.db.ExecContext(ctx, "DELETE FROM salida WHERE id = ?", e.id)
		return
	}
	ctxEnvio, parar := context.WithTimeout(ctx, PlazoConexion)
	err = b.enviarSMTP(ctxEnvio, c, e.de, e.para, crudo)
	parar()
	switch {
	case err == nil:
		_, _ = b.x.db.ExecContext(ctx, "DELETE FROM salida WHERE id = ?", e.id)
		_ = os.Remove(b.rutaSalida(e.id))
		if err := b.copiaEnviada(ctx, c, crudo); err != nil {
			b.fallo(c, fmt.Errorf("correo: el mensaje salió, pero no se pudo guardar en Enviados: %w", err))
		}
		b.avisar()
	case errors.Is(err, ErrSinRed) || errors.Is(err, ErrAcceso):
		// Sin red, o la contraseña: se espera, con una espera que crece.
		espera := min(time.Duration(1<<min(e.intentos, 6))*30*time.Second, 30*time.Minute)
		_, _ = b.x.db.ExecContext(ctx, "UPDATE salida SET estado = ?, intentos = intentos + 1, sale = ?, error = ? WHERE id = ?",
			salidaPendiente, time.Now().Add(espera).Unix(), err.Error(), e.id)
		if errors.Is(err, ErrAcceso) {
			b.marcarEstado(c, err)
			b.fallo(c, err)
		}
	default:
		_, _ = b.x.db.ExecContext(ctx, "UPDATE salida SET estado = ?, error = ? WHERE id = ?", salidaError, err.Error(), e.id)
		b.fallo(c, err)
	}
}

// copiaEnviada deja lo enviado en Enviados: en local ya, y en el servidor con APPEND
// si el servidor no la guarda él solo (con GuardaEnviados, la suya se empareja por
// Message-ID en la próxima vuelta).
func (b *Buzon) copiaEnviada(ctx context.Context, c Cuenta, crudo []byte) error {
	f, err := b.x.carpetaDeRol(ctx, c.ID, "sent")
	if err != nil {
		return err
	}
	s, err := b.guardarLocal(ctx, c.ID, f.nombre, crudo, []string{api.MailFlagSeen})
	if err != nil {
		return err
	}
	if !f.local && !c.GuardaEnviados {
		return encolar(ctx, b.x.db, c.ID, pendiente{tipo: pendAnadir, mensaje: s.id, destino: f.nombre})
	}
	return nil
}

// guardarLocal guarda en local, sin UID, un mensaje compuesto aquí (enviado o
// borrador).
func (b *Buzon) guardarLocal(ctx context.Context, cuenta, carpeta string, crudo []byte, marcas []string) (sobre, error) {
	s := sobre{cuenta: cuenta, carpeta: carpeta, recibido: time.Now().UTC(), tamano: int64(len(crudo)), marcas: marcas}
	refs := sobreDeCabecera(&s, crudo)
	if s.fecha.IsZero() {
		s.fecha = s.recibido
	}
	conCuerpo(ctx, &s, crudo)
	tx, err := b.x.db.BeginTx(ctx, nil)
	if err != nil {
		return sobre{}, err
	}
	defer func() { _ = tx.Rollback() }()
	b.hiloDe(ctx, tx, &s, refs)
	if err := insertar(ctx, tx, &s); err != nil {
		return sobre{}, err
	}
	if err := b.escribirEML(cuenta, s.id, crudo); err != nil {
		return sobre{}, err
	}
	return s, tx.Commit()
}
