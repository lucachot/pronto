package remote

import (
	"log"
	"sync"

	"github.com/LucaChot/pronto/src/remote/kalman"
)


type SGD struct {
    cost        float64
    capacity    float64
    costLearningRate        float64
    capacityLearningRate    float64
}

func (sgd *SGD) Update(podCount float64, signal float64) {
    // latent (pre‐ReLU) prediction
    y := sgd.cost * podCount + sgd.capacity

    // skip *only* if both prediction AND measurement are zero
    if y <= 0 && signal <= 0 {
        return
    }

    // compute error on latent model
    e := y - signal

    // two‐rate SGD on (y - s)^2
    sgd.capacity -= sgd.capacityLearningRate * e          // small drift for b
    sgd.cost -= sgd.costLearningRate * e * podCount      // fast adaptation for a
    log.Printf("(capacity) capacity: %.4f | cost: %.4f", sgd.capacity, sgd.cost)
}

type CapacityState struct {
    mu              sync.Mutex
    filter          kalman.KalmanFilter
}


func NewCapacityState() *CapacityState{
    initX := []float64{1.4, -1}
    initP := []float64{1e-4,0,0,1e-3}
    Q := []float64{1e-4,0,0,1e-4}
    R := 1.0

    kf, err := kalman.NewKalmanFilter2D(
        initX,
        initP,
        Q,
        R)
    if err != nil {
        log.Fatalf("unable to construct kalman filter: %v", err)
    }
    log.Printf("instantiated kalman filter: %+v", kf)

    cs := &CapacityState{
        filter:     kf,
    }

    return cs
}

func (cpp *CapacityState) update(podCount float64, signal float64) {
    // Predict every interval
    cpp.filter.Predict()

    x := cpp.filter.State()

    signal_pred := x[0] + x[1] * podCount

    if signal_pred <= 0 && signal <= 0 {
        return
    }

    cpp.filter.Update(podCount, signal)
    newX := cpp.filter.State()
    if newX[1] >= 0{
        log.Print("(capacity) had to revert")
        cpp.filter.Revert()
    }
    x = cpp.filter.State()
    log.Printf("(kalman-2d) capacity: %f cost: %f", x[0], x[1])
}

func (cs *CapacityState) Update(podCount int, signal float64) {
    cs.mu.Lock()
    defer cs.mu.Unlock()
    cs.update(float64(podCount), signal)
}

func (cs *CapacityState) GetCapacityFromSignal(signal float64) float64 {
    cs.mu.Lock()
    x := cs.filter.State()
    cs.mu.Unlock()

    log.Printf("(capacity) capacity: %.4f | cost: %.4f", x[0], x[1])

    return  -signal * 2 / x[1]
}

func (cs *CapacityState) GetCapacityFromPodCount(podCount int) float64 {
    cs.mu.Lock()
    x := cs.filter.State()
    cs.mu.Unlock()
    return  (-x[0] * 2 / x[1]) - float64(podCount)
}
