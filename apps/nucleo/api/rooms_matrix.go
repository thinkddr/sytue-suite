// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strconv"

	"git.sytue.eu/sytue/one/apps/nucleo/recoverywrap"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// RoomsMatrixBootstrap is routing metadata for the active Sytue person/tenant.
// Credentials and encryption keys are never supplied by this endpoint.
type RoomsMatrixBootstrap struct {
	ControllerUserID       string                   `json:"controller_user_id"`
	Actor                  string                   `json:"actor"`
	Tenant                 string                   `json:"tenant"`
	DraftRoomID            string                   `json:"draft_room_id"`
	DraftRoomIDs           []string                 `json:"draft_room_ids"`
	StorageIdentityVersion storage.IdentityVersion  `json:"storage_identity_version,omitempty"`
	LegacyStorage          *RoomsMatrixBootstrap    `json:"legacy_storage,omitempty"`
	StorageProduct         string                   `json:"storage_product,omitempty"`
	StorageRoomID          string                   `json:"storage_room_id,omitempty"`
	StorageRoomIDs         []string                 `json:"storage_room_ids,omitempty"`
	HomeserverURL          string                   `json:"homeserver_url"`
	SSOProviderID          string                   `json:"sso_provider_id"`
	MatrixUserID           string                   `json:"matrix_user_id"`
	Channels               []RoomsMatrixRoomBinding `json:"channels"`
	Encryption             string                   `json:"encryption"`
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

// MatrixStorageBootstrapForIdentity opts into a separate account without
// changing the default or treating a legacy catalog as an empty new one.
func (r *Rooms) MatrixStorageBootstrapForIdentity(ctx context.Context, product string, version storage.IdentityVersion) (RoomsMatrixBootstrap, error) {
	if !storage.Product(product) || !version.Valid() {
		return RoomsMatrixBootstrap{}, ErrMatrixSSO
	}
	result, err := JSON[RoomsMatrixBootstrap](ctx, r.c, Request{Path: "/matrix/storage/bootstrap", Query: url.Values{"product": {product}, "identity_version": {strconv.Itoa(int(version))}}})
	if err != nil {
		return RoomsMatrixBootstrap{}, err
	}
	if result.StorageProduct != product || result.StorageIdentityVersion.Normalized() != version {
		return RoomsMatrixBootstrap{}, ErrMatrixSSO
	}
	return result, nil
}

// recoveryKeyShape es la forma de una clave de recuperación de Matrix que aceptan
// la custodia del servicio y `client/matrix/recovery.ts` de la web: base58 en grupos
// de cuatro separados por espacios. Lo que no tenga esa forma no sale ni entra.
var recoveryKeyShape = regexp.MustCompile(`^[1-9A-HJ-NP-Za-km-z]{4}( [1-9A-HJ-NP-Za-km-z]{1,4}){8,24}$`)

// ErrMatrixRecoveryKey es una clave que no tiene la forma de una de recuperación.
var ErrMatrixRecoveryKey = errors.New("matrix: recovery key has an unexpected shape")

// MatrixRecoveryKey lee la copia con KEK que ADR 0081 custodiaba para la persona de
// la sesión. Solo para la transición de ADR 0082: se usa una vez, se envuelve en el
// dispositivo y, al guardar el envoltorio, el servicio la borra. "" sin error es que todavía no hay ninguna (404) o
// que lo que llega no es una clave: entonces se sigue a mano, como en la web.
//
// Va con `Cache-Control: no-store` en la PETICIÓN porque es la señal con la que el
// transporte sin conexión (offline.Transport) deja de guardar una respuesta en
// disco: el secreto no puede acabar en la caché del sistema aunque algún día Rooms
// use ese transporte.
func (r *Rooms) MatrixRecoveryKey(ctx context.Context) (string, error) {
	out, err := JSON[struct {
		RecoveryKey string `json:"recovery_key"`
	}](ctx, r.c, Request{
		Path:   "/matrix/recovery-key",
		Header: http.Header{"Cache-Control": {"no-store"}},
	})
	var httpErr *Error
	if errors.As(err, &httpErr) && httpErr.Status == http.StatusNotFound {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if !recoveryKeyShape.MatchString(out.RecoveryKey) {
		return "", nil
	}
	return out.RecoveryKey, nil
}

// MatrixRecoveryAccount elige la cuenta de Matrix de un envoltorio (ADR 0082). La
// vacía es la de Rooms (`@sytue_<sub>`), que es también la del almacenamiento
// heredado. Con IsolatedIdentity es la cuenta aislada de ese producto en la
// organización de la sesión, la de MatrixStorageBootstrapForIdentity.
type MatrixRecoveryAccount struct {
	Product         string
	IdentityVersion storage.IdentityVersion
}

func (a MatrixRecoveryAccount) query() (url.Values, error) {
	if a.Product == "" {
		if a.IdentityVersion != 0 {
			return nil, ErrMatrixRecoveryKey
		}
		return url.Values{}, nil
	}
	if !storage.Product(a.Product) || !a.IdentityVersion.Normalized().Valid() {
		return nil, ErrMatrixRecoveryKey
	}
	query := url.Values{"product": {a.Product}}
	if a.IdentityVersion != 0 {
		query.Set("identity_version", strconv.Itoa(int(a.IdentityVersion)))
	}
	return query, nil
}

// MatrixRecoveryWrappers es lo que el servicio guarda de una cuenta: envoltorios
// que no sabe abrir y si aún queda la copia con KEK de ADR 0081 (solo Rooms).
type MatrixRecoveryWrappers struct {
	Wrappers     []recoverywrap.Wrapper `json:"wrappers"`
	LegacyEscrow bool                   `json:"legacy_escrow"`
}

// MatrixRecoveryWrappers lee los envoltorios de la clave de recuperación de esa
// cuenta. Los que no tienen la forma del formato se descartan aquí, antes de que
// nadie derive 64 MiB de Argon2id con parámetros que no son los nuestros.
func (r *Rooms) MatrixRecoveryWrappers(ctx context.Context, account MatrixRecoveryAccount) (MatrixRecoveryWrappers, error) {
	query, err := account.query()
	if err != nil {
		return MatrixRecoveryWrappers{}, err
	}
	out, err := JSON[MatrixRecoveryWrappers](ctx, r.c, Request{
		Path: "/matrix/recovery-wrappers", Query: query,
		Header: http.Header{"Cache-Control": {"no-store"}},
	})
	if err != nil {
		return MatrixRecoveryWrappers{}, err
	}
	valid := out.Wrappers[:0]
	for _, w := range out.Wrappers {
		if w.Validate() == nil {
			valid = append(valid, w)
		}
	}
	out.Wrappers = valid
	return out, nil
}

// PutMatrixRecoveryWrapper guarda un envoltorio hecho en este dispositivo. El
// servicio retira los de otra clave y, en la cuenta de Rooms, la copia con KEK.
func (r *Rooms) PutMatrixRecoveryWrapper(ctx context.Context, account MatrixRecoveryAccount, wrapper recoverywrap.Wrapper) (string, error) {
	query, err := account.query()
	if err != nil {
		return "", err
	}
	wrapper.ID, wrapper.CreatedAt = "", ""
	if err := wrapper.Validate(); err != nil {
		return "", err
	}
	datos, err := json.Marshal(wrapper)
	if err != nil {
		return "", err
	}
	// SinAviso: guardar un envoltorio no cambia nada de lo que enseña el widget.
	out, err := JSON[struct {
		ID string `json:"id"`
	}](ctx, r.c, Request{
		Method: http.MethodPut, Path: "/matrix/recovery-wrappers", Query: query, Body: datos, SinAviso: true,
		Header: http.Header{"Content-Type": {"application/json"}, "Cache-Control": {"no-store"}},
	})
	return out.ID, err
}

// MatrixNativeAuthorization keeps its PKCE verifier inside the process. The
// browser and incoming custom-scheme URL receive only state and an opaque ticket.
type MatrixNativeAuthorization struct {
	purpose         string
	nativeConsumer  string
	AuthorizeURL    string
	State           string
	verifier        string
	identityVersion storage.IdentityVersion
	product         string
	tenant          string
}
type MatrixNativeCallback struct {
	nativeConsumer string
	product        string
	State          string `json:"state"`
	Ticket         string `json:"ticket"`
	Error          string `json:"error,omitempty"`
}

var ErrMatrixSSO = errors.New("matrix: browser return does not match this app sign-in")

// Product is the destination bound before opening the system browser. Empty
// legacy flows retain the Rooms destination, never a caller-supplied scheme.
func (a MatrixNativeAuthorization) IdentityVersion() storage.IdentityVersion {
	return a.identityVersion.Normalized()
}

func (a MatrixNativeAuthorization) Tenant() string { return a.tenant }

// Purpose is fixed by retained consent before the system browser opens.
func (a MatrixNativeAuthorization) Purpose() string { return a.purpose }

func (a MatrixNativeAuthorization) ForResources(apex, product string) bool {
	return a.purpose == "resources:"+product && a.NativeConsumer() == "" && a.ForProduct(apex, product)
}

func (a MatrixNativeAuthorization) NativeConsumer() string { return a.nativeConsumer }

func (a MatrixNativeAuthorization) ForConsumer(apex, product, consumer string) bool {
	return a.NativeConsumer() == consumer && a.ForProduct(apex, product)
}

func (a MatrixNativeAuthorization) Product() string {
	if a.product == "" {
		return "rooms"
	}
	return a.product
}

func (a MatrixNativeAuthorization) ReturnScheme() string {
	if native := storage.NativeReturnApp(a.Product(), a.NativeConsumer()); native != "" {
		return Esquema(native)
	}
	return ""
}
func (a MatrixNativeAuthorization) ForRooms(apex string) bool { return a.ForProduct(apex, "rooms") }

// ForProduct permits only the fixed first-party browser broker and the product
// and state of this authorization. All products share the existing Rooms broker.
func (a MatrixNativeAuthorization) ForProduct(apex, product string) bool {
	if apex == "" || a.Product() != product || storage.NativeReturnApp(product, a.NativeConsumer()) == "" {
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
	return a.Product() == product && a.NativeConsumer() == callback.nativeConsumer && a.State != "" && subtle.ConstantTimeCompare([]byte(a.State), []byte(callback.State)) == 1
}

// ParseMatrixNativeCallback never accepts a Matrix login token in a custom URL.
func ParseMatrixNativeCallback(u *url.URL) (MatrixNativeCallback, bool) {
	return ParseMatrixNativeCallbackFor(u, "rooms")
}

func ParseMatrixNativeCallbackFor(u *url.URL, product string) (MatrixNativeCallback, bool) {
	return ParseMatrixNativeCallbackForConsumer(u, product, "")
}

func ParseMatrixNativeCallbackForConsumer(u *url.URL, product, consumer string) (MatrixNativeCallback, bool) {
	native := storage.NativeReturnApp(product, consumer)

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
	return MatrixNativeCallback{nativeConsumer: consumer, product: product, State: q.Get("state"), Ticket: q.Get("ticket"), Error: q.Get("error")}, true
}

func (r *Rooms) MatrixNativeStart(ctx context.Context) (MatrixNativeAuthorization, error) {
	return r.MatrixNativeStartFor(ctx, "rooms")
}

func (r *Rooms) MatrixNativeStartFor(ctx context.Context, product string) (MatrixNativeAuthorization, error) {
	return r.MatrixNativeStartForIdentity(ctx, product, storage.LegacyIdentity)
}

func (r *Rooms) MatrixNativeStartForIdentity(ctx context.Context, product string, version storage.IdentityVersion) (MatrixNativeAuthorization, error) {
	return r.MatrixNativeStartForConsumer(ctx, product, "", version)
}

func (r *Rooms) MatrixNativeStartForConsumer(ctx context.Context, product, consumer string, version storage.IdentityVersion) (MatrixNativeAuthorization, error) {
	return r.matrixNativeStart(ctx, product, consumer, version, "")
}

func (r *Rooms) MatrixResourceNativeStart(ctx context.Context, product string) (MatrixNativeAuthorization, error) {
	if !storage.Product(product) {
		return MatrixNativeAuthorization{}, ErrMatrixSSO
	}
	return r.matrixNativeStart(ctx, product, "", storage.LegacyIdentity, "resources:"+product)
}

func (r *Rooms) matrixNativeStart(ctx context.Context, product, consumer string, version storage.IdentityVersion, purpose string) (MatrixNativeAuthorization, error) {
	if storage.NativeReturnApp(product, consumer) == "" || !version.Valid() {
		return MatrixNativeAuthorization{}, ErrMatrixSSO
	}

	verifierBytes := make([]byte, 32)
	if _, err := rand.Read(verifierBytes); err != nil {
		return MatrixNativeAuthorization{}, err
	}
	verifier := base64.RawURLEncoding.EncodeToString(verifierBytes)
	clear(verifierBytes)
	digest := sha256.Sum256([]byte(verifier))
	input := map[string]any{"code_challenge": base64.RawURLEncoding.EncodeToString(digest[:]), "code_challenge_method": "S256"}
	// Existing Rooms installs can still authenticate against the old broker.
	// Other products require the backend that binds a product in the grant.
	if product != "rooms" || purpose != "" {
		input["product"] = product
	}
	if consumer != "" {
		input["native_consumer"] = consumer
	}
	if version != storage.LegacyIdentity {
		input["identity_version"] = version
	}
	path := "/matrix/native/start"
	if purpose != "" {
		path = "/matrix/resources/native/start"
	}
	out, err := PostJSON[matrixNativeStartResponse](ctx, r.c, path, input)
	if err != nil {
		return MatrixNativeAuthorization{}, err
	}
	return checkedMatrixNativeStartPurpose(out, product, consumer, verifier, version, purpose)
}

type matrixNativeStartResponse struct {
	Purpose          string                  `json:"purpose"`
	ResourceApproved *bool                   `json:"resource_approved"`
	NativeConsumer   string                  `json:"native_consumer"`
	IdentityVersion  storage.IdentityVersion `json:"identity_version"`
	Product          string                  `json:"product"`
	Tenant           string                  `json:"tenant"`
	AuthorizeURL     string                  `json:"authorize_url"`
	State            string                  `json:"state"`
}

func validMatrixNativePurpose(out matrixNativeStartResponse, product, consumer, purpose string) bool {
	if out.Purpose != purpose {
		return false
	}
	if purpose == "" {
		return true
	}
	return purpose == "resources:"+product && out.ResourceApproved != nil && !*out.ResourceApproved && consumer == "" && out.Tenant != ""
}

func validMatrixAuthorizeURL(rawURL, state string) bool {
	if state == "" || len(state) > 4096 {
		return false
	}
	u, err := url.Parse(rawURL)
	return err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.Fragment == ""
}

func checkedMatrixNativeStartPurpose(out matrixNativeStartResponse, product, consumer, verifier string, version storage.IdentityVersion, purpose string) (MatrixNativeAuthorization, error) {
	if !validMatrixNativePurpose(out, product, consumer, purpose) {
		return MatrixNativeAuthorization{}, ErrMatrixSSO
	}
	if out.Product == "" {
		out.Product = "rooms"
	}
	if out.Product != product || out.NativeConsumer != consumer || out.IdentityVersion.Normalized() != version {
		return MatrixNativeAuthorization{}, ErrMatrixSSO
	}
	if (version == storage.IsolatedIdentity || purpose != "") && !(storage.Scope{Tenant: out.Tenant, Subject: "authorization", Product: product}).Valid() {
		return MatrixNativeAuthorization{}, ErrMatrixSSO
	}
	if !validMatrixAuthorizeURL(out.AuthorizeURL, out.State) {
		return MatrixNativeAuthorization{}, ErrMatrixSSO
	}
	return MatrixNativeAuthorization{purpose: purpose, nativeConsumer: consumer, AuthorizeURL: out.AuthorizeURL, State: out.State, verifier: verifier, product: product, identityVersion: version, tenant: out.Tenant}, nil
}

// MatrixNativeFinish returns a one-use Matrix login token over authenticated TLS;
// the caller must consume it immediately via m.login.token and pin Whoami.
func matrixNativeFinishInput(authorization MatrixNativeAuthorization, callback MatrixNativeCallback) (map[string]any, error) {
	if !authorization.Accepts(callback) || authorization.verifier == "" {
		return nil, ErrMatrixSSO
	}
	if callback.Error != "" {
		return nil, ErrOAuthDenied
	}
	if callback.Ticket == "" {
		return nil, ErrMatrixSSO
	}
	input := map[string]any{"state": callback.State, "ticket": callback.Ticket, "code_verifier": authorization.verifier}
	return input, nil
}

func (r *Rooms) MatrixNativeFinish(ctx context.Context, authorization MatrixNativeAuthorization, callback MatrixNativeCallback) (string, error) {
	if authorization.purpose != "" {
		return "", ErrMatrixSSO
	}
	input, err := matrixNativeFinishInput(authorization, callback)
	if err != nil {
		return "", err
	}
	if authorization.IdentityVersion() != storage.LegacyIdentity || authorization.NativeConsumer() != "" {
		input["product"], input["identity_version"] = authorization.Product(), authorization.IdentityVersion()
	}
	if authorization.NativeConsumer() != "" {
		input["native_consumer"] = authorization.NativeConsumer()
	}
	out, err := PostJSON[struct {
		NativeConsumer  string                  `json:"native_consumer"`
		IdentityVersion storage.IdentityVersion `json:"identity_version"`
		LoginToken      string                  `json:"login_token"`
	}](ctx, r.c, "/matrix/native/finish", input)
	if err != nil {
		return "", err
	}
	if out.LoginToken == "" || len(out.LoginToken) > 4096 || out.IdentityVersion.Normalized() != authorization.IdentityVersion() || out.NativeConsumer != authorization.NativeConsumer() {
		return "", ErrMatrixSSO
	}
	return out.LoginToken, nil
}

// RoomsMatrixEventRef is the only message payload accepted by the metadata API.
type RoomsMatrixEventRef = storage.Reference

func (r *Rooms) MatrixSend(ctx context.Context, channel string, reference RoomsMatrixEventRef) (RoomsMessage, error) {
	return PostJSON[RoomsMessage](ctx, r.c, "/channels/"+url.PathEscape(channel)+"/messages", map[string]any{"body": "", "matrix_event": reference})
}

// MatrixReply publica la referencia de una respuesta cifrada; con alsoInChannel se ve
// además en el feed del canal (la marca es metadato del servicio, no contenido).
func (r *Rooms) MatrixReply(ctx context.Context, parent string, reference RoomsMatrixEventRef, alsoInChannel bool) (RoomsMessage, error) {
	body := map[string]any{"body": "", "matrix_event": reference}
	if alsoInChannel {
		body["also_in_channel"] = true
	}
	return PostJSON[RoomsMessage](ctx, r.c, "/messages/"+url.PathEscape(parent)+"/replies", body)
}

func (r *Rooms) MatrixEdit(ctx context.Context, message string, reference RoomsMatrixEventRef) (RoomsMessage, error) {
	return conJSON[RoomsMessage](ctx, r.c, http.MethodPatch, "/messages/"+url.PathEscape(message), map[string]any{"body": "", "matrix_event": reference})
}
