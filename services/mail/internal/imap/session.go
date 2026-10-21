// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package imap

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
	"git.sytue.eu/sytue/one/services/mail/internal/store"
)

// backend crea sesiones IMAP.
type backend struct {
	store             *store.Store
	blobs             *objectstore.Client
	auth              Authenticator
	hostname          string
	logger            *slog.Logger
	maxAuthFailPerMin int
}

func (b *backend) newSession(conn *imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
	s := &session{be: b, remoteIP: remoteIP(conn)}
	greeting := &imapserver.GreetingData{PreAuth: false}
	return s, greeting, nil
}

// session implementa imapserver.Session para una conexión. Tras Login, todas las
// operaciones se ejecutan bajo el tenant del usuario (RLS).
type session struct {
	be       *backend
	remoteIP string

	subject string
	tenant  string
	address string
	account uuid.UUID

	selected *domain.Mailbox
}

// remoteIP saca la IP del cliente de la conexión IMAP. Devuelve "" si no se
// puede determinar, y en ese caso el límite de intentos NO se aplica: es
// preferible no frenar a nadie que frenar a todos bajo una clave vacía.
func remoteIP(conn *imapserver.Conn) string {
	if conn == nil {
		return ""
	}
	nc := conn.NetConn()
	if nc == nil || nc.RemoteAddr() == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(nc.RemoteAddr().String())
	if err != nil {
		return ""
	}
	if net.ParseIP(host) == nil {
		return ""
	}
	return host
}

// ctx construye un contexto con el tenant del usuario fijado (para RLS).
func (s *session) ctx() context.Context {
	ctx := context.Background()
	if t, err := tenancy.New(s.tenant); err == nil {
		ctx = tenancy.Into(ctx, t)
	}
	return ctx
}

// --- Autenticación (LOGIN / AUTHENTICATE PLAIN) ---

func (s *session) Login(username, password string) error {
	lim := s.be.maxAuthFailPerMin
	// Si esta IP ya agotó sus fallos en el minuto en curso, ni se consulta al IdP.
	if lim > 0 && s.remoteIP != "" {
		if n, rerr := s.be.store.AuthFailRate(context.Background(), s.remoteIP); rerr == nil && n >= lim {
			s.be.logger.Warn("imap: demasiados fallos de autenticación", "ip", s.remoteIP, "count", n, "limite", lim)
			return imapserver.ErrAuthFailed
		}
	}
	subject, tenant, address, err := s.be.auth.Authenticate(context.Background(), username, password)
	if err != nil {
		// Contar Y REGISTRAR: hasta ahora un diccionario contra el 993 no dejaba
		// ni una línea de log.
		if lim > 0 && s.remoteIP != "" {
			n, _ := s.be.store.HitAuthFailRate(context.Background(), s.remoteIP)
			s.be.logger.Warn("imap: autenticación fallida", "ip", s.remoteIP, "count", n, "limite", lim)
		}
		return imapserver.ErrAuthFailed
	}
	acc, err := s.be.store.AccountBySubject(s.tenantCtx(tenant), subject)
	if err != nil {
		return imapserver.ErrAuthFailed
	}
	s.subject, s.tenant, s.address, s.account = subject, tenant, strings.ToLower(address), acc.ID
	return nil
}

func (s *session) tenantCtx(tenant string) context.Context {
	ctx := context.Background()
	if t, err := tenancy.New(tenant); err == nil {
		ctx = tenancy.Into(ctx, t)
	}
	return ctx
}

// --- Buzones (LIST / SELECT / CREATE) ---

func (s *session) List(w *imapserver.ListWriter, ref string, patterns []string, options *imap.ListOptions) error {
	boxes, err := s.be.store.ListMailboxes(s.ctx(), s.account)
	if err != nil {
		return err
	}
	for _, mb := range boxes {
		data := &imap.ListData{
			Mailbox: mb.Name,
			Delim:   '/',
		}
		if mb.Role != domain.RoleNone {
			data.Attrs = append(data.Attrs, attrForRole(mb.Role))
		}
		if err := w.WriteList(data); err != nil {
			return err
		}
	}
	return nil
}

func (s *session) Select(mailbox string, options *imap.SelectOptions) (*imap.SelectData, error) {
	mb, err := s.findMailbox(mailbox)
	if err != nil {
		return nil, err
	}
	s.selected = &mb
	return &imap.SelectData{
		NumMessages: seqNum(int(mb.TotalCount)),
		UIDValidity: mb.UIDValidity,
		UIDNext:     imap.UID(mb.UIDNext),
	}, nil
}

