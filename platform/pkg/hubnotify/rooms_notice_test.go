// SPDX-License-Identifier: Apache-2.0
package hubnotify

import (
	"strings"
	"testing"
)

func TestRoomsNoticeContainsOnlyAuthenticatedLink(t *testing.T) {
	channel := "11111111-1111-4111-8111-111111111111"
	message := "22222222-2222-4222-8222-222222222222"
	want := TextoAvisoRooms + "\nhttps://rooms.sytue.eu/rooms/acme/channels/" + channel + "?message=" + message
	if got := AvisoMensajeRooms("https://rooms.sytue.eu/rooms", "acme", channel, message); got != want {
		t.Fatalf("notice = %q, want %q", got, want)
	}
	for _, tc := range []struct{ base, tenant, channel, message string }{
		{"http://rooms.sytue.eu", "acme", channel, message},
		{"https://user:secret@rooms.sytue.eu", "acme", channel, message},
		{"https://rooms.sytue.eu?token=secret", "acme", channel, message},
		{"https://rooms.sytue.eu#key", "acme", channel, message},
		{"https://rooms.sytue.eu", "../other", channel, message},
		{"https://rooms.sytue.eu", "", channel, message},
		{"https://rooms.sytue.eu", "acme", "invalid", message},
		{"https://rooms.sytue.eu", "acme", channel, "secret"},
	} {
		if got := AvisoMensajeRooms(tc.base, tc.tenant, tc.channel, tc.message); got != "" {
			t.Errorf("unsafe notice was emitted: %q", got)
		}
	}
	if got := AvisoMensajeRooms("https://rooms.sytue.eu", "acme", channel, ""); !strings.HasPrefix(got, TextoAvisoRooms+"\nhttps://") {
		t.Fatalf("channel probe = %q", got)
	}
}
