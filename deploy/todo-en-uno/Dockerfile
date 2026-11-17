# syntax=docker/dockerfile:1
# Sytue Suite — TODO EN UN CONTENEDOR ==========================================
# La suite completa dentro de un solo contenedor: un Docker interno (Docker in
# Docker) ejecuta ./sytue-deploy --mode servidor y levanta todos los servicios.
# Ventaja: en el anfitrión solo hay UN contenedor y UN volumen de datos.
# Coste: necesita --privileged (lo exige Docker in Docker) y la primera
# arrancada construye las imágenes dentro (20-60 min).
#
#   docker build -f deploy/todo-en-uno/Dockerfile -t sytue-suite .
#   docker run -d --name sytue --privileged --restart unless-stopped \
#     -p 80:80 -p 443:443 -p 7881:7881 -p 50000-50100:50000-50100/udp \
#     -v sytue-datos:/datos -e SYTUE_DOMINIO=suite.miempresa.eu \
#     -e SYTUE_ACME_EMAIL=admin@miempresa.eu sytue-suite
#
# Variables (las mismas preguntas que el asistente):
#   SYTUE_DOMINIO      dominio raíz (por defecto suite.localhost: pruebas en este equipo)
#   SYTUE_TLS          letsencrypt | interno (por defecto: interno con *.localhost,
#                      letsencrypt con cualquier otro dominio)
#   SYTUE_ACME_EMAIL   correo para Let's Encrypt
#   SYTUE_PROPIETARIO  correo del propietario inicial (admin@<dominio>)
#   SYTUE_ORGANIZACION organización inicial
#   SYTUE_USUARIOS     uno (solo el propietario) | varios (equipo por invitación; por defecto)
#   SYTUE_REGISTRO_ABIERTO  true: con «varios», cualquiera puede crearse cuenta en /signup
#   SYTUE_IP_MEDIOS    IP que anuncian las videollamadas (en una red local, la de
#                      este equipo; en Internet se descubre sola)
#   SYTUE_EXTRA        opciones extra para ./sytue-deploy (p. ej. "--sin-erp --sin-devx")
# Todo lo que hay que conservar (imágenes, bases de datos, .env, certificados)
# vive en /datos: borrar el contenedor no borra la suite; borrar el volumen, sí.
FROM docker:29-dind

RUN apk add --no-cache bash coreutils curl findutils grep make mkcert nss-tools openssl perl python3 sed tini

WORKDIR /opt/sytue-suite
COPY . .
COPY deploy/todo-en-uno/arranque.sh /usr/local/bin/sytue-arranque
RUN chmod 755 /usr/local/bin/sytue-arranque sytue-deploy

VOLUME /datos
EXPOSE 80 443 7881 50000-50100/udp
HEALTHCHECK --interval=30s --timeout=10s --start-period=60m --retries=5 \
  CMD test -f /run/sytue-lista && wget -q --no-check-certificate -O /dev/null --header "Host: one.${SYTUE_DOMINIO:-suite.localhost}" https://127.0.0.1/ || exit 1
ENTRYPOINT ["/sbin/tini", "--", "sytue-arranque"]
