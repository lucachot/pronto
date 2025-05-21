package remote

import "github.com/LucaChot/pronto/src/remote/kalman"

type CombinedFilter struct {
    cs  *CapacityState
    cpp *CostPerPodState
    dfs *DualFilterState
}

func NewCombinedFilter() (*CombinedFilter) {

    cpp := NewCostPerPodState (
        WithConstructor(kalman.NewKalmanFilter1D),
        WithGetPodCost(func(cpp *CostPerPodState) {
                        cpp.GetCostFunc = cpp.GetPodCost1D
                    }),
        WithUpdate(func(cpp *CostPerPodState) {
                        cpp.UpdateFunc = cpp.UpdatePodCost1D
                    }),
        WithInitX([]float64{-1}),
        WithInitP([]float64{1e-1}),
        WithQ([]float64{1e-2}),
        WithR(1e-1))

    cs := NewCapacityState()

    dfs := NewDualFilterState()

    return &CombinedFilter {
        cs: cs,
        cpp: cpp,
        dfs: dfs,
    }
}

func (cf *CombinedFilter) Update(podCount int, signal float64) {
   cf.cs.Update(podCount, signal)
   cf.cpp.Update(podCount, signal)
   cf.dfs.Update(podCount, signal)
}

func (cf *CombinedFilter) GetCapacityFromPodCount(podCount int) float64 {
    return cf.dfs.GetCapacityFromPodCount(podCount)
}

func (cf *CombinedFilter) GetCapacityFromSignal(signal float64) float64 {
    return cf.dfs.GetCapacityFromSignal(signal)
}
