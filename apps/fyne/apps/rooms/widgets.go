// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"strconv"

	"git.sytue.eu/sytue/one/apps/nucleo/widgets"
)

// publicarWidget reuses authenticated list reads and successful read receipts.
// It never decrypts messages, includes direct-message peer names or polls on
// behalf of an extension. view belongs to the request's adopted session, not to
// a later settings read.
func (sa *Sala) publicarWidget(view widgets.View) {
	if sa.Widget == nil || !widgets.Disponible() || !sa.widgetCanalesCompletos {
		return
	}
	seen := make(map[string]bool, len(sa.canales))
	var unread int64
	var rawItems []widgets.Item
	for _, channel := range sa.canales {
		if channel.Archived || seen[channel.ID] {
			continue
		}
		if channel.ID == "" {
			return
		}
		seen[channel.ID] = true
		c := sa.canalDe(channel)
		count := sinLeerDe([]Canal{c})
		if count < 0 || int64(count) > 1_000_000_000_000-unread {
			return
		}
		unread += int64(count)
		if channel.Kind == "public" && !channel.External && channel.Name != "" && len(rawItems) < widgets.MaxItems {
			badge := ""
			if sa.widgetPendientesCompletos && count > 0 {
				badge = strconv.Itoa(count)
			}
			rawItems = append(rawItems, widgets.Item{
				Title:    "#" + channel.Name,
				Subtitle: channel.Topic,
				Badge:    badge,
				Unread:   sa.widgetPendientesCompletos && count > 0,
			})
		}
	}
	counters := []widgets.Counter{{Kind: "channels", Value: int64(len(seen))}}
	if sa.widgetPendientesCompletos {
		counters = append([]widgets.Counter{{Kind: "unread", Value: unread}}, counters...)
	}
	detail := widgets.Detail{
		Counters: counters,
		Items:    widgets.SanitizeItems(rawItems),
	}
	pedir(sa, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, view.PublicarDetalle(ctx, detail)
	}, func(struct{}, error) {})
}
