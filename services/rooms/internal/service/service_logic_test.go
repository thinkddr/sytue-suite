// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/events"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/config"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

var errBoom = errors.New("boom")

// fakeStore implementa dataStore en memoria para tests sin PostgreSQL.
type fakeStore struct {
	team      domain.Team
	teamErr   error
	newTeam   *store.NewTeamRecord
	teams     []domain.Team
	members   []domain.Member
	addMemErr error
	rmMemErr  error

	channel    domain.Channel
	channelErr error
	newChan    *store.NewChannelRecord
	channels   []domain.Channel
	joinErr    error
	leaveErr   error
	updChanErr error

	categories []domain.Category
	category   domain.Category
	catErr     error
	newCatName string
	gotUpdCat  *store.UpdateCategoryRecord
	setCatErr  error

	msg       domain.Message
	msgErr    error
	newMsg    *store.NewMessageRecord
	msgs      []domain.Message
	replies   []domain.Message
	editMsg   domain.Message
	editErr   error
	delChan   uuid.UUID
	delErr    error
	searchErr error

	reactChan uuid.UUID
	reactErr  error
	cardVotes []domain.CardVote
	cardErr   error
	markErr   error
	unread    []domain.ReadState
	unreadErr error

	pinChan    uuid.UUID
	pinErr     error
	unpinErr   error
	pins       []domain.Message
	saveErr    error
	unsaveErr  error
	saved      []domain.Message
	savedNext  *time.Time
	savedErr   error
	markUnread error

	drafts       []domain.Draft
	draftBody    string // último cuerpo que llegó al store (nunca lo escribe el borrado)
	draftDeleted bool
	draftErr     error
}

func (f *fakeStore) CreateTeam(_ context.Context, rec store.NewTeamRecord) (domain.Team, error) {
	f.newTeam = &rec
	return f.team, f.teamErr
}

func (f *fakeStore) ListTeams(context.Context, string) ([]domain.Team, error) {
	return f.teams, f.teamErr
}

func (f *fakeStore) EnsureDefaultTeamMembership(context.Context, string, string) (domain.Team, bool, error) {
	if len(f.teams) > 0 {
		return f.teams[0], false, f.teamErr
	}
	return domain.Team{}, true, f.teamErr
}

func (f *fakeStore) GetTeam(context.Context, uuid.UUID, string) (domain.Team, error) {
	return f.team, f.teamErr
}

func (f *fakeStore) ListTeamMembers(context.Context, uuid.UUID, string) ([]domain.Member, error) {
	return f.members, f.teamErr
}

func (f *fakeStore) AddTeamMember(context.Context, uuid.UUID, string, string, string, domain.MemberRole) error {
	return f.addMemErr
}

func (f *fakeStore) RemoveTeamMember(context.Context, uuid.UUID, string, string) error {
	return f.rmMemErr
}

func (f *fakeStore) CreateChannel(_ context.Context, rec store.NewChannelRecord) (domain.Channel, error) {
	f.newChan = &rec
	return f.channel, f.channelErr
}

func (f *fakeStore) ListChannels(context.Context, uuid.UUID, string, bool) ([]domain.Channel, error) {
	return f.channels, f.channelErr
}

func (f *fakeStore) GetChannel(context.Context, uuid.UUID, string) (domain.Channel, error) {
	return f.channel, f.channelErr
}

func (f *fakeStore) UpdateChannel(context.Context, uuid.UUID, string, store.UpdateChannelRecord) (domain.Channel, error) {
	return f.channel, f.updChanErr
}

func (f *fakeStore) JoinChannel(context.Context, uuid.UUID, string, string) (domain.Channel, error) {
	return f.channel, f.joinErr
}

func (f *fakeStore) OpenDirect(context.Context, uuid.UUID, string, string) (domain.Channel, error) {
	return f.channel, f.channelErr
}

func (f *fakeStore) OpenGroupDirect(context.Context, uuid.UUID, string, []string, map[string]string) (domain.Channel, error) {
	return f.channel, f.channelErr
}

func (f *fakeStore) SetChannelPref(context.Context, uuid.UUID, string, string, *bool, *bool, *string) (domain.Channel, error) {
	return f.channel, f.joinErr
}

func (f *fakeStore) ListJoinableChannels(context.Context, uuid.UUID, string, string, int) ([]domain.Channel, error) {
	return nil, nil
}
func (f *fakeStore) LeaveChannel(context.Context, uuid.UUID, string) error { return f.leaveErr }

