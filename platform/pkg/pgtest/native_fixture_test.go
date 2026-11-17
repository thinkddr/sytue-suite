// SPDX-License-Identifier: Apache-2.0
package pgtest

import (
	"bytes"
	"os"
	"slices"
	"testing"
	"time"
)

func TestNativeFixtureRejectsRemoteEndpointsAndCredentialsBeforeExecution(t *testing.T) {
	base := NativeFixture{Module: "apps/nucleo", Package: "./matrix", Test: "TestSynthetic", Marker: "SYNTHETIC_PASS", Timeout: time.Minute}
	for _, env := range []map[string]string{
		{"SYTUE_FIXTURE_URL": "https://production.invalid"},
		{"SYTUE_FIXTURE_URL": "http://user:password@127.0.0.1:8080"},
		{"SYTUE_FIXTURE_URL": "http://127.0.0.1:8080/?secret=value"},
		{"SYTUE_FIXTURE_TOKEN": "synthetic"},
		{"PGTEST_DSN": "synthetic"},
	} {
		f := base
		f.Env = env
		if f.validate() == nil {
			t.Fatal("unsafe fixture admitted")
		}
	}
	base.Env = map[string]string{"SYTUE_FIXTURE_URL": "http://127.0.0.1:8080", "SYTUE_FIXTURE_SCOPE": `{"tenant":"synthetic"}`}
	if err := base.validate(); err != nil {
		t.Fatal(err)
	}
	worker := base
	worker.Package = "./ingress"
	if err := worker.validate(); err != nil {
		t.Fatal("explicit ingress worker fixture rejected", err)
	}
	native := base
	native.Package, native.Module = "./cifrado", "../../../../apps/fyne"
	if err := native.validate(); err != nil {
		t.Fatal("bounded native application fixture rejected", err)
	}
	native.Module = "../../../../another-module"
	if native.validate() == nil {
		t.Fatal("native application fixture escaped its maintained module")
	}
	native.Module, native.Package, native.Test = "../../../../apps/fyne", "./cmd/notes", "TestPrivateNotepadsApplicationAgainstPostgresAndFreshDevice"
	if err := native.validate(); err != nil {
		t.Fatal("exact Notepads native application fixture rejected", err)
	}
	native.Test = "TestArbitraryEntryPoint"
	if native.validate() == nil {
		t.Fatal("Notepads fixture escaped its exact entry point")
	}
	for _, key := range []string{"../matrix", "./...", "-exec=other"} {
		f := base
		f.Package = key
		if f.validate() == nil {
			t.Fatal("arbitrary native package admitted")
		}
	}
}

func TestNativeFixtureEnvironmentDropsCredentialsAndOtherFixtures(t *testing.T) {
	env := nativeFixtureEnvironment([]string{"PATH=/synthetic", "GOFLAGS=-tags=integration", "PGTEST_DSN=private", "AWS_SECRET_ACCESS_KEY=private", "SYTUE_PREVIOUS_FIXTURE=private"}, map[string]string{"SYTUE_CURRENT_FIXTURE": "1"})
	if len(env) != 3 || !slices.Contains(env, "PATH=/synthetic") || !slices.Contains(env, "GOFLAGS=-tags=integration") || !slices.Contains(env, "SYTUE_CURRENT_FIXTURE=1") {
		t.Fatal("native environment leaked credentials or lost its toolchain")
	}
}

func TestNativeFixtureLogBoundsBothSingleAndRepeatedWrites(t *testing.T) {
	var b nativeFixtureLog
	large := bytes.Repeat([]byte("x"), 20<<10)
	if n, err := b.Write(large); n != len(large) || err != nil {
		t.Fatal("log write failed")
	}
	for range 20 {
		_, _ = b.Write(bytes.Repeat([]byte("y"), 1024))
	}
	if b.total != 40<<10 || len(b.tail) != 8192 || !bytes.Equal(b.tail, bytes.Repeat([]byte("y"), 8192)) {
		t.Fatal("native output was not bounded to its final bytes")
	}
}

func TestNativeFixtureFailureDiagnosticIsPrivate(t *testing.T) {
	name := nativeFixtureDiagnostic([]byte("synthetic diagnostic"))
	if name == "unavailable" {
		t.Fatal("cannot retain fixture diagnostic")
	}
	t.Cleanup(func() { _ = os.Remove(name) })
	info, err := os.Stat(name)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatal("fixture diagnostic is not private", err)
	}
	data, err := os.ReadFile(name)
	if err != nil || !bytes.Equal(data, []byte("synthetic diagnostic")) {
		t.Fatal("fixture diagnostic changed", err)
	}
}

