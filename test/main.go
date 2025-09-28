package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os/signal"
	"syscall"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/flowcontrol"

	"net"
	"time"

	pb "github.com/LucaChot/pronto/src/message"
	"github.com/LucaChot/pronto/src/remote/matrix"
	"gonum.org/v1/gonum/mat"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

type Probe struct {
    aggStub     pb.AggregateMergeClient
    ctx context.Context
    clientset   *kubernetes.Clientset
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

    probe := &Probe{
        ctx: ctx,
        clientset: clientset,
    }


    probe.AsClient()

    source := rand.NewSource(time.Now().UnixNano())
	r := rand.New(source)


    matrices := make([]*mat.Dense, 100)
    merges := make([]*mat.Dense, 100)

    for i := range 100 {
        slice := make([]float64, 4)
        for i := 0; i < 4; i++ {
            slice[i] = r.Float64() // Float64 returns a float64 in the range [0.0, 1.0)
        }
        matrices[i] = mat.NewDense(2,2, slice)
        if i == 0 {
            merges[i] = matrices[i]
        } else {
            U, Sigma := matrix.AggMerge(merges[i-1], matrices[i], 2, 1, 1)
            var merged mat.Dense
            merged.Mul(U, Sigma)
            merges[i] = &merged
        }
    }

    probeAgg := func(i int) {
        merged := probe.SendAggRequest(matrices[i])

        if i > 0 {
            var diff mat.Dense
            log.Printf("Original: %#v", merges[i-1].RawMatrix())
            log.Printf("Received: %#v", merged.RawMatrix())
            diff.Sub(merged, merges[i-1])

            norm := mat.Norm(&diff, 2)
            log.Printf("difference in aggregate: %.4f", norm)

        }
    }

    ticker := time.NewTicker(time.Second / 20)
    index := 0
    defer ticker.Stop()
    for index < 100 {
        <-ticker.C
        go probeAgg(index)
        index++
    }
}


func (fp *Probe) AsClient() {
	aggAddr := findAggAddr()
	fp.connectToAgg(aggAddr)
}

func findAggAddr() net.IP {
	for {
		ips, err := net.LookupIP("agg-svc.basic-sched.svc.cluster.local")
		if err != nil {
            log.Fatalf("error finding aggregation sever: %v", err)
		} else {
			return ips[0]
		}
	}
}

func (fp *Probe) connectToAgg(aggAddr net.IP) {

    conn, err := grpc.NewClient(aggAddr.String()+":50052",
        grpc.WithTransportCredentials(insecure.NewCredentials()))

	if err != nil {
        log.Fatalf("error connecting to controller: %v", err)
	}

	fp.aggStub = pb.NewAggregateMergeClient(conn)

}

/*
TODO: Add handler to query connection to  aggregate server in the event of
connection failing
*/
func (fp *Probe) SendAggRequest(inM *mat.Dense) (*mat.Dense) {
    rows, cols := inM.Dims()
    outM, err := fp.aggStub.RequestAggMerge(fp.ctx, &pb.DenseMatrix{ Rows: int64(rows),
        Cols: int64(cols),
        Data: inM.RawMatrix().Data,
    })

    if err != nil {
        st, ok := status.FromError(err)
        if !ok {
            log.Fatalf("error performing aggregation: %v", err)
        }

        switch st.Code() {
            case codes.NotFound:
                log.Print("did not receive aggregate")
            case codes.Unavailable:
                log.Print("aggregation server not available")
        }
        return nil
	}

    //log.Debug("FPCA: COMPLETED AGGREGATION")
    return mat.NewDense(int(outM.Rows), int(outM.Cols), outM.Data)
}