func (f *fakeStore) ListCategories(context.Context, string) ([]domain.Category, error) {
	return f.categories, f.catErr
}

func (f *fakeStore) CreateCategory(_ context.Context, _, name string) (domain.Category, error) {
	f.newCatName = name
	return f.category, f.catErr
}

func (f *fakeStore) UpdateCategory(_ context.Context, _ uuid.UUID, _ string, upd store.UpdateCategoryRecord) (domain.Category, error) {
	f.gotUpdCat = &upd
	return f.category, f.catErr
}

func (f *fakeStore) DeleteCategory(context.Context, uuid.UUID, string) error { return f.catErr }

func (f *fakeStore) SetChannelCategory(context.Context, uuid.UUID, string, string, *uuid.UUID, *int) (domain.Channel, error) {
	return f.channel, f.setCatErr
}

func (f *fakeStore) CreateMessage(_ context.Context, rec store.NewMessageRecord) (domain.Message, error) {
	f.newMsg = &rec
	return f.msg, f.msgErr
}

func (f *fakeStore) ListOwnMessages(context.Context, string, time.Time, uuid.UUID, int) ([]domain.Message, error) {
	return nil, nil
}

func (f *fakeStore) ListMessages(context.Context, uuid.UUID, string, store.MessagePage) ([]domain.Message, error) {
	return f.msgs, f.msgErr
}

func (f *fakeStore) ListThread(context.Context, uuid.UUID, string) (domain.Message, []domain.Message, error) {
	return f.msg, f.replies, f.msgErr
}

func (f *fakeStore) EditMessage(context.Context, uuid.UUID, string, string) (domain.Message, error) {
	return f.editMsg, f.editErr
}

func (f *fakeStore) DeleteMessage(context.Context, uuid.UUID, string) (uuid.UUID, error) {
	return f.delChan, f.delErr
}

func (f *fakeStore) SearchMessages(context.Context, uuid.UUID, string, string, int) ([]domain.Message, error) {
	return f.msgs, f.searchErr
}

func (f *fakeStore) SearchAll(context.Context, string, string, int) ([]domain.Message, error) {
	return f.msgs, f.searchErr
}

func (f *fakeStore) AddReaction(context.Context, uuid.UUID, string, string) (uuid.UUID, error) {
	return f.reactChan, f.reactErr
}

func (f *fakeStore) RemoveReaction(context.Context, uuid.UUID, string, string) (uuid.UUID, error) {
	return f.reactChan, f.reactErr
}

func (f *fakeStore) SetCardVote(context.Context, uuid.UUID, string, string) (uuid.UUID, []domain.CardVote, error) {
	return f.reactChan, f.cardVotes, f.cardErr
}

func (f *fakeStore) ClearCardVote(context.Context, uuid.UUID, string, string) (uuid.UUID, []domain.CardVote, error) {
	return f.reactChan, f.cardVotes, f.cardErr
}

func (f *fakeStore) MarkRead(context.Context, uuid.UUID, string, *uuid.UUID) error { return f.markErr }

func (f *fakeStore) MarkUnread(context.Context, uuid.UUID, string) error { return f.markUnread }

func (f *fakeStore) UnreadByChannel(context.Context, uuid.UUID, string) ([]domain.ReadState, error) {
	return f.unread, f.unreadErr
}

func (f *fakeStore) PinMessage(context.Context, uuid.UUID, string) (domain.Message, error) {
	return domain.Message{ID: uuid.New(), ChannelID: f.pinChan}, f.pinErr
}

func (f *fakeStore) UnpinMessage(context.Context, uuid.UUID, string) (uuid.UUID, error) {
	return f.pinChan, f.unpinErr
}

func (f *fakeStore) ListPins(context.Context, uuid.UUID, string) ([]domain.Message, error) {
	return f.pins, f.pinErr
}
func (f *fakeStore) SaveMessage(context.Context, uuid.UUID, string) error   { return f.saveErr }
func (f *fakeStore) UnsaveMessage(context.Context, uuid.UUID, string) error { return f.unsaveErr }
func (f *fakeStore) ListSaved(context.Context, string, *time.Time, int) ([]domain.Message, *time.Time, error) {
	return f.saved, f.savedNext, f.savedErr
}

