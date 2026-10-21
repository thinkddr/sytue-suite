// SPDX-License-Identifier: Apache-2.0
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// ErrRoomsIntegrationUnavailable means the OAuth gateway cannot complete the request.
var ErrRoomsIntegrationUnavailable = errors.New("rooms: integrations gateway unavailable")

// IntegrationDestination is a channel/space that the connected account can publish to.
type IntegrationDestination struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type roomsGatewayClient struct {
	baseURL string
	secret  string
	http    *http.Client
}

func newRoomsGateway(baseURL, secret string) *roomsGatewayClient {
	return &roomsGatewayClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		secret:  secret,
		http: &http.Client{
			Timeout: 15 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// call talks only to the configured internal service, never to a user supplied URL.
func (c *roomsGatewayClient) call(ctx context.Context, method, path string, body, out any) error {
	if c == nil || c.baseURL == "" || c.secret == "" {
		return ErrRoomsIntegrationUnavailable
	}
	var data io.Reader = http.NoBody
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		data = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, data)
	if err != nil {
		return ErrRoomsIntegrationUnavailable
	}
	req.Header.Set("X-Sytue-Internal", c.secret)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: request failed", ErrRoomsIntegrationUnavailable)
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return domain.ErrForbidden
	case http.StatusNotFound:
		return domain.ErrNotFound
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return domain.ErrInvalidInput
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("%w: HTTP %d", ErrRoomsIntegrationUnavailable, res.StatusCode)
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(out); err != nil {
		return ErrRoomsIntegrationUnavailable
	}
	return nil
}

func (c *roomsGatewayClient) destinations(ctx context.Context, instance string) ([]IntegrationDestination, error) {
	tenant, err := tenancy.From(ctx)
	if err != nil {
		return nil, domain.ErrForbidden
	}
	q := url.Values{"tenant": {tenant.Slug}, "instance": {instance}}
	var out struct {
		Items []IntegrationDestination `json:"items"`
	}
	err = c.call(ctx, http.MethodGet, "/internal/v1/rooms/destinations?"+q.Encode(), nil, &out)
	if out.Items == nil {
		out.Items = []IntegrationDestination{}
	}
	return out.Items, err
}

func (c *roomsGatewayClient) send(ctx context.Context, instance, remote string, channel, message uuid.UUID) error {
	tenant, err := tenancy.From(ctx)
	if err != nil {
		return domain.ErrForbidden
	}
	request := map[string]string{"tenant": tenant.Slug, "instance": instance, "remote_id": remote, "channel_id": channel.String()}
	if message != uuid.Nil {
		request["message_id"] = message.String()
	}
	return c.call(ctx, http.MethodPost, "/internal/v1/rooms/messages", request, nil)
}

// IntegrationDestinations requires organization administration before reading remote data.
func (s *Service) IntegrationDestinations(ctx context.Context, instance string) ([]IntegrationDestination, error) {
	if !roomsIntegrationAdmin(ctx) {
		return nil, domain.ErrForbidden
	}
	if _, err := uuid.Parse(instance); err != nil {
		return nil, domain.ErrInvalidInput
	}
	return s.roomsGateway.destinations(ctx, instance)
}

// CreateOAuthBridge checks the local channel before any provider read or write.
func (s *Service) CreateOAuthBridge(ctx context.Context, channelID uuid.UUID, actor, platform, instance, remote string) (domain.Bridge, error) {
	if !roomsIntegrationAdmin(ctx) {
		return domain.Bridge{}, domain.ErrForbidden
	}
	if !domain.ValidBridgePlatform(platform) || strings.TrimSpace(remote) == "" || len(remote) > 1024 {
		return domain.Bridge{}, domain.ErrInvalidInput
	}
	if _, err := uuid.Parse(instance); err != nil {
		return domain.Bridge{}, domain.ErrInvalidInput
	}
	_, err := s.pstore.ChannelForAdmin(ctx, channelID, actor)
	if err != nil {
		return domain.Bridge{}, err
	}
	destinations, err := s.roomsGateway.destinations(ctx, instance)
	if err != nil {
		return domain.Bridge{}, err
	}
	target := ""
	for _, destination := range destinations {
		if destination.ID == remote {
			target = destination.Name
			break
		}
	}
	if target == "" {
		return domain.Bridge{}, domain.ErrInvalidInput
	}
	bridge := domain.Bridge{Platform: platform, InstanceID: instance, RemoteID: remote}
	if err := s.sendBridge(ctx, bridge, bridgeMessage{ChannelID: channelID}); err != nil {
		return domain.Bridge{}, fmt.Errorf("%w: %w", ErrBridgeRejected, err)
	}
	return s.pstore.CreateOAuthBridge(ctx, channelID, actor, platform, instance, remote, target)
}

// CreateIntegrationMeeting returns a provider URL to the authorized client. The
// server never inserts it into a conversation through a plaintext message path.
func (s *Service) CreateIntegrationMeeting(ctx context.Context, instance string, channelID uuid.UUID, actor, _ string) (string, error) {
	if !roomsIntegrationAdmin(ctx) {
		return "", domain.ErrForbidden
	}
	if _, err := uuid.Parse(instance); err != nil {
		return "", domain.ErrInvalidInput
	}
	if _, err := s.pstore.ChannelForAdmin(ctx, channelID, actor); err != nil {
		return "", err
	}
	tenant, err := tenancy.From(ctx)
	if err != nil {
		return "", domain.ErrForbidden
	}
	var result struct {
		URL string `json:"url"`
	}
	if err := s.roomsGateway.call(ctx, http.MethodPost, "/internal/v1/rooms/meetings", map[string]string{"tenant": tenant.Slug, "instance": instance}, &result); err != nil {
		return "", err
	}
	u, err := url.Parse(result.URL)
	if err != nil || u.Scheme != "https" || u.Host != "meet.google.com" || u.User != nil || u.Path == "" || u.Path == "/" {
		return "", ErrRoomsIntegrationUnavailable
	}
	return u.String(), nil
}

func roomsIntegrationAdmin(ctx context.Context) bool {
	claims, ok := authn.FromContext(ctx)
	return ok && claims.HasAnyRole("owner", "admin")
}
