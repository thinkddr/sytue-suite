// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package cache es el cliente de la cache/pub-sub EFÍMERA de Sytue One
// (DragonflyDB, hablado por red vía protocolo Redis con go-redis, BSD-2).
// Solo transporta lo efímero: presencia con TTL y fan-out pub/sub. El estado
// durable y el outbox transaccional SIGUEN en Postgres (platform/pkg/events).
//
// DEGRADACIÓN: con url=="" (DRAGONFLY_URL sin configurar) el cliente queda
// deshabilitado y todas las operaciones son no-op (Set/Publish nil, Get
// not-found, Subscribe canal vacío). Los servicios arrancan y funcionan sin
// Dragonfly; solo pierden la capa efímera.
package cache

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/logsafe"

	"github.com/redis/go-redis/v9" //nolint:depguard // platform/pkg/cache es el único wrapper permitido para hablar con Dragonfly
)

// connectTimeout acota el Ping de arranque y la confirmación de Subscribe.
// El resto de la I/O queda acotada por los Dial/Read/WriteTimeouts del propio
// go-redis (5s/3s/3s por defecto) además del ctx del llamante.
const connectTimeout = 3 * time.Second

// sondaIntervalo es cada cuánto se reintenta Dragonfly cuando está caído.
//
// EXISTE PORQUE SU AUSENCIA COSTÓ UN INCIDENTE. Hasta el 2026-08-16 el Ping de
// New era la ÚNICA comprobación en toda la vida del proceso: si Dragonfly no
// estaba en ese instante, el servicio se quedaba con el cliente deshabilitado
// PARA SIEMPRE y solo un reinicio del pod lo devolvía. Como el chart va con una
// réplica y `strategy: Recreate`, cualquier rollout de la cache dejaba a los
// siete servicios degradados en silencio: los registros lo decían una vez, al
// arrancar, y nunca más.
//
// Cinco segundos es deliberado: la sonda es un PING sobre una conexión del pool,
// y recuperar la presencia en vivo cinco segundos después de que vuelva la cache
// vale mucho más que los pings ahorrados.
const sondaIntervalo = 5 * time.Second

// Client envuelve la conexión a Dragonfly. El valor cero NO es válido: usar
// New. Con rdb==nil el cliente está deshabilitado (todas las ops no-op).
type Client struct {
	rdb *redis.Client
	// sano refleja el resultado del último intento contra Dragonfly. Lo mueven
	// la sonda de fondo y cada operación, así que una caída se nota en la
	// siguiente llamada y una recuperación como muy tarde en el siguiente tick.
	// NO es lo mismo que «configurado»: ver Enabled.
	sano atomic.Bool
	// firma es la clave HMAC con la que se sellan los valores de SetSignedTTL.
	// Vacía => no se firma NI se acepta nada firmado (ver UsarFirma).
	firma []byte
	// pararSonda corta la goroutine de la sonda; lo llama Close.
	pararSonda func()
	cerrarUna  sync.Once
}

// New crea el cliente. url=="" => cliente deshabilitado (no-op) para arrancar
// sin Dragonfly. url!="" => conecta (redis://host:puerto) y verifica con Ping.
//
// UN ERROR AQUÍ YA NO ES DEFINITIVO, y es el cambio importante: si el Ping falla
// se devuelve el cliente IGUALMENTE, junto al error, y la sonda de fondo lo
// recupera cuando Dragonfly vuelva. El llamante debe registrar el error —para
// que quede dicho que se arrancó degradado— y quedarse con el cliente. Devolver
// nil aquí era lo que convertía una caída de segundos en una degradación
// permanente.
//
// El único error que sí deja el cliente en nil es una URL inválida: eso no lo
// arregla esperar.
func New(ctx context.Context, url string) (*Client, error) {
	return NewConAuth(ctx, url, "")
}

