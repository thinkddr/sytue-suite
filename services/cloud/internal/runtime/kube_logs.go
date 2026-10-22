// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package runtime

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"git.sytue.eu/sytue/one/services/cloud/internal/config"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// KubeLogStreamer hace tail de logs del pod del deployment vía client-go.
type KubeLogStreamer struct {
	cs   kubernetes.Interface
	rest *rest.Config
}

// NewKubeLogStreamer construye el streamer sobre Kapsule.
func NewKubeLogStreamer(cfg config.Config) (*KubeLogStreamer, error) {
	restCfg, cs, err := kubeClientset(cfg.KubeconfigPath)
	if err != nil {
		return nil, err
	}
	return &KubeLogStreamer{cs: cs, rest: restCfg}, nil
}

func (k *KubeLogStreamer) StreamDeploymentLogs(ctx context.Context, dep domain.Deployment, _ string, w io.Writer) error {
	ns := "tenant-" + dep.Tenant
	pod, err := k.findPod(ctx, ns, dep.ID.String())
	if err != nil {
		return err
	}
	opts := &corev1.PodLogOptions{Follow: true, TailLines: ptrInt64(200)}
	rc, err := k.cs.CoreV1().Pods(ns).GetLogs(pod, opts).Stream(ctx)
	if err != nil {
		return fmt.Errorf("runtime: abrir stream de logs: %w", err)
	}
	defer rc.Close()
	sc := bufio.NewScanner(rc)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		if _, err := io.WriteString(w, sc.Text()+"\n"); err != nil {
			return err
		}
	}
	return sc.Err()
}

// CollectDeploymentLogs LEE (no sigue) los logs escritos desde `since` y los
// devuelve ya fechados, para que el recolector los persista y sean buscables.
//
// Es una lectura puntual y no un `Follow` a propósito: seguir el log exigiría una
// goroutine viva por pod, supervisada, sobreviviendo a reinicios y repartida
// entre réplicas. Un pull por tick no necesita nada de eso —el estado cabe en una
// columna— y el retraso máximo es el intervalo del recolector.
//
// ponytail: techo conocido = `limit` líneas por pasada. Si una app escribe más, el
// resto se lee en las pasadas siguientes (el corte avanza, no se pierde nada)
// salvo que el kubelet rote el fichero antes de alcanzarlo. Si eso pasa de verdad:
// subir el tope o acortar el intervalo.
func (k *KubeLogStreamer) CollectDeploymentLogs(ctx context.Context, tenant string, deploymentID uuid.UUID, since time.Time, limit int) ([]domain.LogLine, error) {
	ns := "tenant-" + tenant
	pod, err := k.findPod(ctx, ns, deploymentID.String())
	if err != nil {
		return nil, err
	}
	opts := &corev1.PodLogOptions{Timestamps: true, SinceTime: &metav1.Time{Time: since}}
	rc, err := k.cs.CoreV1().Pods(ns).GetLogs(pod, opts).Stream(ctx)
	if err != nil {
		return nil, fmt.Errorf("runtime: leer logs: %w", err)
	}
	defer rc.Close()

	out := make([]domain.LogLine, 0, 64)
	sc := bufio.NewScanner(rc)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for len(out) < limit && sc.Scan() {
		ts, text := splitLogTimestamp(sc.Text())
		// SinceTime viaja con precisión de SEGUNDO, así que Kubernetes devuelve
		// también líneas del mismo segundo que ya se guardaron: se descartan aquí
		// comparando el instante completo.
		if !ts.After(since) {
			continue
		}
		out = append(out, domain.LogLine{TS: ts, Text: text})
	}
	if err := sc.Err(); err != nil {
		return out, err
	}
	return out, nil
}

// TailPodLogs devuelve las últimas `lines` líneas de los pods que casan con el
// selector, ya fechadas.
//
// Es lo que hace visible un servicio gestionado: una función, un cron o un job no
// son deployments del proyecto, así que ningún camino de los de arriba los
// alcanzaba y su registro era inaccesible desde el producto —creabas una función,
// no arrancaba, y no había forma de saber por qué—.
//
// Lee de TODOS los pods que casan y no solo del primero: un cron deja un pod por
// ejecución, y mirar únicamente uno enseñaría la carrera de ayer.
func (k *KubeLogStreamer) TailPodLogs(ctx context.Context, namespace, selector string, lines int) ([]domain.LogLine, error) {
	if lines <= 0 || lines > 1000 {
		lines = 200
	}
	pods, err := k.cs.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, fmt.Errorf("runtime: listar pods del servicio: %w", err)
	}
	out := make([]domain.LogLine, 0, lines)
	// De más reciente a más antiguo: con varios pods (un cron que ha corrido
	// varias veces) lo primero que interesa es la última ejecución.
	items := pods.Items
	sort.Slice(items, func(i, j int) bool {
		return items[j].CreationTimestamp.Time.Before(items[i].CreationTimestamp.Time)
	})
	for _, pod := range items {
		if len(out) >= lines {
			break
		}
		restantes := int64(lines - len(out))
		opts := &corev1.PodLogOptions{Timestamps: true, TailLines: &restantes}
		rc, lerr := k.cs.CoreV1().Pods(namespace).GetLogs(pod.Name, opts).Stream(ctx)
		if lerr != nil {
			// Un pod que aún no arrancó (ContainerCreating) no tiene log: no es un
			// fallo de la petición, simplemente no aporta líneas.
			continue
		}
		sc := bufio.NewScanner(rc)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		for len(out) < lines && sc.Scan() {
			ts, text := splitLogTimestamp(sc.Text())
			out = append(out, domain.LogLine{TS: ts, Text: text})
		}
		_ = rc.Close()
	}
	// Cronológico para leerlo de arriba abajo, que es como se lee un registro.
	sort.SliceStable(out, func(i, j int) bool { return out[i].TS.Before(out[j].TS) })
	return out, nil
}

