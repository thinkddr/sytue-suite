// SPDX-License-Identifier: Apache-2.0
// Package matrix provisions access metadata in the configured Synapse server.
// It never owns encryption keys, decrypts content or creates crypto devices.
package matrix

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

type Config struct {
	InternalURL, HomeserverURL, ServerName, SSOProviderID, AdminToken string
}

type Client struct {
	cfg          Config
	http         *http.Client
	users        sync.Map
	identityGate IdentityGate
}

func New(cfg Config) *Client {
	cfg.InternalURL = strings.TrimRight(cfg.InternalURL, "/")
	return &Client{cfg: cfg, http: &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

func (c *Client) configured() bool {
	if c == nil || c.cfg.AdminToken == "" || c.cfg.ServerName == "" || c.cfg.SSOProviderID == "" {
		return false
	}
	internal, err := url.Parse(c.cfg.InternalURL)
	if err != nil || internal.Host == "" || (internal.Scheme != "https" && internal.Scheme != "http") || internal.User != nil || internal.RawQuery != "" || internal.Fragment != "" {
		return false
	}
	public, err := url.Parse(c.cfg.HomeserverURL)
	return err == nil && public.Scheme == "https" && public.Host == c.cfg.ServerName && public.User == nil && public.RawQuery == "" && public.Fragment == "" && (public.Path == "" || public.Path == "/")
}

func (c *Client) UserID(subject string) (string, error) {
	id, err := uuid.Parse(subject)
	if err != nil || id == uuid.Nil || id.String() != subject || !c.configured() {
		return "", domain.ErrMatrixUnavailable
	}
	return "@sytue_" + subject + ":" + c.cfg.ServerName, nil
}

func (c *Client) controller() string {
	return "@" + domain.MatrixControllerLocalpart + ":" + c.cfg.ServerName
}

func (c *Client) call(ctx context.Context, method, path string, body, out any) error {
	if !c.configured() {
		return domain.ErrMatrixUnavailable
	}
	var reader io.Reader = http.NoBody
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return domain.ErrMatrixUnavailable
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.cfg.InternalURL+path, reader)
	if err != nil {
		return domain.ErrMatrixUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.AdminToken)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return domain.ErrMatrixUnavailable
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return domain.ErrNotFound
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return domain.ErrMatrixUnavailable
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 8<<20)).Decode(out); err != nil {
		return domain.ErrMatrixUnavailable
	}
	return nil
}

func (c *Client) Ready(ctx context.Context) error {
	var out struct {
		UserID string `json:"user_id"`
	}
	if err := c.call(ctx, http.MethodGet, "/_matrix/client/v3/account/whoami", nil, &out); err != nil {
		return err
	}
	if out.UserID != c.controller() {
		return domain.ErrMatrixUnavailable
	}
	var role struct {
		Admin bool `json:"admin"`
	}
	if err := c.call(ctx, http.MethodGet, "/_synapse/admin/v1/users/"+url.PathEscape(c.controller())+"/admin", nil, &role); err != nil {
		return err
	}
	if !role.Admin {
		return domain.ErrMatrixUnavailable
	}
	return nil
}

func (c *Client) SetAccountAccess(ctx context.Context, subject, status string) error {
	mxid, err := c.UserID(subject)
	if err != nil {
		return domain.ErrInvalidInput
	}
	if status != "active" && status != "suspended" && status != "deleted" {
		return domain.ErrInvalidInput
	}
	err = c.call(ctx, http.MethodPut, "/_synapse/admin/v1/suspend/"+url.PathEscape(mxid), map[string]bool{"suspend": status != "active"}, nil)
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	}
	return err
}

type IdentityGate interface {
	WithMatrixIdentity(context.Context, string, func(context.Context) error) error
}

func (c *Client) SetIdentityGate(g IdentityGate) { c.identityGate = g }

func (c *Client) EnsureUser(ctx context.Context, subject string) (string, error) {
	mxid, err := c.UserID(subject)
	if err != nil {
		return "", err
	}
	if c.identityGate == nil {
		return "", domain.ErrMatrixUnavailable
	}
	err = c.identityGate.WithMatrixIdentity(ctx, subject, func(ctx context.Context) error {
		if _, ok := c.users.Load(mxid); ok {
			return nil
		}
		body := map[string]any{"external_ids": []map[string]string{{"auth_provider": c.cfg.SSOProviderID, "external_id": subject}}, "logout_devices": false}
		if err := c.call(ctx, http.MethodPut, "/_synapse/admin/v2/users/"+url.PathEscape(mxid), body, nil); err != nil {
			return err
		}
		c.users.Store(mxid, true)
		return nil
	})
	return mxid, err
}

