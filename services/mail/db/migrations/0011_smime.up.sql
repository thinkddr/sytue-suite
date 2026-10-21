-- S/MIME sin coste: una autoridad por organización y un certificado por buzón,
-- emitidos y renovados solos. Las claves privadas viven CIFRADAS (AES-256-GCM
-- con la KEK del servicio); la columna nunca guarda material en claro.
--
-- Por qué una autoridad propia: no existe hoy ninguna CA pública que emita
-- certificados S/MIME de dominio gratis y automatizados, y pagar por buzón y
-- año es justamente lo que ha hecho que S/MIME no se use nunca. Dentro de la
-- organización, su ancla se distribuye una vez y la confianza es real.

CREATE TABLE smime_authorities (
    tenant      text        PRIMARY KEY,
    cert_pem    text        NOT NULL,
    key_enc     text        NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE smime_identities (
    account_id  uuid        PRIMARY KEY REFERENCES accounts (id) ON DELETE CASCADE,
    tenant      text        NOT NULL,
    address     text        NOT NULL,
    cert_pem    text        NOT NULL,
    key_enc     text        NOT NULL,
    not_after   timestamptz NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

-- La renovación automática busca por vencimiento.
CREATE INDEX smime_identities_not_after_idx ON smime_identities (tenant, not_after);

-- RLS deny-by-default por organización, igual que el resto del servicio: el
-- material criptográfico de una organización jamás se cruza con el de otra.
ALTER TABLE smime_authorities ENABLE ROW LEVEL SECURITY;
ALTER TABLE smime_authorities FORCE ROW LEVEL SECURITY;
CREATE POLICY smime_authorities_tenant ON smime_authorities
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

ALTER TABLE smime_identities ENABLE ROW LEVEL SECURITY;
ALTER TABLE smime_identities FORCE ROW LEVEL SECURITY;
CREATE POLICY smime_identities_tenant ON smime_identities
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
