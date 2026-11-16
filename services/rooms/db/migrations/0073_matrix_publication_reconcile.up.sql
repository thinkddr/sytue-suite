-- SPDX-License-Identifier: Apache-2.0
-- Read-only, explicit maintenance access to public pending origin metadata.
-- The owner policy and immutable write trigger still govern every transition.
CREATE POLICY matrix_storage_publications_reconcile
 ON matrix_storage_publications FOR SELECT
 USING(current_setting('app.maintenance',true)='on'
  AND current_setting('app.matrix_publication_reconcile',true)='1');
