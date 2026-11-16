// SPDX-License-Identifier: Apache-2.0
package storage

import (
	"slices"
	"testing"
)

func TestMailWriteHistoryIsRetainedStagingNotPending(t *testing.T) {
	if !slices.Contains(PrivateStagingNamespaces("mail"), MailWriteHistoryNamespace) || slices.Contains(AllPrivateJournalNamespaces(), MailWriteHistoryNamespace) {
		t.Fatal("accepted Mail history became unsent work or lost staging routing")
	}
	for _, product := range Products() {
		if product != "mail" && slices.Contains(PrivateStagingNamespaces(product), MailWriteHistoryNamespace) {
			t.Fatal("Mail history routed across products")
		}
	}
}