func (f *fakeStore) ListDrafts(context.Context, string) ([]domain.Draft, error) {
	return f.drafts, f.draftErr
}

func (f *fakeStore) PutDraft(_ context.Context, _ string, channelID uuid.UUID, parentID *uuid.UUID, body string) (domain.Draft, error) {
	f.draftBody = body
	return domain.Draft{ChannelID: channelID, ParentID: parentID, Body: body}, f.draftErr
}

func (f *fakeStore) DeleteDraft(context.Context, string, uuid.UUID, *uuid.UUID) error {
	f.draftDeleted = true
	return f.draftErr
}

var _ dataStore = (*fakeStore)(nil)

// capturePub captura los tipos de evento publicados.
type capturePub struct{ types []string }

func (c *capturePub) Publish(_ context.Context, ev events.Event) error {
	c.types = append(c.types, ev.Type)
	return nil
}

// fakeAttachments implementa Attachments.
type fakeAttachments struct {
	att domain.Attachment
	err error
}

func (f fakeAttachments) Resolve(context.Context, uuid.UUID, uuid.UUID) (domain.Attachment, error) {
	return f.att, f.err
}

func newSvc(fs *fakeStore, pub EventPublisher, files Attachments) *Service {
	return &Service{
		cfg:   config.Config{MaxMessageBytes: 1024, MessagePageSize: 50},
		store: fs,
		pub:   pub,
		files: files,
	}
}

func tenantCtx() context.Context {
	tn, _ := tenancy.New("acme")
	return tenancy.Into(context.Background(), tn)
}

func TestCreateTeamValidation(t *testing.T) {
	s := newSvc(&fakeStore{}, nil, nil)
	if _, err := s.CreateTeam(tenantCtx(), "  ", "d", "sub", "n"); !errors.Is(err, domain.ErrInvalidName) {
		t.Fatalf("nombre inválido: %v", err)
	}
}

func TestCreateTeamDelegates(t *testing.T) {
	fs := &fakeStore{team: domain.Team{Name: "Eng"}}
	s := newSvc(fs, nil, nil)
	if _, err := s.CreateTeam(tenantCtx(), "Engineering", "d", "sub", "Alice"); err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	if fs.newTeam == nil || fs.newTeam.Slug == "" {
		t.Fatalf("slug no derivado: %+v", fs.newTeam)
	}
}

func TestAddTeamMemberValidation(t *testing.T) {
	s := newSvc(&fakeStore{}, nil, nil)
	if err := s.AddTeamMember(tenantCtx(), uuid.New(), "actor", "", "n", domain.RoleMember); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("sub vacío: %v", err)
	}
}

func TestAddTeamMemberDefaultsRole(t *testing.T) {
	fs := &fakeStore{}
	s := newSvc(fs, nil, nil)
	// Rol inválido cae a member y no falla.
	if err := s.AddTeamMember(tenantCtx(), uuid.New(), "actor", "sub", "n", domain.MemberRole("weird")); err != nil {
		t.Fatalf("AddTeamMember: %v", err)
	}
}

func TestCreateChannelValidation(t *testing.T) {
	s := newSvc(&fakeStore{}, nil, nil)
	ctx := tenantCtx()
	if _, err := s.CreateChannel(ctx, CreateChannelInput{Name: " "}); !errors.Is(err, domain.ErrInvalidName) {
		t.Fatalf("nombre inválido: %v", err)
	}
	if _, err := s.CreateChannel(ctx, CreateChannelInput{Name: "general", Kind: domain.ChannelKind("weird")}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("kind inválido: %v", err)
	}
}

func TestCreateChannelDefaultsKind(t *testing.T) {
	fs := &fakeStore{channel: domain.Channel{Name: "general"}}
	s := newSvc(fs, nil, nil)
	if _, err := s.CreateChannel(tenantCtx(), CreateChannelInput{Name: "General", Creator: "sub"}); err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}
	if fs.newChan.Kind != domain.ChannelPublic {
		t.Errorf("kind por defecto = public, got %q", fs.newChan.Kind)
	}
	if fs.newChan.Slug == "" {
		t.Error("slug no derivado")
	}
}

