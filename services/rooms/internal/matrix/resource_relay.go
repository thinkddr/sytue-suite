// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/publication"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// ResourceRelayConfig is operator configuration. It is never populated from a
// publication request. The AS owns no SDK crypto device, user token or room key.
type ResourceRelayConfig struct{ InternalURL, ServerName, ASToken string }

type ResourceRelay struct {
	config ResourceRelayConfig
	http   *http.Client
}

func NewResourceRelay(config ResourceRelayConfig, client *http.Client) (*ResourceRelay, error) {
	base, err := url.Parse(config.InternalURL)
	if err != nil || base.Host == "" || (base.Scheme != "http" && base.Scheme != "https") || base.User != nil || base.RawQuery != "" || base.Fragment != "" || (base.Path != "" && base.Path != "/") || config.ServerName == "" || len(config.ASToken) < 32 || strings.IndexFunc(config.ASToken, unicode.IsSpace) >= 0 {
		return nil, domain.ErrMatrixUnavailable
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	cloned := *client
	cloned.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	config.InternalURL = strings.TrimRight(config.InternalURL, "/")
	return &ResourceRelay{config: config, http: &cloned}, nil
}

func (r *ResourceRelay) sdk(user, device string) (*mautrix.Client, error) {
	if r == nil {
		return nil, domain.ErrMatrixUnavailable
	}
	if user != "" {
		prefix := "@sytue_"
		suffix := ":" + r.config.ServerName
		if !strings.HasPrefix(user, prefix) || !strings.HasSuffix(user, suffix) {
			return nil, resource.ErrInvalid
		}
		subject := strings.TrimSuffix(strings.TrimPrefix(user, prefix), suffix)
		parsed, err := uuid.Parse(subject)
		if err != nil || parsed == uuid.Nil || parsed.String() != subject {
			return nil, resource.ErrInvalid
		}
	}
	client, err := mautrix.NewClient(r.config.InternalURL, id.UserID(user), r.config.ASToken)
	if err != nil {
		return nil, domain.ErrMatrixUnavailable
	}
	client.Client = r.http
	// Durable holds decide retry/reconciliation. An automatic SDK retry must not
	// issue a second request behind that coordinator after an ambiguous result.
	client.DefaultHTTPRetries = 0
	client.IgnoreRateLimit = true
	if user != "" {
		client.SetAppServiceUserID = true
	}
	if device != "" {
		if user == "" {
			return nil, resource.ErrInvalid
		}
		client.DeviceID = id.DeviceID(device)
		// v1.17 stabilized device_id. mautrix v0.32's AS flag still emits the old
		// MSC name; use the maintained request hook for this one stable parameter.
		client.RequestHook = func(request *http.Request) {
			query := request.URL.Query()
			query.Set("device_id", device)
			request.URL.RawQuery = query.Encode()
		}
	}
	return client, nil
}

// Send is only opaque transport after resourcepg's fresh durable sending claim
// AND origin admission. It grants no ACL/CAS authority. The exact original SDK
// producer device is asserted; a new device may reconcile its accepted receipt
// but must never publish a retained attempt as the original producer.
func (r *ResourceRelay) Send(ctx context.Context, candidate resource.Candidate, verified resource.VerifiedPublication, marker publication.Marker) (storage.Reference, error) {
	if err := ctx.Err(); err != nil {
		return storage.Reference{}, err
	}
	value := verified.Value()
	if !value.Valid() || value != candidate.Proof.CipherPublication || !marker.Valid() {
		return storage.Reference{}, resource.ErrInvalid
	}
	hash, err := resource.PublicationDigest(value)
	if err != nil || hash != verified.SHA256() {
		return storage.Reference{}, resource.ErrInvalid
	}
	core, err := resource.CandidateDigest(candidate)
	if err != nil || core != verified.CandidateSHA256() {
		return storage.Reference{}, resource.ErrInvalid
	}
	wire, err := publication.AttachMarker(candidate.EncryptedEvent, marker)
	if err != nil {
		return storage.Reference{}, resource.ErrInvalid
	}
	defer clear(wire)
	client, err := r.sdk(value.Actor.UserID, value.Actor.DeviceID)
	if err != nil {
		return storage.Reference{}, err
	}
	result, err := client.SendMessageEvent(ctx, id.RoomID(value.RoomID), event.EventEncrypted, json.RawMessage(wire), mautrix.ReqSendEvent{TransactionID: value.TransactionID})
	if err != nil || result == nil || result.EventID == "" {
		return storage.Reference{}, domain.ErrMatrixUnavailable
	}
	return storage.Reference{RoomID: value.RoomID, EventID: result.EventID.String()}, nil
}

// AcceptanceReader is read-only and bound to CURRENT independently verified
// resource access. The AS sender never joins encrypted rooms or becomes a key
// recipient. GET asserts the authorized reader's user, never producer A's device.
func (r *ResourceRelay) AcceptanceReader(access resource.VerifiedAccess, fence resource.Fence) (resource.AcceptanceSource, error) {
	if r == nil || fence == nil || (access.Action() != resource.Read && access.Action() != resource.Manage) {
		return nil, resource.ErrInvalid
	}
	capability := access.Capability().Value()
	_, err := resource.Authorize(access.Epoch(), access.Capability(), capability.Subject, access.Action())
	if err != nil {
		return nil, err
	}
	if _, err := r.sdk(capability.UserID, ""); err != nil {
		return nil, err
	}
	// Keep the original opaque access, including its independently enrolled device.
	return resourceAcceptedReader{relay: r, access: access, fence: fence}, nil
}

type resourceAcceptedReader struct {
	relay  *ResourceRelay
	access resource.VerifiedAccess
	fence  resource.Fence
}

func (s resourceAcceptedReader) AcceptedEvent(ctx context.Context, ref storage.Reference) (*event.Event, error) {
	if !ref.Valid() || ref.RoomID != s.access.Epoch().Value().RoomID {
		return nil, resource.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	current, err := resource.Reauthorize(ctx, s.access, s.access.Scope().Subject, s.access.Action())
	if err != nil {
		return nil, err
	}
	if err := s.fence.Revalidate(ctx, current); err != nil {
		return nil, err
	}
	client, err := s.relay.sdk(current.Capability().Value().UserID, "")
	if err != nil {
		return nil, err
	}
	value, err := client.GetEvent(ctx, id.RoomID(ref.RoomID), id.EventID(ref.EventID))
	if err != nil {
		return nil, domain.ErrMatrixUnavailable
	}
	if err := s.fence.Revalidate(ctx, current); err != nil {
		return nil, err
	}
	if _, err := resource.Reauthorize(ctx, current, current.Scope().Subject, current.Action()); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return value, nil
}

var _ resource.AcceptanceSource = resourceAcceptedReader{}
