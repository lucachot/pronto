package main

import (
	"context"
	"flag"
	"fmt"
	//"io"
	"os/signal"
	"syscall"

	"github.com/LucaChot/pronto/src/scheduler"
	"github.com/LucaChot/pronto/src/profiler"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/flowcontrol"

	log "github.com/sirupsen/logrus"
)

func init() {
	flag.Parse()


	log.SetLevel(log.DebugLevel)
	log.SetFormatter(&log.TextFormatter{
		ForceColors: true,
	})

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
    ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

    profiler.StartProfilerServer(":50053")

    clientset, err := GetInClusterClientset()
	if err != nil {
		log.Fatalf("Failed to create k8s client: %v", err)
	}

    ctl := scheduler.New("pronto", scheduler.WithClientset(clientset))

    if err := ctl.Init(); err != nil {
		log.Fatal(err)
	}

	ctl.Start(ctx)

	if err := ctl.RunScheduler(
        ctx); err != nil {
		log.Fatal(err)
	}
}
