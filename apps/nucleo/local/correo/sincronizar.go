// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package correo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
)

// La sincronización de una cuenta (D-16), en una conexión IMAP:
//
//  1. SUBIDA: la cola de cambios pendientes hechos en local (marcas, movimientos,
//     borrados, copias de lo enviado), en orden. Lo que el servidor rechaza se
//     descarta (y un movimiento rechazado se deshace en local); un corte de red deja
//     el resto para la próxima vuelta.
//  2. CARPETAS: LIST con SPECIAL-USE; las que desaparecen del servidor se van.
//  3. BAJADA por carpeta, incremental por UID: lo nuevo desde el último UID visto (la
//     primera vez, los Ventana más recientes); las marcas y los borrados de lo que ya
//     está; y los cuerpos de los más recientes para leer sin conexión.
//
// El servidor manda en lo que no se tocó en local: un mensaje con cambios pendientes
// no recibe las marcas del servidor hasta que se suben.

// Sincronizar hace una vuelta de todas las cuentas y espera a que acabe.
func (b *Buzon) Sincronizar(ctx context.Context) error {
	return b.sincronizarTodo(ctx)
}

func (b *Buzon) sincronizarTodo(ctx context.Context) error { return b.sincronizarCuentas(ctx, nil) }

// sincronizarCuentas hace una vuelta de las cuentas de solo (nil: todas).
func (b *Buzon) sincronizarCuentas(ctx context.Context, solo map[string]bool) error {
	b.sincro.Lock()
	defer b.sincro.Unlock()
	var errs []error
	for _, c := range b.reg.lista() {
		if c.Estado == EstadoReconectar || (solo != nil && !solo[c.ID]) {
			continue // no se insiste con una contraseña rechazada: la persona la cambia
		}
		antes := b.ultimoCambio(ctx)
		err := b.sincronizarCuenta(ctx, c)
		b.marcarEstado(c, err)
		if err != nil {
			if errors.Is(err, ErrAcceso) {
				b.fallo(c, err)
			}
			errs = append(errs, fmt.Errorf("%s: %w", c.Direccion, err))
		}
		if b.ultimoCambio(ctx) != antes {
			b.avisar()
		}
	}
	return errors.Join(errs...)
}

func (b *Buzon) ultimoCambio(ctx context.Context) int64 {
	var n sql.NullInt64
	_ = b.x.db.QueryRowContext(ctx, "SELECT MAX(seq) FROM cambios").Scan(&n)
	return n.Int64
}

func (b *Buzon) sincronizarCuenta(ctx context.Context, c Cuenta) error {
	ctx, parar := context.WithTimeout(ctx, 10*time.Minute)
	defer parar()
	cli, err := b.abrirIMAP(ctx, c)
	if err != nil {
		return err
	}
	defer func() {
		_ = cli.Logout().Wait()
		_ = cli.Close()
	}()
	if err := b.subirPendientes(ctx, cli, c); err != nil {
		return err
	}
	carpetas, err := b.listarCarpetas(ctx, cli, c)
	if err != nil {
		return err
	}
	var nuevosEntrada []int64
	for _, f := range carpetas {
		nuevos, err := b.bajarCarpeta(ctx, cli, c, f)
		if err != nil {
			return fmt.Errorf("%s: %w", f.nombre, err)
		}
		if f.rol == "inbox" {
			nuevosEntrada = append(nuevosEntrada, nuevos...)
		}
	}
	b.leerCuota(ctx, cli, c)
	// Las respuestas de los invitados a lo que organiza la persona, a la cola de Calendar
	// (respuestas.go). Lo que llega a una conversación silenciada se va al Archivo, lo
	// que llega a una pospuesta la hace volver, y luego las reglas locales (M-09); lo que
	// muevan o marquen sube ya.
	if len(nuevosEntrada) > 0 {
		b.procesarRespuestas(ctx, nuevosEntrada)
		cambio := b.silenciarLlegadas(ctx, nuevosEntrada)
		cambio = b.aplicarReglas(ctx, nuevosEntrada) || cambio
		b.anunciarLlegadas(ctx, c, nuevosEntrada)
		if cambio {
			return b.subirPendientes(ctx, cli, c)
		}
	}
	return nil
}

// leerCuota apunta el espacio de la cuenta (QUOTA, RFC 9208) si el servidor lo cuenta.
// Es informativo: un fallo no para la vuelta.
func (b *Buzon) leerCuota(ctx context.Context, cli *imapclient.Client, c Cuenta) {
	if !cli.Caps().Has(imap.CapQuota) {
		return
	}
	qs, err := cli.GetQuotaRoot("INBOX").Wait()
	if err != nil {
		return
	}
	var q cuota
	for _, d := range qs {
		if r, ok := d.Resources[imap.QuotaResourceStorage]; ok && r.Limit > 0 {
			// STORAGE va en unidades de 1024 octetos.
			q.Usado, q.Limite = q.Usado+r.Usage*1024, q.Limite+r.Limit*1024
		}
	}
	_ = b.ponerCuota(ctx, c.ID, q)
}

