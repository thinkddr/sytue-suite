-- SPDX-License-Identifier: Apache-2.0
-- Ciphertext only. Keys and private manifests are held by verified clients.
CREATE TABLE cipher_blobs (
    tenant text NOT NULL,
    owner_sub text NOT NULL CHECK (owner_sub <> ''),
    product text NOT NULL CHECK (product IN ('hub','mail','drive','docs','sheets','slides','pdf','calendar','contacts','notes','photos','rooms','cast','erp','bases')),
    id text NOT NULL CHECK (id ~ '^[A-Z2-7]{26,64}$'),
    storage_key text NOT NULL,
    size_bytes bigint NOT NULL CHECK (size_bytes BETWEEN 0 AND 8388608),
    sha256 text NOT NULL CHECK (sha256 ~ '^[a-f0-9]{64}$'),
    created_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    PRIMARY KEY (tenant,owner_sub,product,id),
    UNIQUE (tenant,storage_key)
);
ALTER TABLE cipher_blobs ENABLE ROW LEVEL SECURITY;
ALTER TABLE cipher_blobs FORCE ROW LEVEL SECURITY;
CREATE POLICY cipher_blobs_owner ON cipher_blobs
    USING (tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.owner_sub',true))
    WITH CHECK (tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.owner_sub',true));
CREATE POLICY cipher_blobs_maintenance ON cipher_blobs
    USING (current_setting('app.maintenance',true)='on')
    WITH CHECK (current_setting('app.maintenance',true)='on');
CREATE POLICY cipher_blobs_platform_merge ON cipher_blobs
    USING (current_setting('app.merge',true)='on')
    WITH CHECK (current_setting('app.merge',true)='on');
