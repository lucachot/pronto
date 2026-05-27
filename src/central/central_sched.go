package central

import (
	"context"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"

	log "github.com/sirupsen/logrus"

	pb "github.com/LucaChot/pronto/src/message"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/flowcontrol"
)

type CentralScheduler struct {
    mu          sync.Mutex
    Name        string
    clientset   *kubernetes.Clientset

    nodeMap     map[string]int
    nodeSignals []atomic.Uint64

    Bins        map[string]string
    pb.UnimplementedPodPlacementServer
}

func (ctl *CentralScheduler) SetClientset() {
	config, err := rest.InClusterConfig()
	if err != nil {
		log.WithFields(log.Fields{
			"ERROR": err,
		}).Error("CONFIG ERROR")
	}
	config.RateLimiter = flowcontrol.NewTokenBucketRateLimiter(80, 100)

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.WithFields(log.Fields{
			"ERROR": err,
		}).Fatal("CLIENTSET ERROR")
	}
	ctl.clientset = clientset
}

/* Creates a new CentralScheduler */
func New() *CentralScheduler {

    /* Initialise scheduler values */
	ctl := &CentralScheduler{
		Name: "pronto",
    }

    ctl.SetClientset()
    ctl.findNodes()
    ctl.nodeSignals = make([]atomic.Uint64, len(ctl.nodeMap))

    for node := range(len(ctl.nodeMap)) {
        ctl.nodeSignals[node].Store(math.Float64bits(1))
    }

    ctl.ctlStartPlacementServer()

	return ctl
}


/* Returns the node of the first remote scheduler to respond to new pod */
func (ctl *CentralScheduler) findNode() string {
    var name string
    var minSignal float64
    minSignal = 1

    for node, index := range ctl.nodeMap {
        signal := math.Float64frombits(ctl.nodeSignals[index].Load())
        if signal < minSignal {
            minSignal = signal
            name = node
        }
    }

    log.WithFields(log.Fields{
        "NODE": name,
        "JOB SIGNAL": minSignal,
    }).Debug("FOUND NODE")

    return name
}

/* Core Scheduling loop */
func (ctl *CentralScheduler) Schedule() {

    /* Creates a watch interface for all pods that use this scheduler */
	watch, err := ctl.clientset.CoreV1().Pods("").Watch(context.TODO(), metav1.ListOptions{
		FieldSelector: fmt.Sprintf("spec.schedulerName=%s,spec.nodeName=", ctl.Name),
	})
	if err != nil {
		log.WithFields(log.Fields{
			"ERROR": err,
		}).Fatal("WATCH ERROR")
	}

    /* Loops over all new pod events we detect */
	for event := range watch.ResultChan() {
        /* Ignore events where pods have been added */
		if event.Type != "ADDED" {
			continue
		}

        start := time.Now().UTC()

		p := event.Object.(*v1.Pod)
		log.WithFields(log.Fields{
			"namespace": p.Namespace,
			"pod":       p.Name,
		}).Debug("BEGIN POD SCHEDULE")


        /* Find a node to place the pod */
        node := ctl.findNode()
        if node == "" {
            log.Debug("FAILED TO FIND SUITABLE NODE")
            continue
        }

        ctl.placePodToNode(p, node)

        /* Collect information for event */
		end := time.Now().UTC()
        nanosecondsSpent := end.Sub(start).Nanoseconds()
        annotations := map[string]string{
            "scheduler/nanoseconds": fmt.Sprintf("%d", nanosecondsSpent),
        }

        /* Creates a new event alerting the binding of the pod */
        err := ctl.createSchedEvent(p, node, end, annotations)
        if err != nil {
            log.WithFields(log.Fields{
                "err": err,
            }).Debug("FAILED TO CREATE EVENT")
        }
	}
}

