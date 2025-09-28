package remote

import (
	"fmt"
	"log"
	"sync"
)

// KalmanFilter1D represents a 1D Kalman filter.
type KalmanFilter1D struct {
	StateEstimate   float64
	ErrorCovariance float64

	ProcessNoise     float64 // Q
	MeasurementNoise float64 // R

	// Predicted values (internal, set by Predict method)
	predictedStateEstimate   float64
	predictedErrorCovariance float64

    // Fields for decay towards its own initial state
    // If 0 or invalid, this specific decay logic is skipped.
	InitialStateTarget float64
	DecayLambda        float64
}

// NewKalmanFilter1D creates and initializes a new KalmanFilter1D.
func NewKalmanFilter1D(initialState, initialCovariance, processNoise, measurementNoise, decayLambda float64) *KalmanFilter1D {
	if initialCovariance < 0 {
		// Error covariance must be non-negative
		initialCovariance = 0
	}
	if processNoise < 0 {
		processNoise = 0
	}
	if measurementNoise <= 0 {
		// Measurement noise variance should be positive
		fmt.Println("Warning: Measurement noise (R) should be positive. Using a small default if zero or negative.")
		measurementNoise = 1e-9
	}
    validDecayLambda := 0.0
	if decayLambda > 0.0 && decayLambda <= 1.0 {
		validDecayLambda = decayLambda
	}
	return &KalmanFilter1D{
		StateEstimate:    initialState,
		ErrorCovariance:  initialCovariance,
		ProcessNoise:     processNoise,
		MeasurementNoise: measurementNoise,
        InitialStateTarget: initialState, // Store the provided initialState as the decay target
		DecayLambda:        validDecayLambda,
	}
}

// Predict performs the prediction step of the Kalman filter.
func (kf *KalmanFilter1D) Predict(applyDecayToInitialTarget bool) {
	F_effective := 1.0
	deterministic_shift_to_target := 0.0

	if applyDecayToInitialTarget && kf.DecayLambda > 0.0 {
		// Model: state_k = (1-lambda)*state_{k-1} + lambda*InitialStateTarget
		F_effective = (1.0 - kf.DecayLambda)
		deterministic_shift_to_target = kf.DecayLambda * kf.InitialStateTarget
	}
	// else, F_effective remains 1.0, and deterministic_shift_to_target remains 0.0,
	// resulting in a standard prediction for a constant state: state_k = state_{k-1}

	// Stochastic part of prediction using F_effective
	kf.predictedStateEstimate = F_effective * kf.StateEstimate
	// Covariance propagation
	kf.predictedErrorCovariance = F_effective*kf.ErrorCovariance*F_effective + kf.ProcessNoise

	// Add the deterministic shift (if any) after stochastic prediction
	kf.predictedStateEstimate += deterministic_shift_to_target
}

// UpdateKFb performs the measurement update step for the Kalman filter estimating 'b'.
// The model for 'b' is: signal_k = 1*b_k + (a_estimate_prev * uVal) + noise.
func (kf *KalmanFilter1D) UpdateKFb(signal, uVal, aEstimatePrev float64) {
	// Measurement matrix H_b is 1 for estimating 'b' directly.
	H_b := 1.0

	expectedMeasurement := H_b*kf.predictedStateEstimate + aEstimatePrev*uVal
	innovation := signal - expectedMeasurement

	innovationCovariance := H_b*kf.predictedErrorCovariance*H_b + kf.MeasurementNoise

	kalmanGain := 0.0
	if innovationCovariance != 0 { // Avoid division by zero
		kalmanGain = (kf.predictedErrorCovariance * H_b) / innovationCovariance
	}

	kf.StateEstimate = kf.predictedStateEstimate + kalmanGain*innovation

	kf.ErrorCovariance = (1.0 - kalmanGain*H_b) * kf.predictedErrorCovariance
	if kf.ErrorCovariance < 0 { // Ensure covariance remains non-negative
		kf.ErrorCovariance = 1e-9 // A small positive value
	}
}