// NewConAuth es New con la contraseña POR SEPARADO de la URL.
//
// Va aparte a propósito. La URL no es un secreto —es `redis://dragonfly:6379`,
// la misma en dev y en prod— y viaja como `value:` a la vista en el manifiesto;
// la contraseña sí lo es y viene por `secretKeyRef`. Metida dentro de la URL
// tendría que viajar entera por Secret, y entonces `kubectl get deploy -o yaml`
// deja de decir a qué cache apunta cada servicio, que es justo lo que comprueba
// el gate de despliegue (todos a la MISMA).
//
// password vacía => sin autenticación, que es el comportamiento previo y sigue
// siendo válido donde la red ya acota quién llega al puerto.
func NewConAuth(ctx context.Context, url, password string) (*Client, error) {
	if url == "" {
		return &Client{}, nil
	}
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("cache: URL inválida: %w", err)
	}
	if password != "" {
		opt.Password = password
	}
	// Tiempos de espera estrictos para evitar que la lentitud de Dragonfly afecte al path principal.
	opt.DialTimeout = 500 * time.Millisecond
	opt.ReadTimeout = 200 * time.Millisecond
	opt.WriteTimeout = 200 * time.Millisecond
	opt.MaxRetries = 1
	// Configuración de pool de conexiones optimizado para alta concurrencia
	opt.PoolSize = 50
	opt.MinIdleConns = 10
	opt.PoolTimeout = 500 * time.Millisecond

	rdb := redis.NewClient(opt)
	c := &Client{rdb: rdb}
	pingCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	pingErr := rdb.Ping(pingCtx).Err()
	c.sano.Store(pingErr == nil)
	c.arrancarSonda()
	if pingErr != nil {
		return c, fmt.Errorf("cache: sin conexión a %s: %w", opt.Addr, pingErr)
	}
	return c, nil
}

// arrancarSonda lanza el reintento periódico.
//
// El contexto es propio (Background), NO el de New: ese suele ser el de arranque
// del servicio y en algunos vive solo hasta que termina la inicialización. Atarle
// la sonda la mataría justo después de nacer, que es el fallo que este código
// viene a evitar. La corta Close.
//
//nolint:contextcheck // el contexto propio es el punto de esta función, no un olvido
func (c *Client) arrancarSonda() {
	ctx, cancel := context.WithCancel(context.Background())
	c.pararSonda = cancel
	go func() {
		t := time.NewTicker(sondaIntervalo)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				pingCtx, cancelPing := context.WithTimeout(ctx, connectTimeout)
				err := c.rdb.Ping(pingCtx).Err()
				cancelPing()
				c.sano.Store(err == nil)
			}
		}
	}()
}

// anotar registra el resultado de una operación para que Enabled lo refleje sin
// esperar al siguiente tick. redis.Nil es «no existe», no un fallo de conexión:
// ese caso cuenta como sano.
func (c *Client) anotar(err error) error {
	if err == nil || errors.Is(err, redis.Nil) {
		c.sano.Store(true)
		return err
	}
	c.sano.Store(false)
	return err
}

// SetTTL guarda un valor efímero con expiración. Deshabilitado => nil.
// MaxValorBytes es lo más grande que se acepta guardar en UNA entrada.
//
// POR QUÉ HAY QUE PONER UN TOPE AQUÍ Y NO SOLO EN DRAGONFLY. El servidor ya se
// acota solo: `--maxmemory=1gb` con `--cache_mode=true`, dentro de un límite de
// contenedor de 1536 MiB, así que evicta lo más viejo antes de que el kernel
// mate el proceso. Eso protege LA MÁQUINA, pero no protege lo que hay dentro:
// cuando entra algo enorme, lo que se evicta es lo demás.
//
// Y lo demás incluye el derecho de suscripción, que se consulta al emitir CADA
// token de la suite. O sea que una libreta de direcciones de una organización
// grande, o la miniatura de una imagen que alguien pegó en un chat, pueden
// vaciar el camino más caliente que hay. Los tres sitios que guardan cosas de
// tamaño ABIERTO —caldav, carddav y el unfurl de imágenes de Rooms— no tenían
// ningún tope.
//
// 256 KiB deja pasar todo lo que esta casa cachea de verdad y hacen falta ~4000
// entradas para llenar el gigabyte. Lo que no quepa NO SE GUARDA y se dice: una
// cache que no guarda algo es correcta por definición —quien lo pidió lo vuelve
// a calcular—, y el aviso es lo que permite enterarse de QUIÉN se pasa.
const MaxValorBytes = 256 * 1024

// SetTTL guarda `val` bajo `key` con caducidad. Lo que pase de MaxValorBytes se
// descarta con un aviso: ver el porqué en esa constante.
func (c *Client) SetTTL(ctx context.Context, key, val string, ttl time.Duration) error {
	if c.rdb == nil {
		return nil
	}
	if len(val) > MaxValorBytes {
		// Sin `key` entera en el log: puede llevar el identificador de una persona
		// o el texto que alguien tecleó. Con el prefijo basta para saber quién es.
		log.From(ctx).Warn("cache: entrada demasiado grande; no se guarda",
			"prefijo", logsafe.Text(prefijoDe(key)), "bytes", len(val), "tope", MaxValorBytes)
		return nil
	}
	return c.anotar(c.rdb.Set(ctx, key, val, ttl).Err())
}

