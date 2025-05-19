package metrics

import (
	"errors"
	"log"
	"sync/atomic"
	"time"

	"gonum.org/v1/gonum/mat"
)

const (
    d = 2
    b = 10
)

type MetricsCollector struct {
    interval    time.Duration
    batchSize   int
    dims        int
    metrics     []func() float64
    y           atomic.Pointer[[]float64]
    output      chan<- *mat.Dense
    filter      Filter
    mr          *MetricReader
}

type metricOptions struct {
    interval    time.Duration
    batchSize   int
    metrics     []func() float64
    filter      Filter
}

// Option configures a Scheduler
type Option func(*metricOptions)

func WithInterval(interval time.Duration) Option {
	return func(o *metricOptions) {
		o.interval = interval
	}
}

func WithBatchSize(batchSize int) Option {
	return func(o *metricOptions) {
		o.batchSize = batchSize
	}
}

func WithMetric(metricFunc func() float64) Option {
	return func(o *metricOptions) {
		o.metrics = append(o.metrics, metricFunc)
	}
}


func WithFilter(filter Filter) Option {
	return func(o *metricOptions) {
		o.filter = filter
	}
}


var defaultMetricOptions = metricOptions{
    interval:       100 * time.Millisecond,
    batchSize:      10,
    metrics:        []func() float64{collectCPU, collectRAM},
    filter:         &NullFilter{},
}

/* Look at potentially parallelising the setup */
func New(output chan<- *mat.Dense, opts ...Option) (*MetricsCollector) {
	options := defaultMetricOptions
	for _, opt := range opts {
		opt(&options)
	}

	mc := MetricsCollector{
        interval:   options.interval,
        batchSize:  options.batchSize,
        dims:       len(options.metrics),
        metrics:    options.metrics,
        output:     output,
        filter:     options.filter,
        mr:         NewMetricReader(),
    }

    go mc.Collect()

	return &mc
}


func (mc *MetricsCollector) GetY() ([]float64, error) {
    y := mc.y.Load()
    if y == nil {
        return nil, errors.New("no y slice is available")
    }
    return *y, nil
}

/*
TODO: Investigate difference between ticker and time.Sleep()
*/
func (mc *MetricsCollector) Collect() {
    ticker := time.NewTicker(mc.interval)
    dims := len(mc.metrics)
    ys := make([]float64, mc.batchSize * dims)
    defer ticker.Stop()
    for {
        for i := range b {
            <-ticker.C

            row := dims * i
            //for j, collection := range mc.metrics {
            //   ys[row + j] = collection()
            //}

            cpuSome, cpuFull :=  mc.mr.collectCPUPressure()
            memorySome, memoryFull := mc.mr.collectMemoryPressure()
            ioSome, ioFull := mc.mr.collectIoPressure()

            log.Printf("(metrics) cpu some: %f full %f memory some: %f full %f io some: %f full %f",
                cpuSome, cpuFull,
                memorySome, memoryFull,
                ioSome, ioFull)

            collected, err := mc.filter.Update([]float64{cpuSome, cpuFull, memorySome, memoryFull, ioSome, ioFull})
            if err != nil {
                log.Printf("unable to filter collected: %v", err)
            }

            log.Printf("(filter-metrics) cpu some: %f full %f memory some: %f full %f io some: %f full %f",
                collected[0], collected[1],
                collected[2], collected[3],
                collected[4], collected[5])

            mc.y.Store(&collected)
            copy(ys[row:row+dims], collected)
        }
        bT := mat.NewDense(mc.batchSize, dims, ys)


        var B mat.Dense
        B.CloneFrom(bT.T())

        mc.output<- &B
    }
}