func TestCanAccessChannel(t *testing.T) {
	// Acceso concedido cuando GetChannel no devuelve error.
	s := newSvc(&fakeStore{channel: domain.Channel{ID: uuid.New()}}, nil, nil)
	if tenant, ok := s.CanAccessChannel(context.Background(), "acme", uuid.New(), "sub"); !ok || tenant != "acme" {
		t.Fatalf("debería tener acceso en su organización: %q %v", tenant, ok)
	}
	// Denegado cuando GetChannel falla.
	s = newSvc(&fakeStore{channelErr: domain.ErrNotFound}, nil, nil)
	if _, ok := s.CanAccessChannel(context.Background(), "acme", uuid.New(), "sub"); ok {
		t.Fatal("no debería tener acceso")
	}
}

// Un borrador en BLANCO no se guarda: se BORRA. Es el caso que ocurre en cada
// envío y en cada composer que se vacía; si se guardara, la vista «Borradores»
// se llenaría de entradas fantasma sin texto.
func TestPutDraftBlankBodyDeletes(t *testing.T) {
	fs := &fakeStore{}
	s := newSvc(fs, nil, nil)
	ch := uuid.New()
	for _, blanco := range []string{"", "   ", "\n\t "} {
		fs.draftDeleted, fs.draftBody = false, ""
		d, err := s.PutDraft(tenantCtx(), "ana", ch, nil, blanco)
		if err != nil {
			t.Fatalf("cuerpo en blanco %q: %v", blanco, err)
		}
		if !fs.draftDeleted {
			t.Fatalf("cuerpo en blanco %q debe BORRAR el borrador", blanco)
		}
		if fs.draftBody != "" {
			t.Fatalf("cuerpo en blanco %q no debe escribirse: %q", blanco, fs.draftBody)
		}
		if d.Body != "" || d.ChannelID != ch {
			t.Fatalf("la respuesta del borrado debe identificar el ámbito vacío: %+v", d)
		}
	}
}

// El borrador se guarda con el MISMO tope que un mensaje: uno que no cabría en un
// mensaje no se podría enviar jamás, así que se rechaza al escribirlo.
func TestPutDraftLimitsAndTrims(t *testing.T) {
	fs := &fakeStore{}
	s := newSvc(fs, nil, nil) // el tope de mensaje que deja montado son 1024 bytes
	if _, err := s.PutDraft(tenantCtx(), "ana", uuid.New(), nil, strings.Repeat("x", 1025)); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("un borrador mayor que el tope de mensaje debe rechazarse, got %v", err)
	}
	if fs.draftBody != "" {
		t.Fatalf("nada debe llegar al store si no valida: %q", fs.draftBody)
	}
	parent := uuid.New()
	d, err := s.PutDraft(tenantCtx(), "ana", uuid.New(), &parent, "hola  \n")
	if err != nil {
		t.Fatalf("borrador válido: %v", err)
	}
	if fs.draftBody != "hola" {
		t.Fatalf("el cuerpo debe llegar recortado por la derecha: %q", fs.draftBody)
	}
	if d.ParentID == nil || *d.ParentID != parent {
		t.Fatalf("el hilo debe conservarse en la respuesta: %+v", d)
	}
	if fs.draftDeleted {
		t.Fatal("un borrador con texto no debe borrarse")
	}
}

func TestPostMessageEmptyBodyNoAttachments(t *testing.T) {
	s := newSvc(&fakeStore{}, nil, nil)
	if _, err := s.PostMessage(tenantCtx(), PostMessageInput{Body: "   "}); err == nil {
		t.Fatal("cuerpo vacío sin adjuntos debe fallar")
	}
}

// Un mensaje de sistema type=call admite cuerpo vacío y se persiste con type "call".
func TestPostMessageCallTypeEmptyBody(t *testing.T) {
	fs := &fakeStore{msg: domain.Message{ID: uuid.New(), ChannelID: uuid.New(), Type: domain.MessageTypeCall}}
	pub := &capturePub{}
	s := newSvc(fs, pub, nil)
	if _, err := s.PostMessage(tenantCtx(), PostMessageInput{
		ChannelID: uuid.New(), Body: "", Type: domain.MessageTypeCall, AuthorSub: "ana",
	}); err != nil {
		t.Fatalf("call con cuerpo vacío no debe fallar: %v", err)
	}
	if fs.newMsg == nil || fs.newMsg.Type != domain.MessageTypeCall {
		t.Fatalf("type no propagado al store: %+v", fs.newMsg)
	}
	if len(pub.types) != 1 || pub.types[0] != EventMessageCreated {
		t.Fatalf("esperaba message.created, got %v", pub.types)
	}
}

