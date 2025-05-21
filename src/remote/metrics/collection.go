package metrics

import (
    "log"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
    "github.com/prometheus/procfs"
)

type MetricReader struct {
    fs procfs.FS
    lastCPUPSI procfs.PSIStats
    lastMemoryPSI procfs.PSIStats
    lastIoPSI procfs.PSIStats
}

func NewMetricReader() (*MetricReader) {
    fs, err := procfs.NewDefaultFS()
    if err != nil {
        log.Fatalf("failed to read default /proc file: %v", err)
    }
    mr := &MetricReader{
        fs : fs,
    }

    mr.lastCPUPSI, err = fs.PSIStatsForResource("cpu")
    if err != nil {
        log.Fatalf("failed to read default /proc/pressure/cpu file: %v", err)
    }
    mr.lastMemoryPSI, err = fs.PSIStatsForResource("memory")
    if err != nil {
        log.Fatalf("failed to read default /proc/pressure/memory file: %v", err)
    }
    mr.lastIoPSI, err = fs.PSIStatsForResource("io")
    if err != nil {
        log.Fatalf("failed to read default /proc/pressure/io file: %v", err)
    }

    return mr
}

func collectCPU() float64 {
    stat, err := cpu.Percent(0, false)
	if err != nil {
        log.Fatalf("failed to read /proc/stat: %v", err)
	}
    return stat[0] / 100
}

func collectRAM() (float64) {
    stat, err := mem.VirtualMemory()
	if err != nil {
        log.Fatalf("failed to read /proc/meminfo: %v", err)
	}
	return stat.UsedPercent / 100
}

func (mr *MetricReader) collectCPUPressure() (float64, float64) {
    latest, err := mr.fs.PSIStatsForResource("cpu")
    if err != nil {
        log.Fatalf("failed to read default /proc/pressure/cpu file: %v", err)
    }
    diffSome := latest.Some.Total - mr.lastCPUPSI.Some.Total
    diffFull := latest.Full.Total - mr.lastCPUPSI.Full.Total
    mr.lastCPUPSI = latest
    if diffSome > 100000 {
        diffSome = 100000
    }
    if diffFull > 100000 {
        diffFull = 100000
    }
    return float64(diffSome), float64(diffFull)
}

func (mr *MetricReader) collectMemoryPressure() (float64, float64) {
    latest, err := mr.fs.PSIStatsForResource("memory")
    if err != nil {
        log.Fatalf("failed to read default /proc/pressure/memory file: %v", err)
    }
    diffSome := latest.Some.Total - mr.lastMemoryPSI.Some.Total
    diffFull := latest.Full.Total - mr.lastMemoryPSI.Full.Total
    mr.lastMemoryPSI = latest
    return float64(diffSome), float64(diffFull)
}

func (mr *MetricReader) collectIoPressure() (float64, float64) {
    latest, err := mr.fs.PSIStatsForResource("io")
    if err != nil {
        log.Fatalf("failed to read default /proc/pressure/io file: %v", err)
    }
    diffSome := latest.Some.Total - mr.lastIoPSI.Some.Total
    diffFull := latest.Full.Total - mr.lastIoPSI.Full.Total
    mr.lastIoPSI = latest
    return float64(diffSome), float64(diffFull)
}


/*
TODO: Look into whether I should collect ReadDisk() vs ReadDiskStat()
*/
func collectMemory() {
    /* TODO: Look into how to include Disk Statistics */
    /* Potentially attach the node directories to the pods */
}

func collectNetwork() {
    /* TODO: Look into how to include Net Statistics without knowing capacity*/
    /* Look into number of packets dropped */
}
