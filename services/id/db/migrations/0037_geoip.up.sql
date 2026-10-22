-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- UBICACIÓN APROXIMADA DE LOS ACCESOS. La lista de sesiones ya enseña la IP, pero
-- una IP no le dice a nadie si un acceso fue suyo; «Valencia, España» sí, y esa es
-- justo la pregunta que se hace quien entra a mirar sus accesos recientes.
--
-- POR QUÉ EN POSTGRES Y NO CON UNA LIBRERÍA GEOIP: leer un fichero MMDB obliga a
-- añadir una dependencia (maxminddb-golang) a un binario que hoy solo usa la
-- stdlib y pgx, y a montar el fichero en cada réplica. Postgres ya está, ya sabe
-- comparar direcciones con el tipo `inet`, y un índice resuelve la búsqueda en
-- microsegundos. Los datos quedan además DENTRO de nuestra infraestructura, que
-- es lo que se le promete a un cliente europeo.
--
-- RANGOS INICIO/FIN Y NO CIDR: las bases libres publican rangos arbitrarios
-- («1.2.3.0 a 1.2.5.255»), que casi nunca coinciden con un prefijo de red.
-- Guardarlos como cidr obligaría a descomponer cada uno en varios prefijos al
-- cargar; con dos columnas inet se guardan tal cual vienen.
--
-- LOS DATOS NO VIENEN EN ESTA MIGRACIÓN: la tabla nace VACÍA y se carga aparte
-- (ver deploy/geoip/). Con la tabla vacía la interfaz simplemente no muestra
-- ubicación, sin errores ni huecos: es una mejora opcional, no un requisito de
-- funcionamiento. Y así la licencia de la base elegida no se mezcla con el código.
BEGIN;

CREATE TABLE IF NOT EXISTS geoip_networks (
    ip_start     inet    NOT NULL,
    ip_end       inet    NOT NULL,
    country_code char(2) NOT NULL,
    country      text    NOT NULL,
    city         text    NOT NULL DEFAULT '',
    PRIMARY KEY (ip_start),
    -- Un rango invertido no casaría nunca y sería invisible: se rechaza al cargar.
    CONSTRAINT geoip_rango_valido CHECK (ip_end >= ip_start)
);

-- El índice que hace viable la consulta: sin él, resolver una IP recorre los
-- millones de filas. La búsqueda es «el último rango que empieza antes o en esta
-- IP», que un btree descendente resuelve con un solo salto.
--
-- inet ordena por FAMILIA primero, así que los rangos IPv4 y los IPv6 no se
-- entremezclan y la misma consulta vale para las dos sin ramas.
CREATE INDEX IF NOT EXISTS geoip_networks_start_idx ON geoip_networks (ip_start DESC);

COMMENT ON TABLE geoip_networks IS
    'Rangos de IP a país/ciudad para mostrar la ubicación aproximada de los accesos. '
    'Se carga con deploy/geoip/cargar.py; vacía es un estado válido (la interfaz omite la ubicación).';

COMMIT;
