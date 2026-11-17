// SPDX-License-Identifier: Apache-2.0
package resource_test

import (
	"bytes"
	"encoding/base64"
	"testing"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/publication"
)

func TestResourceAdmissionMarkerDoesNotReplaceCapturedCipherOrHold(t *testing.T) {
	f, source, capture := prepareHeld(t, false)
	original := bytes.Clone(source.Held.Candidate.EncryptedEvent)
	marker := publication.Marker{Version: 1, ID: base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{3}, 32))}
	marked := source.Held.Candidate
	var err error
	marked.EncryptedEvent, err = publication.AttachMarker(original, marker)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := resource.CandidateDigest(marked)
	if err != nil || hash != source.Held.CandidateSHA256 {
		t.Fatal("late marker replaced prepared immutable candidate", err)
	}
	writer, err := resource.Authorize(f.Epoch, f.Cap, f.Scope.Subject, resource.Publish)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resource.VerifyProducerCandidate(t.Context(), marked, writer, f.Device); err != nil {
		t.Fatal("exact SDK cipher with admission rejected", err)
	}
	restored, err := resource.RestoreReservation(t.Context(), source.Held.Resource, source.Held.OpID, source)
	if err != nil {
		t.Fatal(err)
	}
	reference := storage.Reference{RoomID: marked.Proof.RoomID, EventID: "$marked-accepted"}
	accepted, err := resource.ConfirmReservedAcceptance(t.Context(), restored, reference, acceptanceFixture(func(requested storage.Reference) (*event.Event, error) {
		return &event.Event{ID: id.EventID(reference.EventID), RoomID: id.RoomID(reference.RoomID), Sender: id.UserID(marked.Proof.Actor.UserID), Type: event.EventEncrypted, Content: event.Content{VeryRaw: marked.EncryptedEvent}}, nil
	}))
	if err != nil || accepted.ReservationSHA256() != capture.SHA256() {
		t.Fatal("marked accepted event did not resolve original hold", err)
	}
	if !bytes.Equal(original, source.Held.Candidate.EncryptedEvent) {
		t.Fatal("attaching transport admission mutated durable ciphertext")
	}
}
