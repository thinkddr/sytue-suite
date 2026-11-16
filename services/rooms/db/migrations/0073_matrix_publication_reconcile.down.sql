-- SPDX-License-Identifier: Apache-2.0
-- Removing maintenance visibility never removes retained publication evidence.
DROP POLICY matrix_storage_publications_reconcile ON matrix_storage_publications;