// El tipo vacío se normaliza a "user"; un tipo desconocido se rechaza.
func TestPostMessageTypeValidation(t *testing.T) {
	fs := &fakeStore{msg: domain.Message{ID: uuid.New(), ChannelID: uuid.New()}}
	s := newSvc(fs, &capturePub{}, nil)
	if _, err := s.PostMessage(tenantCtx(), PostMessageInput{ChannelID: uuid.New(), Body: "hola"}); err != nil {
		t.Fatalf("type vacío debe valer: %v", err)
	}
	if fs.newMsg.Type != domain.MessageTypeUser {
		t.Errorf("type vacío debe normalizarse a user, got %q", fs.newMsg.Type)
	}
	if _, err := s.PostMessage(tenantCtx(), PostMessageInput{ChannelID: uuid.New(), Body: "x", Type: "bogus"}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("type desconocido debe ser entrada inválida, got %v", err)
	}
}

func TestPostMessagePublishesAndDedupesMentions(t *testing.T) {
	fs := &fakeStore{msg: domain.Message{ID: uuid.New(), ChannelID: uuid.New()}}
	pub := &capturePub{}
	s := newSvc(fs, pub, nil)
	_, err := s.PostMessage(tenantCtx(), PostMessageInput{
		ChannelID: uuid.New(),
		Body:      "hola @bob",
		Mentions:  []string{"bob", "bob", "", "alice"},
	})
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if len(fs.newMsg.Mentions) != 2 {
		t.Errorf("menciones deduplicadas: %v", fs.newMsg.Mentions)
	}
	if len(pub.types) != 1 || pub.types[0] != EventMessageCreated {
		t.Fatalf("esperaba message.created, got %v", pub.types)
	}
}

func TestPostMessageChannelMentionExpansion(t *testing.T) {
	// @channel/@here/@all marcan el mensaje como "menciona a todo el canal"
	// (mentions_all) sin expandir a N menciones. El resto no lo marca.
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"channel", "atención @channel por favor", true},
		{"here", "@here ¿quién está?", true},
		{"all", "hola @all", true},
		{"solo usuario", "hola @bob y equipo", false},
		{"correo no cuenta", "escribe a foo@channel.com", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fs := &fakeStore{msg: domain.Message{ID: uuid.New(), ChannelID: uuid.New()}}
			s := newSvc(fs, &capturePub{}, nil)
			if _, err := s.PostMessage(tenantCtx(), PostMessageInput{ChannelID: uuid.New(), Body: c.body}); err != nil {
				t.Fatalf("PostMessage: %v", err)
			}
			if fs.newMsg.MentionsAll != c.want {
				t.Fatalf("MentionsAll=%v, quiero %v (body=%q)", fs.newMsg.MentionsAll, c.want, c.body)
			}
		})
	}
}

func TestPostMessageAttachmentsResolved(t *testing.T) {
	fid := uuid.New()
	fs := &fakeStore{msg: domain.Message{ID: uuid.New(), ChannelID: uuid.New()}}
	files := fakeAttachments{att: domain.Attachment{FileID: fid, Name: "doc.pdf", SizeBytes: 10}}
	s := newSvc(fs, &capturePub{}, files)
	// Cuerpo vacío permitido si hay adjuntos.
	_, err := s.PostMessage(tenantCtx(), PostMessageInput{ChannelID: uuid.New(), Body: "", FileIDs: []uuid.UUID{fid, fid, uuid.Nil}})
	if err != nil {
		t.Fatalf("PostMessage con adjuntos: %v", err)
	}
	if len(fs.newMsg.Attachments) != 1 || fs.newMsg.Attachments[0].Name != "doc.pdf" {
		t.Fatalf("adjunto no resuelto/deduplicado: %+v", fs.newMsg.Attachments)
	}
}

func TestPostMessageAttachmentNotFound(t *testing.T) {
	fs := &fakeStore{}
	files := fakeAttachments{err: domain.ErrNotFound}
	s := newSvc(fs, &capturePub{}, files)
	_, err := s.PostMessage(tenantCtx(), PostMessageInput{ChannelID: uuid.New(), Body: "x", FileIDs: []uuid.UUID{uuid.New()}})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("adjunto inexistente debe ser entrada inválida, got %v", err)
	}
}