// prefijoDe recorta la clave a su espacio de nombres (`id:agenda`, `rooms:img`…)
// para poder nombrar al culpable sin registrar de quién era la entrada.
func prefijoDe(key string) string {
	n := 0
	for i, r := range key {
		if r == ':' {
			n++
			if n == 2 {
				return key[:i]
			}
		}
	}
	return key
}

// SumarTTL suma `delta` al contador `key` de forma ATÓMICA y le pone `ttl` la
// primera vez. Devuelve el total tras sumar.
//
// ATÓMICA Y NO leer-sumar-escribir, que es la diferencia que importa: el
// contador lo tocan varias réplicas a la vez y con tres pasos separados dos
// peticiones simultáneas se pisan y una de las dos se pierde. Con INCRBYFLOAT lo
// resuelve Dragonfly.
//
// El TTL se fija SOLO cuando el contador se acaba de crear (total == delta): un
// EXPIRE en cada suma reiniciaría la ventana en cada llamada y el contador no
// caducaría nunca mientras hubiera tráfico.
//
// Cliente deshabilitado => devuelve 0 sin error. Quien lleve un presupuesto con
// esto debe saber que eso significa NO CONTAR: sin cache no hay límite, y es
// preferible a bloquear a todo el mundo porque falte una pieza opcional.
func (c *Client) SumarTTL(ctx context.Context, key string, delta float64, ttl time.Duration) (float64, error) {
	if c.rdb == nil {
		return 0, nil
	}
	total, err := c.rdb.IncrByFloat(ctx, key, delta).Result()
	if err := c.anotar(err); err != nil {
		return 0, err
	}
	if total == delta {
		// El error del EXPIRE no se propaga: el contador ya está sumado y lo
		// peor que pasa es que la clave viva de más. Perder la suma por no
		// poder marcar la caducidad sería peor.
		_ = c.anotar(c.rdb.Expire(ctx, key, ttl).Err())
	}
	return total, nil
}

// Get lee una clave. found=false si no existe (o cliente deshabilitado).
func (c *Client) Get(ctx context.Context, key string) (val string, found bool, err error) {
	if c.rdb == nil {
		return "", false, nil
	}
	val, err = c.rdb.Get(ctx, key).Result()
	if err = c.anotar(err); errors.Is(err, redis.Nil) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return val, true, nil
}

// MGet lee múltiples claves en una única petición (roundtrip).
// Devuelve un mapa con las claves encontradas y su valor.
// Si el cliente está deshabilitado o la lista de claves está vacía, devuelve un mapa vacío sin error.
func (c *Client) MGet(ctx context.Context, keys ...string) (map[string]string, error) {
	if c.rdb == nil || len(keys) == 0 {
		return make(map[string]string), nil
	}
	vals, err := c.rdb.MGet(ctx, keys...).Result()
	if err := c.anotar(err); err != nil {
		return nil, err
	}
	res := make(map[string]string, len(keys))
	for i, val := range vals {
		if val != nil {
			if str, ok := val.(string); ok {
				res[keys[i]] = str
			}
		}
	}
	return res, nil
}

// Del borra claves. Deshabilitado => nil.
func (c *Client) Del(ctx context.Context, keys ...string) error {
	if c.rdb == nil || len(keys) == 0 {
		return nil
	}
	return c.anotar(c.rdb.Del(ctx, keys...).Err())
}

// Publish emite un payload efímero en un canal (fire-and-forget: sin
// suscriptores se pierde, por diseño). Deshabilitado => nil.
func (c *Client) Publish(ctx context.Context, channel string, payload []byte) error {
	if c.rdb == nil {
		return nil
	}
	return c.anotar(c.rdb.Publish(ctx, channel, payload).Err())
}

