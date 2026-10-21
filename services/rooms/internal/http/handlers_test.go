// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chathttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/rooms/internal/config"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/service"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

// --- Fake del servicio -------------------------------------------------------

type fakeService struct {
	// anfitrion es la organización que devuelve Anfitrion (vacía = la del token).
	anfitrion  string
	persona    service.Persona
	personaErr error

	// apnsToken guarda el token que le llegó al servicio: lo comprueba el test de
	// registro del aparato de iOS.
	apnsToken string
	teams     []domain.Team
	// ownMessages son los mensajes propios, en orden, que pagina ListOwnMessages.
	ownMessages []domain.Message
	team        domain.Team
	teamErr     error

	members   []domain.Member
	memberErr error
	gotRole   domain.MemberRole
	gotSub    string

	states    []domain.ReadState
	unreadErr error

	cardVotes []domain.CardVote
	cardValue string
	cardErr   error

	notifyPrefs domain.NotifyPrefs
	vapidKey    string

	channels   []domain.Channel
	channel    domain.Channel
	channelErr error
	gotChanIn  service.CreateChannelInput

	categories  []domain.Category
	category    domain.Category
	categoryErr error
	gotCatName  string
	gotCatID    *uuid.UUID

	messages   []domain.Message
	message    domain.Message
	messageErr error
	// Raíz del hilo que devuelve MessagesAround cuando el objetivo era una respuesta.
	aroundRoot uuid.UUID
	gotPostIn  service.PostMessageInput

	unfurl        domain.LinkUnfurl
	imagen        []byte
	imagenErr     error
	gotImagenHash string
	unfurlErr     error
	gotUnfurlURL  string
	receipts      []domain.Receipt
	receiptsErr   error

	ackCount      int
	readers       []store.MessageReader
	scheduled     domain.ScheduledMessage
	scheduledList []domain.ScheduledMessage
	drafts        []domain.Draft
	gotSchedIn    service.ScheduleMessageInput

	rootMsg   domain.Message
	replies   []domain.Message
	threadErr error

	following bool
	threads   []domain.ThreadSummary

	err error // error genérico para operaciones sin retorno de valor

	gotEmoji   string
	gotMarkMID *uuid.UUID
	gotBefore  *time.Time
	gotSubs    []string

	customEmoji    []domain.CustomEmoji
	customEmojiOne domain.CustomEmoji
	customEmojiErr error
	gotEmojiName   string
	gotEmojiFileID uuid.UUID
}

func (f *fakeService) ListTeams(context.Context, string) ([]domain.Team, error) {
	return f.teams, f.teamErr
}

func (f *fakeService) EnsureDefaultTeams(context.Context, string, string) ([]domain.Team, error) {
	return f.teams, f.teamErr
}

func (f *fakeService) CreateTeam(context.Context, string, string, string, string) (domain.Team, error) {
	return f.team, f.teamErr
}

func (f *fakeService) GetTeam(context.Context, uuid.UUID, string) (domain.Team, error) {
	return f.team, f.teamErr
}

func (f *fakeService) ListTeamMembers(context.Context, uuid.UUID, string) ([]domain.Member, error) {
	return f.members, f.memberErr
}

func (f *fakeService) AddTeamMember(_ context.Context, _ uuid.UUID, _, sub, _ string, role domain.MemberRole) error {
	f.gotSub, f.gotRole = sub, role
	return f.err
}

func (f *fakeService) RemoveTeamMember(_ context.Context, _ uuid.UUID, _, sub string) error {
	f.gotSub = sub
	return f.err
}

func (f *fakeService) Unread(context.Context, uuid.UUID, string) ([]domain.ReadState, error) {
	return f.states, f.unreadErr
}

func (f *fakeService) SearchMessages(context.Context, uuid.UUID, string, string, int) ([]domain.Message, error) {
	return f.messages, f.messageErr
}

func (f *fakeService) SearchAll(context.Context, string, string, int) ([]domain.Message, error) {
	return f.messages, f.messageErr
}

func (f *fakeService) CreateChannel(_ context.Context, in service.CreateChannelInput) (domain.Channel, error) {
	f.gotChanIn = in
	return f.channel, f.channelErr
}

func (f *fakeService) ListChannels(context.Context, uuid.UUID, string, bool) ([]domain.Channel, error) {
	return f.channels, f.channelErr
}

