// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/linkedservices"
	"git.sytue.eu/sytue/one/services/integrations/internal/mailbox"
)

// The summary is what Mail's sidebar paints, as Apple Mail does with each
// account: its folders with their role and the unread count. One account that
// is slow or asks to sign in again must not hold the others back.
const (
	// mailSummaryTimeout is each account's budget. A provider that does not
	// answer in it is «unavailable» for this load; the client asks again later.
	mailSummaryTimeout = 10 * time.Second
	// mailSummaryParallel is how many providers are asked at once.
	mailSummaryParallel = 4
)

// Summary states. «reconnect» is for the person (sign in again); «unavailable»
// is the provider's (try later).
const (
	mailSummaryOK          = "ok"
	mailSummaryReconnect   = "reconnect"
	mailSummaryUnavailable = "unavailable"
)

// mailSummaryState turns a failure into what the sidebar can say about it. The
// error itself is never returned or logged: it can carry the provider's reply.
func mailSummaryState(err error) string {
	if err == nil {
		return mailSummaryOK
	}
	if errors.Is(err, mailbox.ErrAuth) {
		return mailSummaryReconnect
	}
	var apiError *httpx.Error
	if errors.As(err, &apiError) && apiError.Code == "external_reconnect_required" {
		return mailSummaryReconnect
	}
	return mailSummaryUnavailable
}

// mailSummaryAccounts are the accounts Mail lists: the caller's mail
// connections that serve Mail. One connected only for Calendar, Contacts or
// Notes has no mailbox here (B12).
func mailSummaryAccounts(instances []domain.AppInstance) []domain.AppInstance {
	return slices.DeleteFunc(externalMailAccounts(instances), func(inst domain.AppInstance) bool {
		return !mailServesMail(inst)
	})
}

func mailServesMail(inst domain.AppInstance) bool {
	var cfg map[string]string
	_ = json.Unmarshal(inst.Config, &cfg)
	return slices.Contains(connectedMailServices(cfg), linkedservices.Mail)
}

// summarizeMail asks each account for its folders, at most `parallel` at once
// and each within `timeout`, and keeps the list's order. An account that is not
// connected is not asked: it already needs signing in again.
func summarizeMail(ctx context.Context, accounts []domain.AppInstance, parallel int, timeout time.Duration, folders func(context.Context, domain.AppInstance) ([]mailbox.Folder, error)) []map[string]any {
	items := make([]map[string]any, len(accounts))
	slots := make(chan struct{}, max(parallel, 1))
	var wg sync.WaitGroup
	for i, inst := range accounts {
		item := externalMailItem(inst)
		item["folders"] = []mailbox.Folder{}
		items[i] = item
		if !mailConnected(inst) {
			item["state"] = mailSummaryReconnect
			continue
		}
		wg.Go(func() {
			slots <- struct{}{}
			defer func() { <-slots }()
			accountCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			found, err := mailFoldersWithin(accountCtx, inst, folders)
			item["state"] = mailSummaryState(err)
			if err != nil {
				log.From(ctx).Info("integrations: cuenta de correo sin resumen", "instance", inst.ID, "state", item["state"])
				return
			}
			if found != nil {
				item["folders"] = found
			}
		})
	}
	wg.Wait()
	return items
}

// mailFoldersWithin returns when `ctx` ends even if the client does not watch
// it: a protocol client stuck in a read must not hold the whole summary.
func mailFoldersWithin(ctx context.Context, inst domain.AppInstance, folders func(context.Context, domain.AppInstance) ([]mailbox.Folder, error)) ([]mailbox.Folder, error) {
	type result struct {
		folders []mailbox.Folder
		err     error
	}
	done := make(chan result, 1)
	go func() {
		found, err := folders(ctx, inst)
		done <- result{found, err}
	}()
	select {
	case r := <-done:
		return r.folders, r.err
	case <-ctx.Done():
		return nil, mailbox.ErrUnavailable
	}
}

// ExternalMailSummary lists the caller's mailboxes with their folders. It takes
// no instance: it only walks the caller's own connections, so ownership is that
// of the list (RequireExternalChatActor's rule, applied by the store query).
func (h *Handlers) ExternalMailSummary(w http.ResponseWriter, r *http.Request) {
	actor, _ := authn.FromContext(r.Context())
	instances, err := h.store.ListExternalChatInstances(r.Context(), actor.Subject)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	items := summarizeMail(r.Context(), mailSummaryAccounts(instances), mailSummaryParallel, mailSummaryTimeout, func(ctx context.Context, inst domain.AppInstance) ([]mailbox.Folder, error) {
		connected, err := h.mailAccountFor(ctx, inst, "")
		if err != nil {
			return nil, err
		}
		client, err := h.mailboxOf(ctx, connected)
		if err != nil {
			return nil, err
		}
		return client.Folders(ctx)
	})
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}
