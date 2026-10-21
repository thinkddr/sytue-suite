// SPDX-License-Identifier: Apache-2.0
package service

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// This internal response is never returned through a browser-facing handler.
type whatsappConsentAccount struct {
	PhoneNumberID        string `json:"phone_number_id"`
	DisplayPhone         string `json:"display_phone"`
	VerifiedName         string `json:"verified_name"`
	AccessToken          string `json:"access_token"`
	AppSecret            string `json:"app_secret"`
	RegistrationRequired bool   `json:"registration_required"`
}

// ConnectWhatsAppConsent connects a consented number without exposing credentials.
// The callback must exist for Meta's challenge before conversations can be enabled.
func (s *Service) ConnectWhatsAppConsent(ctx context.Context, actor string, teamID uuid.UUID, instance, phoneID, pin string) (domain.WhatsAppAccount, error) {
	instance, phoneID, pin = strings.TrimSpace(instance), strings.TrimSpace(phoneID), strings.TrimSpace(pin)
	if err := validateWhatsAppConsent(teamID, instance, phoneID, pin); err != nil {
		return domain.WhatsAppAccount{}, err
	}
	if err := s.pstore.RequireWhatsAppTeam(ctx, actor, teamID); err != nil {
		return domain.WhatsAppAccount{}, err
	}
	callbackBase, err := whatsappCallbackBase(s.cfg.PublicURL)
	if err != nil {
		return domain.WhatsAppAccount{}, err
	}
	tenant := tenancy.MustFrom(ctx).Slug
	q := url.Values{"tenant": {tenant}, "instance": {instance}, "phone_number_id": {phoneID}}
	var consent whatsappConsentAccount
	if err := s.roomsGateway.call(ctx, http.MethodGet, "/internal/v1/rooms/whatsapp/account?"+q.Encode(), nil, &consent); err != nil {
		return domain.WhatsAppAccount{}, err
	}
	if consent.PhoneNumberID != phoneID || consent.AccessToken == "" || consent.AppSecret == "" {
		return domain.WhatsAppAccount{}, ErrRoomsIntegrationUnavailable
	}
	if consent.RegistrationRequired && pin == "" {
		return domain.WhatsAppAccount{}, domain.ErrInvalidInput
	}
	plain, hash, err := domain.NewOpaqueToken()
	if err != nil {
		return domain.WhatsAppAccount{}, err
	}
	pending, err := s.pstore.PrepareWhatsAppAccount(ctx, domain.WhatsAppAccount{
		TeamID: teamID, PhoneNumberID: phoneID, DisplayPhone: consent.DisplayPhone,
		VerifiedName: consent.VerifiedName, AccessToken: consent.AccessToken,
		AppSecret: consent.AppSecret, CreatedBy: actor, InstanceID: instance,
	}, hash)
	if err != nil {
		return domain.WhatsAppAccount{}, err
	}
	complete := false
	defer func() {
		if complete {
			return
		}
		// Cancellation of the browser request must not leave a visible connection.
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if err := s.pstore.AbortWhatsAppAccount(cleanup, pending.ID, actor); err != nil {
			log.From(ctx).Error("whatsapp: could not clear unfinished connection", "account_id", pending.ID, "error", err)
		}
	}()
	request := map[string]string{
		"tenant": tenant, "instance": instance, "phone_number_id": phoneID,
		"callback_url": callbackBase + "/webhooks/whatsapp/" + plain,
		"verify_token": plain,
	}
	if pin != "" {
		request["pin"] = pin
	}
	// Registration, subscription and Meta's callback challenge are sequential.
	activation := *s.roomsGateway
	client := *activation.http
	client.Timeout = 45 * time.Second
	activation.http = &client
	if err := activation.call(ctx, http.MethodPost, "/internal/v1/rooms/whatsapp/activate", request, nil); err != nil {
		return domain.WhatsAppAccount{}, err
	}
	account, err := s.pstore.FinishWhatsAppAccount(ctx, pending.ID, actor)
	if err != nil {
		return domain.WhatsAppAccount{}, err
	}
	complete = true
	return account, nil
}

func validateWhatsAppConsent(teamID uuid.UUID, instance, phoneID, pin string) error {
	id, err := uuid.Parse(instance)
	if err != nil || id == uuid.Nil || teamID == uuid.Nil || phoneID == "" || len(phoneID) > 32 || strings.Trim(phoneID, "0123456789") != "" {
		return domain.ErrInvalidInput
	}
	if pin != "" && (len(pin) != 6 || strings.Trim(pin, "0123456789") != "") {
		return domain.ErrInvalidInput
	}
	return nil
}

func whatsappCallbackBase(raw string) (string, error) {
	public, err := url.Parse(raw)
	if err != nil || public.Scheme != "https" || public.Host == "" || public.User != nil || public.RawQuery != "" || public.Fragment != "" {
		return "", ErrRoomsIntegrationUnavailable
	}
	return strings.TrimRight(public.String(), "/"), nil
}
