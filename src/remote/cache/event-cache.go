package cache

import (
	"log"
	"sync"
	"time"

	"github.com/LucaChot/pronto/src/remote/types"
)

type PodInfo struct {
    containerInfo  ContainerInfo
    startTime      time.Time
}

func NewPodInfo() *PodInfo {
    return &PodInfo{
        startTime: time.Now(),
    }
}

type EventCache struct {
    mu          sync.Mutex
    informer    EventInformer

    timer       *time.Timer
    ends        time.Time

	creating 	        map[string]struct{}
	deleting	        map[string]struct{}
	podContainers       map[string]*PodInfo
	podCount	        int
    lastSignal          float64

    signal              types.Signal
    capacity            types.Capacity
    publisher           types.Publisher

    createInterval      time.Duration
    deleteInterval      time.Duration

    BaselineEstimator
    overProvision       int
}


type Topic int

const (
    Create Topic = iota
    Start
    Exit
    Delete
)

type ContainerInfo struct {
	creating    int
	running 	int
	deleting		int
}

type Event struct {
	containerID string
	podID       string
	topic       Topic
}



type EventInformer interface {
    Start()
    SetOnEvent(func(e Event))
}

func (ec *EventCache) isWaiting() bool {
	if len(ec.creating) > 0 {
		return true
	}
	if len(ec.deleting) > 0 {
		return true
	}
    if time.Now().Before(ec.ends) {
        return true
    }
	return false
}

func (ec *EventCache) GetOverProvision() float64 {
    ec.mu.Lock()
    defer ec.mu.Unlock()
    return float64(ec.overProvision)
}

func (ec *EventCache) GetPodCount() int {
    ec.mu.Lock()
    defer ec.mu.Unlock()
    return ec.podCount
}

func (ec *EventCache) IsWaiting() bool {
    ec.mu.Lock()
    defer ec.mu.Unlock()
    return ec.isWaiting()
}

func (ec *EventCache) OnTrigger() {
    ec.mu.Lock()
	if ec.isWaiting() {
        log.Print("(cache) event prevented trigger")
        ec.mu.Unlock()
		return
	}
    podCount := ec.podCount
    overProv := ec.overProvision
    ec.mu.Unlock()

    log.Printf("(cache) running signal and cost generation")
	signal, err := ec.signal.CalculateSignal()
    if err != nil {
        log.Printf("(cache) error generating signal: %s", err)
        return
    }
    ec.capacity.Update(podCount, signal)
    capacity := ec.capacity.GetCapacityFromSignal(signal)
    log.Printf("(cache) signal = %.4f", signal)
    log.Printf("(cache) per-pod cost = %.4f", capacity)
	ec.publisher.Publish(
        types.WithSignal(signal),
        types.WithCapacity(capacity),
        types.WithOverprovision(float64(overProv)))
}

func (ec *EventCache) OnEvent(e Event) {
    ec.mu.Lock()
    defer ec.mu.Unlock()
    log.Printf("(cache) received an event: %+v", e)
	switch e.topic {
    case Create:
        if _, ok := ec.podContainers[e.podID]; !ok {
            ec.podCount += 1
            ec.podContainers[e.podID] = NewPodInfo()
        }
        ec.podContainers[e.podID].containerInfo.creating += 1
        ec.creating[e.containerID] = struct{}{}
    case Start:
        delete(ec.creating, e.containerID)
        containerInfo, ok:= ec.podContainers[e.podID]
        if !ok {
            return
        }
        containerInfo.containerInfo.creating -= 1
        containerInfo.containerInfo.running += 1
        if containerInfo.containerInfo.creating == 0 {
            ends := time.Now().Add(ec.createInterval)
            if ends.After(ec.ends) {
                if ec.timer != nil {
                    ec.timer.Stop()
                }
                ec.timer = time.AfterFunc(ec.createInterval, ec.OnTrigger)
                ec.ends = ends
            }
        }
    case Exit:
        containerInfo, ok := ec.podContainers[e.podID]
        if !ok {
            return
        }
        containerInfo.containerInfo.running -= 1
        containerInfo.containerInfo.deleting += 1
        ec.deleting[e.containerID] = struct{}{}
    case Delete:
        delete(ec.deleting, e.containerID)
        containerInfo, ok := ec.podContainers[e.podID]
        if !ok {
            return
        }
        containerInfo.containerInfo.deleting -= 1
        if containerInfo.containerInfo.deleting == 0 {
            if containerInfo.containerInfo.running == 0 {
                //if !containerInfo.startTime.IsZero() {
                    //oversat := ec.BaselineEstimator.AddSample(time.Since(containerInfo.startTime).Seconds())
                    ////available := ec.capacity.GetCapacityFromPodCount(ec.podCount)
                    ////if available < 0.01 {
                    //if !oversat {
                        //ec.overProvision += 1
                    //} else {
                        //ec.overProvision /= 2
                    //}
                //}
                delete(ec.podContainers, e.podID)
                ec.podCount -= 1
            }
            ends := time.Now().Add(ec.deleteInterval)
            if ends.After(ec.ends) {
                if ec.timer != nil {
                    ec.timer.Stop()
                }
                ec.timer = time.AfterFunc(ec.deleteInterval, ec.OnTrigger)
                ec.ends = ends
            }
        }
	}
}

func (ec *EventCache) SetRemote(signal types.Signal) {
    ec.mu.Lock()
    ec.signal = signal
    ec.mu.Unlock()
}

func (ec *EventCache) SetCapacity(capacity types.Capacity) {
    ec.mu.Lock()
    ec.capacity = capacity
    ec.mu.Unlock()
}

func (ec *EventCache) SetPublisher(publisher types.Publisher) {
    ec.mu.Lock()
    ec.publisher = publisher
    ec.mu.Unlock()
}

type eventCacheOptions struct {
    createInterval    time.Duration
    deleteInterval    time.Duration
}


// Option configures a Scheduler
type Option func(*eventCacheOptions)

func WithCreateInterval(interval time.Duration) Option {
	return func(o *eventCacheOptions) {
		o.createInterval = interval
	}
}

func WithExitInterval(interval time.Duration) Option {
	return func(o *eventCacheOptions) {
		o.deleteInterval = interval
	}
}

var defaultEventCacheOptions = eventCacheOptions{
    createInterval:     100 * time.Millisecond,
    deleteInterval:     300 * time.Millisecond,
}

func NewEventCache(informer EventInformer, opts ...Option) *EventCache {
	options := defaultEventCacheOptions
	for _, opt := range opts {
		opt(&options)
	}



    c := &EventCache{
        informer: informer,
        creating: make(map[string]struct{}),
        deleting: make(map[string]struct{}),
        podContainers: make(map[string]*PodInfo),

        ends: time.Now(),

        createInterval: options.createInterval,
        deleteInterval: options.deleteInterval,
    }

    c.SetBaselineEstimator(10, 0.2, 0.05, 1.2, 0.25)

    c.informer.SetOnEvent(c.OnEvent)
    return c
}

func (ec *EventCache) Start() {
    ec.informer.Start()
}
