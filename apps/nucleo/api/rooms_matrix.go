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

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// RoomsMatrixBootstrap is routing metadata for the active Sytue person/tenant.
// Credentials and encryption keys are never supplied by this endpoint.
type RoomsMatrixBootstrap struct {
	ControllerUserID string                   `json:"controller_user_id"`
	Actor            string                   `json:"actor"`
	Tenant           string                   `json:"tenant"`
	DraftRoomID      string                   `json:"draft_room_id"`
	DraftRoomIDs     []string                 `json:"draft_room_ids"`
	StorageProduct   string                   `json:"storage_product,omitempty"`
	StorageRoomID    string                   `json:"storage_room_id,omitempty"`
	StorageRoomIDs   []string                 `json:"storage_room_ids,omitempty"`
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

func (r *Rooms) MatrixStorageBootstrap(ctx context.Context, product string) (RoomsMatrixBootstrap, error) {
	return JSON[RoomsMatrixBootstrap](ctx, r.c, Request{Path: "/matrix/storage/bootstrap", Query: url.Values{"product": {product}}})
}

// MatrixNativeAuthorization keeps its PKCE verifier inside the process. The
// browser and incoming custom-scheme URL receive only state and an opaque ticket.
type MatrixNativeAuthorization struct {
	AuthorizeURL string
	State        string
	verifier     string
	product      string
}
type MatrixNativeCallback struct {
	product string
	State   string `json:"state"`
	Ticket  string `json:"ticket"`
	Error   string `json:"error,omitempty"`
}

var ErrMatrixSSO = errors.New("matrix: browser return does not match this app sign-in")

// Product is the destination bound before opening the system browser. Empty
// legacy flows retain the Rooms destination, never a caller-supplied scheme.
func (a MatrixNativeAuthorization) Product() string {
	if a.product == "" {
		return "rooms"
	}
	return a.product
}

func (a MatrixNativeAuthorization) ReturnScheme() string {
	if native := storage.NativeApp(a.Product()); native != "" {
		return Esquema(native)
	}
	return ""
}
func (a MatrixNativeAuthorization) ForRooms(apex string) bool { return a.ForProduct(apex, "rooms") }

// ForProduct permits only the fixed first-party browser broker and the product
// and state of this authorization. All products share the existing Rooms broker.
func (a MatrixNativeAuthorization) ForProduct(apex, product string) bool {
	if apex == "" || a.Product() != product || storage.NativeApp(product) == "" {
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
	product := callback.product
	if product == "" {
		product = "rooms"
	}
	return a.Product() == product && a.State != "" && subtle.ConstantTimeCompare([]byte(a.State), []byte(callback.State)) == 1
}

// ParseMatrixNativeCallback never accepts a Matrix login token in a custom URL.
func ParseMatrixNativeCallback(u *url.URL) (MatrixNativeCallback, bool) {
	return ParseMatrixNativeCallbackFor(u, "rooms")
}

func ParseMatrixNativeCallbackFor(u *url.URL, product string) (MatrixNativeCallback, bool) {
	native := storage.NativeApp(product)

	if native == "" || u == nil || u.Scheme != Esquema(native) || u.Host != "matrix" || u.Path != "/callback" || u.User != nil || u.Fragment != "" || u.Opaque != "" {
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
	return MatrixNativeCallback{product: product, State: q.Get("state"), Ticket: q.Get("ticket"), Error: q.Get("error")}, true
}

func (r *Rooms) MatrixNativeStart(ctx context.Context) (MatrixNativeAuthorization, error) {
	return r.MatrixNativeStartFor(ctx, "rooms")
}

func (r *Rooms) MatrixNativeStartFor(ctx context.Context, product string) (MatrixNativeAuthorization, error) {
	if storage.NativeApp(product) == "" {
		return MatrixNativeAuthorization{}, ErrMatrixSSO
	}

	verifierBytes := make([]byte, 32)
	if _, err := rand.Read(verifierBytes); err != nil {
		return MatrixNativeAuthorization{}, err
	}
	verifier := base64.RawURLEncoding.EncodeToString(verifierBytes)
	clear(verifierBytes)
	digest := sha256.Sum256([]byte(verifier))
	input := map[string]string{"code_challenge": base64.RawURLEncoding.EncodeToString(digest[:]), "code_challenge_method": "S256"}
	// Existing Rooms installs can still authenticate against the old broker.
	// Other products require the backend that binds a product in the grant.
	if product != "rooms" {
		input["product"] = product
	}
	out, err := PostJSON[struct {
		Product      string `json:"product"`
		AuthorizeURL string `json:"authorize_url"`
		State        string `json:"state"`
	}](ctx, r.c, "/matrix/native/start", input)
	if err != nil {
		return MatrixNativeAuthorization{}, err
	}
	if out.Product == "" {
		out.Product = "rooms"
	}
	if out.Product != product {
		return MatrixNativeAuthorization{}, ErrMatrixSSO
	}
	u, err := url.Parse(out.AuthorizeURL)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || out.State == "" || len(out.State) > 4096 {
		return MatrixNativeAuthorization{}, ErrMatrixSSO
	}
	return MatrixNativeAuthorization{AuthorizeURL: out.AuthorizeURL, State: out.State, verifier: verifier, product: product}, nil
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
type RoomsMatrixEventRef = storage.Reference

func (r *Rooms) MatrixSend(ctx context.Context, channel string, reference RoomsMatrixEventRef) (RoomsMessage, error) {
	return PostJSON[RoomsMessage](ctx, r.c, "/channels/"+url.PathEscape(channel)+"/messages", map[string]any{"body": "", "matrix_event": reference})
}

func (r *Rooms) MatrixReply(ctx context.Context, parent string, reference RoomsMatrixEventRef) (RoomsMessage, error) {
	return PostJSON[RoomsMessage](ctx, r.c, "/messages/"+url.PathEscape(parent)+"/replies", map[string]any{"body": "", "matrix_event": reference})
}

func (r *Rooms) MatrixEdit(ctx context.Context, message string, reference RoomsMatrixEventRef) (RoomsMessage, error) {
	return conJSON[RoomsMessage](ctx, r.c, http.MethodPatch, "/messages/"+url.PathEscape(message), map[string]any{"body": "", "matrix_event": reference})
}