// ---- Carpetas ----

var rolesEspeciales = []struct {
	attr imap.MailboxAttr
	rol  string
}{
	{imap.MailboxAttrSent, "sent"},
	{imap.MailboxAttrDrafts, "drafts"},
	{imap.MailboxAttrTrash, "trash"},
	{imap.MailboxAttrJunk, "junk"},
	{imap.MailboxAttrArchive, "archive"},
}

// nombresDeRol son los nombres que dan los servidores a sus carpetas especiales cuando
// no las marcan (iCloud, Outlook, Dovecot).
var nombresDeRol = map[string]string{
	"sent": "sent", "sent messages": "sent", "sent items": "sent", "sent mail": "sent", "enviados": "sent",
	"drafts": "drafts", "borradores": "drafts",
	"trash": "trash", "deleted messages": "trash", "deleted items": "trash", "papelera": "trash",
	"junk": "junk", "junk e-mail": "junk", "junk email": "junk", "spam": "junk", "correo no deseado": "junk",
	"archive": "archive", "archivo": "archive",
}

func rolPorNombre(nombre string) string {
	n := strings.ToLower(nombre)
	for _, p := range []string{"inbox.", "inbox/"} {
		n = strings.TrimPrefix(n, p)
	}
	return nombresDeRol[n]
}

func (b *Buzon) listarCarpetas(ctx context.Context, cli *imapclient.Client, c Cuenta) ([]carpeta, error) {
	items, err := cli.List("", "*", &imap.ListOptions{ReturnSpecialUse: cli.Caps().Has(imap.CapSpecialUse)}).Collect()
	if err != nil {
		return nil, err
	}
	vistas := carpetasVistas(c, items)
	antes, err := b.x.carpetas(ctx, c.ID)
	if err != nil {
		return nil, err
	}
	if err := b.guardarCarpetas(ctx, c, antes, vistas); err != nil {
		return nil, err
	}
	todas, err := b.x.carpetas(ctx, c.ID)
	if err != nil {
		return nil, err
	}
	// La entrada primero: es lo que la persona espera ver antes.
	slices.SortStableFunc(todas, func(a, x carpeta) int {
		switch {
		case a.rol == "inbox" && x.rol != "inbox":
			return -1
		case x.rol == "inbox" && a.rol != "inbox":
			return 1
		}
		return 0
	})
	return slices.DeleteFunc(todas, func(f carpeta) bool { return f.local }), nil
}

// carpetasVistas son las carpetas que da el servidor y que se bajan, con su rol: el de
// SPECIAL-USE o, si no lo dice, el que se deduce del nombre (un rol, una carpeta).
func carpetasVistas(c Cuenta, items []*imap.ListData) []carpeta {
	var vistas []carpeta
	tomados := map[string]bool{}
	for _, it := range items {
		if slices.Contains(it.Attrs, imap.MailboxAttrNoSelect) || slices.Contains(it.Attrs, imap.MailboxAttrNonExistent) ||
			// «Todos» y «Destacados» de Gmail repiten lo que está en las demás.
			slices.Contains(it.Attrs, imap.MailboxAttrAll) || slices.Contains(it.Attrs, imap.MailboxAttrFlagged) ||
			slices.Contains(it.Attrs, imap.MailboxAttrImportant) {
			continue
		}
		f := carpeta{cuenta: c.ID, nombre: it.Mailbox}
		if strings.EqualFold(it.Mailbox, "INBOX") {
			f.rol = "inbox"
		}
		for _, r := range rolesEspeciales {
			if f.rol == "" && slices.Contains(it.Attrs, r.attr) && !tomados[r.rol] {
				f.rol = r.rol
			}
		}
		tomados[f.rol] = true
		vistas = append(vistas, f)
	}
	for i, f := range vistas {
		if r := rolPorNombre(f.nombre); f.rol == "" && r != "" && !tomados[r] {
			vistas[i].rol, tomados[r] = r, true
		}
	}
	return vistas
}

