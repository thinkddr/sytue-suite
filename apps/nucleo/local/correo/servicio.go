// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package correo

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/mail"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
)

// Lo que la pantalla de Mail (`apps/fyne/apps/mail`, interfaz `Servicio`) pide, sobre
// el almacén local. Lo que no tiene sentido sin un servidor de Sytue —alias, buzones
// compartidos, S/MIME del servicio, vacaciones— devuelve
// ErrNoDisponible y la app lo oculta (§6.9, «No se monta»).

// ErrNoDisponible: la función no existe en Sytue Office.
var ErrNoDisponible = errors.New("correo: no disponible en Sytue Office")

// rolesFijos son las carpetas que se enseñan siempre, aunque ninguna cuenta las tenga
// todavía (los borradores viven en local hasta que hay dónde).
var rolesFijos = []string{"inbox", "snoozed", "drafts", "sent", "trash"}

var ordenRoles = []string{"inbox", "snoozed", "drafts", "sent", "archive", "junk", "trash"}

// Mailboxes son las carpetas: las de rol, juntas entre cuentas, y las de la persona,
// por cuenta. Los contadores son los de lo que hay en el equipo.
func (b *Buzon) Mailboxes(ctx context.Context) ([]api.Mailbox, error) {
	cs, err := b.x.carpetas(ctx, "")
	if err != nil {
		return nil, err
	}
	type cuenta struct{ total, sinLeer int }
	porRol := map[string]*cuenta{}
	porCarpeta := map[string]*cuenta{}
	// Lo pospuesto cuenta en «Pospuestos» y no en la carpeta donde sigue en el servidor.
	filas, err := b.x.db.QueryContext(ctx, `SELECT m.cuenta, m.carpeta, CASE WHEN m.pospuesto > 0 THEN 'snoozed' ELSE COALESCE(f.rol, '') END AS r,
		COUNT(*), SUM(CASE WHEN m.marcas LIKE '%\Seen%' THEN 0 ELSE 1 END)`+desde+`GROUP BY m.cuenta, m.carpeta, r`)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	for filas.Next() {
		var cu, ca, rol string
		var t, n int
		if err := filas.Scan(&cu, &ca, &rol, &t, &n); err != nil {
			return nil, err
		}
		k := porCarpeta[cu+"\x00"+ca]
		if k == nil {
			k = &cuenta{}
			porCarpeta[cu+"\x00"+ca] = k
		}
		k.total, k.sinLeer = t, n
		if rol != "" {
			r := porRol[rol]
			if r == nil {
				r = &cuenta{}
				porRol[rol] = r
			}
			r.total += t
			r.sinLeer += n
		}
	}
	if err := filas.Err(); err != nil {
		return nil, err
	}
	roles := slices.Clone(rolesFijos)
	for _, c := range cs {
		if c.rol != "" && !slices.Contains(roles, c.rol) {
			roles = append(roles, c.rol)
		}
	}
	slices.SortFunc(roles, func(a, x string) int { return slices.Index(ordenRoles, a) - slices.Index(ordenRoles, x) })
	var out []api.Mailbox
	for _, r := range roles {
		m := api.Mailbox{ID: idCarpeta("", "", r), Name: r, Role: r}
		if k := porRol[r]; k != nil {
			m.TotalEmails, m.UnreadEmails = k.total, k.sinLeer
		}
		out = append(out, m)
	}
	cuentas := b.reg.lista()
	for _, c := range cs {
		if c.rol != "" {
			continue
		}
		nombre := c.nombre
		if len(cuentas) > 1 {
			if cu, ok := b.reg.cuenta(c.cuenta); ok {
				nombre += " · " + cu.Titulo()
			}
		}
		m := api.Mailbox{ID: idCarpeta(c.cuenta, c.nombre, ""), Name: nombre}
		if k := porCarpeta[c.cuenta+"\x00"+c.nombre]; k != nil {
			m.TotalEmails, m.UnreadEmails = k.total, k.sinLeer
		}
		out = append(out, m)
	}
	return out, nil
}

func (b *Buzon) pagina(ctx context.Context, donde string, args []any, cursor string, limite int) ([]api.MailMessage, string, error) {
	limite = cmp.Or(min(max(limite, 1), 500), 50)
	if f, i, ok := leerCursor(cursor); ok {
		donde += " AND (" + ordenFecha + " < ? OR (" + ordenFecha + " = ? AND m.id < ?))"
		args = append(args, f, f, i)
	}
	ss, err := b.x.sobres(ctx, "WHERE "+donde+" ORDER BY "+ordenFecha+" DESC, m.id DESC LIMIT ?", append(args, limite+1)...)
	if err != nil {
		return nil, "", err
	}
	siguiente := ""
	if len(ss) > limite {
		ss = ss[:limite]
		siguiente = cursorDe(ss[len(ss)-1])
	}
	ms, err := b.x.mensajes(ctx, ss)
	return ms, siguiente, err
}

