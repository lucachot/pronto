package aggregate

/*
Core Components:
- Aggregator thread
- gRPC threads

Idea:
Aggregator thread takes a queue of (U,Sigma)s and performs the merge operation.

The gRPC threads from the remote schedulers read the address of the pointer to
the current aggregated matrix. It then performs its on merge operation.


This method reduces overall latency. We can avoid gRPC requests from queuing.
In addition, if we did not make gRPC requests queue, merge operations could be lost.

So we use a combination of central thread and gRPC calls to reduce latency while
ensuring consistency


FAULT TOLERANCE:
We could add fault tolerance by having the remote schedulers use the pod service
and so when a pod restarts, they are still able to communicate with this pod.
*/

import (
	"log"
	"sync/atomic"

	"gonum.org/v1/gonum/mat"

	pb "github.com/LucaChot/pronto/src/message"
	mt "github.com/LucaChot/pronto/src/remote/matrix"
)

const (
    MAXWAITING = 20
    R = 2
)

type Aggregator struct {
    matrices chan *mat.Dense
    aggU atomic.Pointer[mat.Dense]
    aggUSigma *mat.Dense
    pb.UnimplementedAggregateMergeServer
}

func New() (*Aggregator) {
    agg := Aggregator {
        matrices: make(chan *mat.Dense, MAXWAITING),
    }

    agg.startAggregateServer()

    return &agg
}

/*
TODO: Check whether the remote schedulers expose the matrix with ranks that are
less than or greater than r?
I.e. Check if there may be a mismatch in the rank
*/
func (agg *Aggregator) Aggregate()  {
    agg.aggUSigma = <- agg.matrices
    agg.aggU.Store(agg.aggUSigma)
    log.Print("(agg) received first matrix")
    for {
        inUSigma := <- agg.matrices

        U, Sigma := mt.AggMerge(agg.aggUSigma, inUSigma, R, 1, 1)


        agg.aggUSigma.Mul(U, Sigma)
        agg.aggU.Store(agg.aggUSigma)

        log.Printf("(agg) U: %#v", U)
        log.Printf("(agg) S: %#v", Sigma)
    }
}

/*
Removed the need to create a new Sigma matrix by passing in the product
_, inc := inUSigma.Dims()
inIData := make([]float64, inc)
for i := range inc {
    inIData[i] = 1.0
}
inI := mat.NewDiagonal(inc, inIData)
*/