// guardarCarpetas deja en el índice las carpetas vistas en el servidor y quita las de
// antes que ya no están (lo local con rol pasa a la del servidor cuando la hay).
func (b *Buzon) guardarCarpetas(ctx context.Context, c Cuenta, antes, vistas []carpeta) error {
	tx, err := b.x.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, f := range vistas {
		if _, err := tx.ExecContext(ctx, `INSERT INTO carpetas (cuenta, nombre, rol) VALUES (?, ?, ?)
			ON CONFLICT(cuenta, nombre) DO UPDATE SET rol = excluded.rol, local = 0`, f.cuenta, f.nombre, f.rol); err != nil {
			return err
		}
	}
	for _, a := range antes {
		sigue := slices.ContainsFunc(vistas, func(f carpeta) bool { return f.nombre == a.nombre })
		switch {
		case sigue:
		case a.local && !slices.ContainsFunc(vistas, func(f carpeta) bool { return f.rol == a.rol && a.rol != "" }):
			// Una carpeta local se queda mientras el servidor no tenga una con su rol.
		case a.local:
			// El servidor ya tiene la carpeta de ese rol: lo local pasa a ella (sin UID:
			// se sube o se empareja por Message-ID).
			i := slices.IndexFunc(vistas, func(f carpeta) bool { return f.rol == a.rol })
			if _, err := tx.ExecContext(ctx, "UPDATE mensajes SET carpeta = ? WHERE cuenta = ? AND carpeta = ?", vistas[i].nombre, c.ID, a.nombre); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, "DELETE FROM carpetas WHERE cuenta = ? AND nombre = ?", c.ID, a.nombre); err != nil {
				return err
			}
		default:
			if err := b.borrarFilas(ctx, tx, "cuenta = ? AND carpeta = ?", c.ID, a.nombre); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, "DELETE FROM carpetas WHERE cuenta = ? AND nombre = ?", c.ID, a.nombre); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// borrarFilas quita mensajes del índice y sus .eml.
func (b *Buzon) borrarFilas(ctx context.Context, tx *sql.Tx, donde string, args ...any) error {
	//nolint:gosec // G202: donde es siempre un literal de este paquete; los valores van en args.
	filas, err := tx.QueryContext(ctx, "SELECT id, cuenta FROM mensajes WHERE "+donde, args...)
	if err != nil {
		return err
	}
	defer filas.Close()
	type fila struct {
		id     int64
		cuenta string
	}
	var fs []fila
	for filas.Next() {
		var f fila
		if err := filas.Scan(&f.id, &f.cuenta); err != nil {
			return err
		}
		fs = append(fs, f)
	}
	// Next ya cerró las filas al acabarlas: borrar puede usar la transacción.
	if err := filas.Err(); err != nil {
		return err
	}
	for _, f := range fs {
		if err := b.borrarFila(ctx, tx, f.id, f.cuenta); err != nil {
			return err
		}
	}
	return nil
}

func (b *Buzon) borrarFila(ctx context.Context, tx execer, id int64, cuenta string) error {
	for _, q := range []string{"DELETE FROM mensajes WHERE id = ?", "DELETE FROM mensaje_etiquetas WHERE mensaje = ?"} {
		if _, err := tx.ExecContext(ctx, q, id); err != nil {
			return err
		}
	}
	_ = os.Remove(b.rutaEML(cuenta, id))
	return apuntar(ctx, tx, id)
}

// ---- Bajada de una carpeta ----

var (
	seccionCabecera = &imap.FetchItemBodySection{Specifier: imap.PartSpecifierHeader, Peek: true}
	seccionEntera   = &imap.FetchItemBodySection{Peek: true}
)

// marcasIMAP deja solo las marcas que la pantalla entiende.
func marcasIMAP(fs []imap.Flag) []string {
	var out []string
	for _, f := range fs {
		switch f {
		case imap.FlagSeen, imap.FlagFlagged, imap.FlagAnswered, imap.FlagDraft:
			out = append(out, string(f))
		default:
			// Las demás (\Deleted, $Junk, las del servidor…) no las enseña la pantalla.
		}
	}
	return out
}

func mismasMarcas(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for _, x := range a {
		if !tieneMarca(b, x) {
			return false
		}
	}
	return true
}

func (b *Buzon) bajarCarpeta(ctx context.Context, cli *imapclient.Client, c Cuenta, f carpeta) (nuevos []int64, err error) {
	// CONDSTORE (o QRESYNC, que lo incluye): el servidor dice el HIGHESTMODSEQ de la
	// carpeta, y las marcas solo se piden si ha cambiado.
	condstore := cli.Caps().Has(imap.CapCondStore)
	sel, err := cli.Select(f.nombre, &imap.SelectOptions{ReadOnly: true, CondStore: condstore}).Wait()
	if err != nil {
		if errorIMAP(err) {
			return nil, nil // la carpeta no se deja abrir: se salta
		}
		return nil, err
	}
	if sel.UIDValidity != f.uidvalidity {
		// Los UID de antes ya no valen: lo de esta carpeta se baja de nuevo.
		if err := b.reiniciarCarpeta(ctx, c, f, sel.UIDValidity); err != nil {
			return nil, err
		}
		f.uidvalidity, f.ultimo, f.modseq, f.existen, f.estrenada = sel.UIDValidity, 0, 0, 0, false
	}
	// La PRIMERA vuelta de una carpeta trae lo que ya estaba (los Ventana más recientes),
	// que no es correo que llega: ni avisa ni se mira como respuesta de un invitado. Una
	// carpeta estrenada VACÍA no tiene nada de antes, así que lo que haya ahora llegó
	// después y es nuevo; salvo que de golpe pase de la ventana (se ha traído un archivo
	// entero de otro sitio): eso es historia y se baja como la primera vez, sin avisar.
	primera := f.ultimo == 0 && (!f.estrenada || sel.NumMessages > Ventana)

	// 1. Lo nuevo.
	conjunto := conjuntoNuevo(sel, f, primera)
	if conjunto != nil {
		if nuevos, err = b.bajarNuevos(ctx, cli, c, f, conjunto, primera); err != nil {
			return nil, err
		}
	} else if !f.estrenada {
		// Vacía en su primera vuelta: queda estrenada, y lo que llegue será nuevo.
		if _, err := b.x.db.ExecContext(ctx, "UPDATE carpetas SET estrenada = 1 WHERE cuenta = ? AND nombre = ?", c.ID, f.nombre); err != nil {
			return nil, err
		}
	}

	// 2. Marcas y borrados de lo que ya estaba.
	if err := b.revisarCarpeta(ctx, cli, c, f, sel, condstore, conjunto != nil); err != nil {
		return nuevos, err
	}

	// 3. Cuerpos para leer sin conexión.
	return nuevos, b.bajarCuerpos(ctx, cli, c, f)
}

// revisarCarpeta mira las marcas y los borrados de lo que ya estaba y apunta el
// HIGHESTMODSEQ y el EXISTS de esta vuelta. Con CONDSTORE, nada que mirar si son los de
// la última vuelta y no ha llegado nada.
func (b *Buzon) revisarCarpeta(ctx context.Context, cli *imapclient.Client, c Cuenta, f carpeta, sel *imap.SelectData, condstore, llego bool) error {
	switch {
	case condstore && f.modseq != 0 && sel.HighestModSeq == f.modseq && sel.NumMessages == f.existen && !llego:
	case condstore && f.modseq != 0 && sel.HighestModSeq != 0:
		if err := b.reconciliarDesde(ctx, cli, c, f, sel.NumMessages != f.existen || llego); err != nil {
			return err
		}
	default:
		if err := b.reconciliar(ctx, cli, c, f); err != nil {
			return err
		}
	}
	_, err := b.x.db.ExecContext(ctx, "UPDATE carpetas SET modseq = ?, existen = ? WHERE cuenta = ? AND nombre = ?",
		sel.HighestModSeq, sel.NumMessages, c.ID, f.nombre)
	return err
}

// reiniciarCarpeta olvida lo bajado con UID de una carpeta cuyo UIDVALIDITY cambió.
func (b *Buzon) reiniciarCarpeta(ctx context.Context, c Cuenta, f carpeta, uidvalidity uint32) error {
	tx, err := b.x.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := b.borrarFilas(ctx, tx, "cuenta = ? AND carpeta = ? AND uid > 0", c.ID, f.nombre); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE carpetas SET uidvalidity = ?, ultimo = 0, modseq = 0, existen = 0, estrenada = 0 WHERE cuenta = ? AND nombre = ?", uidvalidity, c.ID, f.nombre); err != nil {
		return err
	}
	return tx.Commit()
}