// Subscribe se suscribe a un canal y devuelve el flujo de payloads y un
// cancel idempotente que corta la suscripción y cierra el canal. Solo retorna
// tras confirmar la suscripción: un Publish posterior ya no se pierde.
// Deshabilitado => canal que nunca entrega y se cierra con cancel.
func (c *Client) Subscribe(ctx context.Context, channel string) (<-chan []byte, func(), error) {
	out := make(chan []byte, 64)
	if c.rdb == nil {
		done := make(chan struct{})
		var once sync.Once
		cancel := func() {
			once.Do(func() {
				close(done)
				close(out)
			})
		}
		go func() {
			select {
			case <-ctx.Done():
				cancel()
			case <-done:
			}
		}()
		return out, cancel, nil
	}
	sub := c.rdb.Subscribe(ctx, channel)
	confCtx, cancelConf := context.WithTimeout(ctx, connectTimeout)
	defer cancelConf()
	if _, err := sub.Receive(confCtx); err != nil {
		_ = sub.Close()
		return nil, nil, fmt.Errorf("cache: subscribe %q: %w", channel, err)
	}
	done := make(chan struct{})
	go func() {
		defer close(out)
		ch := sub.Channel()
		for {
			select {
			case <-ctx.Done():
				_ = sub.Close()
				return
			case <-done:
				_ = sub.Close()
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				select {
				case out <- []byte(msg.Payload):
				default:
					// ponytail: consumidor lento => se descarta el mensaje (es
					// fan-out efímero, reconstruible); así la goroutine nunca
					// queda bloqueada. Si hiciera falta backpressure real, subir
					// el buffer o entregar bloqueando con ctx.
				}
			}
		}
	}()
	var once sync.Once
	cancel := func() { once.Do(func() { close(done) }) }
	return out, cancel, nil
}

// Close corta la sonda y cierra la conexión. Deshabilitado => nil. Es idempotente:
// los servicios lo dejan en un defer y algunos caminos de error lo llaman antes.
func (c *Client) Close() error {
	if c == nil || c.rdb == nil {
		return nil
	}
	c.cerrarUna.Do(func() {
		if c.pararSonda != nil {
			c.pararSonda()
		}
	})
	return c.rdb.Close()
}

// Enabled devuelve true si el cliente de caché está configurado y habilitado.
//
// Tolera el receptor NIL a propósito, y no es defensa por si acaso: «sin cache» es
// un estado NORMAL de este paquete —así arranca cualquier servicio sin
// DRAGONFLY_URL—, y el patrón natural para consultarlo es `if s.cache.Enabled()`
// sobre un campo que puede ser nil. Sin esta guarda, ese `if` panica justo en el
// caso que la cache existe para tolerar: que no haya cache.
//
// Mismo criterio que pg.AppRole.Activo(), y por el mismo motivo: ahí un receptor
// nil hacía que el servicio ARRANCARA y muriera dentro de la primera transacción,
// con el panic señalando a este paquete en vez de a quien no lo inicializó. Le
// costó a Sytue Docs un despliegue entero.
// Ahora incluye además la SALUD, no solo la configuración: un cliente configurado
// contra un Dragonfly caído responde false y los llamantes se van a PostgreSQL sin
// pagar el viaje ni el timeout. Cuando la sonda lo recupera, vuelve a true solo.
func (c *Client) Enabled() bool {
	return c != nil && c.rdb != nil && c.sano.Load()
}

// Configurada dice si hay URL de Dragonfly, esté sano o no. Enabled responde a
// «¿tiro por la cache AHORA?»; esto responde a «¿se esperaba que hubiera cache?»,
// que es lo que hay que mirar para distinguir un despliegue sin Dragonfly de uno
// con Dragonfly caído.
func (c *Client) Configurada() bool {
	return c != nil && c.rdb != nil
}

// ---------------------------------------------------------------------------
// Valores firmados
// ---------------------------------------------------------------------------

// POR QUÉ ESTO EXISTE. Guardar en Dragonfly la resolución de un token de API es
// guardar una DECISIÓN DE AUTENTICACIÓN, y quien pueda escribir en la cache la
// dicta: basta con poner el JSON de un tenant ajeno bajo la clave adecuada para
// que el servicio lo acepte como identidad. Eso convierte «alcanzar el 6379» en
// «emitir credenciales de cualquier tenant», que es un salto que no debe existir
// aunque la red y la contraseña estén bien puestas.
//
// El sello es HMAC-SHA256 y cubre EL NOMBRE DE LA CLAVE ADEMÁS DEL VALOR. Sin el
// nombre, un valor legítimamente firmado se podría copiar a otra clave —el
// derecho de una organización a la de otra, la resolución de un token al hash de
// otro— y seguiría verificando. Es el mismo motivo por el que la clave de la
// cache de tokens lleva el hash completo y no un prefijo.
//
// NO sustituye a la contraseña ni a la NetworkPolicy: quien escriba en la cache
// sigue pudiendo borrar entradas o llenar la memoria. Lo que cierra es la
// falsificación de identidades, que es el único de esos daños que no se arregla
// solo.