type stateEvent struct {
	Type     string          `json:"type"`
	StateKey string          `json:"state_key"`
	Sender   string          `json:"sender"`
	Content  json.RawMessage `json:"content"`
}

type roomPolicy struct {
	Members                       map[string]string
	Encryption, JoinRule, History string
	Controller                    string
	Version                       string
	Powers                        json.RawMessage
}

//nolint:gocyclo // parsing room state requires evaluating all event types according to the Matrix specification
func (c *Client) roomState(ctx context.Context, room string) (roomPolicy, error) {
	var response struct {
		State []stateEvent `json:"state"`
	}
	if err := c.call(ctx, http.MethodGet, "/_synapse/admin/v1/rooms/"+url.PathEscape(room)+"/state", nil, &response); err != nil {
		return roomPolicy{}, err
	}
	p := roomPolicy{Members: map[string]string{}}
	for _, event := range response.State {
		var fields map[string]any
		if json.Unmarshal(event.Content, &fields) != nil {
			return roomPolicy{}, domain.ErrMatrixUnavailable
		}
		switch event.Type {
		case "m.room.power_levels":
			p.Powers = event.Content
		case "m.room.encryption":
			p.Encryption, _ = fields["algorithm"].(string)
		case "m.room.join_rules":
			p.JoinRule, _ = fields["join_rule"].(string)
		case "m.room.history_visibility":
			p.History, _ = fields["history_visibility"].(string)
		case "m.room.create":
			p.Controller = event.Sender
			p.Version, _ = fields["room_version"].(string)
			federation, ok := fields["m.federate"].(bool)
			if !ok || federation {
				return roomPolicy{}, domain.ErrMatrixUnavailable
			}
			if creators, exists := fields["additional_creators"]; exists {
				list, ok := creators.([]any)
				if !ok || len(list) != 0 {
					return roomPolicy{}, domain.ErrMatrixUnavailable
				}
			}
		case "m.room.member":
			p.Members[event.StateKey], _ = fields["membership"].(string)
		}
	}
	if p.Encryption != domain.MatrixEncryption || p.JoinRule != "invite" || p.History != "joined" || p.Controller != c.controller() || (p.Version != "11" && p.Version != "12") {
		return roomPolicy{}, domain.ErrMatrixUnavailable
	}
	return p, nil
}

// Room v12 gives the creator implicit power and forbids listing it in users.
// Existing v11 rooms retain their explicit controller grant during updates.
func (c *Client) powerLevels(archived bool, version ...string) map[string]any {
	messageLevel := 0
	if archived {
		messageLevel = 100
	}
	users := map[string]int{}
	if len(version) > 0 && version[0] == "11" {
		users[c.controller()] = 100
	}
	return map[string]any{
		"users": users, "users_default": 0,
		"events_default": 100, "state_default": 100, "invite": 100, "kick": 100, "ban": 100, "redact": 100,
		"events": map[string]int{"m.room.encrypted": messageLevel, "m.room.message": 100},
	}
}

// EnsureRoom uses a deterministic alias to recover a completed remote create
// after a local transaction failure. Existing rooms must belong to our controller.
func (c *Client) EnsureRoom(ctx context.Context, tenant, channel, room string, subjects []string, archived bool) (string, error) {
	if !tenancy.ValidSlug(tenant) {
		return "", domain.ErrInvalidInput
	}
	id, err := uuid.Parse(channel)
	if err != nil || id == uuid.Nil {
		return "", domain.ErrInvalidInput
	}
	return c.ensureRoom(ctx, "rooms_"+tenant+"_"+id.String(), room, subjects, archived)
}

func (c *Client) EnsureDraftRoom(ctx context.Context, tenant, subject, room string) (string, error) {
	if !tenancy.ValidSlug(tenant) {
		return "", domain.ErrInvalidInput
	}
	if _, err := c.UserID(subject); err != nil {
		return "", err
	}
	return c.ensureRoom(ctx, "drafts_"+tenant+"_"+subject, room, []string{subject}, false)
}

