package remote

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/LucaChot/pronto/src/remote/cache"
	"github.com/LucaChot/pronto/src/remote/fpca"
	"github.com/LucaChot/pronto/src/remote/metrics"
	"github.com/LucaChot/pronto/src/remote/types"
	"gonum.org/v1/gonum/mat"
	clientset "k8s.io/client-go/kubernetes"
)

type RemoteScheduler struct {
    podName     string
    nodeName    string
    signal      float64

    mc *metrics.MetricsCollector
    fp *fpca.FPCAAgent

    cache       *cache.EventCache
    //cache       *cache.Cache
    //costPerPod  *CostPerPodState
    //capacity    *CapacityState
    capacity    *DualFilterState

    client   clientset.Interface
    // Close this to shut down the scheduler.
	ctx         context.Context
    Publisher
}


func GetPodName() (string) {
    n, err :=  os.Hostname()
    if err != nil {
        log.Printf("unable to get hostname from kernel")
        return ""
    }
    return n
}

func GetNodeName() (string) {
    return os.Getenv("NODE_NAME")
}

type remoteOptions struct {
    podName     string
    nodeName    string
    trigger     bool
}

// Option configures a Scheduler
type Option func(*remoteOptions)

func WithTrigger() Option {
	return func(o *remoteOptions) {
		o.trigger = true
	}
}

var defaultRemoteOptions = remoteOptions{
    podName: GetPodName(),
    nodeName: GetNodeName(),
    trigger: false,
}

// New returns a Scheduler
func New(ctx context.Context,
	client clientset.Interface,
    //cache   *cache.Cache,
    cache   *cache.EventCache,
    //cpp     *CostPerPodState,
    //cs      *CapacityState,
    dfs      *DualFilterState,
	opts ...Option) (*RemoteScheduler, error) {

//	stopEverything := ctx.Done()

	options := defaultRemoteOptions
	for _, opt := range opts {
		opt(&options)
	}

    /* Run metrics collection */
    pipe := make(chan *mat.Dense)
    mc := metrics.New(pipe,
        metrics.WithFilter(&metrics.DynEMA{
            AlphaUpLow: 0.09,
            AlphaDownLow: 0.0625,
            AlphaUpHigh: 0.333,
            AlphaDownHigh: 0.2,
            NoiseWindow: 4,
        }))

    /* Run fpca */
    fp := fpca.New(pipe)

    rmt := &RemoteScheduler{
        podName:        options.podName,
        nodeName:       options.nodeName,
        mc:             mc,
        fp:             fp,
        cache:          cache,
        //costPerPod:     cpp,
        capacity:       dfs,
        client:         client,
        ctx: ctx,
    }

    if options.trigger {
        cache.SetRemote(rmt)
        cache.SetCapacity(dfs)
        cache.SetPublisher(&rmt.Publisher)
    }

    cache.Start()

    rmt.Publisher.SetUp(ctx, options.nodeName)

	return rmt, nil
}

/* Core Scheduling loop */
/*
TODO: Implement error handling
*/
func (rmt *RemoteScheduler) Start() {
    ticker := time.NewTicker(time.Second)
    //lastPodCount := rmt.cache.GetPodCount()
    //lastSignal := 0.0
    defer ticker.Stop()
    for {
        <-ticker.C
        signal, err := rmt.CalculateSignal()
        if err != nil {
            log.Printf("error generating signal: %s", err)
            continue
        }
        log.Printf("(remote) signal = %.4f", signal)


        if !rmt.cache.IsWaiting() {
            podCount := rmt.cache.GetPodCount()
            rmt.capacity.Update(podCount, signal)
        }
        //currPodCount := rmt.cache.GetPodCount()
        //rmt.costPerPod.Update(currPodCount - lastPodCount, signal - lastSignal)
        //cost := rmt.costPerPod.GetPodCost()
        //log.Printf("(remote) per-pod cost = %.4f", cost)
        //lastPodCount = currPodCount
        //lastSignal = signal
        available := rmt.capacity.GetCapacityFromSignal(signal)
        over := rmt.cache.GetOverProvision()


        //log.Printf("(remote) available = %.4f", available)
        rmt.Publish(
            types.WithSignal(signal),
            types.WithCapacity(available),
            types.WithOverprovision(over))
    }
}

