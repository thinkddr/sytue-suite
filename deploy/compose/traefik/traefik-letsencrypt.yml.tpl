# Plantilla de ./sytue-deploy --mode servidor --tls letsencrypt: la misma
# configuración estática que traefik.yml más el emisor ACME. El asistente cambia
# __ACME_EMAIL__ por tu correo y la monta desde deploy/compose/.servidor/ (Traefik
# no interpola variables en este fichero ni mezcla fichero y parámetros).
# Certificados por host con el reto HTTP-01: los nombres de tu dominio tienen que
# resolver a este servidor y el puerto 80 tiene que llegar desde Internet.
# Traefik 3 — configuración estática (Sytue One dev)
# Routing por *.sytue.localhost con TLS local (mkcert). Ver dynamic/ para TLS.

global:
  checkNewVersion: false
  sendAnonymousUsage: false

api:
  dashboard: true
  # insecure: false -> el dashboard se expone vía router con TLS (traefik.sytue.localhost)

ping:
  entryPoint: web

log:
  level: INFO

# Sin la RUTA: una URL puede llevar una credencial (el token de un enlace compartido,
# `/docs/s/<token>`; el de un álbum publicado) y el log de accesos lo lee cualquiera
# con acceso a la máquina (ADR 0052). Queda el router y el servicio que la atendió.
# Lo vigila platform/pkg/httpx (TestLosLogsDeAccesoNoLlevanLaRuta).
accessLog:
  fields:
    names:
      RequestPath: drop
      RequestLine: drop

entryPoints:
  web:
    address: ":80"
    http:
      redirections:
        entryPoint:
          to: websecure
          scheme: https
          permanent: true
  websecure:
    address: ":443"
    http:
      tls:
        certResolver: letsencrypt

providers:
  docker:
    endpoint: "unix:///var/run/docker.sock"
    exposedByDefault: false
    network: sytue-edge
    watch: true
  file:
    directory: /etc/traefik/dynamic
    watch: true

certificatesResolvers:
  letsencrypt:
    acme:
      email: "__ACME_EMAIL__"
      storage: /acme/acme.json
      httpChallenge:
        entryPoint: web