// ListOwnMessages pagina f.ownMessages como el store: después del cursor, hasta limit.
func (f *fakeService) ListOwnMessages(_ context.Context, _ string, afterAt time.Time, afterID uuid.UUID, limit int) ([]domain.Message, error) {
	var out []domain.Message
	for _, m := range f.ownMessages {
		if m.CreatedAt.After(afterAt) || (m.CreatedAt.Equal(afterAt) && m.ID.String() > afterID.String()) {
			out = append(out, m)
		}
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

func (f *fakeService) GetChannel(context.Context, uuid.UUID, string) (domain.Channel, error) {
	return f.channel, f.channelErr
}

func (f *fakeService) UpdateChannel(context.Context, uuid.UUID, string, store.UpdateChannelRecord) (domain.Channel, error) {
	return f.channel, f.channelErr
}

func (f *fakeService) JoinChannel(context.Context, uuid.UUID, string, string) (domain.Channel, error) {
	return f.channel, f.channelErr
}

func (f *fakeService) OpenGroupDirect(_ context.Context, _ uuid.UUID, _ string, otherSubs []string) (domain.Channel, error) {
	f.gotSubs = otherSubs
	return f.channel, f.channelErr
}

func (f *fakeService) OpenDirect(context.Context, uuid.UUID, string, string) (domain.Channel, error) {
	return f.channel, f.channelErr
}

func (f *fakeService) ResolvePerson(context.Context, string) (service.Persona, error) {
	return f.persona, f.personaErr
}

func (f *fakeService) Invitaciones(context.Context, string) ([]domain.Invitacion, error) {
	return nil, nil
}

func (f *fakeService) AceptarInvitacion(context.Context, string, string, uuid.UUID) error { return nil }

func (f *fakeService) RechazarInvitacion(context.Context, string, uuid.UUID) error { return nil }

func (f *fakeService) ListExternalChannels(context.Context, string) ([]domain.Channel, []domain.ReadState, error) {
	return f.channels, f.states, f.channelErr
}

func (f *fakeService) Anfitrion(context.Context, string, service.RefAnfitrion) (string, error) {
	return f.anfitrion, nil
}

func (f *fakeService) GetNotifyPrefs(context.Context, string) (domain.NotifyPrefs, error) {
	return f.notifyPrefs, f.err
}

func (f *fakeService) PutNotifyPrefs(_ context.Context, _ string, p domain.NotifyPrefs) (domain.NotifyPrefs, error) {
	if f.err != nil {
		return domain.NotifyPrefs{}, f.err
	}
	f.notifyPrefs = p
	return p, nil
}

func (f *fakeService) SetChannelPref(context.Context, uuid.UUID, string, string, *bool, *bool, *string) (domain.Channel, error) {
	return f.channel, f.channelErr
}

func (f *fakeService) VAPIDPublicKey() string { return f.vapidKey }

func (f *fakeService) SubscribePush(context.Context, string, domain.PushSubscription) error {
	return f.err
}

func (f *fakeService) UnsubscribePush(context.Context, string, string) error { return f.err }

func (f *fakeService) RegisterAPNSDevice(_ context.Context, _, token string) error {
	f.apnsToken = token
	return f.err
}

func (f *fakeService) ForgetAPNSDevice(_ context.Context, _, token string) error {
	f.apnsToken = token
	return f.err
}

func (f *fakeService) ListJoinableChannels(context.Context, uuid.UUID, string, string, int) ([]domain.Channel, error) {
	return nil, f.channelErr
}
func (f *fakeService) LeaveChannel(context.Context, uuid.UUID, string) error { return f.err }
func (f *fakeService) DeleteChannel(_ context.Context, _ uuid.UUID, actor string) error {
	f.gotSub = actor
	return f.err
}

func (f *fakeService) HideDirect(_ context.Context, _ uuid.UUID, sub string) error {
	f.gotSub = sub
	return f.err
}

func (f *fakeService) ListChannelMembers(context.Context, uuid.UUID, string) ([]domain.Member, error) {
	return f.members, f.memberErr
}

func (f *fakeService) AddChannelMember(_ context.Context, _ uuid.UUID, _, sub, name string) (domain.Member, error) {
	f.gotSub = sub
	return domain.Member{Subject: sub, Name: name, Role: domain.RoleMember}, f.memberErr
}

func (f *fakeService) RemoveChannelMember(_ context.Context, _ uuid.UUID, _, sub string) error {
	f.gotSub = sub
	return f.memberErr
}

func (f *fakeService) SetChannelMemberRole(_ context.Context, _ uuid.UUID, _, sub string, role domain.MemberRole) (domain.Member, error) {
	f.gotSub, f.gotRole = sub, role
	return domain.Member{Subject: sub, Role: role}, f.memberErr
}

func (f *fakeService) ListCategories(context.Context, string) ([]domain.Category, error) {
	return f.categories, f.categoryErr
}

func (f *fakeService) CreateCategory(_ context.Context, _, name string) (domain.Category, error) {
	f.gotCatName = name
	return f.category, f.categoryErr
}

func (f *fakeService) UpdateCategory(context.Context, uuid.UUID, string, store.UpdateCategoryRecord) (domain.Category, error) {
	return f.category, f.categoryErr
}

func (f *fakeService) DeleteCategory(context.Context, uuid.UUID, string) error { return f.err }

func (f *fakeService) SetChannelCategory(_ context.Context, _ uuid.UUID, _, _ string, categoryID *uuid.UUID, _ *int) (domain.Channel, error) {
	f.gotCatID = categoryID
	return f.channel, f.channelErr
}

func (f *fakeService) MarkRead(_ context.Context, _ uuid.UUID, _ string, mid *uuid.UUID) error {
	f.gotMarkMID = mid
	return f.err
}

func (f *fakeService) ListMessages(_ context.Context, _ uuid.UUID, _ string, before *time.Time, _ int) ([]domain.Message, error) {
	f.gotBefore = before
	return f.messages, f.messageErr
}

func (f *fakeService) MessagesAround(_ context.Context, _ uuid.UUID, _ string, messageID uuid.UUID, _ int) ([]domain.Message, uuid.UUID, error) {
	f.gotMarkMID = &messageID
	return f.messages, f.aroundRoot, f.messageErr
}

func (f *fakeService) Unfurl(_ context.Context, url string) (domain.LinkUnfurl, error) {
	f.gotUnfurlURL = url
	return f.unfurl, f.unfurlErr
}

func (f *fakeService) ImagenDeUnfurl(_ context.Context, hash string) ([]byte, string, string, error) {
	f.gotImagenHash = hash
	if f.imagenErr != nil {
		return nil, "", "", f.imagenErr
	}
	return f.imagen, "image/jpeg", `"` + hash + `"`, nil
}

func (f *fakeService) DMReceipts(context.Context, uuid.UUID, string) ([]domain.Receipt, error) {
	return f.receipts, f.receiptsErr
}

func (f *fakeService) PostMessage(_ context.Context, in service.PostMessageInput) (domain.Message, error) {
	f.gotPostIn = in
	return f.message, f.messageErr
}

func (f *fakeService) AckMessage(context.Context, uuid.UUID, string) (int, error) {
	return f.ackCount, f.err
}

func (f *fakeService) MessageReaders(
	context.Context, uuid.UUID, string,
) ([]store.MessageReader, int, error) {
	return f.readers, len(f.readers), f.err
}

func (f *fakeService) PostScheduled(_ context.Context, in service.ScheduleMessageInput) (domain.ScheduledMessage, error) {
	f.gotSchedIn = in
	return f.scheduled, f.messageErr
}

func (f *fakeService) ListScheduled(context.Context, string, *uuid.UUID) ([]domain.ScheduledMessage, error) {
	return f.scheduledList, f.messageErr
}

func (f *fakeService) ListDrafts(context.Context, string) ([]domain.Draft, error) {
	return f.drafts, f.messageErr
}

func (f *fakeService) PutDraft(_ context.Context, _ string, channelID uuid.UUID, parentID *uuid.UUID, body string) (domain.Draft, error) {
	return domain.Draft{ChannelID: channelID, ParentID: parentID, Body: body}, f.messageErr
}

func (f *fakeService) UpdateScheduled(context.Context, uuid.UUID, string, *string, *time.Time, *[]uuid.UUID) (domain.ScheduledMessage, error) {
	return f.scheduled, f.messageErr
}

func (f *fakeService) CancelScheduled(context.Context, uuid.UUID, string) error { return f.err }

func (f *fakeService) Thread(context.Context, uuid.UUID, string) (domain.Message, []domain.Message, error) {
	return f.rootMsg, f.replies, f.threadErr
}

func (f *fakeService) FollowThread(context.Context, uuid.UUID, string) error   { return f.err }
func (f *fakeService) UnfollowThread(context.Context, uuid.UUID, string) error { return f.err }
func (f *fakeService) IsFollowingThread(context.Context, uuid.UUID, string) (bool, error) {
	return f.following, f.err
}

func (f *fakeService) ListFollowedThreads(context.Context, string, *time.Time, int) ([]domain.ThreadSummary, *time.Time, error) {
	return f.threads, nil, f.messageErr
}

func (f *fakeService) EditMessage(context.Context, uuid.UUID, string, string) (domain.Message, error) {
	return f.message, f.messageErr
}
func (f *fakeService) DeleteMessage(context.Context, uuid.UUID, string) error { return f.err }
func (f *fakeService) AddReaction(_ context.Context, _ uuid.UUID, _, emoji string) error {
	f.gotEmoji = emoji
	return f.err
}

func (f *fakeService) SetCardVote(_ context.Context, _ uuid.UUID, _, value string) ([]domain.CardVote, error) {
	f.cardValue = value
	return f.cardVotes, f.cardErr
}

func (f *fakeService) ClearCardVote(_ context.Context, _ uuid.UUID, _, value string) ([]domain.CardVote, error) {
	f.cardValue = value
	return f.cardVotes, f.cardErr
}

func (f *fakeService) RemoveReaction(_ context.Context, _ uuid.UUID, _, emoji string) error {
	f.gotEmoji = emoji
	return f.err
}

func (f *fakeService) MarkUnread(context.Context, uuid.UUID, string) error { return f.err }

func (f *fakeService) PinMessage(context.Context, uuid.UUID, string) (domain.Message, error) {
	return f.message, f.messageErr
}
func (f *fakeService) UnpinMessage(context.Context, uuid.UUID, string) error { return f.err }
func (f *fakeService) ListPins(context.Context, uuid.UUID, string) ([]domain.Message, error) {
	return f.messages, f.messageErr
}
func (f *fakeService) SaveMessage(context.Context, uuid.UUID, string) error   { return f.err }
func (f *fakeService) UnsaveMessage(context.Context, uuid.UUID, string) error { return f.err }
func (f *fakeService) ListSaved(context.Context, string, *time.Time, int) ([]domain.Message, *time.Time, error) {
	return f.messages, nil, f.messageErr
}

func (f *fakeService) ListCustomEmoji(context.Context) ([]domain.CustomEmoji, error) {
	return f.customEmoji, f.customEmojiErr
}

func (f *fakeService) CreateCustomEmoji(_ context.Context, name string, fileID uuid.UUID, _ string) (domain.CustomEmoji, error) {
	f.gotEmojiName, f.gotEmojiFileID = name, fileID
	return f.customEmojiOne, f.customEmojiErr
}

func (f *fakeService) DeleteCustomEmoji(_ context.Context, name, _ string) error {
	f.gotEmojiName = name
	return f.err
}

// --- Web Push (Ola 18) --------------------------------------------------------

func TestPushKeyOK(t *testing.T) {
	f := &fakeService{vapidKey: "BPk-public"}
	rec := httptest.NewRecorder()
	newHandlers(f).PushKey(rec, req(http.MethodGet, "/", "", claims(), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if decode(t, rec)["key"] != "BPk-public" {
		t.Errorf("clave VAPID inesperada: %v", decode(t, rec)["key"])
	}
}

func TestSubscribePushOK(t *testing.T) {
	f := &fakeService{}
	rec := httptest.NewRecorder()
	body := `{"endpoint":"https://push.example.com/ep","keys":{"p256dh":"BPk","auth":"abc"}}`
	newHandlers(f).SubscribePush(rec, req(http.MethodPost, "/", body, claims(), nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("code = %d, want 204", rec.Code)
	}
}

func TestSubscribePushBadJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeService{}).SubscribePush(rec, req(http.MethodPost, "/", `{bad`, claims(), nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestUnsubscribePushOK(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeService{}).UnsubscribePush(rec, req(http.MethodPost, "/", `{"endpoint":"https://x/y"}`, claims(), nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("code = %d, want 204", rec.Code)
	}
}

// --- Helpers -----------------------------------------------------------------

func newHandlers(f *fakeService) *Handlers {
	return &Handlers{svc: f, cfg: config.Config{}}
}

func req(method, target, body string, claims *authn.Claims, params map[string]string) *http.Request {
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
	} else {
		r = httptest.NewRequest(method, target, http.NoBody)
	}
	ctx := r.Context()
	if claims != nil {
		ctx = authn.Into(ctx, *claims)
	}
	if params != nil {
		rc := chi.NewRouteContext()
		for k, v := range params {
			rc.URLParams.Add(k, v)
		}
		ctx = context.WithValue(ctx, chi.RouteCtxKey, rc)
	}
	return r.WithContext(ctx)
}

func claims() *authn.Claims {
	return &authn.Claims{Subject: "user-1", Name: "Ana", Email: "a@ex.com", Tenant: "acme"}
}

func teamID(id string) map[string]string    { return map[string]string{"teamID": id} }
func channelID(id string) map[string]string { return map[string]string{"channelID": id} }
func messageID(id string) map[string]string { return map[string]string{"messageID": id} }

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("body no es JSON: %v (%q)", err, rec.Body.String())
	}
	return m
}

// --- Teams -------------------------------------------------------------------

func TestListTeamsOK(t *testing.T) {
	f := &fakeService{teams: []domain.Team{{Name: "eng"}}}
	rec := httptest.NewRecorder()
	newHandlers(f).ListTeams(rec, req(http.MethodGet, "/", "", claims(), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("content-type = %q", ct)
	}
	if _, ok := decode(t, rec)["teams"]; !ok {
		t.Error("falta clave teams")
	}
}

func TestListTeamsError(t *testing.T) {
	f := &fakeService{teamErr: context.DeadlineExceeded}
	rec := httptest.NewRecorder()
	newHandlers(f).ListTeams(rec, req(http.MethodGet, "/", "", claims(), nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("code = %d, want 500", rec.Code)
	}
}

func TestCreateTeamOK(t *testing.T) {
	f := &fakeService{team: domain.Team{Name: "eng"}}
	rec := httptest.NewRecorder()
	newHandlers(f).CreateTeam(rec, req(http.MethodPost, "/", `{"name":"eng"}`, claims(), nil))
	if rec.Code != http.StatusCreated {
		t.Fatalf("code = %d, want 201", rec.Code)
	}
}

func TestCreateTeamBadJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeService{}).CreateTeam(rec, req(http.MethodPost, "/", `{bad`, claims(), nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestCreateTeamInvalidName(t *testing.T) {
	f := &fakeService{teamErr: domain.ErrInvalidName}
	rec := httptest.NewRecorder()
	newHandlers(f).CreateTeam(rec, req(http.MethodPost, "/", `{"name":""}`, claims(), nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestGetTeamBadID(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeService{}).GetTeam(rec, req(http.MethodGet, "/", "", claims(), teamID("nope")))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestGetTeamForbidden(t *testing.T) {
	f := &fakeService{teamErr: domain.ErrNotMember}
	rec := httptest.NewRecorder()
	newHandlers(f).GetTeam(rec, req(http.MethodGet, "/", "", claims(), teamID(uuid.NewString())))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("code = %d, want 403", rec.Code)
	}
}

func TestListTeamMembersOK(t *testing.T) {
	f := &fakeService{members: []domain.Member{{Subject: "u"}}}
	rec := httptest.NewRecorder()
	newHandlers(f).ListTeamMembers(rec, req(http.MethodGet, "/", "", claims(), teamID(uuid.NewString())))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if _, ok := decode(t, rec)["members"]; !ok {
		t.Error("falta clave members")
	}
}

func TestAddTeamMemberOK(t *testing.T) {
	f := &fakeService{}
	rec := httptest.NewRecorder()
	newHandlers(f).AddTeamMember(rec, req(http.MethodPost, "/", `{"subject":" u2 ","role":"admin"}`, claims(), teamID(uuid.NewString())))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("code = %d, want 204", rec.Code)
	}
	if f.gotSub != "u2" {
		t.Errorf("subject = %q, want u2 (recortado)", f.gotSub)
	}
	if f.gotRole != domain.RoleAdmin {
		t.Errorf("role = %q, want admin", f.gotRole)
	}
}

func TestRemoveTeamMemberEmptySub(t *testing.T) {
	rec := httptest.NewRecorder()
	params := map[string]string{"teamID": uuid.NewString(), "sub": "  "}
	newHandlers(&fakeService{}).RemoveTeamMember(rec, req(http.MethodDelete, "/", "", claims(), params))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestRemoveTeamMemberOK(t *testing.T) {
	f := &fakeService{}
	rec := httptest.NewRecorder()
	params := map[string]string{"teamID": uuid.NewString(), "sub": "u2"}
	newHandlers(f).RemoveTeamMember(rec, req(http.MethodDelete, "/", "", claims(), params))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("code = %d, want 204", rec.Code)
	}
}

func TestUnreadOK(t *testing.T) {
	f := &fakeService{states: []domain.ReadState{{Unread: 3}}}
	rec := httptest.NewRecorder()
	newHandlers(f).Unread(rec, req(http.MethodGet, "/", "", claims(), teamID(uuid.NewString())))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if _, ok := decode(t, rec)["channels"]; !ok {
		t.Error("falta clave channels")
	}
}

func TestSearchMessagesMissingQuery(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeService{}).SearchMessages(rec, req(http.MethodGet, "/?q=", "", claims(), teamID(uuid.NewString())))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestSearchAllMissingQuery(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeService{}).SearchAll(rec, req(http.MethodGet, "/?q=", "", claims(), nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestSearchAllOK(t *testing.T) {
	f := &fakeService{messages: []domain.Message{{Body: "hola"}}}
	rec := httptest.NewRecorder()
	newHandlers(f).SearchAll(rec, req(http.MethodGet, "/?q=hola&limit=5", "", claims(), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
}

func TestSearchMessagesOK(t *testing.T) {
	f := &fakeService{messages: []domain.Message{}}
	rec := httptest.NewRecorder()
	newHandlers(f).SearchMessages(rec, req(http.MethodGet, "/?q=hola&limit=5", "", claims(), teamID(uuid.NewString())))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
}

// --- Canales -----------------------------------------------------------------

func TestCreateChannelOK(t *testing.T) {
	f := &fakeService{channel: domain.Channel{Name: "general"}}
	rec := httptest.NewRecorder()
	body := `{"name":"general","kind":"public","members":["u2"]}`
	newHandlers(f).CreateChannel(rec, req(http.MethodPost, "/", body, claims(), teamID(uuid.NewString())))
	if rec.Code != http.StatusCreated {
		t.Fatalf("code = %d, want 201", rec.Code)
	}
	if f.gotChanIn.Name != "general" || f.gotChanIn.Creator != "user-1" {
		t.Errorf("CreateChannelInput mal mapeado: %+v", f.gotChanIn)
	}
}

func TestCreateChannelBadTeamID(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeService{}).CreateChannel(rec, req(http.MethodPost, "/", `{}`, claims(), teamID("x")))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestListChannelsOK(t *testing.T) {
	f := &fakeService{channels: []domain.Channel{{Name: "c"}}}
	rec := httptest.NewRecorder()
	newHandlers(f).ListChannels(rec, req(http.MethodGet, "/?archived=1", "", claims(), teamID(uuid.NewString())))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if _, ok := decode(t, rec)["channels"]; !ok {
		t.Error("falta clave channels")
	}
}

func TestGetChannelNotFound(t *testing.T) {
	f := &fakeService{channelErr: domain.ErrNotFound}
	rec := httptest.NewRecorder()
	newHandlers(f).GetChannel(rec, req(http.MethodGet, "/", "", claims(), channelID(uuid.NewString())))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d, want 404", rec.Code)
	}
}

func TestUpdateChannelNothingToUpdate(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeService{}).UpdateChannel(rec, req(http.MethodPatch, "/", `{}`, claims(), channelID(uuid.NewString())))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestUpdateChannelOK(t *testing.T) {
	f := &fakeService{channel: domain.Channel{Name: "renamed"}}
	rec := httptest.NewRecorder()
	newHandlers(f).UpdateChannel(rec, req(http.MethodPatch, "/", `{"name":"renamed"}`, claims(), channelID(uuid.NewString())))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
}

func TestUpdateChannelForbidden(t *testing.T) {
	f := &fakeService{channelErr: domain.ErrForbidden}
	rec := httptest.NewRecorder()
	newHandlers(f).UpdateChannel(rec, req(http.MethodPatch, "/", `{"archived":true}`, claims(), channelID(uuid.NewString())))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("code = %d, want 403", rec.Code)
	}
}

func TestJoinChannelOK(t *testing.T) {
	f := &fakeService{channel: domain.Channel{Name: "c"}}
	rec := httptest.NewRecorder()
	newHandlers(f).JoinChannel(rec, req(http.MethodPost, "/", "", claims(), channelID(uuid.NewString())))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
}

func TestSetChannelPrefsEmptyBadRequest(t *testing.T) {
	f := &fakeService{channel: domain.Channel{Name: "c"}}
	rec := httptest.NewRecorder()
	newHandlers(f).SetChannelPrefs(rec, req(http.MethodPost, "/", `{}`, claims(), channelID(uuid.NewString())))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400 (nada que actualizar)", rec.Code)
	}
}

func TestSetChannelPrefsOK(t *testing.T) {
	f := &fakeService{channel: domain.Channel{Name: "c", Favorite: true}}
	rec := httptest.NewRecorder()
	newHandlers(f).SetChannelPrefs(rec, req(http.MethodPost, "/", `{"favorite":true}`, claims(), channelID(uuid.NewString())))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
}

func TestOpenDirectOK(t *testing.T) {
	f := &fakeService{channel: domain.Channel{Name: "Bob", Kind: domain.ChannelDirect}}
	rec := httptest.NewRecorder()
	newHandlers(f).OpenDirect(rec, req(http.MethodPost, "/", `{"subject":"bob"}`, claims(), teamID(uuid.NewString())))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
}

func TestOpenDirectMissingSubject(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeService{}).OpenDirect(rec, req(http.MethodPost, "/", `{"subject":"  "}`, claims(), teamID(uuid.NewString())))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

// {subjects:[…]} enruta a OpenGroupDirect (group DM) con la lista intacta.
func TestOpenGroupDirectOK(t *testing.T) {
	f := &fakeService{channel: domain.Channel{Name: "Ana, Beto", Kind: domain.ChannelDirect}}
	rec := httptest.NewRecorder()
	newHandlers(f).OpenDirect(rec, req(http.MethodPost, "/", `{"subjects":["ana","beto"]}`, claims(), teamID(uuid.NewString())))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if len(f.gotSubs) != 2 || f.gotSubs[0] != "ana" || f.gotSubs[1] != "beto" {
		t.Fatalf("gotSubs = %v, want [ana beto]", f.gotSubs)
	}
}

// Sin subject ni subjects → 400 (no llega al servicio).
func TestOpenDirectMissingBoth(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeService{}).OpenDirect(rec, req(http.MethodPost, "/", `{}`, claims(), teamID(uuid.NewString())))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestListJoinableChannelsOK(t *testing.T) {
	f := &fakeService{}
	rec := httptest.NewRecorder()
	newHandlers(f).ListJoinableChannels(rec, req(http.MethodGet, "/?q=gen", "", claims(), teamID(uuid.NewString())))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
}

func TestLeaveChannelOK(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeService{}).LeaveChannel(rec, req(http.MethodPost, "/", "", claims(), channelID(uuid.NewString())))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("code = %d, want 204", rec.Code)
	}
}

// TestDeleteChannel: borrar un canal contesta 204 y pasa el actor de la sesión; un
// no-admin (ErrForbidden) ve 403 y un canal que no existe, 404.
func TestDeleteChannel(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{nil, http.StatusNoContent},
		{domain.ErrForbidden, http.StatusForbidden},
		{domain.ErrNotFound, http.StatusNotFound},
	}
	for _, c := range cases {
		f := &fakeService{err: c.err}
		rec := httptest.NewRecorder()
		newHandlers(f).DeleteChannel(rec, req(http.MethodDelete, "/", "", claims(), channelID(uuid.NewString())))
		if rec.Code != c.want {
			t.Fatalf("err=%v: code = %d, want %d", c.err, rec.Code, c.want)
		}
		if f.gotSub == "" {
			t.Fatal("el servicio no recibió el actor")
		}
	}
	rec := httptest.NewRecorder()
	newHandlers(&fakeService{}).DeleteChannel(rec, req(http.MethodDelete, "/", "", claims(), channelID("no-es-uuid")))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("id inválido: code = %d, want 400", rec.Code)
	}
}

// TestHideDirect: borrar un directo para uno mismo contesta 204; si no es un
// directo o no es miembro (ErrForbidden), 403.
func TestHideDirect(t *testing.T) {
	rec := httptest.NewRecorder()
	f := &fakeService{}
	newHandlers(f).HideDirect(rec, req(http.MethodPost, "/", "", claims(), channelID(uuid.NewString())))
	if rec.Code != http.StatusNoContent || f.gotSub == "" {
		t.Fatalf("code = %d, sub = %q", rec.Code, f.gotSub)
	}
	rec = httptest.NewRecorder()
	newHandlers(&fakeService{err: domain.ErrForbidden}).HideDirect(rec, req(http.MethodPost, "/", "", claims(), channelID(uuid.NewString())))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("code = %d, want 403", rec.Code)
	}
}

func TestMarkReadNoBody(t *testing.T) {
	f := &fakeService{}
	rec := httptest.NewRecorder()
	r := req(http.MethodPost, "/", "", claims(), channelID(uuid.NewString()))
	newHandlers(f).MarkRead(rec, r)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("code = %d, want 204", rec.Code)
	}
	if f.gotMarkMID != nil {
		t.Error("sin body message_id debe ser nil")
	}
}

func TestMarkReadWithMessageID(t *testing.T) {
	f := &fakeService{}
	mid := uuid.NewString()
	rec := httptest.NewRecorder()
	newHandlers(f).MarkRead(rec, req(http.MethodPost, "/", `{"message_id":"`+mid+`"}`, claims(), channelID(uuid.NewString())))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("code = %d, want 204", rec.Code)
	}
	if f.gotMarkMID == nil || f.gotMarkMID.String() != mid {
		t.Errorf("message_id = %v, want %s", f.gotMarkMID, mid)
	}
}

func TestMarkReadBadMessageID(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeService{}).MarkRead(rec, req(http.MethodPost, "/", `{"message_id":"nope"}`, claims(), channelID(uuid.NewString())))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

// --- Mensajes / hilos --------------------------------------------------------

func TestListMessagesOK(t *testing.T) {
	now := time.Now().UTC()
	f := &fakeService{messages: []domain.Message{{Body: "hola", CreatedAt: now}}}
	rec := httptest.NewRecorder()
	newHandlers(f).ListMessages(rec, req(http.MethodGet, "/?limit=10", "", claims(), channelID(uuid.NewString())))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	m := decode(t, rec)
	if _, ok := m["next_before"]; !ok {
		t.Error("con mensajes debe incluir next_before")
	}
}

func TestListMessagesBadBefore(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeService{}).ListMessages(rec, req(http.MethodGet, "/?before=nope", "", claims(), channelID(uuid.NewString())))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestMessagesAroundOK(t *testing.T) {
	target := uuid.New()
	f := &fakeService{messages: []domain.Message{{Body: "a"}, {Body: "b"}}}
	rec := httptest.NewRecorder()
	newHandlers(f).MessagesAround(rec,
		req(http.MethodGet, "/?message="+target.String()+"&limit=10", "", claims(), channelID(uuid.NewString())))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if f.gotMarkMID == nil || *f.gotMarkMID != target {
		t.Fatalf("el messageID objetivo no llegó al servicio: %v", f.gotMarkMID)
	}
	if _, ok := decode(t, rec)["messages"]; !ok {
		t.Error("la respuesta debe incluir messages")
	}
}

func TestMessagesAroundBadMessage(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeService{}).MessagesAround(rec,
		req(http.MethodGet, "/?message=nope", "", claims(), channelID(uuid.NewString())))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400 (message no-UUID)", rec.Code)
	}
}

