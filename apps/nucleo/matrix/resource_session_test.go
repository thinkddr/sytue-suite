// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func TestResourceSDKDeviceStoreAndLeafAreProductScoped(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("local scope validation reached network") }))
	defer server.Close()
	cfg := configFor(t, server)
	cfg.Subject = uuid.NewString()
	cfg.Bootstrap.Actor = cfg.Subject
	cfg.Bootstrap.MatrixUserID = "@sytue_" + cfg.Subject + ":matrix.test"
	cfg.Bootstrap.Channels = nil
	ordinary, err := newClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ResourceProduct = "docs"
	docs, err := newClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ResourceProduct = "mail"
	mail, err := newClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	routes := docs.cfg.Bootstrap
	routes.Channels = []RoomBinding{{ChannelID: "raw", RoomID: "!raw"}}
	if err := docs.UpdateBindings(routes); !errors.Is(err, ErrIdentity) || docs.allowedRoom("!raw") {
		t.Fatal("raw Rooms routing enabled a resource room", err)
	}
	if filepath.Base(docs.directory) != docs.account || strings.ContainsAny(docs.account, ":/\\\x00") {
		t.Fatal("logical resource namespace leaked into an OS path component")
	}
	if ordinary.account == docs.account || docs.account == mail.account || docs.directory == mail.directory || docs.sessionKey() == mail.sessionKey() {
		t.Fatal("resource product reused another SDK device/session store")
	}
	docs.session = credentials{ResourceProduct: "docs", Homeserver: docs.cfg.Bootstrap.HomeserverURL, UserID: id.UserID(docs.cfg.Bootstrap.MatrixUserID), DeviceID: "DOCS_A", AccessToken: "artificial-private-credential", Subject: docs.cfg.Subject, Tenant: docs.cfg.Tenant}
	first, err := docs.resourceLeaf(true)
	if err != nil {
		t.Fatal(err)
	}
	same, err := docs.resourceLeaf(false)
	if err != nil || same.Recipient().String() != first.Recipient().String() {
		t.Fatal("app update replaced stable device leaf", err)
	}
	docs.session.DeviceID = "DOCS_B"
	next, err := docs.resourceLeaf(true)
	if err != nil || next.Recipient().String() == first.Recipient().String() {
		t.Fatal("new SDK device cloned old leaf", err)
	}
	docs.session.DeviceID = "DOCS_A"
	raw, err := json.Marshal(docs.session)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := docs.decodeSession(raw); err != nil {
		t.Fatal(err)
	}
	if _, err := mail.decodeSession(raw); !errors.Is(err, ErrIdentity) {
		t.Fatal("Mail accepted Docs credentials", err)
	}
	if key := docs.resourceLeafKey("DOCS_A"); key == docs.resourceLeafKey("DOCS_B") || key == mail.resourceLeafKey("DOCS_A") {
		t.Fatal("keychain leaf not bound to exact SDK device and product")
	}
	if bytes.Contains(raw, []byte(first.String())) {
		t.Fatal("serialized SDK credential exposed private PQ leaf")
	}
	docs.resourceGrant = &api.MatrixResourceGrant{Grant: "artificial-unconsumed-consent"}
	if err := docs.Close(); err != nil || docs.resourceGrant != nil {
		t.Fatal("closing device retained pending resource consent", err)
	}
	for name, change := range map[string]func(*Config){
		"legacy-room-route": func(v *Config) {
			v.Bootstrap.Channels = []RoomBinding{{ChannelID: "server-body", RoomID: "!server-body"}}
		},
		"legacy-storage":  func(v *Config) { v.Bootstrap.StorageProduct = "docs" },
		"native-consumer": func(v *Config) { v.NativeConsumer = "mail" },
		"another-person":  func(v *Config) { v.Bootstrap.MatrixUserID = "@sytue_" + uuid.NewString() + ":matrix.test" },
		"invalid-product": func(v *Config) { v.ResourceProduct = "invented" },
	} {
		t.Run(name, func(t *testing.T) {
			changed := docs.cfg
			change(&changed)
			if _, err := newClient(changed); !errors.Is(err, ErrIdentity) {
				t.Fatal("resource constructor accepted legacy/unbound metadata", err)
			}
		})
	}
}