func (s *session) Create(mailbox string, options *imap.CreateOptions) error {
	name := lastSegment(mailbox)
	_, err := s.be.store.CreateMailbox(s.ctx(), s.account, nil, name)
	return err
}

func (s *session) findMailbox(name string) (domain.Mailbox, error) {
	boxes, err := s.be.store.ListMailboxes(s.ctx(), s.account)
	if err != nil {
		return domain.Mailbox{}, err
	}
	for _, mb := range boxes {
		if strings.EqualFold(mb.Name, lastSegment(name)) {
			return mb, nil
		}
	}
	return domain.Mailbox{}, &imap.Error{Type: imap.StatusResponseTypeNo, Code: imap.ResponseCodeNonExistent, Text: "Mailbox does not exist"}
}

// --- Lectura de mensajes (FETCH) ---

func (s *session) Fetch(w *imapserver.FetchWriter, numSet imap.NumSet, options *imap.FetchOptions) error {
	if s.selected == nil {
		return &imap.Error{Type: imap.StatusResponseTypeBad, Text: "No mailbox selected"}
	}
	// Lista ordenada por UID: el número de secuencia es la posición 1-based.
	msgs, err := s.be.store.ListMessagesByUID(s.ctx(), s.selected.ID)
	if err != nil {
		return err
	}
	for _, sel := range resolveSet(msgs, numSet) {
		m := sel.Msg
		mw := w.CreateMessage(sel.Seq)
		if options.UID {
			mw.WriteUID(imap.UID(m.UID))
		}
		if options.Flags {
			mw.WriteFlags(toIMAPFlags(m.Flags))
		}
		if options.RFC822Size {
			mw.WriteRFC822Size(m.SizeBytes)
		}
		if options.Envelope {
			mw.WriteEnvelope(toIMAPEnvelope(m))
		}
		if options.InternalDate {
			mw.WriteInternalDate(m.ReceivedAt)
		}
		if len(options.BodySection) > 0 {
			if err := s.writeBody(mw, m, options.BodySection); err != nil {
				s.be.logger.Warn("imap: fetch body", "error", err)
			}
		}
		if err := mw.Close(); err != nil {
			return err
		}
	}
	return nil
}

func (s *session) writeBody(mw *imapserver.FetchResponseWriter, m domain.Message, sections []*imap.FetchItemBodySection) error {
	t, _ := tenancy.New(s.tenant)
	rc, err := s.be.blobs.Get(s.ctx(), t, m.BlobKey)
	if err != nil {
		return err
	}
	defer rc.Close()
	body, err := io.ReadAll(rc)
	if err != nil {
		return err
	}
	for _, sec := range sections {
		wc := mw.WriteBodySection(sec, int64(len(body)))
		_, _ = wc.Write(body)
		_ = wc.Close()
	}
	return nil
}

// --- Modificación (STORE flags / EXPUNGE / MOVE) ---

func (s *session) Store(w *imapserver.FetchWriter, numSet imap.NumSet, flags *imap.StoreFlags, options *imap.StoreOptions) error {
	if s.selected == nil {
		return &imap.Error{Type: imap.StatusResponseTypeBad, Text: "No mailbox selected"}
	}
	msgs, err := s.be.store.ListMessagesByUID(s.ctx(), s.selected.ID)
	if err != nil {
		return err
	}
	for _, sel := range resolveSet(msgs, numSet) {
		if err := s.applyStore(sel.Msg, flags); err != nil {
			return err
		}
		// Devolver los flags resultantes salvo que el cliente pida SILENT.
		if w != nil && !flags.Silent {
			updated, err := s.be.store.GetMessage(s.ctx(), sel.Msg.ID)
			if err == nil {
				mw := w.CreateMessage(sel.Seq)
				mw.WriteUID(imap.UID(updated.UID))
				mw.WriteFlags(toIMAPFlags(updated.Flags))
				_ = mw.Close()
			}
		}
	}
	return nil
}