func TestUnfurlOK(t *testing.T) {
	f := &fakeService{unfurl: domain.LinkUnfurl{URL: "https://x.eu", Title: "T"}}
	rec := httptest.NewRecorder()
	newHandlers(f).Unfurl(rec, req(http.MethodGet, "/?url=https%3A%2F%2Fx.eu", "", claims(), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if f.gotUnfurlURL != "https://x.eu" {
		t.Fatalf("URL mal decodificada: %q", f.gotUnfurlURL)
	}
	if _, ok := decode(t, rec)["unfurl"]; !ok {
		t.Error("la respuesta debe incluir unfurl")
	}
}

func TestUnfurlMissingURL(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeService{}).Unfurl(rec, req(http.MethodGet, "/", "", claims(), nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400 (sin url)", rec.Code)
	}
}

func TestReceiptsOK(t *testing.T) {
	f := &fakeService{receipts: []domain.Receipt{{Subject: "bob", Name: "Bob"}}}
	rec := httptest.NewRecorder()
	newHandlers(f).Receipts(rec, req(http.MethodGet, "/", "", claims(), channelID(uuid.NewString())))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if _, ok := decode(t, rec)["receipts"]; !ok {
		t.Error("la respuesta debe incluir receipts")
	}
}

func TestPostMessageOK(t *testing.T) {
	f := &fakeService{message: domain.Message{Body: "hola"}}
	rec := httptest.NewRecorder()
	newHandlers(f).PostMessage(rec, req(http.MethodPost, "/", `{"body":"hola"}`, claims(), channelID(uuid.NewString())))
	if rec.Code != http.StatusCreated {
		t.Fatalf("code = %d, want 201", rec.Code)
	}
	if f.gotPostIn.AuthorSub != "user-1" || f.gotPostIn.ParentID != nil {
		t.Errorf("PostMessageInput mal mapeado: %+v", f.gotPostIn)
	}
}

func TestPostMessageCallType(t *testing.T) {
	// Un mensaje type=call con cuerpo vacío llega al servicio (que admite el vacío);
	// el handler propaga el type sin tocarlo.
	f := &fakeService{message: domain.Message{Type: domain.MessageTypeCall}}
	rec := httptest.NewRecorder()
	newHandlers(f).PostMessage(rec, req(http.MethodPost, "/", `{"type":"call"}`, claims(), channelID(uuid.NewString())))
	if rec.Code != http.StatusCreated {
		t.Fatalf("code = %d, want 201, body=%s", rec.Code, rec.Body.String())
	}
	if f.gotPostIn.Type != domain.MessageTypeCall {
		t.Errorf("Type = %q, want call", f.gotPostIn.Type)
	}
}

func TestPostMessageEmptyBody(t *testing.T) {
	f := &fakeService{messageErr: domain.ErrEmptyMessage}
	rec := httptest.NewRecorder()
	newHandlers(f).PostMessage(rec, req(http.MethodPost, "/", `{"body":""}`, claims(), channelID(uuid.NewString())))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestPostMessageBadFileID(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeService{}).PostMessage(rec, req(http.MethodPost, "/", `{"body":"x","file_ids":["nope"]}`, claims(), channelID(uuid.NewString())))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

// --- Ola 12: prioridad + acuse + programados ---------------------------------

func TestPostMessagePriorityAndAck(t *testing.T) {
	f := &fakeService{message: domain.Message{Body: "urge"}}
	rec := httptest.NewRecorder()
	newHandlers(f).PostMessage(rec, req(http.MethodPost, "/",
		`{"body":"urge","priority":"urgent","requires_ack":true}`, claims(), channelID(uuid.NewString())))
	if rec.Code != http.StatusCreated {
		t.Fatalf("code = %d, want 201, body=%s", rec.Code, rec.Body.String())
	}
	if f.gotPostIn.Priority != "urgent" || !f.gotPostIn.RequiresAck {
		t.Errorf("priority/ack mal mapeados: %+v", f.gotPostIn)
	}
}

func TestPostMessageScheduledBranch(t *testing.T) {
	// send_at presente => el handler programa (PostScheduled), no publica ya.
	sendAt := time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)
	f := &fakeService{scheduled: domain.ScheduledMessage{Body: "luego"}}
	rec := httptest.NewRecorder()
	newHandlers(f).PostMessage(rec, req(http.MethodPost, "/",
		`{"body":"luego","send_at":"`+sendAt+`"}`, claims(), channelID(uuid.NewString())))
	if rec.Code != http.StatusCreated {
		t.Fatalf("code = %d, want 201, body=%s", rec.Code, rec.Body.String())
	}
	if f.gotSchedIn.Body != "luego" || f.gotSchedIn.SendAt.IsZero() {
		t.Errorf("ScheduleMessageInput mal mapeado: %+v", f.gotSchedIn)
	}
}

func TestPostMessageScheduledBadSendAt(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeService{}).PostMessage(rec, req(http.MethodPost, "/",
		`{"body":"x","send_at":"nope"}`, claims(), channelID(uuid.NewString())))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestAckMessageOK(t *testing.T) {
	f := &fakeService{ackCount: 3}
	rec := httptest.NewRecorder()
	newHandlers(f).AckMessage(rec, req(http.MethodPost, "/", "", claims(), messageID(uuid.NewString())))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if got := decode(t, rec)["count"]; got != float64(3) {
		t.Errorf("count = %v, want 3", got)
	}
}

func TestListScheduledOK(t *testing.T) {
	f := &fakeService{scheduledList: []domain.ScheduledMessage{{Body: "prog"}}}
	rec := httptest.NewRecorder()
	newHandlers(f).ListScheduled(rec, req(http.MethodGet, "/", "", claims(), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if _, ok := decode(t, rec)["scheduled"]; !ok {
		t.Error("falta clave scheduled")
	}
}

func TestUpdateScheduledNothing(t *testing.T) {
	rec := httptest.NewRecorder()
	params := map[string]string{"scheduledID": uuid.NewString()}
	newHandlers(&fakeService{}).UpdateScheduled(rec, req(http.MethodPatch, "/", `{}`, claims(), params))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestCancelScheduledOK(t *testing.T) {
	f := &fakeService{}
	rec := httptest.NewRecorder()
	params := map[string]string{"scheduledID": uuid.NewString()}
	newHandlers(f).CancelScheduled(rec, req(http.MethodDelete, "/", "", claims(), params))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("code = %d, want 204", rec.Code)
	}
}

func TestPostReplyOK(t *testing.T) {
	chID := uuid.New()
	f := &fakeService{
		rootMsg: domain.Message{ChannelID: chID},
		message: domain.Message{Body: "reply"},
	}
	rec := httptest.NewRecorder()
	root := uuid.New()
	newHandlers(f).PostReply(rec, req(http.MethodPost, "/", `{"body":"reply"}`, claims(), messageID(root.String())))
	if rec.Code != http.StatusCreated {
		t.Fatalf("code = %d, want 201, body=%s", rec.Code, rec.Body.String())
	}
	// El parent es el mensaje raíz y el canal se hereda de él.
	if f.gotPostIn.ParentID == nil || *f.gotPostIn.ParentID != root {
		t.Errorf("ParentID = %v, want %v", f.gotPostIn.ParentID, root)
	}
	if f.gotPostIn.ChannelID != chID {
		t.Errorf("ChannelID = %v, want %v (heredado)", f.gotPostIn.ChannelID, chID)
	}
}

func TestPostReplyRootNotFound(t *testing.T) {
	f := &fakeService{threadErr: domain.ErrNotFound}
	rec := httptest.NewRecorder()
	newHandlers(f).PostReply(rec, req(http.MethodPost, "/", `{"body":"x"}`, claims(), messageID(uuid.NewString())))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d, want 404", rec.Code)
	}
}

func TestGetThreadOK(t *testing.T) {
	f := &fakeService{rootMsg: domain.Message{Body: "root"}, replies: []domain.Message{{Body: "r1"}}}
	rec := httptest.NewRecorder()
	newHandlers(f).GetThread(rec, req(http.MethodGet, "/", "", claims(), messageID(uuid.NewString())))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	m := decode(t, rec)
	if _, ok := m["root"]; !ok {
		t.Error("falta clave root")
	}
	if _, ok := m["replies"]; !ok {
		t.Error("falta clave replies")
	}
}

func TestGetThreadIncludesFollowing(t *testing.T) {
	f := &fakeService{rootMsg: domain.Message{Body: "root"}, following: true}
	rec := httptest.NewRecorder()
	newHandlers(f).GetThread(rec, req(http.MethodGet, "/", "", claims(), messageID(uuid.NewString())))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if following, _ := decode(t, rec)["following"].(bool); !following {
		t.Error("GetThread debe devolver following=true")
	}
}

func TestFollowThreadOK(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeService{}).FollowThread(rec, req(http.MethodPost, "/", "", claims(), messageID(uuid.NewString())))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("code = %d, want 204", rec.Code)
	}
}

func TestUnfollowThreadNotFound(t *testing.T) {
	f := &fakeService{err: domain.ErrNotFound}
	rec := httptest.NewRecorder()
	newHandlers(f).UnfollowThread(rec, req(http.MethodDelete, "/", "", claims(), messageID(uuid.NewString())))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d, want 404 (no seguía)", rec.Code)
	}
}

