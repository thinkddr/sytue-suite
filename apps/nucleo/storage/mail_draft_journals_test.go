// SPDX-License-Identifier: Apache-2.0
package storage

import (
	"slices"
	"testing"
)

func TestMailDraftRevisionRetainsIntentAndSourceInTheirRespectiveRegistries(t *testing.T) {
	if !slices.Contains(PrivateJournalNamespaces("mail"), MailDraftRevisionNamespace) || !slices.Contains(AllPrivateJournalNamespaces(), MailDraftRevisionNamespace) {
		t.Fatal("admitted draft revisions escaped retained work ownership")
	}
	if !slices.Contains(PrivateStagingNamespaces("mail"), MailDraftRevisionStagingNamespace) || slices.Contains(AllPrivateJournalNamespaces(), MailDraftRevisionStagingNamespace) {
		t.Fatal("draft source retention was confused with independent unsent operations")
	}
	for _, product := range Products() {
		if product == "mail" {
			continue
		}
		if slices.Contains(PrivateJournalNamespaces(product), MailDraftRevisionNamespace) || slices.Contains(PrivateStagingNamespaces(product), MailDraftRevisionStagingNamespace) {
			t.Fatal("draft revisions crossed a product boundary", product)
		}
	}
}