// applyStore aplica la operación STORE (+FLAGS / -FLAGS / FLAGS) sobre un mensaje.
func (s *session) applyStore(m domain.Message, flags *imap.StoreFlags) error {
	switch flags.Op {
	case imap.StoreFlagsAdd:
		for _, f := range flags.Flags {
			if err := s.be.store.SetFlags(s.ctx(), s.account, m.ID, string(f), true); err != nil {
				return err
			}
		}
	case imap.StoreFlagsDel:
		for _, f := range flags.Flags {
			if err := s.be.store.SetFlags(s.ctx(), s.account, m.ID, string(f), false); err != nil {
				return err
			}
		}
	case imap.StoreFlagsSet:
		// Reemplazo total: quitar los ausentes, añadir los presentes.
		want := make(map[string]bool, len(flags.Flags))
		for _, f := range flags.Flags {
			want[string(f)] = true
		}
		for _, existing := range m.Flags {
			if !want[existing] {
				if err := s.be.store.SetFlags(s.ctx(), s.account, m.ID, existing, false); err != nil {
					return err
				}
			}
		}
		for _, f := range flags.Flags {
			if err := s.be.store.SetFlags(s.ctx(), s.account, m.ID, string(f), true); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *session) Expunge(w *imapserver.ExpungeWriter, uids *imap.UIDSet) error {
	if s.selected == nil {
		return &imap.Error{Type: imap.StatusResponseTypeBad, Text: "No mailbox selected"}
	}
	msgs, err := s.be.store.ListMessagesByUID(s.ctx(), s.selected.ID)
	if err != nil {
		return err
	}
	// EXPUNGE borra los mensajes con \Deleted. Si el cliente acota por UID
	// (UID EXPUNGE, RFC 9051 §6.4.3) solo esos; si uids es nil, todos.
	// Cada EXPUNGE renumera los mensajes posteriores (RFC 9051 §7.5.1): el
	// número de secuencia reportado descuenta los ya expulsados en esta pasada.
	expunged := 0
	for i, m := range msgs {
		if !m.HasFlag(domain.FlagDeleted) {
			continue
		}
		if uids != nil && !uids.Contains(imap.UID(m.UID)) {
			continue
		}
		if err := s.be.store.DeleteMessage(s.ctx(), s.account, m.ID); err != nil {
			return err
		}
		if w != nil {
			_ = w.WriteExpunge(seqNum(i + 1 - expunged))
		}
		expunged++
	}
	return nil
}

func (s *session) Copy(numSet imap.NumSet, dest string) (*imap.CopyData, error) {
	if s.selected == nil {
		return nil, &imap.Error{Type: imap.StatusResponseTypeBad, Text: "No mailbox selected"}
	}
	destMb, err := s.findMailbox(dest)
	if err != nil {
		return nil, err
	}
	msgs, err := s.be.store.ListMessagesByUID(s.ctx(), s.selected.ID)
	if err != nil {
		return nil, err
	}
	var srcUIDs, dstUIDs imap.UIDSet
	for _, sel := range resolveSet(msgs, numSet) {
		newUID, err := s.be.store.CopyMessage(s.ctx(), sel.Msg.ID, destMb.ID)
		if err != nil {
			return nil, err
		}
		srcUIDs.AddNum(imap.UID(sel.Msg.UID))
		dstUIDs.AddNum(imap.UID(newUID))
	}
	return &imap.CopyData{
		UIDValidity: destMb.UIDValidity,
		SourceUIDs:  srcUIDs,
		DestUIDs:    dstUIDs,
	}, nil
}

func (s *session) Move(w *imapserver.MoveWriter, numSet imap.NumSet, dest string) error {
	if s.selected == nil {
		return &imap.Error{Type: imap.StatusResponseTypeBad, Text: "No mailbox selected"}
	}
	destMb, err := s.findMailbox(dest)
	if err != nil {
		return err
	}
	msgs, err := s.be.store.ListMessagesByUID(s.ctx(), s.selected.ID)
	if err != nil {
		return err
	}
	for _, sel := range resolveSet(msgs, numSet) {
		if err := s.be.store.MoveMessage(s.ctx(), s.account, sel.Msg.ID, destMb.ID); err != nil {
			return err
		}
	}
	return nil
}

// --- Operaciones no soportadas / placeholders mínimos ---

func (s *session) Unselect() error { s.selected = nil; return nil }
func (s *session) Close() error    { s.selected = nil; return nil }
func (s *session) Idle(w *imapserver.UpdateWriter, stop <-chan struct{}) error {
	<-stop
	return nil
}

func (s *session) Delete(mailbox string) error {
	mb, err := s.findMailbox(mailbox)
	if err != nil {
		return err
	}
	if err := s.be.store.DeleteMailbox(s.ctx(), s.account, mb.ID); err != nil {
		return mapIMAPErr(err)
	}
	return nil
}

func (s *session) Rename(mailbox, newName string, options *imap.RenameOptions) error {
	mb, err := s.findMailbox(mailbox)
	if err != nil {
		return err
	}
	if err := s.be.store.RenameMailbox(s.ctx(), s.account, mb.ID, lastSegment(newName)); err != nil {
		return mapIMAPErr(err)
	}
	return nil
}
func (s *session) Subscribe(mailbox string) error   { return nil }
func (s *session) Unsubscribe(mailbox string) error { return nil }
func (s *session) Namespace() (*imap.NamespaceData, error) {
	return &imap.NamespaceData{Personal: []imap.NamespaceDescriptor{{Prefix: "", Delim: '/'}}}, nil
}

func (s *session) Status(mailbox string, options *imap.StatusOptions) (*imap.StatusData, error) {
	mb, err := s.findMailbox(mailbox)
	if err != nil {
		return nil, err
	}
	total := seqNum(int(mb.TotalCount))
	unseen := seqNum(int(mb.UnreadCount))
	uidNext := imap.UID(mb.UIDNext)
	return &imap.StatusData{
		Mailbox:     mb.Name,
		NumMessages: &total,
		NumUnseen:   &unseen,
		UIDNext:     uidNext,
		UIDValidity: mb.UIDValidity,
	}, nil
}

func (s *session) Append(mailbox string, r imap.LiteralReader, options *imap.AppendOptions) (*imap.AppendData, error) {
	return nil, notSupported("APPEND")
}
func (s *session) Poll(w *imapserver.UpdateWriter, allowExpunge bool) error { return nil }

// Search implementa SEARCH / UID SEARCH (RFC 9051 §7.4.4). Traduce el subconjunto
// de criterios más habitual (texto, from/subject/body, flags, tamaño, conjuntos
// de seq/uid) a un filtrado sobre el buzón seleccionado y devuelve los números
// de secuencia (SEARCH) o UID (UID SEARCH) coincidentes.
func (s *session) Search(kind imapserver.NumKind, criteria *imap.SearchCriteria, options *imap.SearchOptions) (*imap.SearchData, error) {
	if s.selected == nil {
		return nil, &imap.Error{Type: imap.StatusResponseTypeBad, Text: "No mailbox selected"}
	}
	msgs, err := s.be.store.ListMessagesByUID(s.ctx(), s.selected.ID)
	if err != nil {
		return nil, err
	}

	seqNums := make([]uint32, 0, len(msgs))
	var uids imap.UIDSet
	var minv, maxv, count uint32
	for i, m := range msgs {
		seq := seqNum(i + 1)
		if !matchCriteria(m, seq, criteria) {
			continue
		}
		if minv == 0 || seq < minv {
			minv = seq
		}
		if seq > maxv {
			maxv = seq
		}
		count++
		seqNums = append(seqNums, seq)
		uids.AddNum(imap.UID(m.UID))
	}

	data := &imap.SearchData{Min: minv, Max: maxv, Count: count}
	if kind == imapserver.NumKindUID {
		data.All = uids
	} else {
		var seqSet imap.SeqSet
		seqSet.AddNum(seqNums...)
		data.All = seqSet
	}
	return data, nil
}

func notSupported(op string) error {
	return &imap.Error{Type: imap.StatusResponseTypeNo, Text: fmt.Sprintf("%s not supported", op)}
}

func toIMAPFlags(flags []string) []imap.Flag {
	out := make([]imap.Flag, 0, len(flags))
	for _, f := range flags {
		out = append(out, imap.Flag(f))
	}
	return out
}

func attrForRole(role domain.SpecialUse) imap.MailboxAttr {
	switch role {
	case domain.RoleSent:
		return imap.MailboxAttrSent
	case domain.RoleDrafts:
		return imap.MailboxAttrDrafts
	case domain.RoleTrash:
		return imap.MailboxAttrTrash
	case domain.RoleJunk:
		return imap.MailboxAttrJunk
	case domain.RoleArchive:
		return imap.MailboxAttrArchive
	default:
		return imap.MailboxAttr("")
	}
}

func lastSegment(name string) string {
	parts := strings.Split(name, "/")
	return parts[len(parts)-1]
}