// marcaFirma versiona el formato: "v1:<mac en base64url>:<valor>". Va delante para
// que un valor sin firmar —de una versión anterior, o puesto por un atacante— no
// pueda confundirse con uno firmado.
const marcaFirma = "v1:"

// ClaveDeFirma deriva la clave de un dominio a partir del secreto maestro.
//
// Se deriva en vez de usar el secreto tal cual para que un servicio no pueda
// sellar los valores de otro: con el mismo secreto repartido a toda la suite,
// cloud y id obtienen claves distintas y ninguno puede falsificar las entradas
// del otro. El dominio es el nombre del servicio.
func ClaveDeFirma(maestra []byte, dominio string) []byte {
	m := hmac.New(sha256.New, maestra)
	m.Write([]byte("sytue-cache-v1:" + dominio))
	return m.Sum(nil)
}

// UsarFirma cablea la clave HMAC. Hay que llamarlo ANTES de usar el cliente.
// Con clave vacía, los métodos firmados quedan inertes (ver SetSignedTTL).
func (c *Client) UsarFirma(clave []byte) {
	if c == nil {
		return
	}
	c.firma = clave
}

// FirmaActiva dice si hay clave de firma cableada.
func (c *Client) FirmaActiva() bool {
	return c != nil && len(c.firma) > 0
}

// SetSignedTTL guarda un valor sellado.
//
// SIN CLAVE NO ESCRIBE NADA, a propósito. La alternativa —guardarlo en claro— es
// justo el agujero que esto cierra: un despliegue al que se le olvidó el secreto
// se quedaría cacheando decisiones de autenticación falsificables, y no lo diría
// nadie. Así el peor caso de una mala configuración es ir siempre a PostgreSQL:
// más lento, nunca inseguro.
func (c *Client) SetSignedTTL(ctx context.Context, key, val string, ttl time.Duration) error {
	if c.rdb == nil || len(c.firma) == 0 {
		return nil
	}
	return c.SetTTL(ctx, key, sellar(c.firma, key, val), ttl)
}

// GetSigned lee un valor sellado y lo devuelve solo si el sello cuadra.
//
// Un valor ausente, sin sellar o con el sello equivocado se tratan IGUAL: found
// falso y error nil. No es dejadez, es la degradación correcta —el llamante va a
// PostgreSQL, que es lo que haría sin cache— y además no le regala al atacante un
// oráculo con el que distinguir «no está» de «no verifica».
func (c *Client) GetSigned(ctx context.Context, key string) (string, bool, error) {
	if c.rdb == nil || len(c.firma) == 0 {
		return "", false, nil
	}
	raw, found, err := c.Get(ctx, key)
	if err != nil || !found {
		return "", false, err
	}
	val, ok := comprobarSello(c.firma, key, raw)
	if !ok {
		return "", false, nil
	}
	return val, true, nil
}

func sello(clave []byte, nombre, val string) []byte {
	m := hmac.New(sha256.New, clave)
	// El separador NUL evita la ambigüedad entre (nombre, valor) distintos que
	// concatenados darían la misma cadena.
	m.Write([]byte(nombre))
	m.Write([]byte{0})
	m.Write([]byte(val))
	return m.Sum(nil)
}

func sellar(clave []byte, nombre, val string) string {
	return marcaFirma + base64.RawURLEncoding.EncodeToString(sello(clave, nombre, val)) + ":" + val
}

func comprobarSello(clave []byte, nombre, raw string) (string, bool) {
	resto, ok := strings.CutPrefix(raw, marcaFirma)
	if !ok {
		return "", false
	}
	mac, val, ok := strings.Cut(resto, ":")
	if !ok {
		return "", false
	}
	dado, err := base64.RawURLEncoding.DecodeString(mac)
	if err != nil {
		return "", false
	}
	// hmac.Equal y no ==: comparación en tiempo constante.
	if !hmac.Equal(dado, sello(clave, nombre, val)) {
		return "", false
	}
	return val, true
}
