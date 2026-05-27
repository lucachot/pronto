package remote

import (
	"context"
	"fmt"
	"math"
	"os"
	"time"

	log "github.com/sirupsen/logrus"
	"gonum.org/v1/gonum/mat"

	"github.com/LucaChot/pronto/src/fpca"
	pb "github.com/LucaChot/pronto/src/message"
	"github.com/LucaChot/pronto/src/metrics"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	TR                   = 0.5
	defaultNamespace     = "basic-sched"
)

func getNamespace() string {
	if ns := os.Getenv("PRONTO_NAMESPACE"); ns != "" {
		return ns
	}
	return defaultNamespace
}

type RemoteScheduler struct {
    hostname string
    onNode *v1.Node

    mc *metrics.MetricsCollector
    fp *fpca.FPCAAgent

    tr float64

    clientset   *kubernetes.Clientset
    ctlPlStub  pb.PodPlacementClient
}

func (rmt *RemoteScheduler) SetClientset() {
	config, err := rest.InClusterConfig()
	if err != nil {
		log.WithFields(log.Fields{
			"ERROR": err,
		}).Error("CONFIG ERROR")
	}

	clientset, err := kubernetes.NewForConfig(config)
	rmt.clientset = clientset
}

func (rmt *RemoteScheduler) SetHostname() {
	hostname, err := os.Hostname()
	if err != nil {
		log.WithFields(log.Fields{
			"ERROR": err,
		}).Error("HOSTNAME ERROR")
	}
    rmt.hostname = hostname;
}

func (rmt *RemoteScheduler) SetOnNode() {
    pods, err := rmt.clientset.CoreV1().Pods(getNamespace()).List(context.TODO(), metav1.ListOptions{
		FieldSelector: fmt.Sprintf("metadata.name=%s", rmt.hostname),
	})
	if err != nil {
		log.WithFields(log.Fields{
			"ERROR": err,
		}).Error("LOCATING POD ERROR")
	}

	n, err := rmt.clientset.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", "kubernetes.io/hostname", pods.Items[0].Spec.NodeName),
	})
	if err != nil {
		log.WithFields(log.Fields{
			"ERROR": err,
		}).Error("LOCATING NODE ERROR")
	}

    rmt.onNode = &n.Items[0]
}

/* Creates a new CentralScheduler */
func New() *RemoteScheduler {

    /* Initialise scheduler values */
    rmt := &RemoteScheduler{
        tr: TR,
    }

    /* Run metrics collection */
    var sender <-chan *mat.Dense
    rmt.mc, sender = metrics.New()
    log.Debug("RMT: INITIALISE METRIC COLLECTOR")

    /* Run fpca */
    rmt.fp = fpca.New(sender)
    log.Debug("RMT: INITIALISE FPCA")


    /* Set the remote scheduler variables */
    rmt.SetClientset()
    rmt.SetHostname()
    rmt.SetOnNode()
    rmt.AsClient()

    log.Debug("RMT: FINISHED INITIALISATION")
	return rmt
}

func absFunc(i, j int, v float64) (float64) {
    return math.Abs(v)
}


func (rmt *RemoteScheduler) JobSignal() float64 {
    /* TODO: How to ensure that the B, U and Sigma we load are for the same
    * timestep. Will have to use an atomic pointer that points to be U and
    * Sigma */
    y := rmt.mc.Y.Load()

    uSigmaPair := rmt.fp.USIgma.Load()
    u := uSigmaPair.U
    sigma := uSigmaPair.Sigma

    log.WithFields(log.Fields{
        "Y" : *y,
        "U" : *u,
        "SIGMA" : *sigma,
    }).Debug("RMT: CALCULATING JOB SIGNAL")

    var temp, p, wP mat.Dense
    temp.Mul(y.T(), u)
    p.Apply(absFunc, &temp)

    wP.Mul(&p, sigma)

    return mat.Sum(&wP)
}

/* Core Scheduling loop */
/*
TODO: Determine whether I calculate this on pod event or whether I send signal
periodically
TODO: Change to periodic as this will reduce delay, scheduler can use the
latest value received
*/
func (rmt *RemoteScheduler) Schedule() {
    ticker := time.NewTicker(time.Second)
    defer ticker.Stop()
    for {
        <-ticker.C
		log.Debug("RMT: BEGIN POD REQUEST")

        signal := rmt.JobSignal()
        log.WithFields(log.Fields{
            "R" : signal,
        }).Debug("RMT: CALCULATED JOB SIGNAL")
        if signal < rmt.tr {
            rmt.RequestPod(signal)
        }
	}
}