func TestListMyThreadsOK(t *testing.T) {
	f := &fakeService{threads: []domain.ThreadSummary{{UnreadReplies: 2}}}
	rec := httptest.NewRecorder()
	newHandlers(f).ListMyThreads(rec, req(http.MethodGet, "/?limit=10", "", claims(), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if _, ok := decode(t, rec)["threads"]; !ok {
		t.Error("falta clave threads")
	}
}

func TestListMyThreadsBadBefore(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeService{}).ListMyThreads(rec, req(http.MethodGet, "/?before=nope", "", claims(), nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestEditMessageOK(t *testing.T) {
	f := &fakeService{message: domain.Message{Body: "editado"}}
	rec := httptest.NewRecorder()
	newHandlers(f).EditMessage(rec, req(http.MethodPatch, "/", `{"body":"editado"}`, claims(), messageID(uuid.NewString())))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
}

func TestEditMessageForbidden(t *testing.T) {
	f := &fakeService{messageErr: domain.ErrForbidden}
	rec := httptest.NewRecorder()
	newHandlers(f).EditMessage(rec, req(http.MethodPatch, "/", `{"body":"x"}`, claims(), messageID(uuid.NewString())))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("code = %d, want 403", rec.Code)
	}
}

func TestDeleteMessageOK(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeService{}).DeleteMessage(rec, req(http.MethodDelete, "/", "", claims(), messageID(uuid.NewString())))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("code = %d, want 204", rec.Code)
	}
}

// --- Reacciones --------------------------------------------------------------

func TestAddReactionOK(t *testing.T) {
	f := &fakeService{}
	rec := httptest.NewRecorder()
	params := map[string]string{"messageID": uuid.NewString(), "emoji": " thumbsup "}
	newHandlers(f).AddReaction(rec, req(http.MethodPut, "/", "", claims(), params))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("code = %d, want 204", rec.Code)
	}
	if f.gotEmoji != "thumbsup" {
		t.Errorf("emoji = %q, want thumbsup (recortado)", f.gotEmoji)
	}
}

