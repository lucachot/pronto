package aggregate

import (
	"context"
	"log"
	"net"

	pb "github.com/LucaChot/pronto/src/message"
	"gonum.org/v1/gonum/mat"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)



func (agg *Aggregator) startAggregateServer() {
    lis, err := net.Listen("tcp", ":50052")
	if err != nil {
        log.Printf("failed to serve start serve: %v", err)
	}

	s := grpc.NewServer()
    pb.RegisterAggregateMergeServer(s, agg)

    log.Printf("started aggregate server: %s", lis.Addr().String())

	go func() {
		if err := s.Serve(lis); err != nil {
            log.Fatalf("failed to start server: %v", err)
		}
	}()
}

/*
Read the pointer to the struct containing both the U and Sigma
Uses atomic.Pointer[T] to ensure atomicity
Research into Golang's memory model
*/
func (agg *Aggregator) RequestAggMerge(ctx context.Context, in *pb.DenseMatrix) (*pb.DenseMatrix, error) {
    inUSigma := mat.NewDense(int(in.Rows), int(in.Cols), in.Data)
    agg.matrices<- inUSigma

    aggU := agg.aggU.Load()

    if aggU == nil {
        log.Print("(server) no aggregate so returned empty")
        return nil, status.Errorf(codes.NotFound, "NO AGGREGATE TO RETURN")
    }

    log.Print("(server) returned aggregate")

    rows, cols := aggU.Dims()
    return &pb.DenseMatrix{
        Rows: int64(rows),
        Cols: int64(cols),
        Data: aggU.RawMatrix().Data,
    }, nil
}

/*
Move Merge into Aggregate Server
U, Sigma = pointer.read()

agg.channel <- (in.U, in.Sigma)
U', Sigma' := Merge(U, Sigma, in.U, in.Sigma, r)

return &Response {
    U: U'
    Sigma: Sigma'
}, nil
*/
