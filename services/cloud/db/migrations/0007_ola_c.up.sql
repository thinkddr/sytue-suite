-- Ola C: credenciales de repo privado (deploy key cifrada) por proyecto.
ALTER TABLE projects
    ADD COLUMN IF NOT EXISTS repo_creds_enc bytea;