func TestNativeFixtureExtendedTimeoutIsExclusiveToMailWebComposite(t *testing.T) {
	composite := NativeFixture{Module: "../../../../apps/fyne", Package: "./cifrado", Test: "TestNativeMailApplicationAgainstAuthorityQueueAndSynapse", Marker: "NATIVE_MAIL_AUTHORITY_QUEUE_SDK_MIME_PASS", Timeout: 14 * time.Minute, Env: map[string]string{
		"SYTUE_MAIL_WEB_PROOF": "1", "SYTUE_MAIL_WEB_PROOF_PARENT": "TestPrivateIngressLiveNativeMailSharedOutbound", "SYTUE_INGRESS_DEVICE_PROOF": "1", "SYTUE_INGRESS_HANDOFF_CASE": "native-mail-sql", "SYTUE_MAIL_SHARED_OUTBOUND_PROOF": "1",
	}}
	if err := composite.validate(); err != nil {
		t.Fatal("exact measured composite rejected", err)
	}
	ceiling := composite
	ceiling.Timeout = 15 * time.Minute
	if err := ceiling.validate(); err != nil {
		t.Fatal("bounded composite ceiling rejected", err)
	}
	ceiling.Timeout += time.Nanosecond
	if ceiling.validate() == nil {
		t.Fatal("composite timeout exceeded fifteen minutes")
	}
	for _, field := range []string{"module", "package", "test", "marker", "SYTUE_MAIL_WEB_PROOF", "SYTUE_MAIL_WEB_PROOF_PARENT", "SYTUE_INGRESS_DEVICE_PROOF", "SYTUE_INGRESS_HANDOFF_CASE", "SYTUE_MAIL_SHARED_OUTBOUND_PROOF"} {
		t.Run(field, func(t *testing.T) {
			f := composite
			f.Env = make(map[string]string, len(composite.Env))
			for key, value := range composite.Env {
				f.Env[key] = value
			}
			switch field {
			case "module":
				f.Module = "apps/nucleo"
			case "package":
				f.Package = "./matrix"
			case "test":
				f.Test = "TestAnotherFixture"
			case "marker":
				f.Marker = "ANOTHER_PASS"
			default:
				delete(f.Env, field)
			}
			if f.validate() == nil {
				t.Fatal("another fixture inherited composite timeout")
			}
			f.Timeout = 10 * time.Minute
			if field != "module" {
				if err := f.validate(); err != nil {
					t.Fatal("general ten-minute allowance changed", err)
				}
			}
		})
	}
	for _, value := range []string{"true", " 1", "1\n", ""} {
		f := composite
		f.Env = make(map[string]string, len(composite.Env))
		for key, entry := range composite.Env {
			f.Env[key] = entry
		}
		f.Env["SYTUE_MAIL_WEB_PROOF"] = value
		if f.validate() == nil {
			t.Fatal("malformed composite flag admitted")
		}
	}
	unsafe := composite
	unsafe.Env = map[string]string{"SYTUE_FIXTURE_TOKEN": "private"}
	if unsafe.validate() == nil {
		t.Fatal("extended timeout bypassed environment validation")
	}
}

func TestNativeFixtureBrowserFullSourceRequiresExactTestMarkerAndParent(t *testing.T) {
	original := NativeFixture{Module: "../../../../apps/fyne", Package: "./cifrado", Test: "TestNativeMailBrowserFullSourceAgainstAuthorityAndSynapse", Marker: "NATIVE_MAIL_BROWSER_FULL_SOURCE_MIME_RELOAD_RETAINED_ACK_PASS", Timeout: 14 * time.Minute, Env: map[string]string{"SYTUE_MAIL_WEB_PROOF": "1", "SYTUE_MAIL_WEB_PROOF_PARENT": "TestPrivateIngressLiveNativeMailBrowserFullSource", "SYTUE_INGRESS_DEVICE_PROOF": "1", "SYTUE_INGRESS_HANDOFF_CASE": "native-mail-sql", "SYTUE_MAIL_SHARED_OUTBOUND_PROOF": "1"}}
	if err := original.validate(); err != nil {
		t.Fatal("exact BrowserFull fixture rejected", err)
	}
	for _, field := range []string{"test", "marker", "parent", "proof", "handoff", "package"} {
		t.Run(field, func(t *testing.T) {
			value := original
			value.Env = make(map[string]string, len(original.Env))
			for k, v := range original.Env {
				value.Env[k] = v
			}
			switch field {
			case "test":
				value.Test = "TestNativeMailApplicationAgainstAuthorityQueueAndSynapse"
			case "marker":
				value.Marker = "NATIVE_MAIL_AUTHORITY_QUEUE_SDK_MIME_PASS"
			case "parent":
				value.Env["SYTUE_MAIL_WEB_PROOF_PARENT"] = "TestPrivateIngressLiveNativeMailSharedOutbound"
			case "proof":
				delete(value.Env, "SYTUE_MAIL_WEB_PROOF")
			case "handoff":
				value.Env["SYTUE_INGRESS_HANDOFF_CASE"] = "native-mail"
			case "package":
				value.Package = "./matrix"
			}
			if value.validate() == nil {
				t.Fatal("BrowserFull inherited another case's authorization", field)
			}
		})
	}
}
