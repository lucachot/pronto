package metrics

import "errors"

type Filter interface {
    Update(newY []float64) ([]float64, error)
}

type NullFilter struct {}

func (nf * NullFilter) Update(newY []float64) ([]float64, error) {return newY, nil}


type DynEMA struct {
    AlphaUpLow  float64
    AlphaDownLow  float64
    AlphaUpHigh  float64
    AlphaDownHigh  float64

    NoiseWindow int
    upCount     int
    downCount   int

    y           []float64
}

func (dema *DynEMA) Update(newY []float64) ([]float64, error) {
    if dema.y == nil {
        dema.y = make([]float64, len(newY))
        for i, x := range newY {
            dema.y[i] = x
        }
        return newY, nil
    }
    if len(dema.y) != len(newY) {
        return dema.y, errors.New("new y needs to have the same dimensions as original")
    }

    for i := range dema.y {
        old := dema.y[i]
        unfilter := newY[i]
        var alpha float64
        if unfilter > old {
            dema.upCount += 1
            dema.downCount = 0
            if dema.upCount >= dema.NoiseWindow {
                alpha = dema.AlphaUpHigh
            } else {
                alpha = dema.AlphaUpLow
            }
        } else if unfilter < old {
            dema.downCount += 1
            dema.upCount = 0
            if dema.downCount >= dema.NoiseWindow {
                alpha = dema.AlphaDownHigh
            } else {
                alpha = dema.AlphaDownLow
            }
        } else {
            dema.upCount = 0
            dema.downCount = 0
            alpha = dema.AlphaDownLow
        }
        dema.y[i] = alpha * unfilter + (1 - alpha) * old
    }
    return dema.y, nil
}