// EnsureStorageRoom provisions only routing and membership. The controller is
// excluded from key sharing by the clients, just as in encrypted Rooms channels.
func (c *Client) EnsureStorageRoom(ctx context.Context, tenant, subject, product, room string) (string, error) {
	if !domain.StorageProduct(product) {
		return "", domain.ErrInvalidInput
	}
	if _, err := c.UserID(subject); err != nil {
		return "", err
	}
	return c.ensureRoom(ctx, "storage_"+product+"_"+tenant+"_"+subject, room, []string{subject}, false)
}

func (c *Client) ensureRoom(ctx context.Context, local, room string, subjects []string, archived bool) (string, error) {
	if room == "" {
		var resolved struct {
			RoomID string `json:"room_id"`
		}
		err := c.call(ctx, http.MethodGet, "/_matrix/client/v3/directory/room/"+url.PathEscape("#"+local+":"+c.cfg.ServerName), nil, &resolved)
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			return "", err
		}
		room = resolved.RoomID
		if room == "" {
			body := map[string]any{
				"room_alias_name": local, "visibility": "private", "preset": "private_chat", "room_version": "12",
				"creation_content":             map[string]bool{"m.federate": false},
				"power_level_content_override": c.powerLevels(archived),
				"initial_state": []map[string]any{
					{"type": "m.room.encryption", "state_key": "", "content": map[string]any{"algorithm": domain.MatrixEncryption}},
					{"type": "m.room.history_visibility", "state_key": "", "content": map[string]string{"history_visibility": "joined"}},
					{"type": "m.room.guest_access", "state_key": "", "content": map[string]string{"guest_access": "forbidden"}},
				},
			}
			if err := c.call(ctx, http.MethodPost, "/_matrix/client/v3/createRoom", body, &resolved); err != nil {
				return "", err
			}
			room = resolved.RoomID
		}
	}
	if !modernRoomID.MatchString(room) && (!strings.HasPrefix(room, "!") || !strings.HasSuffix(room, ":"+c.cfg.ServerName) || strings.ContainsAny(room, "\r\n\x00")) {
		return "", domain.ErrMatrixUnavailable
	}
	if err := c.Reconcile(ctx, room, subjects, archived, true); err != nil {
		return "", err
	}
	return room, nil
}

// Reconcile revokes undesired members before granting access. During a local
// revocation transaction invite=false prevents granting access before its commit.
var modernRoomID = regexp.MustCompile(`^![A-Za-z0-9_-]{43}$`)

//nolint:gocyclo // power levels verification checks all power level rules according to the Matrix spec
func (c *Client) validPowers(raw json.RawMessage, archived bool, version ...string) bool {
	var p struct {
		Users         map[string]int `json:"users"`
		UsersDefault  int            `json:"users_default"`
		EventsDefault int            `json:"events_default"`
		StateDefault  int            `json:"state_default"`
		Invite        int            `json:"invite"`
		Kick          int            `json:"kick"`
		Ban           int            `json:"ban"`
		Redact        int            `json:"redact"`
		Events        map[string]int `json:"events"`
	}
	if json.Unmarshal(raw, &p) != nil {
		return false
	}
	legacy := len(version) > 0 && version[0] == "11"
	if (!legacy && len(p.Users) != 0) || (legacy && (len(p.Users) != 1 || p.Users[c.controller()] != 100)) || p.UsersDefault != 0 || p.EventsDefault != 100 || p.StateDefault != 100 || p.Invite != 100 || p.Kick != 100 || p.Ban != 100 || p.Redact != 100 {
		return false
	}
	expected := 0
	if archived {
		expected = 100
	}
	level, ok := p.Events["m.room.encrypted"]
	if !ok || level != expected {
		return false
	}
	for event, level := range p.Events {
		if event != "m.room.encrypted" && level < 100 {
			return false
		}
	}
	return true
}

// The metadata controller must remain joined: Synapse cannot re-invite a
// departed private-room administrator. Clients exclude this identity from all
// encryption/verification recipients, including signed devices and key requests.
func (c *Client) withController(ctx context.Context, _ string, joined bool, fn func(context.Context) error) error {
	if !joined {
		return domain.ErrMatrixUnavailable
	}
	return fn(ctx)
}

func (c *Client) desiredMembers(ctx context.Context, subjects []string, provision bool) (map[string]bool, error) {
	desired := map[string]bool{}
	for _, subject := range subjects {
		if domain.IsBotAuthor(subject) {
			continue
		}
		mxid, err := c.UserID(subject)
		if err != nil {
			return nil, err
		}
		if provision {
			mxid, err = c.EnsureUser(ctx, subject)
			if err != nil {
				return nil, err
			}
		}
		desired[mxid] = true
	}
	return desired, nil
}

