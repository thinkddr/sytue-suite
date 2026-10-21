-- Color por evento (RFC 7986 COLOR / X-APPLE-CALENDAR-COLOR): proyeccion
-- consultable del color del VEVENT para render en la UI. El recurso CalDAV se
-- sirve verbatim desde calendar_objects.data, asi que el round-trip del color no
-- depende de esta columna; aqui solo se expone a la API REST (instancias/evento).
-- Hex #RRGGBB de la paleta de la suite o nombre CSS3; '' = hereda el del calendario.
ALTER TABLE events ADD COLUMN color text NOT NULL DEFAULT '';