func TestRemoveReactionOK(t *testing.T) {
	f := &fakeService{}
	rec := httptest.NewRecorder()
	params := map[string]string{"messageID": uuid.NewString(), "emoji": "heart"}
	newHandlers(f).RemoveReaction(rec, req(http.MethodDelete, "/", "", claims(), params))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("code = %d, want 204", rec.Code)
	}
}

func TestReactionBadMessageID(t *testing.T) {
	rec := httptest.NewRecorder()
	params := map[string]string{"messageID": "nope", "emoji": "x"}
	newHandlers(&fakeService{}).AddReaction(rec, req(http.MethodPut, "/", "", claims(), params))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

// --- Tarjetas: encuestas y tareas --------------------------------------------

func TestSetCardVoteDevuelveElRecuento(t *testing.T) {
	f := &fakeService{cardVotes: []domain.CardVote{{Value: "1", Count: 2, Subs: []string{"a", "b"}}}}
	rec := httptest.NewRecorder()
	params := map[string]string{"messageID": uuid.NewString(), "value": " 1 "}
	newHandlers(f).SetCardVote(rec, req(http.MethodPut, "/", "", claims(), params))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if f.cardValue != "1" {
		t.Errorf("value = %q, want 1 (recortado)", f.cardValue)
	}
	var got struct {
		Votes []domain.CardVote `json:"votes"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("respuesta no es JSON: %v", err)
	}
	if len(got.Votes) != 1 || got.Votes[0].Count != 2 {
		t.Fatalf("votes = %+v, quiero el recuento agregado", got.Votes)
	}
}

func TestClearCardVoteSinVotosDevuelveListaVacia(t *testing.T) {
	// Nunca `null`: el cliente sustituye el recuento entero por lo que llegue, y un
	// null obligaría a cada consumidor a defenderse.
	f := &fakeService{}
	rec := httptest.NewRecorder()
	params := map[string]string{"messageID": uuid.NewString(), "value": "done"}
	newHandlers(f).ClearCardVote(rec, req(http.MethodDelete, "/", "", claims(), params))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"votes":[]`) {
		t.Fatalf("cuerpo = %s, quiero votes:[]", rec.Body.String())
	}
}

func TestCardVoteBadMessageID(t *testing.T) {
	rec := httptest.NewRecorder()
	params := map[string]string{"messageID": "nope", "value": "0"}
	newHandlers(&fakeService{}).SetCardVote(rec, req(http.MethodPut, "/", "", claims(), params))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

// --- Categorías (Ola 11) -----------------------------------------------------

func categoryID(id string) map[string]string { return map[string]string{"categoryID": id} }

func TestListCategoriesOK(t *testing.T) {
	f := &fakeService{categories: []domain.Category{{Name: "Proyectos"}}}
	rec := httptest.NewRecorder()
	newHandlers(f).ListCategories(rec, req(http.MethodGet, "/", "", claims(), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if _, ok := decode(t, rec)["categories"]; !ok {
		t.Error("falta clave categories")
	}
}

func TestCreateCategoryOK(t *testing.T) {
	f := &fakeService{category: domain.Category{Name: "Proyectos"}}
	rec := httptest.NewRecorder()
	newHandlers(f).CreateCategory(rec, req(http.MethodPost, "/", `{"name":"Proyectos"}`, claims(), nil))
	if rec.Code != http.StatusCreated {
		t.Fatalf("code = %d, want 201", rec.Code)
	}
	if f.gotCatName != "Proyectos" {
		t.Errorf("name = %q, want Proyectos", f.gotCatName)
	}
}

func TestCreateCategoryInvalidName(t *testing.T) {
	f := &fakeService{categoryErr: domain.ErrInvalidName}
	rec := httptest.NewRecorder()
	newHandlers(f).CreateCategory(rec, req(http.MethodPost, "/", `{"name":""}`, claims(), nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestUpdateCategoryNothingToUpdate(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeService{}).UpdateCategory(rec, req(http.MethodPatch, "/", `{}`, claims(), categoryID(uuid.NewString())))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400 (nada que actualizar)", rec.Code)
	}
}

func TestUpdateCategoryCollapseOK(t *testing.T) {
	f := &fakeService{category: domain.Category{Name: "Proyectos", Collapsed: true}}
	rec := httptest.NewRecorder()
	newHandlers(f).UpdateCategory(rec, req(http.MethodPatch, "/", `{"collapsed":true}`, claims(), categoryID(uuid.NewString())))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
}

func TestUpdateCategoryNotFound(t *testing.T) {
	f := &fakeService{categoryErr: domain.ErrNotFound}
	rec := httptest.NewRecorder()
	newHandlers(f).UpdateCategory(rec, req(http.MethodPatch, "/", `{"name":"x"}`, claims(), categoryID(uuid.NewString())))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d, want 404", rec.Code)
	}
}

func TestDeleteCategoryOK(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeService{}).DeleteCategory(rec, req(http.MethodDelete, "/", "", claims(), categoryID(uuid.NewString())))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("code = %d, want 204", rec.Code)
	}
}

// category_id="" mueve el canal a la categoría por defecto (catID nil llega al servicio).
func TestSetChannelCategoryDefaultOK(t *testing.T) {
	f := &fakeService{channel: domain.Channel{Name: "c"}}
	rec := httptest.NewRecorder()
	newHandlers(f).SetChannelCategory(rec, req(http.MethodPost, "/", `{"category_id":""}`, claims(), channelID(uuid.NewString())))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if f.gotCatID != nil {
		t.Errorf("category_id vacío debe llegar como nil, got %v", f.gotCatID)
	}
}

func TestSetChannelCategoryAssignOK(t *testing.T) {
	f := &fakeService{channel: domain.Channel{Name: "c"}}
	cid := uuid.NewString()
	rec := httptest.NewRecorder()
	newHandlers(f).SetChannelCategory(rec, req(http.MethodPost, "/", `{"category_id":"`+cid+`","sort_order":3}`, claims(), channelID(uuid.NewString())))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if f.gotCatID == nil || f.gotCatID.String() != cid {
		t.Errorf("category_id = %v, want %s", f.gotCatID, cid)
	}
}

func TestSetChannelCategoryBadUUID(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeService{}).SetChannelCategory(rec, req(http.MethodPost, "/", `{"category_id":"nope"}`, claims(), channelID(uuid.NewString())))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

// --- Miembros de canal (roster) ----------------------------------------------

func TestChannelMembersRoutes(t *testing.T) {
	cid := channelID(uuid.NewString())
	withSub := map[string]string{"channelID": cid["channelID"], "sub": "bob"}

	f := &fakeService{members: []domain.Member{{Subject: "bob", Role: domain.RoleMember}}}
	rec := httptest.NewRecorder()
	newHandlers(f).ListChannelMembers(rec, req(http.MethodGet, "/", "", claims(), cid))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET code = %d, want 200", rec.Code)
	}
	if _, ok := decode(t, rec)["members"]; !ok {
		t.Error("falta clave members")
	}

	rec = httptest.NewRecorder()
	newHandlers(f).AddChannelMember(rec, req(http.MethodPost, "/", `{"subject":"bob","name":"Bob"}`, claims(), cid))
	if rec.Code != http.StatusCreated || f.gotSub != "bob" {
		t.Fatalf("POST code = %d sub = %q, want 201 y bob", rec.Code, f.gotSub)
	}

	rec = httptest.NewRecorder()
	newHandlers(f).SetChannelMemberRole(rec, req(http.MethodPatch, "/", `{"role":"admin"}`, claims(), withSub))
	if rec.Code != http.StatusOK || f.gotRole != domain.RoleAdmin {
		t.Fatalf("PATCH code = %d rol = %q, want 200 y admin", rec.Code, f.gotRole)
	}

	rec = httptest.NewRecorder()
	newHandlers(f).RemoveChannelMember(rec, req(http.MethodDelete, "/", "", claims(), withSub))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE code = %d, want 204", rec.Code)
	}
}