func (c *Client) memberChanges(p roomPolicy, desired map[string]bool, invite bool) (extras, missing []string) {
	for mxid, membership := range p.Members {
		if mxid != c.controller() && !desired[mxid] && (membership == "join" || membership == "invite" || membership == "knock") {
			extras = append(extras, mxid)
		}
	}
	if invite {
		for mxid := range desired {
			if p.Members[mxid] != "join" && p.Members[mxid] != "invite" {
				missing = append(missing, mxid)
			}
		}
	}
	sort.Strings(extras)
	sort.Strings(missing)
	return extras, missing
}

func (c *Client) applyMembers(ctx context.Context, room string, p roomPolicy, extras, missing []string) error {
	base := "/_matrix/client/v3/rooms/" + url.PathEscape(room)
	for _, mxid := range extras {
		if err := c.call(ctx, http.MethodPost, base+"/kick", map[string]string{"user_id": mxid}, nil); err != nil {
			return err
		}
	}
	for _, mxid := range missing {
		if p.Members[mxid] == "ban" {
			if err := c.call(ctx, http.MethodPost, base+"/unban", map[string]string{"user_id": mxid}, nil); err != nil {
				return err
			}
		}
		if err := c.call(ctx, http.MethodPost, base+"/invite", map[string]string{"user_id": mxid}, nil); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) Reconcile(ctx context.Context, room string, subjects []string, archived, invite bool) error {
	p, err := c.roomState(ctx, room)
	if err != nil {
		return err
	}
	desired, err := c.desiredMembers(ctx, subjects, invite)
	if err != nil {
		return err
	}
	extras, missing := c.memberChanges(p, desired, invite)
	policyOK := c.validPowers(p.Powers, archived, p.Version)
	controllerJoined := p.Members[c.controller()] == "join"
	if len(extras) == 0 && len(missing) == 0 && policyOK && controllerJoined {
		return nil
	}
	return c.withController(ctx, room, controllerJoined, func(ctx context.Context) error {
		if !policyOK {
			if err := c.call(ctx, http.MethodPut, "/_matrix/client/v3/rooms/"+url.PathEscape(room)+"/state/m.room.power_levels/", c.powerLevels(archived, p.Version), nil); err != nil {
				return err
			}
		}
		return c.applyMembers(ctx, room, p, extras, missing)
	})
}

func (c *Client) ValidateEvent(ctx context.Context, ref domain.MatrixEventRef, subject string) error {
	mxid, err := c.UserID(subject)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(ref.EventID, "$") || len(ref.EventID) > 1024 || strings.ContainsAny(ref.EventID, "\r\n\x00") {
		return domain.ErrInvalidInput
	}
	var response struct {
		Event struct {
			Type     string  `json:"type"`
			RoomID   string  `json:"room_id"`
			EventID  string  `json:"event_id"`
			Sender   string  `json:"sender"`
			StateKey *string `json:"state_key"`
			Content  struct {
				Algorithm string `json:"algorithm"`
			} `json:"content"`
		} `json:"event"`
	}
	if err := c.call(ctx, http.MethodGet, "/_synapse/admin/v1/rooms/"+url.PathEscape(ref.RoomID)+"/context/"+url.PathEscape(ref.EventID)+"?limit=0", nil, &response); err != nil {
		return err
	}
	event := response.Event
	if event.Type != "m.room.encrypted" || event.RoomID != ref.RoomID || event.EventID != ref.EventID || event.Sender != mxid || event.StateKey != nil || event.Content.Algorithm != domain.MatrixEncryption {
		return domain.ErrForbidden
	}
	return nil
}

func (c *Client) Redact(ctx context.Context, ref domain.MatrixEventRef) error {
	p, err := c.roomState(ctx, ref.RoomID)
	if err != nil {
		return err
	}
	return c.withController(ctx, ref.RoomID, p.Members[c.controller()] == "join", func(ctx context.Context) error {
		return c.call(ctx, http.MethodPut, "/_matrix/client/v3/rooms/"+url.PathEscape(ref.RoomID)+"/redact/"+url.PathEscape(ref.EventID)+"/"+uuid.NewString(), map[string]string{}, nil)
	})
}
