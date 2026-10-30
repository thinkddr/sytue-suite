-- SPDX-License-Identifier: Apache-2.0
-- The app destination is fixed before browser authorization, never taken from
-- an untrusted return_to query or a callback supplied after consent.
ALTER TABLE matrix_native_consents ADD COLUMN product text NOT NULL DEFAULT 'rooms'
    CHECK (product IN ('hub','mail','drive','docs','sheets','slides','pdf','calendar','contacts','notes','photos','rooms','cast','erp','bases'));
