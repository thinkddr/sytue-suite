// SPDX-License-Identifier: Apache-2.0
package storage

import "slices"

const (
	ResourcePublicationNamespace             = "resource-publication-prepared-v1"
	ResourcePublicationHistoryNamespace      = "resource-publication-history-v1"
	ResourceBootstrapPublicationNamespace    = "resource-bootstrap-prepared-v1"
	ResourceBootstrapRecoveryNamespace       = "resource-bootstrap-recovery-v1"
	NativePublicationNamespace               = "native-publication-prepared-v1"
	NativePublicationHistoryNamespace        = "native-publication-history-v1"
	MailOutboundNamespace                    = "mail-private-outbound-v1"
	MailOutboundStagingNamespace             = "mail-private-outbound-stage-v1"
	MailOutboundReceiptNamespace             = "mail-private-outbound-receipts-v1"
	MailWriteHistoryNamespace                = "mail-private-writes-history-v1"
	MailDraftRevisionNamespace               = "mail-private-draft-revisions-v1"
	MailDraftRevisionStagingNamespace        = "mail-private-draft-revision-stage-v1"
	PrivateIngressNamespace                  = "private-ingress-v1"
	PrivateIngressStagingNamespace           = "private-ingress-stage-v1"
	PrivateIngressEnrollmentNamespace        = "private-ingress-enrollment-v1"
	PrivateIngressEnrollmentHistoryNamespace = "private-ingress-enrollment-history-v1"
)

// IdentityJournalNamespace and IdentityHistoryNamespace retain the existing
// Drive routing contract for every application using the shared transfer.
func IdentityJournalNamespace(product string) string {
	if !Product(product) {
		return ""
	}
	return product + "-identity-migration-v1"
}

func IdentityHistoryNamespace(product string) string {
	if !Product(product) {
		return ""
	}
	return product + "-private-cutover-history-v1"
}

// AtomicJournalNamespace retains roots, all member intents and manifest pages.
// Every row is pending work until exact accepted-root and member cleanup finish.
func AtomicJournalNamespace(product string) string {
	if !Product(product) {
		return ""
	}
	return product + "-private-atomic-work-v1"
}

// AtomicHistoryNamespace retains accepted/finalizing batch receipts. Roots and
// member work stay pending until their verified cleanup is complete; this retained
// history is never itself counted as an additional unsent operation.
func AtomicHistoryNamespace(product string) string {
	if !Product(product) {
		return ""
	}
	return product + "-private-atomic-history-v1"
}

// PrivateJournalNamespaces is the shared durable-work routing registry. The
// products come from the existing catalog; applications do not copy this list.
// File uploads are separate because every product uses the maintained SDK WAL.
func privateJournalNamespaces(product string) []string {
	if !Product(product) {
		return nil
	}
	if product == "drive" {
		return []string{"drive-private-migration-v1", "drive-identity-migration-v1", PrivateIngressNamespace}
	}
	if product == "mail" {
		return []string{MailDraftRevisionNamespace, MailOutboundNamespace, "mail-private-writes-v1", "mail-private-ingest-v1", "mail-private-source-import-v1", IdentityJournalNamespace(product), PrivateIngressNamespace, PrivateIngressEnrollmentNamespace}
	}
	namespaces := []string{product + "-private-v1", IdentityJournalNamespace(product), PrivateIngressNamespace}
	if product == "calendar" {
		namespaces = append(namespaces, PrivateIngressEnrollmentNamespace)
	}
	return namespaces
}

// PrivateStagingNamespaces are retained local source blocks, not independent
// unsent operations. They must survive an aborted shutdown without inflating the
// number of messages shown to the user.
func privateStagingNamespaces(product string) []string {
	if !Product(product) {
		return nil
	}
	if product == "drive" {
		return []string{"drive-private-upload-source-v1", "drive-private-cutover-history-v1", PrivateIngressStagingNamespace}
	}
	if product == "mail" {
		return []string{MailWriteHistoryNamespace, MailDraftRevisionStagingNamespace, MailOutboundStagingNamespace, "mail-private-source-stage-v1", IdentityHistoryNamespace(product), PrivateIngressStagingNamespace, PrivateIngressEnrollmentHistoryNamespace}
	}
	namespaces := []string{product + "-private-stage-v1", IdentityHistoryNamespace(product), PrivateIngressStagingNamespace}
	if product == "calendar" {
		namespaces = append(namespaces, PrivateIngressEnrollmentHistoryNamespace)
	}
	return namespaces
}

func AllPrivateJournalNamespaces() []string {
	namespaces := make([]string, 1, 1+2*len(productIDs))
	namespaces[0] = PendingUploadsNamespace
	for _, product := range Products() {
		for _, namespace := range PrivateJournalNamespaces(product) {
			if !slices.Contains(namespaces, namespace) {
				namespaces = append(namespaces, namespace)
			}
		}
	}
	return namespaces
}

// Native SDK preparation is retained before reserve. Accepted wire history is
// staging, while an unresolved publication must block private-work discard.
func PrivateJournalNamespaces(product string) []string {
	namespaces := privateJournalNamespaces(product)
	if namespaces != nil {
		namespaces = append(namespaces, NativePublicationNamespace, AtomicJournalNamespace(product), ResourceBootstrapRecoveryNamespace, ResourcePublicationNamespace, ResourceBootstrapPublicationNamespace)
	}
	return namespaces
}

func PrivateStagingNamespaces(product string) []string {
	namespaces := privateStagingNamespaces(product)
	if namespaces != nil {
		namespaces = append(namespaces, NativePublicationHistoryNamespace, ResourcePublicationHistoryNamespace, AtomicHistoryNamespace(product))
	}
	return namespaces
}