// conjuntoNuevo es lo que se pide de lo nuevo: en la primera vuelta, los Ventana más
// recientes por número; después, lo que pase del último UID visto. nil si no hay nada.
func conjuntoNuevo(sel *imap.SelectData, f carpeta, primera bool) imap.NumSet {
	switch {
	case sel.NumMessages == 0:
	case primera:
		desde := uint32(1)
		if sel.NumMessages > Ventana {
			desde = sel.NumMessages - Ventana + 1
		}
		return imap.SeqSet{{Start: desde, Stop: sel.NumMessages}}
	case sel.UIDNext == 0 || uint32(sel.UIDNext) > f.ultimo+1:
		return imap.UIDSet{{Start: imap.UID(f.ultimo + 1), Stop: 0}}
	}
	return nil
}

// bajarNuevos baja las cabeceras de conjunto, las guarda y apunta el último UID visto.
// Devuelve los ids de lo que llegó (en la primera vuelta, nada: es lo que ya estaba).
func (b *Buzon) bajarNuevos(ctx context.Context, cli *imapclient.Client, c Cuenta, f carpeta, conjunto imap.NumSet, primera bool) ([]int64, error) {
	bufs, err := cli.Fetch(conjunto, &imap.FetchOptions{
		UID: true, Flags: true, InternalDate: true, RFC822Size: true,
		BodySection: []*imap.FetchItemBodySection{seccionCabecera},
	}).Collect()
	if err != nil {
		return nil, err
	}
	ultimo := f.ultimo
	tx, err := b.x.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var nuevos []int64
	for _, m := range bufs {
		uid := uint32(m.UID)
		if uid <= f.ultimo { // «n:*» devuelve el último aunque no haya nada nuevo
			continue
		}
		ultimo = max(ultimo, uid)
		id, nuevo, err := b.guardarSobre(ctx, tx, c, f, uid, m)
		if err != nil {
			return nil, err
		}
		if nuevo && !primera {
			nuevos = append(nuevos, id)
		}
	}
	if _, err := tx.ExecContext(ctx, "UPDATE carpetas SET ultimo = ?, estrenada = 1 WHERE cuenta = ? AND nombre = ?", ultimo, c.ID, f.nombre); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return nuevos, nil
}

