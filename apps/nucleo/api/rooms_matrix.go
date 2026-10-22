// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
)

// RoomsMatrixBootstrap is routing metadata for the active Sytue person/tenant.
// Credentials and encryption keys are never supplied by this endpoint.
type RoomsMatrixBootstrap struct {
	ControllerUserID string                   `json:"controller_user_id"`
	Actor            string                   `json:"actor"`
	Tenant           string                   `json:"tenant"`
	DraftRoomID      string                   `json:"draft_room_id"`
	DraftRoomIDs     []string                 `json:"draft_room_ids"`
	HomeserverURL    string                   `json:"homeserver_url"`
	SSOProviderID    string                   `json:"sso_provider_id"`
	MatrixUserID     string                   `json:"matrix_user_id"`
	Channels         []RoomsMatrixRoomBinding `json:"channels"`
	Encryption       string                   `json:"encryption"`
}
type RoomsMatrixRoomBinding struct {
	ChannelID string `json:"channel_id"`
	RoomID    string `json:"matrix_room_id"`
}

func (r *Rooms) MatrixBootstrap(ctx context.Context) (RoomsMatrixBootstrap, error) {
	return JSON[RoomsMatrixBootstrap](ctx, r.c, Request{Path: "/matrix/bootstrap"})
}

// MatrixNativeAuthorization keeps its PKCE verifier inside the process. The
// browser and incoming custom-scheme URL receive only state and an opaque ticket.
type MatrixNativeAuthorization struct {
	AuthorizeURL string
	State        string
	verifier     string
}
type MatrixNativeCallback struct {
	State  string `json:"state"`
	Ticket string `json:"ticket"`
	Error  string `json:"error,omitempty"`
}

var ErrMatrixSSO = errors.New("matrix: browser return does not match this Rooms sign-in")

// ForRooms permits only the first-party fixed browser bootstrap, including the
// exact state generated for this flow. Synapse is reached after the BFF cookie.
func (a MatrixNativeAuthorization) ForRooms(apex string) bool {
	if apex == "" {
		return false
	}
	expected, err := url.Parse(AppURL(apex, "rooms", "", "matrix/native/start"))
	if err != nil {
		return false
	}
	target, err := url.Parse(a.AuthorizeURL)
	if err != nil || target.Scheme != "https" || target.Host != expected.Host || target.Path != expected.Path || target.User != nil || target.Fragment != "" {
		return false
	}
	query := target.Query()
	return len(query) == 1 && len(query["state"]) == 1 && a.State != "" && subtle.ConstantTimeCompare([]byte(query.Get("state")), []byte(a.State)) == 1
}

func (a MatrixNativeAuthorization) Accepts(callback MatrixNativeCallback) bool {
	return a.State != "" && subtle.ConstantTimeCompare([]byte(a.State), []byte(callback.State)) == 1
}

// ParseMatrixNativeCallback never accepts a Matrix login token in a custom URL.
func ParseMatrixNativeCallback(u *url.URL) (MatrixNativeCallback, bool) {
	if u == nil || u.Scheme != Esquema("rooms") || u.Host != "matrix" || u.Path != "/callback" || u.User != nil || u.Fragment != "" || u.Opaque != "" {
		return MatrixNativeCallback{}, false
	}
	q := u.Query()
	for key, values := range q {
		if key != "state" && key != "ticket" && key != "error" || len(values) != 1 || len(values[0]) > 4096 {
			return MatrixNativeCallback{}, false
		}
	}
	if q.Get("state") == "" || (q.Get("ticket") == "" && q.Get("error") == "") {
		return MatrixNativeCallback{}, false
	}
	return MatrixNativeCallback{State: q.Get("state"), Ticket: q.Get("ticket"), Error: q.Get("error")}, true
}

func (r *Rooms) MatrixNativeStart(ctx context.Context) (MatrixNativeAuthorization, error) {
	verifierBytes := make([]byte, 32)
	if _, err := rand.Read(verifierBytes); err != nil {
		return MatrixNativeAuthorization{}, err
	}
	verifier := base64.RawURLEncoding.EncodeToString(verifierBytes)
	clear(verifierBytes)
	digest := sha256.Sum256([]byte(verifier))
	out, err := PostJSON[struct {
		AuthorizeURL string `json:"authorize_url"`
		State        string `json:"state"`
	}](ctx, r.c, "/matrix/native/start", map[string]string{"code_challenge": base64.RawURLEncoding.EncodeToString(digest[:]), "code_challenge_method": "S256"})
	if err != nil {
		return MatrixNativeAuthorization{}, err
	}
	u, err := url.Parse(out.AuthorizeURL)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || out.State == "" || len(out.State) > 4096 {
		return MatrixNativeAuthorization{}, ErrMatrixSSO
	}
	return MatrixNativeAuthorization{AuthorizeURL: out.AuthorizeURL, State: out.State, verifier: verifier}, nil
}

// MatrixNativeFinish returns a one-use Matrix login token over authenticated TLS;
// the caller must consume it immediately via m.login.token and pin Whoami.
func (r *Rooms) MatrixNativeFinish(ctx context.Context, authorization MatrixNativeAuthorization, callback MatrixNativeCallback) (string, error) {
	if !authorization.Accepts(callback) || authorization.verifier == "" {
		return "", ErrMatrixSSO
	}
	if callback.Error != "" {
		return "", ErrOAuthDenied
	}
	if callback.Ticket == "" {
		return "", ErrMatrixSSO
	}
	out, err := PostJSON[struct {
		LoginToken string `json:"login_token"`
	}](ctx, r.c, "/matrix/native/finish", map[string]string{"state": callback.State, "ticket": callback.Ticket, "code_verifier": authorization.verifier})
	if err != nil {
		return "", err
	}
	if out.LoginToken == "" || len(out.LoginToken) > 4096 {
		return "", ErrMatrixSSO
	}
	return out.LoginToken, nil
}

// RoomsMatrixEventRef is the only message payload accepted by the metadata API.
type RoomsMatrixEventRef struct {
	RoomID  string `json:"room_id"`
	EventID string `json:"event_id"`
}

func (r *Rooms) MatrixSend(ctx context.Context, channel string, reference RoomsMatrixEventRef) (RoomsMessage, error) {
	return PostJSON[RoomsMessage](ctx, r.c, "/channels/"+url.PathEscape(channel)+"/messages", map[string]any{"body": "", "matrix_event": reference})
}

func (r *Rooms) MatrixReply(ctx context.Context, parent string, reference RoomsMatrixEventRef) (RoomsMessage, error) {
	return PostJSON[RoomsMessage](ctx, r.c, "/messages/"+url.PathEscape(parent)+"/replies", map[string]any{"body": "", "matrix_event": reference})
}

func (r *Rooms) MatrixEdit(ctx context.Context, message string, reference RoomsMatrixEventRef) (RoomsMessage, error) {
	return conJSON[RoomsMessage](ctx, r.c, http.MethodPatch, "/messages/"+url.PathEscape(message), map[string]any{"body": "", "matrix_event": reference})
}
