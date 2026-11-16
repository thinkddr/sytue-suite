// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"encoding/json"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestReceiptInventoryRequiresCommittedAtomicRevision(t *testing.T) {
	for _, committed := range []bool{false, true} {
		t.Run(map[bool]string{false: "orphan", true: "committed"}[committed], func(t *testing.T) {
			crypto := newCryptoFixture()
			old := crypto.seed(t, "mailbox", "inbox", map[string]string{"id": "inbox"})
			oldRecord := crypto.records[old]
			var commit storage.ObjectCommit
			if err := json.Unmarshal(oldRecord.Payload, &commit); err != nil {
				t.Fatal(err)
			}
			commit.Parents = []storage.Reference{old}
			preparedRef := storage.Reference{RoomID: old.RoomID, EventID: "$prepared"}
			payload, err := json.Marshal(storage.AtomicPrepared{Kind: storage.ObjectPreparedKind, BatchID: "batch", Commit: commit})
			if err != nil {
				t.Fatal(err)
			}
			prepared := storage.Record{Version: 1, Scope: testScope, ObjectID: oldRecord.ObjectID, RevisionID: "prepared", Role: "index", Previous: []string{old.EventID}, Payload: payload}
			crypto.records[preparedRef] = prepared
			if committed {
				sum, err := storage.AtomicRecordSHA256(prepared)
				if err != nil {
					t.Fatal(err)
				}
				pageRef := storage.Reference{RoomID: old.RoomID, EventID: "$page"}
				page := storage.AtomicBatchPage{Kind: storage.ObjectBatchPageKind, BatchID: "batch", Members: []storage.AtomicMember{{Reference: preparedRef, ObjectID: prepared.ObjectID, RevisionID: prepared.RevisionID, SHA256: sum}}}
				pagePayload, _ := json.Marshal(page)
				pageRecord := storage.Record{Version: 1, Scope: testScope, ObjectID: storage.AtomicBatchObjectID("batch"), RevisionID: "page", Role: "manifest", Payload: pagePayload}
				pageSum, err := storage.AtomicRecordSHA256(pageRecord)
				if err != nil {
					t.Fatal(err)
				}
				crypto.records[pageRef] = pageRecord
				root := storage.AtomicBatchCommit{Kind: storage.ObjectBatchKind, BatchID: "batch", Count: 1, Last: storage.AtomicPagePointer{Reference: pageRef, RevisionID: "page", SHA256: pageSum}}
				rootPayload, _ := json.Marshal(root)
				crypto.records[storage.Reference{RoomID: old.RoomID, EventID: "$root"}] = storage.Record{Version: 1, Scope: testScope, ObjectID: storage.AtomicBatchObjectID("batch"), RevisionID: "root", Role: "index", Payload: rootPayload}
			}
			snapshot, err := storage.LoadAtomicCatalog(t.Context(), crypto, testScope)
			if err != nil {
				t.Fatal(err)
			}
			repo := &Repository{cfg: Config{Scope: testScope}}
			versions, _, _, err := repo.receiptInventory(snapshot, ImportReceipt{Records: []storage.Reference{preparedRef}})
			if committed && (err != nil || len(versions) != 1 || versions[0].Reference != preparedRef) {
				t.Fatalf("committed member missing: %v", err)
			}
			if !committed && err == nil {
				t.Fatal("orphan prepared revision was accepted")
			}
		})
	}
}