// TestAddChannelMemberNotInTeam: el sujeto ajeno al equipo da 403 con mensaje propio
// (ErrNotMember), distinguible del 403 por falta de permiso del actor.
func TestAddChannelMemberNotInTeam(t *testing.T) {
	f := &fakeService{memberErr: domain.ErrNotMember}
	rec := httptest.NewRecorder()
	newHandlers(f).AddChannelMember(rec, req(http.MethodPost, "/", `{"subject":"ext"}`, claims(), channelID(uuid.NewString())))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("code = %d, want 403", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "not a member of the channel's team") {
		t.Errorf("cuerpo = %q, quiero el mensaje «not a member of the channel's team»", rec.Body.String())
	}
}

func TestAddChannelMemberSinSubject(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeService{}).AddChannelMember(rec, req(http.MethodPost, "/", `{"subject":"  "}`, claims(), channelID(uuid.NewString())))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

// --- Helpers puros -----------------------------------------------------------

func TestToAPIErrorMapping(t *testing.T) {
	if toAPIError(nil) != nil {
		t.Fatal("toAPIError(nil) debe ser nil")
	}
	cases := map[error]int{
		domain.ErrNotFound:       http.StatusNotFound,
		domain.ErrConflict:       http.StatusConflict,
		domain.ErrInvalidName:    http.StatusBadRequest,
		domain.ErrInvalidInput:   http.StatusBadRequest,
		domain.ErrEmptyMessage:   http.StatusBadRequest,
		domain.ErrForbidden:      http.StatusForbidden,
		domain.ErrNotMember:      http.StatusForbidden,
		context.DeadlineExceeded: http.StatusInternalServerError,
	}
	for in, want := range cases {
		if got := httpx.AsError(toAPIError(in)).Status; got != want {
			t.Errorf("toAPIError(%v) status = %d, want %d", in, got, want)
		}
	}
}

