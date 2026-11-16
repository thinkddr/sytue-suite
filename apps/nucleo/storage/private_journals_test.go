// SPDX-License-Identifier: Apache-2.0
package storage

import (
	"slices"
	"testing"
)

func TestPrivateJournalRegistryUsesProductCatalogAndPreservesMailStages(t *testing.T) {
	all := AllPrivateJournalNamespaces()
	seen := map[string]bool{}
	for _, namespace := range all {
		if namespace == "" || seen[namespace] {
			t.Fatal("ambiguous durable namespace", namespace)
		}
		seen[namespace] = true
	}
	for _, product := range Products() {
		for _, namespace := range []string{ResourcePublicationNamespace, ResourceBootstrapPublicationNamespace} {
			if !slices.Contains(PrivateJournalNamespaces(product), namespace) || !slices.Contains(all, namespace) {
				t.Fatal("resource prepared work omitted", product)
			}
		}
		if !slices.Contains(PrivateStagingNamespaces(product), ResourcePublicationHistoryNamespace) || slices.Contains(all, ResourcePublicationHistoryNamespace) {
			t.Fatal("resource completion history misclassified", product)
		}
		if !slices.Contains(PrivateJournalNamespaces(product), ResourceBootstrapRecoveryNamespace) || !slices.Contains(all, ResourceBootstrapRecoveryNamespace) {
			t.Fatal("resource recovery retention omitted", product)
		}
		if !slices.Contains(PrivateStagingNamespaces(product), AtomicHistoryNamespace(product)) || slices.Contains(PrivateJournalNamespaces(product), AtomicHistoryNamespace(product)) || slices.Contains(all, AtomicHistoryNamespace(product)) {
			t.Fatal("atomic completion history misclassified", product)
		}
		if !slices.Contains(PrivateJournalNamespaces(product), AtomicJournalNamespace(product)) || !slices.Contains(all, AtomicJournalNamespace(product)) {
			t.Fatal("atomic root/member/page work omitted", product)
		}
		if !slices.Contains(PrivateJournalNamespaces(product), NativePublicationNamespace) || !slices.Contains(PrivateStagingNamespaces(product), NativePublicationHistoryNamespace) || slices.Contains(all, NativePublicationHistoryNamespace) {
			t.Fatal("native prepared publication inventory/history misclassified", product)
		}
		if !slices.Contains(PrivateJournalNamespaces(product), PrivateIngressNamespace) || !slices.Contains(PrivateStagingNamespaces(product), PrivateIngressStagingNamespace) || slices.Contains(all, PrivateIngressStagingNamespace) {
			t.Fatal("common ingress work/staging omitted or counted incorrectly", product)
		}
		if !slices.Contains(PrivateJournalNamespaces(product), IdentityJournalNamespace(product)) || !slices.Contains(PrivateStagingNamespaces(product), IdentityHistoryNamespace(product)) || slices.Contains(all, IdentityHistoryNamespace(product)) {
			t.Fatal("identity work or retained history misclassified", product)
		}
		if len(PrivateJournalNamespaces(product)) == 0 {
			t.Fatal("product work omitted", product)
		}
	}
	if AtomicHistoryNamespace("invalid") != "" {
		t.Fatal("unknown product gained atomic history")
	}
	if len(PrivateJournalNamespaces("invalid")) != 0 {
		t.Fatal("unknown product gained private journal")
	}
	if len(PrivateStagingNamespaces("invalid")) != 0 {
		t.Fatal("unknown product gained staging storage")
	}
	for _, product := range []string{"photos", "notes", "calendar", "contacts"} {
		stages := PrivateStagingNamespaces(product)
		expected := 6
		if product == "calendar" {
			expected++
		}
		if len(stages) != expected || stages[0] != product+"-private-stage-v1" || slices.Contains(all, stages[0]) {
			t.Fatal("record stages omitted or counted as work", product)
		}
	}
	for _, namespace := range []string{MailOutboundNamespace, "mail-private-writes-v1", "mail-private-ingest-v1", "mail-private-source-import-v1", PendingUploadsNamespace} {
		if !slices.Contains(all, namespace) {
			t.Fatal("durable work omitted", namespace)
		}
	}
	stages := PrivateStagingNamespaces("mail")
	if !slices.Equal(stages, []string{MailWriteHistoryNamespace, MailDraftRevisionStagingNamespace, MailOutboundStagingNamespace, "mail-private-source-stage-v1", IdentityHistoryNamespace("mail"), PrivateIngressStagingNamespace, PrivateIngressEnrollmentHistoryNamespace, NativePublicationHistoryNamespace, ResourcePublicationHistoryNamespace, AtomicHistoryNamespace("mail")}) {
		t.Fatal("Mail retained source catalog changed")
	}
	for _, namespace := range stages {
		if slices.Contains(all, namespace) {
			t.Fatal("source blocks counted as messages", namespace)
		}
	}
	if !slices.Contains(PrivateStagingNamespaces("drive"), "drive-private-cutover-history-v1") || slices.Contains(all, "drive-private-cutover-history-v1") {
		t.Fatal("acknowledged Drive history is lost or counted as pending work")
	}
	for _, product := range []string{"mail", "calendar"} {
		if !slices.Contains(PrivateJournalNamespaces(product), PrivateIngressEnrollmentNamespace) || !slices.Contains(PrivateStagingNamespaces(product), PrivateIngressEnrollmentHistoryNamespace) || slices.Contains(all, PrivateIngressEnrollmentHistoryNamespace) {
			t.Fatal("recipient enrollment work/history misclassified", product)
		}
	}
	all[0] = "mutated"
	if AllPrivateJournalNamespaces()[0] != PendingUploadsNamespace {
		t.Fatal("registry exposes mutable shared data")
	}
}