func TestPostMessageAttachmentDriveErrorFallsBack(t *testing.T) {
	fs := &fakeStore{msg: domain.Message{ID: uuid.New()}}
	files := fakeAttachments{err: errBoom} // error transitorio de Drive
	s := newSvc(fs, &capturePub{}, files)
	fid := uuid.New()
	_, err := s.PostMessage(tenantCtx(), PostMessageInput{ChannelID: uuid.New(), Body: "x", FileIDs: []uuid.UUID{fid}})
	if err != nil {
		t.Fatalf("error transitorio de Drive no debe romper el post: %v", err)
	}
	// Conserva el id sin metadatos.
	if len(fs.newMsg.Attachments) != 1 || fs.newMsg.Attachments[0].FileID != fid {
		t.Fatalf("adjunto fallback incorrecto: %+v", fs.newMsg.Attachments)
	}
}

func TestListMessagesDefaultLimit(t *testing.T) {
	fs := &fakeStore{msgs: []domain.Message{{ID: uuid.New()}}}
	s := newSvc(fs, nil, nil)
	if _, err := s.ListMessages(tenantCtx(), uuid.New(), "sub", nil, 0); err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	before := time.Now()
	if _, err := s.ListMessages(tenantCtx(), uuid.New(), "sub", &before, 10); err != nil {
		t.Fatalf("ListMessages con before: %v", err)
	}
}

func TestEditMessageValidationAndPublish(t *testing.T) {
	// Cuerpo inválido tras normalizar.
	s := newSvc(&fakeStore{}, nil, nil)
	if _, err := s.EditMessage(tenantCtx(), uuid.New(), "actor", "   "); err == nil {
		t.Fatal("cuerpo vacío en edición debe fallar")
	}
	// Éxito publica evento.
	fs := &fakeStore{editMsg: domain.Message{ID: uuid.New(), ChannelID: uuid.New()}}
	pub := &capturePub{}
	s = newSvc(fs, pub, nil)
	if _, err := s.EditMessage(tenantCtx(), uuid.New(), "actor", "nuevo texto"); err != nil {
		t.Fatalf("EditMessage: %v", err)
	}
	if len(pub.types) != 1 || pub.types[0] != EventMessageEdited {
		t.Fatalf("esperaba message.edited, got %v", pub.types)
	}
}

func TestDeleteMessagePublishes(t *testing.T) {
	fs := &fakeStore{delChan: uuid.New()}
	pub := &capturePub{}
	s := newSvc(fs, pub, nil)
	if err := s.DeleteMessage(tenantCtx(), uuid.New(), "actor"); err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}
	if len(pub.types) != 1 || pub.types[0] != EventMessageDeleted {
		t.Fatalf("esperaba message.deleted, got %v", pub.types)
	}
	// Error del store no publica.
	fs.delErr = errBoom
	pub.types = nil
	if err := s.DeleteMessage(tenantCtx(), uuid.New(), "actor"); !errors.Is(err, errBoom) {
		t.Fatalf("error debe propagarse, got %v", err)
	}
	if len(pub.types) != 0 {
		t.Fatal("no debe publicar tras error")
	}
}

func TestSearchMessagesValidation(t *testing.T) {
	s := newSvc(&fakeStore{}, nil, nil)
	if _, err := s.SearchMessages(tenantCtx(), uuid.New(), "sub", "   ", 10); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("query vacía: %v", err)
	}
}

func TestReactionsValidationAndPublish(t *testing.T) {
	s := newSvc(&fakeStore{}, nil, nil)
	if err := s.AddReaction(tenantCtx(), uuid.New(), "sub", "this-emoji-value-is-way-too-long-to-be-valid-because-it-exceeds-sixty-four-bytes"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("emoji inválido en add: %v", err)
	}
	if err := s.RemoveReaction(tenantCtx(), uuid.New(), "sub", "this-emoji-value-is-way-too-long-to-be-valid-because-it-exceeds-sixty-four-bytes"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("emoji inválido en remove: %v", err)
	}

	fs := &fakeStore{reactChan: uuid.New()}
	pub := &capturePub{}
	s = newSvc(fs, pub, nil)
	if err := s.AddReaction(tenantCtx(), uuid.New(), "sub", "👍"); err != nil {
		t.Fatalf("AddReaction: %v", err)
	}
	if err := s.RemoveReaction(tenantCtx(), uuid.New(), "sub", "👍"); err != nil {
		t.Fatalf("RemoveReaction: %v", err)
	}
	if len(pub.types) != 2 {
		t.Fatalf("esperaba 2 eventos de reacción, got %v", pub.types)
	}
	for _, ty := range pub.types {
		if ty != EventReactionChanged {
			t.Fatalf("tipo de evento inesperado: %q", ty)
		}
	}
}