func TestParseUUIDs(t *testing.T) {
	out, err := parseUUIDs([]string{uuid.NewString(), " ", uuid.NewString()})
	if err != nil {
		t.Fatalf("parseUUIDs: %v", err)
	}
	if len(out) != 2 {
		t.Errorf("len = %d, want 2 (vacío descartado)", len(out))
	}
	if _, err := parseUUIDs([]string{"nope"}); err == nil {
		t.Error("parseUUIDs debe fallar con basura")
	}
	if out, _ := parseUUIDs(nil); out != nil {
		t.Error("parseUUIDs(nil) debe ser nil")
	}
}

// --- Emoji personalizados (Ola 14) -------------------------------------------

func TestListCustomEmojiOK(t *testing.T) {
	f := &fakeService{customEmoji: []domain.CustomEmoji{{Name: "party_parrot"}}}
	rec := httptest.NewRecorder()
	newHandlers(f).ListCustomEmoji(rec, req(http.MethodGet, "/", "", claims(), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if _, ok := decode(t, rec)["emoji"]; !ok {
		t.Error("falta clave emoji")
	}
}

func TestCreateCustomEmojiOK(t *testing.T) {
	fileID := uuid.New()
	f := &fakeService{customEmojiOne: domain.CustomEmoji{Name: "shipit", FileID: fileID}}
	rec := httptest.NewRecorder()
	body := `{"name":"shipit","file_id":"` + fileID.String() + `"}`
	newHandlers(f).CreateCustomEmoji(rec, req(http.MethodPost, "/", body, claims(), nil))
	if rec.Code != http.StatusCreated {
		t.Fatalf("code = %d, want 201 (%s)", rec.Code, rec.Body.String())
	}
	if f.gotEmojiName != "shipit" || f.gotEmojiFileID != fileID {
		t.Errorf("recibido (%q,%v), want (shipit,%v)", f.gotEmojiName, f.gotEmojiFileID, fileID)
	}
}

func TestCreateCustomEmojiBadFileID(t *testing.T) {
	f := &fakeService{}
	rec := httptest.NewRecorder()
	newHandlers(f).CreateCustomEmoji(rec, req(http.MethodPost, "/", `{"name":"x","file_id":"nope"}`, claims(), nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestCreateCustomEmojiInvalidName(t *testing.T) {
	f := &fakeService{customEmojiErr: domain.ErrInvalidName}
	rec := httptest.NewRecorder()
	body := `{"name":"NoMayus","file_id":"` + uuid.New().String() + `"}`
	newHandlers(f).CreateCustomEmoji(rec, req(http.MethodPost, "/", body, claims(), nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestDeleteCustomEmojiOK(t *testing.T) {
	f := &fakeService{}
	rec := httptest.NewRecorder()
	r := req(http.MethodDelete, "/", "", claims(), map[string]string{"name": "shipit"})
	newHandlers(f).DeleteCustomEmoji(rec, r)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("code = %d, want 204", rec.Code)
	}
	if f.gotEmojiName != "shipit" {
		t.Errorf("gotEmojiName = %q, want shipit", f.gotEmojiName)
	}
}

func TestDeleteCustomEmojiForbidden(t *testing.T) {
	f := &fakeService{err: domain.ErrForbidden}
	rec := httptest.NewRecorder()
	r := req(http.MethodDelete, "/", "", claims(), map[string]string{"name": "shipit"})
	newHandlers(f).DeleteCustomEmoji(rec, r)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("code = %d, want 403", rec.Code)
	}
}

func TestCallerFallsBackToEmail(t *testing.T) {
	r := req(http.MethodGet, "/", "", &authn.Claims{Subject: "s", Email: "e@x.com"}, nil)
	sub, name := caller(r)
	if sub != "s" || name != "e@x.com" {
		t.Errorf("caller = (%q,%q), want (s, e@x.com)", sub, name)
	}
	// Sin claims: vacío.
	r2 := req(http.MethodGet, "/", "", nil, nil)
	if s, n := caller(r2); s != "" || n != "" {
		t.Errorf("caller sin claims = (%q,%q), want vacío", s, n)
	}
}

// El registro del aparato de iOS propaga el token y valida su forma en el servicio.
func TestRegisterAPNSDevice(t *testing.T) {
	f := &fakeService{}
	rec := httptest.NewRecorder()
	newHandlers(f).RegisterAPNSDevice(rec, req(http.MethodPost, "/",
		`{"token":" abcdef0123456789abcdef0123456789 "}`, claims(), nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("código = %d, quiero 204 (%s)", rec.Code, rec.Body.String())
	}
	if f.apnsToken != "abcdef0123456789abcdef0123456789" {
		t.Fatalf("token = %q (debe llegar sin espacios)", f.apnsToken)
	}
}
