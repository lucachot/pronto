package main

import (
	"context"
	"flag"
	"fmt"
	"os/signal"
	"syscall"
    "log"

	"os"

	"github.com/LucaChot/pronto/src/remote"
	"github.com/LucaChot/pronto/src/remote/cache"
	"github.com/LucaChot/pronto/src/remote/kalman"
	"gopkg.in/yaml.v2"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/flowcontrol"
)

var (
    // Informer flags
    informerType       string

    // PodCost flags
    podCostFunc        string
    kalmanConfig        string
    podCostLowerBound  float64
    podCostUpperBound  float64
)

func init() {
    // Informer flags
    flag.StringVar(&informerType, "informer", "static", "Informer type: one of {static, api, containerd}")

    // PodCost flags
    flag.StringVar(&podCostFunc, "podcost-func", "const", "Cost function for PodCost (e.g. const, kalman1d, kalman2d)")
    flag.StringVar(&kalmanConfig, "kalman-config", "", "Config file for kalman filter")
    flag.Float64Var(&podCostLowerBound, "podcost-lower", 0.0, "Lower bound for cost")
    flag.Float64Var(&podCostUpperBound, "podcost-upper", -1.0, "Upper bound for cost")
}

func init() {
	flag.Parse()
    log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	//log.SetLevel(log.DebugLevel)
	//log.SetFormatter(&log.TextFormatter{
		//ForceColors: true,
	//})

    //log.SetOutput(io.Discard)
}

func GetInClusterClientset() (*kubernetes.Clientset, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
        return nil, fmt.Errorf("failed to get in-cluster config: %w", err)
	}
	config.RateLimiter = flowcontrol.NewTokenBucketRateLimiter(80, 100)

	return kubernetes.NewForConfig(config)
}


func main() {
    flag.Parse()

    ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()


    clientset, err := GetInClusterClientset()
	if err != nil {
		log.Fatalf("Failed to create k8s client: %v", err)
	}

    /*
    var informer cache.PodCountInformer
    switch informerType {
    case "static":
        informer = cache.NewStaticInformer()
    case "api":
        informer = cache.NewApiInformer(
            ctx,
            clientset,
            remote.GetNodeName())
    case "containerd":
        informer = cache.NewContainerInformer()
    }
    */

    //cache := cache.New(informer)
    cache := cache.NewEventCache(cache.NewContainerEventInformer())


    cppOptions := make([]remote.KalmanStateOption, 0)

    switch podCostFunc {
    case "const":
        cppOptions = append(cppOptions,
            remote.WithConstructor(kalman.NewConstant),
            remote.WithUpdate(func(cpp *remote.CostPerPodState) {cpp.UpdateFunc = cpp.UpdateConst}),
            remote.WithGetPodCost(func(cpp *remote.CostPerPodState) {cpp.GetCostFunc = cpp.GetPodCostConst}))
    case "kalman1d":
        cppOptions = append(cppOptions,
            remote.WithConstructor(kalman.NewKalmanFilter1D),
            remote.WithUpdate(func(cpp *remote.CostPerPodState) {cpp.UpdateFunc = cpp.UpdatePodCost1D}),
            remote.WithGetPodCost(func(cpp *remote.CostPerPodState) {cpp.GetCostFunc = cpp.GetPodCost1D}))
    case "kalman2d":
        cppOptions = append(cppOptions,
            remote.WithConstructor(kalman.NewKalmanFilter2D),
            remote.WithUpdate(func(cpp *remote.CostPerPodState) {cpp.UpdateFunc = cpp.UpdatePodCost2D}),
            remote.WithGetPodCost(func(cpp *remote.CostPerPodState) {cpp.GetCostFunc = cpp.GetPodCost2D}))
    }

    if kalmanConfig != "" {
        data, err := os.ReadFile(kalmanConfig)
        if err != nil {
            log.Printf("reading config: %v", err)
        } else {
            var cfg kalman.KalmanConfig
            if err := yaml.Unmarshal(data, &cfg); err != nil {
                log.Fatalf("parsing config: %v", err)
            } else {
                cppOptions = append(cppOptions,
                    remote.WithInitX(cfg.InitX),
                    remote.WithInitP(cfg.InitP),
                    remote.WithQ(cfg.Q),
                    remote.WithR(cfg.R))
            }
        }
    }

    cppOptions = append(cppOptions,
        remote.WithLowerBounds(podCostLowerBound),
        remote.WithUpperBounds(podCostUpperBound))

    //cpp := remote.NewCostPerPodState(cppOptions...)

    //cs := remote.NewCapacityState()
    dfs :=  remote.NewDualFilterState()

	rmt, err := remote.New(
        ctx,
        clientset,
        cache,
        //cpp,
        //cs,
        dfs,
        remote.WithTrigger())
    if err != nil {
        log.Fatal(err)
    }

    rmt.Start()
}