// guardarSobre guarda un mensaje recién visto en el servidor. Si ya estaba en local
// sin UID (un enviado, un movido) con el mismo Message-ID, se le pone el UID en vez de
// duplicarlo.
func (b *Buzon) guardarSobre(ctx context.Context, tx *sql.Tx, c Cuenta, f carpeta, uid uint32, m *imapclient.FetchMessageBuffer) (int64, bool, error) {
	var ya int64
	err := tx.QueryRowContext(ctx, "SELECT id FROM mensajes WHERE cuenta = ? AND carpeta = ? AND uid = ?", c.ID, f.nombre, uid).Scan(&ya)
	if err == nil {
		return ya, false, nil
	}
	s := sobre{cuenta: c.ID, carpeta: f.nombre, uid: uid, recibido: m.InternalDate.UTC(), tamano: m.RFC822Size, marcas: marcasIMAP(m.Flags)}
	refs := sobreDeCabecera(&s, m.FindBodySection(seccionCabecera))
	if s.fecha.IsZero() {
		s.fecha = s.recibido
	}
	if s.messageID != "" {
		err := tx.QueryRowContext(ctx, "SELECT id FROM mensajes WHERE cuenta = ? AND carpeta = ? AND uid = 0 AND message_id = ? LIMIT 1", c.ID, f.nombre, s.messageID).Scan(&ya)
		if err == nil {
			if _, err := tx.ExecContext(ctx, "UPDATE mensajes SET uid = ?, recibido = ? WHERE id = ?", uid, unix(s.recibido), ya); err != nil {
				return 0, false, err
			}
			return ya, false, apuntar(ctx, tx, ya)
		}
	}
	b.hiloDe(ctx, tx, &s, refs)
	s.buscable = textoBuscable(&s, "")
	if err := insertar(ctx, tx, &s); err != nil {
		return 0, false, err
	}
	return s.id, true, nil
}

