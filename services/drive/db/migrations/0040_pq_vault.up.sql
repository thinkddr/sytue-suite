-- SPDX-License-Identifier: Apache-2.0
-- Bóveda post-cuántica directa (ADR 0086): solo texto cifrado (`age-MLKEM768-X25519-v1`)
-- y envoltorios públicos cerrados con la clave de desbloqueo de Sytue ID.
-- Las tres tablas son scope-bound: su AAD autentica (tenant, product, owner_sub, key_id)
-- y una fusión SQL nunca puede reetiquetar su tenant.
-- sytue:scope-bound-merge-v1

CREATE TABLE pq_vaults (
    tenant text NOT NULL,
    owner_sub text NOT NULL CHECK (owner_sub <> ''),
    product text NOT NULL CHECK (product IN ('hub','mail','drive','docs','sheets','slides','pdf','calendar','contacts','notes','photos','rooms','cast','erp','bases')),
    vault_id text NOT NULL CHECK (vault_id ~ '^!pqv_[A-Z2-7]{26,64}$'),
    active_key_id text NOT NULL CHECK (active_key_id ~ '^[a-f0-9]{64}$'),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant,owner_sub,product),
    UNIQUE (tenant,vault_id)
);
CREATE POLICY pq_vaults_owner ON pq_vaults
    USING (tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.owner_sub',true))
    WITH CHECK (tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.owner_sub',true));
CREATE POLICY pq_vaults_maintenance ON pq_vaults
    USING (current_setting('app.maintenance',true)='on')
    WITH CHECK (current_setting('app.maintenance',true)='on');
SELECT sytue_scope_merge_register('pq_vaults'::regclass);
SELECT sytue_scope_admission_register('pq_vaults'::regclass);

CREATE TABLE pq_vault_keys (
    tenant text NOT NULL,
    owner_sub text NOT NULL CHECK (owner_sub <> ''),
    product text NOT NULL CHECK (product IN ('hub','mail','drive','docs','sheets','slides','pdf','calendar','contacts','notes','photos','rooms','cast','erp','bases')),
    key_id text NOT NULL CHECK (key_id ~ '^[a-f0-9]{64}$'),
    profile text NOT NULL CHECK (profile = 'age-MLKEM768-X25519-v1'),
    recipient text NOT NULL CHECK (length(recipient) BETWEEN 1 AND 4096),
    wrapper_json jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant,owner_sub,product,key_id)
);
CREATE POLICY pq_vault_keys_owner ON pq_vault_keys
    USING (tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.owner_sub',true))
    WITH CHECK (tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.owner_sub',true));
CREATE POLICY pq_vault_keys_maintenance ON pq_vault_keys
    USING (current_setting('app.maintenance',true)='on')
    WITH CHECK (current_setting('app.maintenance',true)='on');
SELECT sytue_scope_merge_register('pq_vault_keys'::regclass);
SELECT sytue_scope_admission_register('pq_vault_keys'::regclass);

CREATE TABLE pq_vault_records (
    tenant text NOT NULL,
    owner_sub text NOT NULL CHECK (owner_sub <> ''),
    product text NOT NULL CHECK (product IN ('hub','mail','drive','docs','sheets','slides','pdf','calendar','contacts','notes','photos','rooms','cast','erp','bases')),
    vault_id text NOT NULL CHECK (vault_id ~ '^!pqv_[A-Z2-7]{26,64}$'),
    seq bigint GENERATED ALWAYS AS IDENTITY,
    event_id text NOT NULL CHECK (event_id ~ '^\$pqe_[A-Z2-7]{26,64}$'),
    transaction_id text NOT NULL CHECK (length(transaction_id) BETWEEN 1 AND 255),
    payload_json jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant,owner_sub,product,seq),
    UNIQUE (tenant,owner_sub,product,event_id),
    UNIQUE (tenant,owner_sub,product,transaction_id)
);
CREATE INDEX idx_pq_vault_records_hist ON pq_vault_records (tenant,owner_sub,product,vault_id,seq DESC);
CREATE POLICY pq_vault_records_owner ON pq_vault_records
    USING (tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.owner_sub',true))
    WITH CHECK (tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.owner_sub',true));
CREATE POLICY pq_vault_records_maintenance ON pq_vault_records
    USING (current_setting('app.maintenance',true)='on')
    WITH CHECK (current_setting('app.maintenance',true)='on');
SELECT sytue_scope_merge_register('pq_vault_records'::regclass);
SELECT sytue_scope_admission_register('pq_vault_records'::regclass);