// Messages es una página de una carpeta, de lo más nuevo a lo más viejo.
func (b *Buzon) Messages(ctx context.Context, carpeta, cursor string, limite int) ([]api.MailMessage, string, error) {
	donde, args, err := filtroCarpeta(carpeta)
	if err != nil {
		return nil, "", err
	}
	return b.pagina(ctx, donde, args, cursor, limite)
}

// LabelMessages es una página de una etiqueta (local).
func (b *Buzon) LabelMessages(ctx context.Context, etiqueta, cursor string, limite int) ([]api.MailMessage, string, error) {
	return b.pagina(ctx, "m.id IN (SELECT mensaje FROM mensaje_etiquetas WHERE etiqueta = ?)", []any{etiqueta}, cursor, limite)
}

// Search busca en lo que hay en el equipo: sobres y cuerpos descargados, sin acentos
// ni mayúsculas. Entiende `from:`, `to:`, `subject:`, `is:unread`, `is:starred` y
// `has:attachment`; el resto son palabras que tienen que aparecer todas.
func (b *Buzon) Search(ctx context.Context, q string, desdePos, limite int) ([]api.MailMessage, error) {
	ts := buscar.Terminos(q)
	conds := make([]string, 0, len(ts))
	var args []any
	for _, t := range ts {
		cond, as := condicionBusqueda(t)
		conds = append(conds, cond)
		args = append(args, as...)
	}
	if len(conds) == 0 {
		return nil, nil
	}
	limite = cmp.Or(min(max(limite, 1), 500), 50)
	ss, err := b.x.sobres(ctx, "WHERE "+strings.Join(conds, " AND ")+" ORDER BY m.fecha DESC, m.id DESC LIMIT ? OFFSET ?", append(args, limite, max(desdePos, 0))...)
	if err != nil {
		return nil, err
	}
	return b.x.mensajes(ctx, ss)
}

// condicionBusqueda es la condición SQL (con sus argumentos) de un término de Search.
func condicionBusqueda(t string) (string, []any) {
	clave, valor, ok := strings.Cut(t, ":")
	es := func(nombres ...string) bool {
		return ok && slices.ContainsFunc(nombres, func(n string) bool { return strings.EqualFold(clave, n) })
	}
	switch {
	case es("is") && (valor == "unread" || valor == "noleido"):
		return "m.marcas NOT LIKE '%\\Seen%'", nil
	case es("is") && (valor == "starred" || valor == "flagged" || valor == "destacado"):
		return "m.marcas LIKE '%\\Flagged%'", nil
	case es("has") && strings.HasPrefix(valor, "attach"):
		return "m.adjuntos = 1", nil
	case valor != "" && es("from", "de"):
		v := "%" + strings.ToLower(valor) + "%"
		return "(lower(m.de) LIKE ? OR lower(m.de_nombre) LIKE ?)", []any{v, v}
	case valor != "" && es("to", "para"):
		v := "%" + strings.ToLower(valor) + "%"
		return "(lower(m.para) LIKE ? OR lower(m.cc) LIKE ?)", []any{v, v}
	case valor != "" && es("subject", "asunto"):
		return "m.buscable LIKE ?", []any{"%" + buscar.Plegar(valor) + "%"}
	}
	return "m.buscable LIKE ?", []any{"%" + buscar.Plegar(t) + "%"}
}

// Thread es la conversación entera, entre carpetas y cuentas, de la más vieja a la más
// nueva.
func (b *Buzon) Thread(ctx context.Context, hilo string) ([]api.MailMessage, error) {
	ss, err := b.x.sobres(ctx, "WHERE m.hilo = ? ORDER BY m.fecha, m.id", hilo)
	if err != nil {
		return nil, err
	}
	return b.x.mensajes(ctx, ss)
}

func leerID(id string) (int64, error) {
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return 0, ErrNoEncontrado
	}
	return n, nil
}

// Raw es el .eml del mensaje. Si aún no está en el equipo, se baja ahora (y se queda).
func (b *Buzon) Raw(ctx context.Context, id string) (io.ReadCloser, error) {
	n, err := leerID(id)
	if err != nil {
		return nil, err
	}
	s, err := b.x.sobre(ctx, n)
	if err != nil {
		return nil, err
	}
	if f, err := os.Open(b.rutaEML(s.cuenta, s.id)); err == nil {
		return f, nil
	}
	crudo, err := b.traerCuerpo(ctx, s)
	if err != nil {
		return nil, err
	}
	b.avisar()
	return io.NopCloser(bytes.NewReader(crudo)), nil
}

// esLocal dice si una carpeta solo existe en este equipo.
func (b *Buzon) esLocal(ctx context.Context, tx execer, cuenta, carpeta string) bool {
	var l bool
	if err := tx.QueryRowContext(ctx, "SELECT local FROM carpetas WHERE cuenta = ? AND nombre = ?", cuenta, carpeta).Scan(&l); err != nil {
		return true
	}
	return l
}

