package matrix

import (
	"fmt"
    "math"

	"gonum.org/v1/gonum/mat"
)

// TODO: Add assert statements that ensure the correct values are returned
/*
Function Structure
- Assertions
- Calculation
- Handle Subsequent Errors
*/


func SVDR(b mat.Matrix, r int) (*mat.Dense, *mat.DiagDense) {
    br, bc := b.Dims()
    if min(br, bc) < r {
		panic(fmt.Errorf("rank r must be smaller than the dimensions of matrix"))
	}

    var svd mat.SVD
    svd.Factorize(b, mat.SVDThinU)

    var fullU, u mat.Dense
    svd.UTo(&fullU)
    m, _ := fullU.Dims()
    u.CloneFrom(fullU.Slice(0, m, 0, r))

    sData := svd.Values(nil)
    sigma := mat.NewDiagDense(r, sData[:r])

    return &u, sigma
}

/* Concatenates two matrices A and B */
func Concatenate(A, B *mat.Dense) (*mat.Dense) {
    ar, ac := A.Dims()
    br, bc := B.Dims()

    if ar != br {
		panic(fmt.Errorf("input matrices must have the same number of rows"))
	}

    /* Creates matrix storing the concatenation */
    C := mat.NewDense(ar, ac + bc, nil)

    /* Use of raw matrices to access slices */
    aRaw := A.RawMatrix()
    bRaw := B.RawMatrix()
    cRaw := C.RawMatrix()

    /* For each row copy the slices of A and B into that of C */
    for i := range ar {
        destCRowSlice := cRaw.Data[i*cRaw.Stride : i*cRaw.Stride + ac + bc]
        srcARowSlice := aRaw.Data[i*aRaw.Stride : i*aRaw.Stride + ac]
        srcBRowSlice := bRaw.Data[i*bRaw.Stride : i*bRaw.Stride + bc]

        copy(destCRowSlice[:ac], srcARowSlice)
        copy(destCRowSlice[ac:], srcBRowSlice)
    }

    return C
}

func Merge(U1 mat.Matrix, Sigma1 mat.Matrix, U2 mat.Matrix, Sigma2 mat.Matrix, r int, forget float64, enhance float64) (*mat.Dense, *mat.DiagDense) {
    /*
    Z = U_1.transpose() * U_2
    Q, R = QR(U_2 - (U_1 * Z))

    U', Sigma'' = SVD( [[Sigma_1, Z * Sigma_2], [0, R * Sigma_2]], r)

    U'' = [U_1, Q]U'
    */
    total := forget + enhance
    var temp1 mat.Dense
    temp1.Mul(U1, Sigma1)
    temp1.Scale(math.Sqrt(forget / total), &temp1)

    var temp2 mat.Dense
    temp2.Mul(U2, Sigma2)
    temp2.Scale(math.Sqrt(enhance / total), &temp2)

    concat := Concatenate(&temp1, &temp2)

    return SVDR(concat, r)
}

func ImpactOfRank(Sigma *mat.DiagDense, r int) (float64) {
    if Sigma == nil {
		panic(fmt.Errorf("input matrix Sigma is nil"))
	}
    sr, _ := Sigma.Dims()
    if r < 1 || sr < r {
		panic(fmt.Errorf("rank is out of bounds"))
    }

    diagData := Sigma.RawBand().Data
    var total float64
    for i := range r {
        total += diagData[i]
    }

    if total == 0 {
        if diagData[r-1] == 0 {
            return 0
        }
		panic(fmt.Errorf("division by zero: sum of first %d diagonal elements is zero", r))
    }

    return diagData[r-1] / total
}

func Rank(inU *mat.Dense, inSigma *mat.DiagDense, r int, alpha, beta float64) (*mat.Dense, *mat.DiagDense) {
    /*
    total_variance = sum(variance(Sigma,i))
    Epsilon = variance(Sigma, r) / total_variance

    if Epsilon < alpha:
        U[:r-1], Sigma[:r-1]

    if Epsilon > beta:
        e = canonical(r+1)
        [U,e], Sigma[:r+1]
    */

    ur, uc := inU.Dims()
    _, sc := inU.Dims()

    // Assume r < U.Cols(), Sigma.Cols()
    if uc <= r || sc <= r {
        panic(fmt.Errorf("r must be smaller than the number of columns in U"))
    }

    impact := ImpactOfRank(inSigma, r)
    var rank int
    if impact < alpha {
        rank = r-1
    } else if impact <= beta {
        rank = r
    } else {
        /* TODO: Complete with understanding from Andreas */
        rank = r+1
    }

    var outU mat.Dense
    outU.CloneFrom(inU.Slice(0, ur, 0, rank))

    outDiag := make([]float64, rank)
    copy(outDiag, inSigma.RawBand().Data[:rank])
    outSigma := mat.NewDiagDense(rank, outDiag)



    return &outU, outSigma
}

func AggMerge(USigma1 *mat.Dense, USigma2 *mat.Dense, r int, forget float64, enhance float64) (*mat.Dense, *mat.DiagDense) {
    /*
    Z = U_1.transpose() * U_2
    Q, R = QR(U_2 - (U_1 * Z))

    U', Sigma'' = SVD( [[Sigma_1, Z * Sigma_2], [0, R * Sigma_2]], r)

    U'' = [U_1, Q]U'

    Different Version:
    var Z *mat.Dense
    Z.Mul(U_1.T(), U_2)


    var MultU1AndZ, SubtractU2AndMult *mat.Dense
    MultU1AndZ.Mul(U_1, Z)
    SubtractU2AndMult.Sub(U_2, MultU1AndZ)

    var qr mat.QR
    var Q, R *mat.Dense
    qr.Factorize(SubtractU2AndMult)
    qr.QTo(Q)
    qr.RTo(R)
    */
    total := forget + enhance
    var temp1, temp2 mat.Dense
    temp1.Scale(math.Sqrt(forget / total), USigma1)
    temp2.Scale(math.Sqrt(enhance / total), USigma2)

    concat := Concatenate(&temp1, &temp2)

    return SVDR(concat, r)

}
