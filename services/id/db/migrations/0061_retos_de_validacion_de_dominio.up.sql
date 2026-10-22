-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- RETOS DE VALIDACIÓN DE DOMINIO (DCV) PARA S/MIME.
--
-- Para que un correo firmado se valide SOLO en Gmail, Outlook y Apple Mail hace
-- falta un certificado de una CA pública, y las reglas del CA/Browser Forum
-- obligan a validar TODOS los dominios de las direcciones. Cada organización
-- tiene el suyo (`<slug>.sytue.eu`), así que hay una validación por organización.
--
-- El método que toda CA ofrece es el fichero en
-- `http://<dominio>/.well-known/pki-validation/<nombre>` con un contenido exacto.
-- Aquí se guarda ese par para poder servirlo; el nombre y el contenido los pone
-- la CA y por eso se guardan TAL CUAL, sin interpretarlos: cada emisor usa su
-- formato y adivinarlo sería atarnos a uno.
--
-- SE VALIDA POR HTTP Y NO POR DNS aunque la zona sea nuestra: el comodín
-- `*.sytue.eu` ya enruta cada organización, así que responder es inmediato y no
-- hay que esperar propagación —que en una emisión automática es la diferencia
-- entre acabar en segundos o en horas, y el motivo por el que un alta se
-- quedaría a medias sin que nadie sepa por qué—.
CREATE TABLE IF NOT EXISTS domain_validation_challenges (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id      uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  -- El dominio que se está validando, en minúsculas: `<slug>.sytue.eu` o el
  -- dominio propio de la organización.
  domain      text NOT NULL,
  -- Nombre del fichero bajo `/.well-known/pki-validation/`. Sin barras ni puntos
  -- dobles: lo comprueba también el handler, pero una ruta no se compone nunca
  -- con algo que no esté acotado en los dos sitios.
  name        text NOT NULL CHECK (name ~ '^[A-Za-z0-9._-]{1,128}$' AND name !~ '\.\.'),
  content     text NOT NULL CHECK (length(content) BETWEEN 1 AND 4096),
  created_at  timestamptz NOT NULL DEFAULT now(),
  -- CADUCAN. Un reto es para una emisión concreta; dejarlo servido para siempre
  -- es publicar un fichero que acredita control del dominio mucho después de que
  -- esa emisión terminara.
  expires_at  timestamptz NOT NULL,
  UNIQUE (domain, name)
);

-- NO HAY ÍNDICE DE BÚSQUEDA APARTE. El handler consulta
-- `WHERE domain = lower($1) AND name = $2 AND expires_at > now()`, y el UNIQUE de
-- arriba ya crea el btree sobre `(domain, name)`: resuelve a UNA fila y el
-- `expires_at` es un filtro sobre esa fila. Aquí hubo un índice parcial con
-- `WHERE expires_at > now()` que PostgreSQL rechaza —el predicado de un índice
-- tiene que ser IMMUTABLE y `now()` es STABLE—, y como el fallo deja la
-- migración «dirty», Sytue ID no volvía a arrancar.

CREATE INDEX IF NOT EXISTS domain_validation_challenges_org
  ON domain_validation_challenges (org_id);