// SetFlagMany pone o quita una marca (`\Seen`, `\Flagged`, `\Answered`) en local y la
// apunta para subirla.
func (b *Buzon) SetFlagMany(ctx context.Context, ids []string, marca string, poner bool) error {
	tx, err := b.x.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, id := range ids {
		n, err := leerID(id)
		if err != nil {
			continue
		}
		var cuenta, carpeta, marcas string
		if err := tx.QueryRowContext(ctx, "SELECT cuenta, carpeta, marcas FROM mensajes WHERE id = ?", n).Scan(&cuenta, &carpeta, &marcas); err != nil {
			continue
		}
		ms := deLista(marcas)
		if tieneMarca(ms, marca) == poner {
			continue
		}
		if _, err := tx.ExecContext(ctx, "UPDATE mensajes SET marcas = ? WHERE id = ?", jsonLista(conMarca(ms, marca, poner)), n); err != nil {
			return err
		}
		if err := apuntar(ctx, tx, n); err != nil {
			return err
		}
		if !b.esLocal(ctx, tx, cuenta, carpeta) {
			if err := encolar(ctx, tx, cuenta, pendiente{tipo: pendMarcar, mensaje: n, marca: marca, poner: poner}); err != nil {
				return err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	b.SincronizarYa()
	return nil
}

// destinoMovimiento es la carpeta adonde va s al moverlo: la del rol en su cuenta o, sin
// rol, la carpeta nombreDest de cuentaDest, que tiene que ser la suya.
func (b *Buzon) destinoMovimiento(ctx context.Context, s sobre, rol, cuentaDest, nombreDest string) (carpeta, error) {
	if rol != "" {
		return b.x.carpetaDeRol(ctx, s.cuenta, rol)
	}
	if cuentaDest != s.cuenta {
		return carpeta{}, errors.New("correo: no se puede mover un mensaje a una carpeta de otra cuenta")
	}
	dest := carpeta{cuenta: cuentaDest, nombre: nombreDest}
	dest.local = b.esLocal(ctx, b.x.db, cuentaDest, nombreDest)
	return dest, nil
}

// MoveMany lleva mensajes a otra carpeta: en local ya, en el servidor en la próxima
// vuelta. Una carpeta de rol es la de la cuenta de cada mensaje; entre cuentas no se
// mueve.
func (b *Buzon) MoveMany(ctx context.Context, ids []string, carpetaID string) error {
	rol, cuentaDest, nombreDest, ok := leerIDCarpeta(carpetaID)
	if !ok {
		return ErrNoEncontrado
	}
	type mov struct {
		id             int64
		cuenta, origen string
		uid            uint32
		destino        carpeta
	}
	var movs []mov
	for _, id := range ids {
		n, err := leerID(id)
		if err != nil {
			continue
		}
		s, err := b.x.sobre(ctx, n)
		if err != nil {
			continue
		}
		dest, err := b.destinoMovimiento(ctx, s, rol, cuentaDest, nombreDest)
		if err != nil {
			return err
		}
		if dest.nombre == s.carpeta {
			continue
		}
		origenLocal := b.esLocal(ctx, b.x.db, s.cuenta, s.carpeta)
		if dest.local && !origenLocal && s.uid > 0 {
			return fmt.Errorf("correo: la cuenta no tiene carpeta «%s» en el servidor", rol)
		}
		movs = append(movs, mov{id: n, cuenta: s.cuenta, origen: s.carpeta, uid: s.uid, destino: dest})
	}
	tx, err := b.x.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, m := range movs {
		if _, err := tx.ExecContext(ctx, "UPDATE mensajes SET carpeta = ?, uid = 0 WHERE id = ?", m.destino.nombre, m.id); err != nil {
			return err
		}
		if err := apuntar(ctx, tx, m.id); err != nil {
			return err
		}
		switch {
		case m.uid > 0 && !m.destino.local:
			err = encolar(ctx, tx, m.cuenta, pendiente{tipo: pendMover, mensaje: m.id, carpeta: m.origen, uid: m.uid, destino: m.destino.nombre})
		case m.uid == 0:
			// Ya tenía un movimiento pendiente: ahora va a otro sitio.
			_, err = tx.ExecContext(ctx, "UPDATE pendientes SET destino = ? WHERE mensaje = ? AND tipo = ?", m.destino.nombre, m.id, pendMover)
		}
		if err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	b.SincronizarYa()
	return nil
}

// Delete borra un mensaje para siempre (desde Papelera).
func (b *Buzon) Delete(ctx context.Context, id string) error {
	n, err := leerID(id)
	if err != nil {
		return err
	}
	s, err := b.x.sobre(ctx, n)
	if errors.Is(err, ErrNoEncontrado) {
		return nil
	}
	if err != nil {
		return err
	}
	tx, err := b.x.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if s.uid > 0 && !b.esLocal(ctx, tx, s.cuenta, s.carpeta) {
		if err := encolar(ctx, tx, s.cuenta, pendiente{tipo: pendBorrar, carpeta: s.carpeta, uid: s.uid}); err != nil {
			return err
		}
	} else if _, err := tx.ExecContext(ctx, "UPDATE pendientes SET tipo = ?, mensaje = 0 WHERE mensaje = ? AND tipo = ?", pendBorrar, n, pendMover); err != nil {
		// Movido y sin subir todavía: se borra en su carpeta de origen.
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM pendientes WHERE mensaje = ? AND tipo != ?", n, pendBorrar); err != nil {
		return err
	}
	if err := b.borrarFila(ctx, tx, n, s.cuenta); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	b.SincronizarYa()
	return nil
}

// Changes es el delta desde un estado: lo que cambió en el índice (llegado, movido,
// marcado) y lo que ya no está.
func (b *Buzon) Changes(ctx context.Context, desdeEstado string) (api.MailCambios, error) {
	ultimo := b.ultimoCambio(ctx)
	out := api.MailCambios{State: strconv.FormatInt(ultimo, 10)}
	n, err := strconv.ParseInt(desdeEstado, 10, 64)
	if desdeEstado == "" || err != nil || n >= ultimo {
		// Sin estado, o uno que no es de este índice: se contesta con el actual y nada más.
		return out, nil //nolint:nilerr // un estado que no se entiende vale lo mismo que «sin estado».
	}
	const tope = 500
	filas, err := b.x.db.QueryContext(ctx, "SELECT mensaje, MAX(seq) FROM cambios WHERE seq > ? GROUP BY mensaje ORDER BY MAX(seq) LIMIT ?", n, tope+1)
	if err != nil {
		return api.MailCambios{}, err
	}
	defer filas.Close()
	var ids []int64
	hasta := n
	for filas.Next() {
		var id, seq int64
		if err := filas.Scan(&id, &seq); err != nil {
			return api.MailCambios{}, err
		}
		ids = append(ids, id)
		hasta = max(hasta, seq)
	}
	// Next ya cerró las filas al acabarlas: lo de abajo puede usar la única conexión.
	if err := filas.Err(); err != nil {
		return api.MailCambios{}, err
	}
	if len(ids) > tope {
		ids, out.HasMore = ids[:tope], true
		out.State = strconv.FormatInt(hasta, 10)
	}
	vivos := map[int64]sobre{}
	if len(ids) > 0 {
		marcas := strings.Repeat(",?", len(ids))[1:]
		args := make([]any, len(ids))
		for i, id := range ids {
			args[i] = id
		}
		ss, err := b.x.sobres(ctx, "WHERE m.id IN ("+marcas+")", args...)
		if err != nil {
			return api.MailCambios{}, err
		}
		for _, s := range ss {
			vivos[s.id] = s
		}
		var lista []sobre
		for _, id := range ids {
			if s, ok := vivos[id]; ok {
				lista = append(lista, s)
			} else {
				out.Destroyed = append(out.Destroyed, strconv.FormatInt(id, 10))
			}
		}
		if out.Changed, err = b.x.mensajes(ctx, lista); err != nil {
			return api.MailCambios{}, err
		}
	}
	return out, nil
}

// Llegadas avisa cada vez que una sincronización trae algo. No vuelve hasta que se
// corta el contexto.
func (b *Buzon) Llegadas(ctx context.Context, alLlegar func()) error {
	quitar := b.AlCambiar(alLlegar)
	defer quitar()
	<-ctx.Done()
	return ctx.Err()
}

// ---- Etiquetas (solo locales: no se sincronizan, M-08) ----

func (b *Buzon) Labels(ctx context.Context) ([]api.MailLabel, error) {
	filas, err := b.x.db.QueryContext(ctx, `SELECT e.id, e.nombre, e.color, COUNT(me.mensaje),
		COALESCE(SUM(CASE WHEN m.marcas LIKE '%\Seen%' THEN 0 WHEN m.id IS NULL THEN 0 ELSE 1 END), 0)
		FROM etiquetas e LEFT JOIN mensaje_etiquetas me ON me.etiqueta = e.id LEFT JOIN mensajes m ON m.id = me.mensaje
		GROUP BY e.id ORDER BY e.nombre`)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	var out []api.MailLabel
	for filas.Next() {
		var l api.MailLabel
		if err := filas.Scan(&l.ID, &l.Name, &l.Color, &l.TotalEmails, &l.UnreadEmails); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, filas.Err()
}

func (b *Buzon) CreateLabel(ctx context.Context, nombre, color string) (api.MailLabel, error) {
	nombre = strings.TrimSpace(nombre)
	if nombre == "" {
		return api.MailLabel{}, errors.New("correo: la etiqueta necesita un nombre")
	}
	l := api.MailLabel{ID: "et-" + uuid.NewString()[:8], Name: nombre, Color: color}
	_, err := b.x.db.ExecContext(ctx, "INSERT INTO etiquetas (id, nombre, color) VALUES (?, ?, ?)", l.ID, l.Name, l.Color)
	return l, err
}

func (b *Buzon) UpdateLabel(ctx context.Context, id, nombre, color string) error {
	_, err := b.x.db.ExecContext(ctx, "UPDATE etiquetas SET nombre = ?, color = ? WHERE id = ?", strings.TrimSpace(nombre), color, id)
	return err
}

func (b *Buzon) DeleteLabel(ctx context.Context, id string) error {
	tx, err := b.x.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, "INSERT INTO cambios (mensaje) SELECT mensaje FROM mensaje_etiquetas WHERE etiqueta = ?", id); err != nil {
		return err
	}
	for _, q := range []string{"DELETE FROM mensaje_etiquetas WHERE etiqueta = ?", "DELETE FROM etiquetas WHERE id = ?"} {
		if _, err := tx.ExecContext(ctx, q, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (b *Buzon) ApplyLabel(ctx context.Context, etiqueta string, ids []string, poner bool) error {
	tx, err := b.x.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	q := "DELETE FROM mensaje_etiquetas WHERE mensaje = ? AND etiqueta = ?"
	if poner {
		q = "INSERT OR IGNORE INTO mensaje_etiquetas (mensaje, etiqueta) VALUES (?, ?)"
	}
	for _, id := range ids {
		n, err := leerID(id)
		if err != nil {
			continue
		}
		if _, err := tx.ExecContext(ctx, q, n, etiqueta); err != nil {
			return err
		}
		if err := apuntar(ctx, tx, n); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ---- Escribir ----

func (b *Buzon) Signature(ctx context.Context) (string, error) { return b.x.ajuste(ctx, "firma") }

func (b *Buzon) SetSignature(ctx context.Context, firma string) error {
	if len(firma) > 8<<10 {
		return errors.New("correo: la firma es demasiado larga")
	}
	return b.x.ponerAjuste(ctx, "firma", firma)
}

// Aliases son las direcciones «De»: la de cada cuenta conectada.
func (b *Buzon) Aliases(context.Context) (string, []api.MailAlias, error) {
	cs := b.reg.lista()
	if len(cs) == 0 {
		return "", nil, nil
	}
	var otras []api.MailAlias
	for _, c := range cs[1:] {
		otras = append(otras, api.MailAlias{ID: c.ID, Address: c.Direccion})
	}
	return cs[0].Direccion, otras, nil
}

// Send deja el mensaje en la bandeja de salida; sale al vencer la ventana de deshacer
// (o en cuanto hay red).
func (b *Buzon) Send(ctx context.Context, de string, destinatarios []string, crudo []byte, undoSegundos int) (api.MailEnvio, error) {
	sale := time.Now().Add(time.Duration(max(undoSegundos, 0)) * time.Second)
	id, err := b.encolarSalida(ctx, de, destinatarios, crudo, sale, salidaPendiente)
	if err != nil {
		return api.MailEnvio{}, err
	}
	estado := "undo"
	if undoSegundos <= 0 {
		estado = "queued"
		b.SincronizarYa()
	}
	return api.MailEnvio{Status: estado, ID: id, ReleaseAt: sale}, nil
}

func (b *Buzon) Schedule(ctx context.Context, de string, destinatarios []string, crudo []byte, cuando time.Time) (api.MailEnvio, error) {
	id, err := b.encolarSalida(ctx, de, destinatarios, crudo, cuando, salidaProgramada)
	if err != nil {
		return api.MailEnvio{}, err
	}
	return api.MailEnvio{Status: "scheduled", ID: id, ReleaseAt: cuando}, nil
}

func (b *Buzon) Scheduled(ctx context.Context) ([]api.MailScheduled, error) {
	filas, err := b.x.db.QueryContext(ctx, "SELECT id, de, destinatarios, asunto, sale, creado FROM salida WHERE estado = ? ORDER BY sale", salidaProgramada)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	var out []api.MailScheduled
	for filas.Next() {
		var s api.MailScheduled
		var para string
		var sale, creado int64
		if err := filas.Scan(&s.ID, &s.From, &para, &s.Subject, &sale, &creado); err != nil {
			return nil, err
		}
		s.Recipients, s.ReleaseAt, s.CreatedAt = deLista(para), deUnix(sale), deUnix(creado)
		out = append(out, s)
	}
	return out, filas.Err()
}

func (b *Buzon) Reschedule(ctx context.Context, id string, cuando time.Time) error {
	r, err := b.x.db.ExecContext(ctx, "UPDATE salida SET sale = ? WHERE id = ? AND estado = ?", cuando.Unix(), id, salidaProgramada)
	if err != nil {
		return err
	}
	if n, _ := r.RowsAffected(); n == 0 {
		return api.ErrDemasiadoTarde
	}
	return nil
}

// CancelOutbound deshace un envío que aún no ha salido.
func (b *Buzon) CancelOutbound(ctx context.Context, id string) error {
	r, err := b.x.db.ExecContext(ctx, "DELETE FROM salida WHERE id = ? AND estado IN (?, ?, ?)", id, salidaPendiente, salidaProgramada, salidaError)
	if err != nil {
		return err
	}
	if n, _ := r.RowsAffected(); n == 0 {
		return api.ErrDemasiadoTarde
	}
	_ = os.Remove(b.rutaSalida(id))
	return nil
}

// SaveDraft guarda un borrador en los Borradores de su cuenta y sustituye al anterior
// (id): en local ya y, si la cuenta tiene «Drafts» en el servidor, con APPEND en la
// próxima vuelta (M-02), de modo que el borrador se sigue en otro cliente. El anterior
// se borra allí con el mismo cambio.
func (b *Buzon) SaveDraft(ctx context.Context, id, de string, crudo []byte) (api.MailMessage, error) {
	c, err := b.cuentaDe(de)
	if err != nil {
		return api.MailMessage{}, err
	}
	if id != "" {
		if err := b.Delete(ctx, id); err != nil && !errors.Is(err, ErrNoEncontrado) {
			return api.MailMessage{}, err
		}
	}
	f, err := b.x.carpetaDeRol(ctx, c.ID, "drafts")
	if err != nil {
		return api.MailMessage{}, err
	}
	s, err := b.guardarLocal(ctx, c.ID, f.nombre, crudo, []string{api.MailFlagSeen, string(imap.FlagDraft)})
	if err != nil {
		return api.MailMessage{}, err
	}
	if !f.local {
		if err := encolar(ctx, b.x.db, c.ID, pendiente{tipo: pendAnadir, mensaje: s.id, destino: f.nombre}); err != nil {
			return api.MailMessage{}, err
		}
		b.despertarCuenta(c.ID)
	}
	s, err = b.x.sobre(ctx, s.id)
	if err != nil {
		return api.MailMessage{}, err
	}
	ms, err := b.x.mensajes(ctx, []sobre{s})
	if err != nil || len(ms) == 0 {
		return api.MailMessage{}, cmp.Or(err, ErrNoEncontrado)
	}
	b.avisar()
	return ms[0], nil
}

// CuentaImportados es la «cuenta» de los .eml abiertos desde el disco: solo local.
const CuentaImportados = "local"

// nombreImportados es el nombre de su carpeta.
const nombreImportados = "Importados"

// CarpetaImportados es el id de la carpeta «Importados» (la de ImportarEML), para leerla
// con Messages: «Traer desde Sytue» mira lo que ya hay en ella para no repetirlo.
var CarpetaImportados = idCarpeta(CuentaImportados, nombreImportados, "")

// MessageIDDe es el Message-ID de un .eml como lo guarda el índice (sin «<>»); vacío si
// no lo lleva o no se lee.
func MessageIDDe(crudo []byte) string {
	m, err := mail.ReadMessage(bytes.NewReader(crudo))
	if err != nil {
		return ""
	}
	if ids := idsMensaje(m.Header.Get("Message-ID")); len(ids) > 0 {
		return ids[0]
	}
	return ""
}

// ImportarEML guarda un .eml abierto desde el equipo («Abrir con», arrastrar) en la
// carpeta local «Importados», para leerlo con el lector de siempre.
func (b *Buzon) ImportarEML(ctx context.Context, crudo []byte) (api.MailMessage, error) {
	const nombre = nombreImportados
	if _, err := b.x.db.ExecContext(ctx, "INSERT OR IGNORE INTO carpetas (cuenta, nombre, local) VALUES (?, ?, 1)", CuentaImportados, nombre); err != nil {
		return api.MailMessage{}, err
	}
	s, err := b.guardarLocal(ctx, CuentaImportados, nombre, crudo, []string{api.MailFlagSeen})
	if err != nil {
		return api.MailMessage{}, err
	}
	if s, err = b.x.sobre(ctx, s.id); err != nil {
		return api.MailMessage{}, err
	}
	ms, err := b.x.mensajes(ctx, []sobre{s})
	if err != nil || len(ms) == 0 {
		return api.MailMessage{}, cmp.Or(err, ErrNoEncontrado)
	}
	b.avisar()
	return ms[0], nil
}

// ---- Carpetas de la persona (en el servidor: piden red) ----

func (b *Buzon) conCuenta(ctx context.Context, cuentaID string, f func(context.Context, Cuenta, *imapConexion) error) error {
	c, ok := b.reg.cuenta(cuentaID)
	if !ok {
		return ErrSinCuenta
	}
	ctx, parar := context.WithTimeout(ctx, PlazoConexion)
	defer parar()
	cli, err := b.abrirIMAP(ctx, c)
	if err != nil {
		return err
	}
	defer func() { _ = cli.Close() }()
	return f(ctx, c, &imapConexion{cli})
}

// CreateMailbox crea la carpeta en la primera cuenta. Con varias, la pantalla pregunta
// en cuál y llama a CreateMailboxIn.
func (b *Buzon) CreateMailbox(ctx context.Context, nombre string) (api.Mailbox, error) {
	cs := b.reg.lista()
	if len(cs) == 0 {
		return api.Mailbox{}, errors.New("correo: añade una cuenta de correo primero")
	}
	return b.CreateMailboxIn(ctx, cs[0].ID, nombre)
}

// CuentasCarpetas son las cuentas donde se puede crear una carpeta: id y dirección.
func (b *Buzon) CuentasCarpetas() []api.MailAlias {
	cs := b.reg.lista()
	out := make([]api.MailAlias, 0, len(cs))
	for _, c := range cs {
		out = append(out, api.MailAlias{ID: c.ID, Address: c.Titulo()})
	}
	return out
}

// errCarpeta traduce lo que contesta el servidor a lo que la pantalla sabe explicar: una
// carpeta que ya existe es el 409 del servicio.
func errCarpeta(err error) error {
	var ie *imap.Error
	if errors.As(err, &ie) && ie.Code == imap.ResponseCodeAlreadyExists {
		return &api.Error{Status: 409, Body: ie.Text}
	}
	return err
}

// CreateMailboxIn crea la carpeta nombre en la cuenta del servidor (CREATE).
func (b *Buzon) CreateMailboxIn(ctx context.Context, cuentaID, nombre string) (api.Mailbox, error) {
	nombre = strings.TrimSpace(nombre)
	if nombre == "" || strings.ContainsAny(nombre, "\x00\r\n") {
		return api.Mailbox{}, &api.Error{Status: 400}
	}
	err := b.conCuenta(ctx, cuentaID, func(ctx context.Context, c Cuenta, x *imapConexion) error {
		if err := x.cli.Create(nombre, nil).Wait(); err != nil {
			return errCarpeta(err)
		}
		_, err := b.x.db.ExecContext(ctx, "INSERT OR IGNORE INTO carpetas (cuenta, nombre) VALUES (?, ?)", c.ID, nombre)
		return err
	})
	if err != nil {
		return api.Mailbox{}, err
	}
	b.avisar()
	return api.Mailbox{ID: idCarpeta(cuentaID, nombre, ""), Name: nombre}, nil
}

// sinCuenta quita del nombre que se escribe el « · cuenta» con que Mailboxes distingue
// carpetas de cuentas distintas.
func (b *Buzon) sinCuenta(cuenta, nombre string) string {
	if c, ok := b.reg.cuenta(cuenta); ok {
		nombre = strings.TrimSuffix(nombre, " · "+c.Titulo())
	}
	return strings.TrimSpace(nombre)
}

func (b *Buzon) RenameMailbox(ctx context.Context, id, nombre string) (api.MailboxSetError, error) {
	rol, cuenta, viejo, ok := leerIDCarpeta(id)
	if !ok || rol != "" {
		return "", ErrNoDisponible
	}
	nombre = b.sinCuenta(cuenta, nombre)
	if nombre == "" || nombre == viejo {
		return "", nil
	}
	err := b.conCuenta(ctx, cuenta, func(ctx context.Context, c Cuenta, x *imapConexion) error {
		if err := x.cli.Rename(viejo, nombre, nil).Wait(); err != nil {
			var ie *imap.Error
			if errors.As(err, &ie) && ie.Code == imap.ResponseCodeAlreadyExists {
				return errYaExiste
			}
			return err
		}
		tx, err := b.x.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback() }()
		for _, q := range []string{
			"UPDATE carpetas SET nombre = ? WHERE cuenta = ? AND nombre = ?", "UPDATE mensajes SET carpeta = ? WHERE cuenta = ? AND carpeta = ?",
			"UPDATE pendientes SET carpeta = ? WHERE cuenta = ? AND carpeta = ?",
		} {
			if _, err := tx.ExecContext(ctx, q, nombre, cuenta, viejo); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO cambios (mensaje) SELECT id FROM mensajes WHERE cuenta = ? AND carpeta = ?", cuenta, nombre); err != nil {
			return err
		}
		return tx.Commit()
	})
	if errors.Is(err, errYaExiste) {
		return "alreadyExists", nil
	}
	if err == nil {
		b.avisar()
	}
	return "", err
}

var errYaExiste = errors.New("correo: ya hay una carpeta con ese nombre")

func (b *Buzon) DeleteMailbox(ctx context.Context, id string) (api.MailboxSetError, error) {
	rol, cuenta, nombre, ok := leerIDCarpeta(id)
	if !ok || rol != "" {
		return "", ErrNoDisponible
	}
	return "", b.conCuenta(ctx, cuenta, func(ctx context.Context, c Cuenta, x *imapConexion) error {
		if err := x.cli.Delete(nombre).Wait(); err != nil {
			return err
		}
		tx, err := b.x.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback() }()
		if err := b.borrarFilas(ctx, tx, "cuenta = ? AND carpeta = ?", cuenta, nombre); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM carpetas WHERE cuenta = ? AND nombre = ?", cuenta, nombre); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM pendientes WHERE cuenta = ? AND (carpeta = ? OR destino = ?)", cuenta, nombre, nombre); err != nil {
			return err
		}
		return tx.Commit()
	})
}

// ---- Reglas (locales, M-09) ----

func (b *Buzon) reglas(ctx context.Context) ([]api.MailFilter, error) {
	v, err := b.x.ajuste(ctx, "reglas")
	if err != nil || v == "" {
		return nil, err
	}
	var rs []api.MailFilter
	if err := json.Unmarshal([]byte(v), &rs); err != nil {
		return nil, err
	}
	return rs, nil
}

func (b *Buzon) ponerReglas(ctx context.Context, rs []api.MailFilter) error {
	raw, err := json.Marshal(rs)
	if err != nil {
		return err
	}
	return b.x.ponerAjuste(ctx, "reglas", string(raw))
}

func (b *Buzon) Filters(ctx context.Context) ([]api.MailFilter, error) { return b.reglas(ctx) }

func (b *Buzon) CreateFilter(ctx context.Context, f api.MailFilter) (api.MailFilter, error) {
	rs, err := b.reglas(ctx)
	if err != nil {
		return api.MailFilter{}, err
	}
	f.ID = uuid.NewString()
	return f, b.ponerReglas(ctx, append(rs, f))
}

func (b *Buzon) UpdateFilter(ctx context.Context, f api.MailFilter) error {
	rs, err := b.reglas(ctx)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(rs, func(r api.MailFilter) bool { return r.ID == f.ID })
	if i < 0 {
		return ErrNoEncontrado
	}
	rs[i] = f
	return b.ponerReglas(ctx, rs)
}

func (b *Buzon) DeleteFilter(ctx context.Context, id string) error {
	rs, err := b.reglas(ctx)
	if err != nil {
		return err
	}
	return b.ponerReglas(ctx, slices.DeleteFunc(rs, func(r api.MailFilter) bool { return r.ID == id }))
}

// aplicarReglas pasa las reglas activas por lo recién llegado a la entrada. Dice si
// apuntó algún cambio.
func (b *Buzon) aplicarReglas(ctx context.Context, ids []int64) bool {
	rs, err := b.reglas(ctx)
	if err != nil || len(rs) == 0 {
		return false
	}
	slices.SortStableFunc(rs, func(a, x api.MailFilter) int { return a.Priority - x.Priority })
	hubo := false
	for _, n := range ids {
		s, err := b.x.sobre(ctx, n)
		if err != nil {
			continue
		}
		// La primera que encaja, como en el servicio.
		if i := slices.IndexFunc(rs, func(r api.MailFilter) bool { return encaja(r, s) }); i >= 0 {
			hubo = b.aplicarRegla(ctx, strconv.FormatInt(n, 10), rs[i]) || hubo
		}
	}
	return hubo
}

// encaja dice si una regla activa se aplica a un sobre.
func encaja(r api.MailFilter, s sobre) bool {
	if !r.Enabled || (r.MatchFrom == "" && r.MatchSubject == "") {
		return false
	}
	if r.MatchFrom != "" && !buscar.Contiene(s.de+" "+s.deNombre, r.MatchFrom) {
		return false
	}
	return r.MatchSubject == "" || buscar.Contiene(s.asunto, r.MatchSubject)
}

// aplicarRegla hace lo que pide una regla con el mensaje id. Dice si apuntó algún cambio.
func (b *Buzon) aplicarRegla(ctx context.Context, id string, r api.MailFilter) bool {
	hubo := false
	if r.MarkRead {
		hubo = b.SetFlagMany(ctx, []string{id}, api.MailFlagSeen, true) == nil || hubo
	}
	if r.MarkFlagged {
		hubo = b.SetFlagMany(ctx, []string{id}, api.MailFlagFlagged, true) == nil || hubo
	}
	if r.MoveTo != "" {
		hubo = b.MoveMany(ctx, []string{id}, r.MoveTo) == nil || hubo
	}
	return hubo
}

// ---- Lo que no se monta en Sytue Office ----

func (b *Buzon) Vacation(context.Context) (api.MailVacation, error) {
	return api.MailVacation{}, ErrNoDisponible
}
func (b *Buzon) SetVacation(context.Context, api.MailVacation) error { return ErrNoDisponible }
func (b *Buzon) CreateAlias(context.Context, string) (api.MailAlias, error) {
	return api.MailAlias{}, ErrNoDisponible
}

func (b *Buzon) DeleteAlias(context.Context, string) error { return ErrNoDisponible }

func (b *Buzon) SmimeAnchor(context.Context) ([]byte, error) { return nil, ErrNoDisponible }

func (b *Buzon) SetSmimeIdentity(context.Context, string) error            { return ErrNoDisponible }
func (b *Buzon) SharedMailboxes(context.Context) ([]api.MailShared, error) { return nil, nil }
func (b *Buzon) CreateSharedMailbox(context.Context, string, string) (api.MailShared, error) {
	return api.MailShared{}, ErrNoDisponible
}

func (b *Buzon) SharedFolders(context.Context, string) ([]api.Mailbox, error) {
	return nil, ErrNoDisponible
}

func (b *Buzon) SharedMessages(context.Context, string, string, string, int) ([]api.MailMessage, string, error) {
	return nil, "", ErrNoDisponible
}

func (b *Buzon) Grants(context.Context, string) ([]api.MailGrant, error) { return nil, ErrNoDisponible }
func (b *Buzon) Grant(context.Context, string, string, string) error     { return ErrNoDisponible }
func (b *Buzon) Revoke(context.Context, string, string) error            { return ErrNoDisponible }
func (b *Buzon) Delegations(context.Context) ([]api.MailGrant, error)    { return nil, nil }
func (b *Buzon) Delegate(context.Context, string, string) error          { return ErrNoDisponible }
func (b *Buzon) Undelegate(context.Context, string) error                { return ErrNoDisponible }
