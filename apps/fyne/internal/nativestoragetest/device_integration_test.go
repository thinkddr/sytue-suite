// SPDX-License-Identifier: Apache-2.0
//go:build integration && cgo

package nativestoragetest

import (
	"context"
	"errors"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"maunium.net/go/mautrix"

	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
)

func TestActualSDKFixtureKeepsRecoveryAcknowledgmentWithCaller(t *testing.T) {
	if os.Getenv("SYTUE_NATIVE_PRIVATE_APPLICATION") != "1" {
		t.Skip("requires disposable remote Synapse")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	origin, err := matrixprooftest.DisposableHTTPOrigin(os.Getenv("SYTUE_NATIVE_PRIVATE_APPLICATION_URL"))
	if err != nil {
		t.Fatal(err)
	}
	const password = "artificial-disposable-native-storage-password"
	register := func(username string) *mautrix.Client {
		client, err := mautrix.NewClient(origin.String(), "", "")
		if err != nil {
			t.Fatal(err)
		}
		matrixprooftest.Register(ctx, t, client, username, password, "REGISTRATION")
		return client
	}
	scope := storage.Scope{Tenant: "native-fixture", Subject: uuid.NewString(), Product: "mail"}
	controller := register("rooms_controller")
	user := register(strings.TrimSuffix(strings.TrimPrefix(storage.IsolatedMatrixUserPrefix(scope), "@"), ":"))
	room := matrixprooftest.EncryptedStorageRoom(ctx, t, controller, user.UserID)
	configuration := Config{Scope: scope, Origin: origin.String(), User: user, Controller: controller, Room: room, Password: password}
	first, err := Open(ctx, t, configuration)
	if err != nil {
		t.Fatal(err)
	}
	status, err := first.Client.VerificationStatus(ctx)
	if err != nil || !status.Verified || !status.RecoverySetupPending {
		t.Fatal("fixture acknowledged recovery or manufactured verification", status, err)
	}
	if err := first.Client.AcknowledgeRecoverySetup(ctx); err != nil {
		t.Fatal(err)
	}
	configuration.Recovery = first.Recovery
	second, err := Open(ctx, t, configuration)
	if err != nil {
		t.Fatal(err)
	}
	if first.Client.DeviceID() == second.Client.DeviceID() {
		t.Fatal("fresh recovery reused the first SDK device")
	}
	if first.Config.Bootstrap.HomeserverURL != second.Config.Bootstrap.HomeserverURL {
		t.Fatal("one Synapse acquired a different origin for each SSO bridge")
	}
	status, err = second.Client.VerificationStatus(ctx)
	if err != nil || !status.Verified || status.RecoverySetupPending {
		t.Fatal("fresh SDK did not recover its own verified root", status, err)
	}
	if second.Client.StorageIdentityVersion() != storage.IsolatedIdentity {
		t.Fatal("fixture changed isolated identity routing")
	}
}

func TestActualSDKFixtureKeepsDistinctSynapseOriginsBeforeFileMigration(t *testing.T) {
	if os.Getenv("SYTUE_NATIVE_PRIVATE_APPLICATION") != "1" || os.Getenv("SYTUE_NATIVE_PRIVATE_APPLICATION_OTHER_URL") == "" {
		t.Skip("requires two disposable Synapse origins")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	first, err := matrixprooftest.DisposableHTTPOrigin(os.Getenv("SYTUE_NATIVE_PRIVATE_APPLICATION_URL"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := matrixprooftest.DisposableHTTPOrigin(os.Getenv("SYTUE_NATIVE_PRIVATE_APPLICATION_OTHER_URL"))
	if err != nil || first.String() == second.String() {
		t.Fatal("two independent Synapse origins required", err)
	}
	const password = "artificial-disposable-native-storage-password"
	register := func(origin, username string) *mautrix.Client {
		client, err := mautrix.NewClient(origin, "", "")
		if err != nil {
			t.Fatal(err)
		}
		if username == "rooms_controller" {
			response, err := client.Login(ctx, &mautrix.ReqLogin{Type: mautrix.AuthTypePassword, Identifier: mautrix.UserIdentifier{Type: mautrix.IdentifierTypeUser, User: username}, Password: password})
			if err == nil {
				client.UserID, client.DeviceID, client.AccessToken = response.UserID, response.DeviceID, response.AccessToken
				return client
			}
		}
		matrixprooftest.Register(ctx, t, client, username, password, "REGISTRATION")
		return client
	}
	scope := storage.Scope{Tenant: "native-two-origins", Subject: uuid.NewString(), Product: "mail"}
	var operations atomic.Int32
	open := func(origin string, legacy bool) *Device {
		controller := register(origin, "rooms_controller")
		name := strings.TrimSuffix(strings.TrimPrefix(storage.IsolatedMatrixUserPrefix(scope), "@"), ":")
		if legacy {
			name = "sytue_" + scope.Subject
		}
		user := register(origin, name)
		room := matrixprooftest.EncryptedStorageRoom(ctx, t, controller, user.UserID)
		device, err := Open(ctx, t, Config{Scope: scope, Origin: origin, User: user, Controller: controller, Room: room, Password: password, Intercept: func(_ http.ResponseWriter, request *http.Request, _ *httputil.ReverseProxy) bool {
			if strings.Contains(request.URL.Path, "/rooms/") {
				operations.Add(1)
			}
			return false
		}})
		if err != nil {
			t.Fatal(err)
		}
		if err := device.Client.AcknowledgeRecoverySetup(ctx); err != nil {
			t.Fatal(err)
		}
		return device
	}
	source := open(first.String(), true)
	target := open(second.String(), false)
	if source.Config.Bootstrap.HomeserverURL == target.Config.Bootstrap.HomeserverURL {
		t.Fatal("two independent Synapse servers were unified by artificial routing")
	}
	publication, err := target.Client.StoragePublicationTarget()
	if err != nil {
		t.Fatal(err)
	}
	before := operations.Load()
	_, err = source.Client.MigrateStorageFileContentAt(ctx, target.Client, nil, nil, "never_copy", publication, storage.Reference{RoomID: source.Room.String(), EventID: "$unread"}, "object", "revision")
	if !errors.Is(err, mx.ErrIdentity) || operations.Load() != before {
		t.Fatal("foreign Synapse file migration reached SDK/provider IO", err, operations.Load()-before)
	}
}