// UpdateKFa performs the measurement update step for the Kalman filter estimating 'a'.
// The model for 'a' is: signal_k = uVal*a_k + b_estimate_current + noise.
func (kf *KalmanFilter1D) UpdateKFa(signal, uVal, bEstimateCurrent float64) {
	// Measurement matrix H_a for estimating 'a' is uVal (u_k).
	H_a := uVal

	expectedMeasurement := H_a*kf.predictedStateEstimate + bEstimateCurrent
	innovation := signal - expectedMeasurement

	innovationCovariance := H_a*kf.predictedErrorCovariance*H_a + kf.MeasurementNoise

	kalmanGain := 0.0
	if innovationCovariance != 0 { // Avoid division by zero
		kalmanGain = (kf.predictedErrorCovariance * H_a) / innovationCovariance
	}

	kf.StateEstimate = kf.predictedStateEstimate + kalmanGain*innovation

	kf.ErrorCovariance = (1.0 - kalmanGain*H_a) * kf.predictedErrorCovariance
	if kf.ErrorCovariance < 0 { // Ensure covariance remains non-negative
		kf.ErrorCovariance = 1e-9 // A small positive value
	}
}

type DualFilterState struct {
    mu                  sync.Mutex
    costFilter          *KalmanFilter1D
    capacityFilter      *KalmanFilter1D
}


func NewDualFilterState() *DualFilterState{
	initialB := 1.3  // Initial guess for 'b'
	initialPb := 0.01 // Initial uncertainty (variance) for 'b'
	Qb := 1e-4       // Process noise for 'b'

	initialA := -1.0  // Initial guess for 'a'
	initialPa := 0.2 // Initial uncertainty (variance) for 'a'
	Qa := 1e-3       // Process noise for 'a'

	// This is the variance of the noise inherent in measurement
	Rsignal := 0.5

	// Create the Kalman filter instances
	kfB := NewKalmanFilter1D(initialB, initialPb, Qb, Rsignal, 0.0)
	kfA := NewKalmanFilter1D(initialA, initialPa, Qa, Rsignal, 0.05)

    dfs := &DualFilterState{
        capacityFilter: kfB,
        costFilter:     kfA,}

    return dfs
}

func (dfs *DualFilterState) update(podCount float64, signal float64) {
    // Predict every interval
    log.Printf("(capacity) podCount = %f", podCount)
    dfs.capacityFilter.Predict(false)
    dfs.costFilter.Predict(podCount == 0)

    signal_pred := dfs.capacityFilter.StateEstimate + dfs.costFilter.StateEstimate * podCount

    if signal_pred <= 0 && signal <= 0 {
        return
    }

    // 2. Measurement Update Step for kfB (estimates 'b')
    dfs.capacityFilter.UpdateKFb(signal, podCount, dfs.costFilter.StateEstimate)

    // 3. Measurement Update Step for kfA (estimates 'a')
    dfs.costFilter.UpdateKFa(signal, podCount, dfs.capacityFilter.StateEstimate)
    capacity := dfs.capacityFilter.StateEstimate
    cost := dfs.costFilter.StateEstimate
    log.Printf("(two-kalman-1d) capacity: %f cost: %f", capacity, cost)
}

func (dfs *DualFilterState) Update(podCount int, signal float64) {
    dfs.mu.Lock()
    defer dfs.mu.Unlock()
    dfs.update(float64(podCount), signal)
}

func (dfs *DualFilterState) GetCapacityFromSignal(signal float64) float64 {
    dfs.mu.Lock()
    capacity := dfs.capacityFilter.StateEstimate
    cost := dfs.costFilter.StateEstimate
    dfs.mu.Unlock()

    log.Printf("(capacity) capacity: %.4f | cost: %.4f", capacity, cost)

    return  -signal / cost
}

func (dfs *DualFilterState) GetCapacityFromPodCount(podCount int) float64 {
    dfs.mu.Lock()
    capacity := dfs.capacityFilter.StateEstimate
    cost := dfs.costFilter.StateEstimate
    dfs.mu.Unlock()
    return  (-capacity / cost) - float64(podCount)
}