func TestPinMessagePublishes(t *testing.T) {
	fs := &fakeStore{pinChan: uuid.New()}
	pub := &capturePub{}
	s := newSvc(fs, pub, nil)
	if _, err := s.PinMessage(tenantCtx(), uuid.New(), "sub"); err != nil {
		t.Fatalf("PinMessage: %v", err)
	}
	if len(pub.types) != 1 || pub.types[0] != EventPinChanged {
		t.Fatalf("esperaba %s, got %v", EventPinChanged, pub.types)
	}
	// Error del store no publica.
	fs.pinErr = errBoom
	pub.types = nil
	if _, err := s.PinMessage(tenantCtx(), uuid.New(), "sub"); !errors.Is(err, errBoom) {
		t.Fatalf("error debe propagarse: %v", err)
	}
	if len(pub.types) != 0 {
		t.Fatal("no debe publicar tras error")
	}
}

func TestUnpinMessagePublishes(t *testing.T) {
	fs := &fakeStore{pinChan: uuid.New()}
	pub := &capturePub{}
	s := newSvc(fs, pub, nil)
	if err := s.UnpinMessage(tenantCtx(), uuid.New(), "sub"); err != nil {
		t.Fatalf("UnpinMessage: %v", err)
	}
	if len(pub.types) != 1 || pub.types[0] != EventPinChanged {
		t.Fatalf("esperaba %s, got %v", EventPinChanged, pub.types)
	}
	fs.unpinErr = errBoom
	pub.types = nil
	if err := s.UnpinMessage(tenantCtx(), uuid.New(), "sub"); !errors.Is(err, errBoom) {
		t.Fatalf("error debe propagarse: %v", err)
	}
	if len(pub.types) != 0 {
		t.Fatal("no debe publicar tras error")
	}
}

// TestSaveMarkUnreadNoEvent: guardar/marcar-no-leído son per-usuario y NO emiten
// evento de fan-out (solo afectan a quien los pide).
func TestSaveMarkUnreadNoEvent(t *testing.T) {
	fs := &fakeStore{}
	pub := &capturePub{}
	s := newSvc(fs, pub, nil)
	ctx := tenantCtx()
	if err := s.SaveMessage(ctx, uuid.New(), "sub"); err != nil {
		t.Fatalf("SaveMessage: %v", err)
	}
	if err := s.UnsaveMessage(ctx, uuid.New(), "sub"); err != nil {
		t.Fatalf("UnsaveMessage: %v", err)
	}
	if err := s.MarkUnread(ctx, uuid.New(), "sub"); err != nil {
		t.Fatalf("MarkUnread: %v", err)
	}
	if len(pub.types) != 0 {
		t.Fatalf("acciones per-usuario no deben publicar eventos, got %v", pub.types)
	}
	// Errores se propagan.
	fs.saveErr, fs.unsaveErr, fs.markUnread = errBoom, errBoom, errBoom
	if err := s.SaveMessage(ctx, uuid.New(), "sub"); !errors.Is(err, errBoom) {
		t.Fatalf("SaveMessage error: %v", err)
	}
	if err := s.UnsaveMessage(ctx, uuid.New(), "sub"); !errors.Is(err, errBoom) {
		t.Fatalf("UnsaveMessage error: %v", err)
	}
	if err := s.MarkUnread(ctx, uuid.New(), "sub"); !errors.Is(err, errBoom) {
		t.Fatalf("MarkUnread error: %v", err)
	}
}

