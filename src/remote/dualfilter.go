package remote

import (
	"fmt"
	"log"
	"sync"
	// For more realistic random noise in simulation, you might use:
	// "math/rand"
	// "time"
)

// KalmanFilter1D represents a 1D Kalman filter.
type KalmanFilter1D struct {
	StateEstimate   float64 // x_hat: current state estimate (e.g., 'a' or 'b')
	ErrorCovariance float64 // P: current error covariance of the state estimate

	ProcessNoise     float64 // Q: process noise covariance (variance of how much the state can change between steps)
	MeasurementNoise float64 // R: measurement noise covariance (variance of the sensor/signal noise)

	// Predicted values (internal, set by Predict method)
	predictedStateEstimate   float64 // x_hat_minus: state estimate after prediction step
	predictedErrorCovariance float64 // P_minus: error covariance after prediction step

    // Fields for decay towards its own initial state (primarily for kfA)
	InitialStateTarget float64 // The original initialState this filter was constructed with.
	DecayLambda        float64 // The factor lambda (0 < lambda <= 1) for decaying towards InitialStateTarget.
	                         // If 0 or invalid, this specific decay logic is skipped.
}

// NewKalmanFilter1D creates and initializes a new KalmanFilter1D.
// initialState: Initial guess for the state (e.g., 'a' or 'b').
// initialCovariance: Initial uncertainty (variance) of the initialState guess.
// processNoise: Variance representing how much the true state is expected to change between time steps.
//               For parameters assumed to be constant or slowly varying, this is small.
// measurementNoise: Variance of the noise in the measurements (the `R_signal` value).
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
		// Setting a very small default if an invalid value is passed, but ideally, it's well-defined.
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
// For this problem, parameters 'a' and 'b' are modeled as constants,
// so the state transition matrix F is effectively 1.
//func (kf *KalmanFilter1D) Predict() {
	//// Predict next state (for F=1, state doesn't change in prediction)
	//kf.predictedStateEstimate = kf.StateEstimate
//
	//// Predict next error covariance: P_minus = P + Q
	//kf.predictedErrorCovariance = kf.ErrorCovariance + kf.ProcessNoise
//}

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
// signal: The current raw signal measurement (signal_k).
// uVal: The current value of 'u' (u_k).
// aEstimatePrev: The estimate of 'a' from the previous time step (a_hat_{k-1}).
func (kf *KalmanFilter1D) UpdateKFb(signal, uVal, aEstimatePrev float64) {
	// Measurement matrix H_b is 1 for estimating 'b' directly.
	H_b := 1.0

	// Innovation (measurement residual): y_b = signal_k - (H_b * b_hat_minus + a_hat_prev * u_k)
	// This is: observed_signal - expected_signal_based_on_prediction_and_other_known_terms
	expectedMeasurement := H_b*kf.predictedStateEstimate + aEstimatePrev*uVal
	innovation := signal - expectedMeasurement

	// Innovation covariance: S_b = H_b * P_b_minus * H_b^T + R_b
	// Since H_b = 1, S_b = P_b_minus + R_b
	// kf.MeasurementNoise is used as R_b (R_signal).
	innovationCovariance := H_b*kf.predictedErrorCovariance*H_b + kf.MeasurementNoise

	// Kalman Gain: K_b = P_b_minus * H_b^T * S_b^-1
	// Since H_b = 1, K_b = P_b_minus / S_b
	kalmanGain := 0.0
	if innovationCovariance != 0 { // Avoid division by zero
		kalmanGain = (kf.predictedErrorCovariance * H_b) / innovationCovariance
	}

	// Update state estimate: b_hat_k = b_hat_minus + K_b * y_b
	kf.StateEstimate = kf.predictedStateEstimate + kalmanGain*innovation

	// Update error covariance: P_b_k = (I - K_b * H_b) * P_b_minus
	kf.ErrorCovariance = (1.0 - kalmanGain*H_b) * kf.predictedErrorCovariance
	if kf.ErrorCovariance < 0 { // Ensure covariance remains non-negative
		kf.ErrorCovariance = 1e-9 // A small positive value
	}
}

// UpdateKFa performs the measurement update step for the Kalman filter estimating 'a'.
// The model for 'a' is: signal_k = uVal*a_k + b_estimate_current + noise.
// signal: The current raw signal measurement (signal_k).
// uVal: The current value of 'u' (u_k). This acts as H_a.
// bEstimateCurrent: The estimate of 'b' from KFb in the current time step (b_hat_k).
func (kf *KalmanFilter1D) UpdateKFa(signal, uVal, bEstimateCurrent float64) {
	// Measurement matrix H_a for estimating 'a' is uVal (u_k).
	H_a := uVal

	// Innovation (measurement residual): y_a = signal_k - (H_a * a_hat_minus + b_hat_current)
	expectedMeasurement := H_a*kf.predictedStateEstimate + bEstimateCurrent
	innovation := signal - expectedMeasurement

	// Innovation covariance: S_a = H_a * P_a_minus * H_a^T + R_a
	// kf.MeasurementNoise is used as R_a (R_signal).
	innovationCovariance := H_a*kf.predictedErrorCovariance*H_a + kf.MeasurementNoise

	// Kalman Gain: K_a = P_a_minus * H_a^T * S_a^-1
	// If uVal (H_a) is 0, the numerator P_a_minus * H_a becomes 0, so K_a is 0.
	// This means 'a' is not updated by the measurement if uVal = 0, fulfilling
	// the condition to primarily learn 'a' when u >= 1 (assuming u is integer pods).
	kalmanGain := 0.0
	if innovationCovariance != 0 { // Avoid division by zero
		kalmanGain = (kf.predictedErrorCovariance * H_a) / innovationCovariance
	}

	// Update state estimate: a_hat_k = a_hat_minus + K_a * y_a
	kf.StateEstimate = kf.predictedStateEstimate + kalmanGain*innovation

	// Update error covariance: P_a_k = (I - K_a * H_a) * P_a_minus
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
	// KF for 'b' (parameter for offset)
	initialB := 1.3  // Initial guess for 'b'
	initialPb := 0.01 // Initial uncertainty (variance) for 'b'
	Qb := 1e-4       // Process noise for 'b'. Small value if 'b' is expected to be fairly constant.
	// This value heavily influences how quickly 'b' can adapt vs. how smooth it is.

	// KF for 'a' (parameter for slope related to 'u')
	initialA := -1.0  // Initial guess for 'a'
	initialPa := 0.2 // Initial uncertainty (variance) for 'a'
	Qa := 1e-3       // Process noise for 'a'. Small value if 'a' is fairly constant when active.

	// Shared Measurement Noise for the raw signal
	// This is the variance of the noise inherent in your `signal` measurement.
	Rsignal := 0.5 // Example: signal noise has variance 0.1

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
    // kfB uses the signal_k, u_k, and the *previous* estimate of 'a' (aEstimatePrev).
    dfs.capacityFilter.UpdateKFb(signal, podCount, dfs.costFilter.StateEstimate)

    // 3. Measurement Update Step for kfA (estimates 'a')
    // kfA uses the signal_k, u_k (which forms its H_a matrix), and the
    // *current* estimate of 'b' (currentBEstimate) from kfB in this same time step.
    // Learning of 'a' primarily happens when u_k >= 1 because H_a = u_k.
    // If u_k = 0, H_a = 0, and the Kalman Gain for 'a' becomes 0.
    if podCount > 0 {
        dfs.costFilter.UpdateKFa(signal, podCount, dfs.capacityFilter.StateEstimate)
    }
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