func (b *Buzon) reconciliar(ctx context.Context, cli *imapclient.Client, c Cuenta, f carpeta) error {
	porUID, uids, err := b.conUID(ctx, c, f)
	if err != nil {
		return err
	}
	if len(porUID) == 0 {
		return nil
	}
	bufs, err := cli.Fetch(uids, &imap.FetchOptions{UID: true, Flags: true}).Collect()
	if err != nil {
		return err
	}
	vistos := map[imap.UID]bool{}
	tx, err := b.x.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, m := range bufs {
		l, ok := porUID[m.UID]
		if !ok {
			continue
		}
		vistos[m.UID] = true
		if err := ponerMarcas(ctx, tx, l, m.Flags); err != nil {
			return err
		}
	}
	for uid, l := range porUID {
		if !vistos[uid] {
			// Ya no está en el servidor: otro cliente lo borró o lo movió.
			if err := b.borrarFila(ctx, tx, l.id, c.ID); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// enLocal es un mensaje del índice que ya tiene UID en el servidor.
type enLocal struct {
	id     int64
	marcas []string
	// pendiente: tiene un cambio hecho aquí sin subir, que el servidor no pisa.
	pendiente bool
}

// conUID son los mensajes de una carpeta que ya tienen UID, por UID, y el conjunto de
// esos UID.
func (b *Buzon) conUID(ctx context.Context, c Cuenta, f carpeta) (map[imap.UID]enLocal, imap.UIDSet, error) {
	filas, err := b.x.db.QueryContext(ctx, `SELECT m.id, m.uid, m.marcas, EXISTS (SELECT 1 FROM pendientes p WHERE p.mensaje = m.id)
		FROM mensajes m WHERE m.cuenta = ? AND m.carpeta = ? AND m.uid > 0`, c.ID, f.nombre)
	if err != nil {
		return nil, nil, err
	}
	defer filas.Close()
	porUID := map[imap.UID]enLocal{}
	var uids imap.UIDSet
	for filas.Next() {
		var l enLocal
		var uid int64
		var marcas string
		if err := filas.Scan(&l.id, &uid, &marcas, &l.pendiente); err != nil {
			return nil, nil, err
		}
		l.marcas = deLista(marcas)
		porUID[imap.UID(u32(uid))] = l
		uids.AddNum(imap.UID(u32(uid)))
	}
	if err := filas.Err(); err != nil {
		return nil, nil, err
	}
	return porUID, uids, nil
}

// ponerMarcas copia a local las marcas que tiene un mensaje en el servidor, salvo que
// tenga un cambio de aquí por subir.
func ponerMarcas(ctx context.Context, tx *sql.Tx, l enLocal, fs []imap.Flag) error {
	ms := marcasIMAP(fs)
	if l.pendiente || mismasMarcas(ms, l.marcas) {
		return nil
	}
	if _, err := tx.ExecContext(ctx, "UPDATE mensajes SET marcas = ? WHERE id = ?", jsonLista(ms), l.id); err != nil {
		return err
	}
	return apuntar(ctx, tx, l.id)
}

// reconciliarDesde es reconciliar con CONDSTORE: solo las marcas que cambiaron desde
// el último HIGHESTMODSEQ (FETCH … CHANGEDSINCE) y, si el número de mensajes se movió,
// cuáles siguen (UID SEARCH sobre los de aquí, solo números). QRESYNC daría los borrados
// con VANISHED en el propio SELECT, pero el cliente IMAP no lo lee todavía: con esto
// se ahorra lo mismo en la vuelta que no trae nada, que es casi todas.
func (b *Buzon) reconciliarDesde(ctx context.Context, cli *imapclient.Client, c Cuenta, f carpeta, buscarBorrados bool) error {
	porUID, uids, err := b.conUID(ctx, c, f)
	if err != nil {
		return err
	}
	if len(porUID) == 0 {
		return nil
	}
	bufs, err := cli.Fetch(uids, &imap.FetchOptions{UID: true, Flags: true, ChangedSince: f.modseq}).Collect()
	if err != nil {
		return err
	}
	var siguen map[imap.UID]bool
	if buscarBorrados {
		d, err := cli.UIDSearch(&imap.SearchCriteria{UID: []imap.UIDSet{uids}}, nil).Wait()
		if err != nil {
			return err
		}
		siguen = map[imap.UID]bool{}
		for _, u := range d.AllUIDs() {
			siguen[u] = true
		}
	}
	tx, err := b.x.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, m := range bufs {
		if l, ok := porUID[m.UID]; ok {
			if err := ponerMarcas(ctx, tx, l, m.Flags); err != nil {
				return err
			}
		}
	}
	if siguen != nil {
		for uid, l := range porUID {
			if !siguen[uid] {
				if err := b.borrarFila(ctx, tx, l.id, c.ID); err != nil {
					return err
				}
			}
		}
	}
	return tx.Commit()
}

func (b *Buzon) bajarCuerpos(ctx context.Context, cli *imapclient.Client, c Cuenta, f carpeta) error {
	ss, err := b.x.sobres(ctx, "WHERE m.cuenta = ? AND m.carpeta = ? AND m.uid > 0 AND m.eml = 0 AND m.tamano <= ? ORDER BY m.fecha DESC LIMIT ?",
		c.ID, f.nombre, TopeCuerpo, CuerposPorVuelta)
	if err != nil || len(ss) == 0 {
		return err
	}
	var uids imap.UIDSet
	porUID := map[imap.UID]sobre{}
	for _, s := range ss {
		uids.AddNum(imap.UID(s.uid))
		porUID[imap.UID(s.uid)] = s
	}
	bufs, err := cli.Fetch(uids, &imap.FetchOptions{UID: true, BodySection: []*imap.FetchItemBodySection{seccionEntera}}).Collect()
	if err != nil {
		return err
	}
	for _, m := range bufs {
		s, ok := porUID[m.UID]
		crudo := m.FindBodySection(seccionEntera)
		if !ok || crudo == nil {
			continue
		}
		if err := b.guardarCuerpo(ctx, s, crudo); err != nil {
			return err
		}
	}
	return nil
}

// guardarCuerpo escribe el .eml de un sobre y completa su fila.
func (b *Buzon) guardarCuerpo(ctx context.Context, s sobre, crudo []byte) error {
	if err := b.escribirEML(s.cuenta, s.id, crudo); err != nil {
		return err
	}
	conCuerpo(ctx, &s, crudo)
	if _, err := b.x.db.ExecContext(ctx, "UPDATE mensajes SET eml = 1, vista = ?, adjuntos = ?, buscable = ?, tamano = ? WHERE id = ?",
		s.vista, s.adjuntos, s.buscable, s.tamano, s.id); err != nil {
		return err
	}
	return apuntar(ctx, b.x.db, s.id)
}

// traerCuerpo baja en el acto el .eml de un mensaje que aún no está en el equipo.
func (b *Buzon) traerCuerpo(ctx context.Context, s sobre) ([]byte, error) {
	c, ok := b.reg.cuenta(s.cuenta)
	if !ok {
		return nil, ErrSinCuenta
	}
	if s.uid == 0 {
		return nil, ErrNoEncontrado
	}
	ctx, parar := context.WithTimeout(ctx, PlazoConexion)
	defer parar()
	cli, err := b.abrirIMAP(ctx, c)
	if err != nil {
		return nil, err
	}
	defer func() { _ = cli.Close() }()
	if _, err := cli.Select(s.carpeta, &imap.SelectOptions{ReadOnly: true}).Wait(); err != nil {
		return nil, err
	}
	bufs, err := cli.Fetch(imap.UIDSetNum(imap.UID(s.uid)), &imap.FetchOptions{UID: true, BodySection: []*imap.FetchItemBodySection{seccionEntera}}).Collect()
	if err != nil {
		return nil, err
	}
	for _, m := range bufs {
		if crudo := m.FindBodySection(seccionEntera); crudo != nil {
			if err := b.guardarCuerpo(ctx, s, crudo); err != nil {
				return nil, err
			}
			return crudo, nil
		}
	}
	return nil, ErrNoEncontrado
}

// ---- Subida: la cola de pendientes ----

// Tipos de cambio pendiente.
const (
	pendMarcar = "marcar"
	pendMover  = "mover"
	pendBorrar = "borrar"
	pendAnadir = "anadir" // APPEND de la copia de un enviado
)

type pendiente struct {
	id, mensaje    int64
	tipo, carpeta  string
	uid            uint32
	destino, marca string
	poner          bool
}

func (b *Buzon) subirPendientes(ctx context.Context, cli *imapclient.Client, c Cuenta) error {
	ps, err := b.pendientes(ctx, c.ID)
	if err != nil {
		return err
	}
	elegida := ""
	elegir := func(n string) error {
		if n == elegida {
			return nil
		}
		if _, err := cli.Select(n, nil).Wait(); err != nil {
			return err
		}
		elegida = n
		return nil
	}
	for _, p := range ps {
		hecho, err := b.subir(ctx, cli, c, p, elegir)
		if err != nil {
			if !errorIMAP(err) {
				return err // la red: lo que queda, en la próxima vuelta
			}
			slog.Warn("correo: el servidor rechazó un cambio", "tipo", p.tipo, "error", err)
			if p.tipo == pendMover {
				// El mensaje no se movió: vuelve a su sitio en local.
				_, _ = b.x.db.ExecContext(ctx, "UPDATE mensajes SET carpeta = ?, uid = ? WHERE id = ? AND uid = 0", p.carpeta, p.uid, p.mensaje)
				_ = apuntar(ctx, b.x.db, p.mensaje)
				b.avisar()
			}
			hecho = true
		}
		if hecho {
			if _, err := b.x.db.ExecContext(ctx, "DELETE FROM pendientes WHERE id = ?", p.id); err != nil {
				return err
			}
		}
	}
	return nil
}

// pendientes es la cola de cambios por subir de una cuenta, en orden.
func (b *Buzon) pendientes(ctx context.Context, cuenta string) ([]pendiente, error) {
	filas, err := b.x.db.QueryContext(ctx, "SELECT id, tipo, mensaje, carpeta, uid, destino, marca, poner FROM pendientes WHERE cuenta = ? ORDER BY id", cuenta)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	var ps []pendiente
	for filas.Next() {
		var p pendiente
		var uid int64
		if err := filas.Scan(&p.id, &p.tipo, &p.mensaje, &p.carpeta, &uid, &p.destino, &p.marca, &p.poner); err != nil {
			return nil, err
		}
		p.uid = u32(uid)
		ps = append(ps, p)
	}
	if err := filas.Err(); err != nil {
		return nil, err
	}
	return ps, nil
}

// subir aplica un cambio en el servidor. hecho=false lo deja en la cola (un mensaje
// cuyo UID aún no se conoce).
func (b *Buzon) subir(ctx context.Context, cli *imapclient.Client, c Cuenta, p pendiente, elegir func(string) error) (hecho bool, err error) {
	switch p.tipo {
	case pendMarcar:
		return b.subirMarca(ctx, cli, p, elegir)
	case pendMover:
		return b.subirMovido(ctx, cli, p, elegir)
	case pendBorrar:
		return subirBorrado(cli, p, elegir)
	case pendAnadir:
		return b.subirCopia(ctx, cli, c, p)
	}
	return true, nil
}

func (b *Buzon) subirMarca(ctx context.Context, cli *imapclient.Client, p pendiente, elegir func(string) error) (bool, error) {
	s, err := b.x.sobre(ctx, p.mensaje)
	if errors.Is(err, ErrNoEncontrado) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if s.uid == 0 {
		return false, nil
	}
	if err := elegir(s.carpeta); err != nil {
		return false, err
	}
	op := imap.StoreFlagsDel
	if p.poner {
		op = imap.StoreFlagsAdd
	}
	return true, cli.Store(imap.UIDSetNum(imap.UID(s.uid)), &imap.StoreFlags{Op: op, Silent: true, Flags: []imap.Flag{imap.Flag(p.marca)}}, nil).Close()
}

func (b *Buzon) subirMovido(ctx context.Context, cli *imapclient.Client, p pendiente, elegir func(string) error) (bool, error) {
	if err := elegir(p.carpeta); err != nil {
		return false, err
	}
	d, err := cli.Move(imap.UIDSetNum(imap.UID(p.uid)), p.destino).Wait()
	if err != nil {
		return false, err
	}
	if d != nil {
		if dest, ok := d.DestUIDs.(imap.UIDSet); ok {
			if us, ok := dest.Nums(); ok && len(us) == 1 {
				_, _ = b.x.db.ExecContext(ctx, "UPDATE mensajes SET uid = ? WHERE id = ? AND uid = 0 AND carpeta = ?", uint32(us[0]), p.mensaje, p.destino)
			}
		}
	}
	return true, nil
}

func subirBorrado(cli *imapclient.Client, p pendiente, elegir func(string) error) (bool, error) {
	if err := elegir(p.carpeta); err != nil {
		return false, err
	}
	set := imap.UIDSetNum(imap.UID(p.uid))
	if err := cli.Store(set, &imap.StoreFlags{Op: imap.StoreFlagsAdd, Silent: true, Flags: []imap.Flag{imap.FlagDeleted}}, nil).Close(); err != nil {
		return false, err
	}
	if cli.Caps().Has(imap.CapUIDPlus) {
		return true, cli.UIDExpunge(set).Close()
	}
	return true, cli.Expunge().Close()
}

// subirCopia sube con APPEND la copia de un enviado.
func (b *Buzon) subirCopia(ctx context.Context, cli *imapclient.Client, c Cuenta, p pendiente) (bool, error) {
	s, err := b.x.sobre(ctx, p.mensaje)
	if errors.Is(err, ErrNoEncontrado) || (err == nil && s.uid != 0) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	crudo, err := os.ReadFile(b.rutaEML(c.ID, s.id))
	if err != nil {
		// Sin el .eml no hay qué subir: el cambio se da por hecho.
		slog.Warn("correo: la copia de un enviado no se sube, falta su .eml", "error", err)
		return true, nil
	}
	marcas := make([]imap.Flag, 0, len(s.marcas))
	for _, m := range s.marcas {
		marcas = append(marcas, imap.Flag(m))
	}
	cmd := cli.Append(p.destino, int64(len(crudo)), &imap.AppendOptions{Flags: marcas, Time: s.fecha})
	if _, err := cmd.Write(crudo); err != nil {
		return false, err
	}
	if err := cmd.Close(); err != nil {
		return false, err
	}
	d, err := cmd.Wait()
	if err != nil {
		return false, err
	}
	if d != nil && d.UID != 0 {
		_, _ = b.x.db.ExecContext(ctx, "UPDATE mensajes SET uid = ? WHERE id = ? AND uid = 0", uint32(d.UID), s.id)
	}
	return true, nil
}

// encolar apunta un cambio para subir en la próxima vuelta.
func encolar(ctx context.Context, tx execer, cuenta string, p pendiente) error {
	_, err := tx.ExecContext(ctx, "INSERT INTO pendientes (cuenta, tipo, mensaje, carpeta, uid, destino, marca, poner) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		cuenta, p.tipo, p.mensaje, p.carpeta, p.uid, p.destino, p.marca, p.poner)
	return err
}