func TestListSavedDefaultLimitAndPassthrough(t *testing.T) {
	next := time.Now()
	fs := &fakeStore{
		saved:     []domain.Message{{ID: uuid.New()}},
		savedNext: &next,
		pins:      []domain.Message{{ID: uuid.New()}},
	}
	s := newSvc(fs, nil, nil)
	ctx := tenantCtx()
	msgs, gotNext, err := s.ListSaved(ctx, "sub", nil, 0)
	if err != nil {
		t.Fatalf("ListSaved: %v", err)
	}
	if len(msgs) != 1 || gotNext == nil {
		t.Fatalf("ListSaved passthrough: msgs=%v next=%v", msgs, gotNext)
	}
	if _, err := s.ListPins(ctx, uuid.New(), "sub"); err != nil {
		t.Fatalf("ListPins: %v", err)
	}
}

func TestPassthroughMethods(t *testing.T) {
	fs := &fakeStore{
		teams:    []domain.Team{{ID: uuid.New()}},
		members:  []domain.Member{{Subject: "a"}},
		channels: []domain.Channel{{ID: uuid.New()}},
		msgs:     []domain.Message{{ID: uuid.New()}},
		unread:   []domain.ReadState{{}},
	}
	s := newSvc(fs, nil, nil)
	ctx := tenantCtx()
	if _, err := s.ListTeams(ctx, "sub"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetTeam(ctx, uuid.New(), "sub"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ListTeamMembers(ctx, uuid.New(), "sub"); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveTeamMember(ctx, uuid.New(), "actor", "sub"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ListChannels(ctx, uuid.New(), "sub", false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetChannel(ctx, uuid.New(), "sub"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateChannel(ctx, uuid.New(), "actor", store.UpdateChannelRecord{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.JoinChannel(ctx, uuid.New(), "sub", "n"); err != nil {
		t.Fatal(err)
	}
	if err := s.LeaveChannel(ctx, uuid.New(), "sub"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Thread(ctx, uuid.New(), "sub"); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkRead(ctx, uuid.New(), "sub", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Unread(ctx, uuid.New(), "sub"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SearchMessages(ctx, uuid.New(), "sub", "query", 10); err != nil {
		t.Fatal(err)
	}
}

// --- Categorías (Ola 11) -----------------------------------------------------

func TestCreateCategoryValidation(t *testing.T) {
	s := newSvc(&fakeStore{}, nil, nil)
	if _, err := s.CreateCategory(tenantCtx(), "sub", "   "); !errors.Is(err, domain.ErrInvalidName) {
		t.Fatalf("nombre vacío debe ser ErrInvalidName: %v", err)
	}
}

func TestCreateCategoryDelegates(t *testing.T) {
	fs := &fakeStore{category: domain.Category{Name: "Proyectos"}}
	s := newSvc(fs, nil, nil)
	if _, err := s.CreateCategory(tenantCtx(), "sub", "Proyectos"); err != nil {
		t.Fatalf("CreateCategory: %v", err)
	}
	if fs.newCatName != "Proyectos" {
		t.Errorf("nombre no propagado al store: %q", fs.newCatName)
	}
}

func TestUpdateCategoryValidation(t *testing.T) {
	s := newSvc(&fakeStore{}, nil, nil)
	bad := "   "
	if _, err := s.UpdateCategory(tenantCtx(), uuid.New(), "sub", store.UpdateCategoryRecord{Name: &bad}); !errors.Is(err, domain.ErrInvalidName) {
		t.Fatalf("nombre inválido debe ser ErrInvalidName: %v", err)
	}
	// Colapsar sin renombrar no valida nombre y delega.
	fs := &fakeStore{}
	s = newSvc(fs, nil, nil)
	yes := true
	if _, err := s.UpdateCategory(tenantCtx(), uuid.New(), "sub", store.UpdateCategoryRecord{Collapsed: &yes}); err != nil {
		t.Fatalf("colapsar no debe fallar: %v", err)
	}
	if fs.gotUpdCat == nil || fs.gotUpdCat.Collapsed == nil || !*fs.gotUpdCat.Collapsed {
		t.Fatalf("update no propagado al store: %+v", fs.gotUpdCat)
	}
}

func TestCategoryPassthrough(t *testing.T) {
	fs := &fakeStore{categories: []domain.Category{{Name: "A"}}}
	s := newSvc(fs, nil, nil)
	ctx := tenantCtx()
	if _, err := s.ListCategories(ctx, "sub"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteCategory(ctx, uuid.New(), "sub"); err != nil {
		t.Fatal(err)
	}
	cid := uuid.New()
	if _, err := s.SetChannelCategory(ctx, uuid.New(), "sub", "Ana", &cid, nil); err != nil {
		t.Fatal(err)
	}
}