// RestartDeployment reinicia en rodaje los Deployments que casan con el selector
// y devuelve cuántos tocó.
//
// Es lo mismo que hace `kubectl rollout restart`: tocar una anotación del POD
// TEMPLATE. Cambiar el template crea un ReplicaSet nuevo, y de ahí sale el
// rodaje —arriba el pod nuevo, abajo el viejo—; borrar los pods a mano, que es la
// otra forma evidente, deja la app caída mientras el ReplicaSet los repone.
//
// La versión desplegada NO cambia: misma imagen, mismo deployment, mismo commit.
// Por eso hace falta un verbo propio y no vale «redesplegar»: esto no compila
// nada, no cuesta minutos de build y no altera lo que está publicado.
func (k *KubeLogStreamer) RestartDeployment(ctx context.Context, namespace, selector string) (int, error) {
	deps, err := k.cs.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return 0, fmt.Errorf("runtime: listar deployments: %w", err)
	}
	// La anotación lleva NUESTRO prefijo, no el de kubectl: el valor que se escribe
	// aquí lo escribe Sytue, y atribuirlo a kubectl confundiría a quien mire el
	// objeto buscando quién lo reinició.
	patch := fmt.Sprintf(`{"spec":{"template":{"metadata":{"annotations":{"sytue.eu/restarted-at":%q}}}}}`,
		time.Now().UTC().Format(time.RFC3339))
	n := 0
	for i := range deps.Items {
		_, err := k.cs.AppsV1().Deployments(namespace).Patch(
			ctx, deps.Items[i].Name, types.StrategicMergePatchType, []byte(patch), metav1.PatchOptions{},
		)
		if err != nil {
			return n, fmt.Errorf("runtime: reiniciar %s: %w", deps.Items[i].Name, err)
		}
		n++
	}
	return n, nil
}

// splitLogTimestamp separa el instante que antepone Kubernetes (Timestamps:true,
// RFC3339Nano + espacio) del texto de la línea. Una línea sin prefijo válido no se
// tira: se fecha AHORA, que es una aproximación honesta y la deja buscable.
func splitLogTimestamp(line string) (time.Time, string) {
	if i := strings.IndexByte(line, ' '); i > 0 {
		if ts, err := time.Parse(time.RFC3339Nano, line[:i]); err == nil {
			return ts, line[i+1:]
		}
	}
	return time.Now().UTC(), line
}

func (k *KubeLogStreamer) findPod(ctx context.Context, ns, deploymentID string) (string, error) {
	pods, err := k.cs.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{
		LabelSelector: "sytue.eu/deployment=" + deploymentID,
	})
	if err != nil {
		return "", fmt.Errorf("runtime: listar pods: %w", err)
	}
	for _, p := range pods.Items {
		if p.Status.Phase == corev1.PodRunning || p.Status.Phase == corev1.PodPending {
			return p.Name, nil
		}
	}
	if len(pods.Items) > 0 {
		return pods.Items[0].Name, nil
	}
	return "", fmt.Errorf("runtime: no hay pods para deployment %s", deploymentID)
}

// KubeExecSession adjunta un shell al pod del deployment.
type KubeExecSession struct {
	cs   kubernetes.Interface
	rest *rest.Config
}

// NewKubeExecSession construye la sesión exec sobre Kapsule.
func NewKubeExecSession(cfg config.Config) (*KubeExecSession, error) {
	restCfg, cs, err := kubeClientset(cfg.KubeconfigPath)
	if err != nil {
		return nil, err
	}
	return &KubeExecSession{cs: cs, rest: restCfg}, nil
}

func (k *KubeExecSession) Run(ctx context.Context, dep domain.Deployment, _ string, stdin io.Reader, stdout, stderr io.Writer) error {
	ns := "tenant-" + dep.Tenant
	ls := &KubeLogStreamer{cs: k.cs, rest: k.rest}
	pod, err := ls.findPod(ctx, ns, dep.ID.String())
	if err != nil {
		return err
	}
	return execInPod(ctx, k.rest, k.cs, ns, pod, []string{"/bin/sh"}, stdin, stdout, stderr)
}

func kubeClientset(kubeconfigPath string) (*rest.Config, kubernetes.Interface, error) {
	var restCfg *rest.Config
	var err error
	if kubeconfigPath == "" {
		restCfg, err = rest.InClusterConfig()
	} else {
		restCfg, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("runtime: config k8s: %w", err)
	}
	cs, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, nil, fmt.Errorf("runtime: clientset: %w", err)
	}
	return restCfg, cs, nil
}

func ptrInt64(v int64) *int64 { return &v }
