package fpca

import (
	"log"
	"math"
	"sync/atomic"

	pb "github.com/LucaChot/pronto/src/message"
	mt "github.com/LucaChot/pronto/src/remote/matrix"
	"gonum.org/v1/gonum/mat"
)

const (
    d = 2
    r = 2
    b = 10
)

var identity *mat.DiagDense

func init(){
    ones := make([]float64, b)
    for i := range ones {
        ones[i] = 1
    }
    identity = mat.NewDiagDense(b, ones)
}

type FPCAAgent struct {
    inB         <-chan *mat.Dense
    SumProbU      atomic.Pointer[[]float64]

    adaptive    bool

    b           *mat.Dense
    u           *mat.Dense
    lastU       *mat.Dense
    sigma       *mat.DiagDense
    localU      *mat.Dense
    localSigma  *mat.DiagDense

    r           int
    forget      float64
    enhance     float64
    alpha       float64
    beta        float64
    epsilon     float64

    aggStub     pb.AggregateMergeClient
}

func New(ch <-chan *mat.Dense) *FPCAAgent {
	fp := FPCAAgent{
        inB: ch,
        adaptive: false,
        r: r,
        enhance: 0.25,
        forget: 0.75,
        epsilon: 0,
    }

    fp.AsClient()

	go fp.RunLocalUpdates()

	return &fp
}

/*
TODO: See if I need to change the standard subspace merge operation to be like
the aggregate merge operation
TODO: Update the AggMerge threshold to take into account the distribution of U,
e.g. USigma vs U
*/
func (fp *FPCAAgent) RunLocalUpdates() {
    // Initial B matrix
    fp.b = <-fp.inB
    fp.InitFPCAEdge()

    for {
        fp.b = <-fp.inB
		fp.FPCAEdge()

        if !mat.EqualApprox(fp.u, fp.lastU, fp.epsilon) {
            fp.AggMerge()
        }

        fp.UpdateProbU()

        fp.lastU = fp.u
    }
}

func (fp *FPCAAgent) UpdateProbU() {
    var uSigma, probU mat.Dense
    uSigma.Mul(fp.u, fp.sigma)
    probU.Scale(1 / fp.sigma.Trace(), &uSigma)

    rows, _ := fp.u.Dims()
    sumProbU := make([]float64, rows)
    for i := range rows {
        row := probU.RawRowView(i)
        sum := 0.0
        for _, v := range row {
            sum += math.Abs(v)
        }
        sumProbU[i] = sum
    }

    log.Printf("(fpca) U: %#v", fp.u.RawMatrix())
    log.Printf("(fpca) S: %#v", fp.sigma.RawBand())
    log.Printf("(fpca) sumProbU: %#v", sumProbU)

    fp.SumProbU.Store(&sumProbU)
}

// TODO: Look into whether I can move the key AGG retrieval out of this
// function so that I can retrieve from agg before B is calculated
func (fp *FPCAAgent) AggMerge() {
    //var uSigma, scaledUSigma mat.Dense
    var uSigma mat.Dense
    uSigma.Mul(fp.u, fp.sigma)
    //scaledUSigma.Scale(1 / fp.sigma.Trace(), &uSigma)

    //aggU := fp.SendAggRequest(&scaledUSigma)
    aggU := fp.SendAggRequest(&uSigma)
    if aggU == nil {
        return
    }

    /*
    var uTB mat.Dense
    uTB.Mul(aggU.T(), fp.b)
    rUTB, cUTB := uTB.Dims()

    aggSigmaData := make([]float64, rUTB)
    for i := range rUTB {
        row := mat.Row(nil, i, &uTB)

        v := mat.NewVecDense(cUTB, row)

        dist := mat.Norm(v, 2)
        aggSigmaData[i] = dist
    }

    aggSigma := mat.NewDiagDense(rUTB, aggSigmaData)
    var aggUSigma mat.Dense
    aggUSigma.Mul(aggU, aggSigma)
    */

    //fp.u, fp.sigma = mt.AggMerge(&aggUSigma, &uSigma, fp.r, 0.8, 0.2)
    fp.u, fp.sigma = mt.AggMerge(aggU, &uSigma, fp.r, 0.8, 0.2)

}

/*
TODO: Current system must wait for b samples before it can set it u and sigma.
Look into fetching the u and sigma from the aggregator on setup. May need a new
grpc or a modify existing one with a flag
*/
func (fp *FPCAAgent) InitFPCAEdge() {
    /*
    * Update embedding estimates *
    if mc.localU, mc.localSigma = 0,0:
        mc.LocalU, mc.LocalSigma := SVD(B,r)
    */
    fp.localU, fp.localSigma = mt.SVDR(fp.b, fp.r)
    fp.u = fp.localU
    fp.sigma = fp.localSigma

    fp.AggMerge()

    fp.UpdateProbU()
    fp.lastU = fp.u
    log.Print("(fpca) finished initiating u and sigma")
}

/*
TODO: Ask andreas about the pseudocode of the paper, in the rank function, it
assumes that the sigma is of size r x r, so what does Sigma_[r+1] do?
TODO: Ask which version of Merge should I implement
*/
func (fp *FPCAAgent) FPCAEdge() {
    /*
    * Update embedding estimates *
    mc.LocalU, mc.LocalSigma := Merge(mc.LocalU, mc.localSigma, B, I, r)

    * Merge with previous estimate *
    mc.GlobalU, mc.GlobalSigma := Merge(mc.LocalU, mc.localSigma, mc.GlobalU, mc.GlobalSigma, r)

    * Adjust the rank based on information it has seen so far *
    mc.GlobalU, mc.GlobalSigma := Rank(mc.GlobalU, mc.GlobalSigma, alpha, beta, r)
    */

    fp.localU, fp.localSigma = mt.Merge(fp.localU, fp.localSigma, fp.b, identity, fp.r, fp.forget, fp.enhance)

    var uSigma, localUSigma mat.Dense
    uSigma.Mul(fp.u, fp.sigma)
    localUSigma.Mul(fp.localU, fp.localSigma)


    if fp.adaptive {
        /* Pass in rank r+1 so that we can increase the rank in the next step */
        tempU, tempSigma := mt.AggMerge(&uSigma, &localUSigma, fp.r + 1, 0.8, 0.2)
        fp.u, fp.sigma = mt.Rank(tempU, tempSigma, fp.r, fp.alpha, fp.beta)
    } else {
        fp.u, fp.sigma = mt.AggMerge(&uSigma, &localUSigma, fp.r, 0.8, 0.2)
    }
}

