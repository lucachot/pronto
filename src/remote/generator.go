package remote

import (
	"log"
	"math"
	"sync"

	"github.com/LucaChot/pronto/src/remote/kalman"
)

const epsilon = 1e-3

type CostPerPodState struct {
    mu              sync.Mutex
    filter          kalman.KalmanFilter
    lastSignal      float64
    lastBeta        float64
    lastPodCount    int

    lowerBounds     float64
    upperBounds     float64

    UpdateFunc          func(podCount int, signal float64)
    GetCostFunc      func() float64
}

type KalmanStateOptions struct {
    initX           []float64
    initP           []float64
    Q               []float64
    R               float64
    contructor      func(initX []float64, initP []float64, Q []float64, R float64) (kalman.KalmanFilter, error)

    lowerBounds     float64
    upperBounds     float64

    initSignal      float64
    initPodCount    int

    setUpdate       func(cpp *CostPerPodState)
    setGetPodCost      func(cpp *CostPerPodState)
}

// Option configures a Scheduler
type KalmanStateOption func(*KalmanStateOptions)

// WithClientSet sets clientSet for the scheduling frameworkImpl.
func WithInitX(initX []float64) KalmanStateOption {
	return func(o *KalmanStateOptions) {
		o.initX = initX
	}
}

func WithInitP(initP []float64) KalmanStateOption {
	return func(o *KalmanStateOptions) {
		o.initP = initP
	}
}

func WithQ(q []float64) KalmanStateOption {
	return func(o *KalmanStateOptions) {
		o.Q = q
	}
}

func WithR(r float64) KalmanStateOption {
	return func(o *KalmanStateOptions) {
		o.R = r
	}
}

func WithConstructor(contructor func(initX []float64, initP []float64, Q []float64, R float64) (kalman.KalmanFilter, error)) KalmanStateOption {
	return func(o *KalmanStateOptions) {
		o.contructor = contructor
	}
}

func WithLowerBounds(lowerBounds float64) KalmanStateOption {
	return func(o *KalmanStateOptions) {
		o.lowerBounds = lowerBounds
	}
}

func WithUpperBounds(upperBounds float64) KalmanStateOption {
	return func(o *KalmanStateOptions) {
		o.upperBounds = upperBounds
	}
}

func WithUpdate(setUpdate func(cpp *CostPerPodState)) KalmanStateOption {
	return func(o *KalmanStateOptions) {
		o.setUpdate = setUpdate
	}
}

func WithGetPodCost(setPodCost func(cpp *CostPerPodState)) KalmanStateOption {
	return func(o *KalmanStateOptions) {
		o.setGetPodCost = setPodCost

	}
}

var defaultKalmanState = KalmanStateOptions{
    initX:          []float64{0, -0.1},
    initP:          []float64{0,0,0,1e3},
    Q:              []float64{0,0,0,1e-2},
    R:              1e-1,
    contructor:     kalman.NewKalmanFilter2D,
    lowerBounds:    0,
    upperBounds:    -1.4,
    initSignal:     1.4,
    initPodCount:   0,
    setUpdate:      func(cpp *CostPerPodState) {
                        cpp.UpdateFunc = cpp.UpdateConst
                    },
    setGetPodCost:  func(cpp *CostPerPodState) {
                        cpp.GetCostFunc = cpp.GetPodCostConst
                    },
}


func NewCostPerPodState(opts ...KalmanStateOption) *CostPerPodState{
    options := defaultKalmanState
	for _, opt := range opts {
		opt(&options)
	}

    kf, err := options.contructor(
        options.initX,
        options.initP,
        options.Q,
        options.R)

    if err != nil {
        log.Fatalf("unable to construct kalman filter: %v", err)
    }
    log.Printf("instantiated kalman filter: %+v", kf)

    cpp := &CostPerPodState{
        filter:     kf,
        lastSignal: options.initSignal,
        lastPodCount: options.initPodCount,
        lowerBounds: options.lowerBounds,
        upperBounds: options.upperBounds,
    }

    options.setUpdate(cpp)
    options.setGetPodCost(cpp)

    return cpp
}

func (cpp *CostPerPodState) Update(podDiff int, signalDiff float64) float64 {
    cpp.mu.Lock()
    defer cpp.mu.Unlock()
    cpp.UpdateFunc(podDiff, signalDiff)
    return cpp.GetCostFunc()
}

func (cpp *CostPerPodState) GetPodCost() float64 {
    cpp.mu.Lock()
    defer cpp.mu.Unlock()
    return cpp.GetCostFunc()
}

func (cpp *CostPerPodState) UpdateConst(podCount int, signal float64, ) {}

func (cpp *CostPerPodState) GetPodCostConst() float64 {
    return math.Abs(cpp.filter.State()[0])
}

func (cpp *CostPerPodState) UpdatePodCost1D(podCount int, signal float64) {
    y := signal - cpp.lastSignal
    u := podCount - cpp.lastPodCount
    cpp.lastSignal = signal
    cpp.lastPodCount = podCount
    x := cpp.filter.State()

    // Predict every interval
    cpp.filter.Predict()

    // Only update for pod-start events with positive drop
    // Skip if no pods started or signal floor-limited
    //if u <= 0 || 3 * y >= float64(u) * cpp.lastBeta {
    if u <= 0 || y >= -epsilon {
        log.Printf("(kalman-1d) cost: %f", x[0])
        return
    }

    // Optional: skip if measurement indicates no drop (censored)
    //if y >= 0 {
        //log.Printf("(kalman-1d) cost: %f", x[0])
        //return
    //}

    cpp.filter.Update(float64(u), y)
    x = cpp.filter.State()
    if x[0] > 0 {
        newX := [1]float64{-epsilon}
        cpp.filter.ForceState(newX[:])
    }
    log.Printf("(kalman-1d) cost: %f", x[0])
}

func (cpp *CostPerPodState) GetPodCost1D() float64 {
    return math.Abs(cpp.filter.State()[0])
}

func (cpp *CostPerPodState) UpdatePodCost2D(podCount int, signal float64) {
    y := signal - cpp.lastSignal
    u := podCount - cpp.lastPodCount
    cpp.lastSignal = signal
    cpp.lastPodCount = podCount

    // Predict every interval
    cpp.filter.Predict()

    // Only update for pod-start events with positive drop
    // Skip if no pods started or signal floor-limited
    //if u <= 0 || 3 * y >= float64(u) * cpp.lastBeta {
    if u <= 0 || y >= -epsilon {
        return
    }

    // Optional: skip if measurement indicates no drop (censored)
    if y >= 0 {
        return
    }

    cpp.filter.Update(float64(u), y)
    x := cpp.filter.State()
    if x[1] > cpp.lowerBounds {
        newX := [2]float64{0, cpp.lowerBounds}
        cpp.filter.ForceState(newX[:])
    }
    if x[1] < cpp.upperBounds {
        newX := [2]float64{0, cpp.upperBounds}
        cpp.filter.ForceState(newX[:])
    }
}

func (cpp *CostPerPodState) GetPodCost2D() float64 {
    return math.Abs(cpp.filter.State()[1])
}
