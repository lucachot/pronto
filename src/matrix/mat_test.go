package matrix_test

import (
	"math"
	"testing"

	"github.com/LucaChot/pronto/src/matrix"
	"gonum.org/v1/gonum/mat"
)

// assertPanics verifies that f panics.
func assertPanics(t *testing.T, name string, f func()) {
	t.Helper()
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("%s: expected panic, got none", name)
		}
	}()
	f()
}

// isColumnOrthonormal checks that U^T * U ≈ I_r within tolerance.
func isColumnOrthonormal(t *testing.T, U *mat.Dense, tol float64) {
	t.Helper()
	_, c := U.Dims()
	var utu mat.Dense
	utu.Mul(U.T(), U)
	eye := mat.NewDiagDense(c, nil)
	for i := range c {
		eye.SetDiag(i, 1.0)
	}
	if !mat.EqualApprox(&utu, eye, tol) {
		t.Errorf("U is not column-orthonormal: U^T*U =\n%v", mat.Formatted(&utu))
	}
}

// --- SVDR ---

func TestSVDR_Dimensions(t *testing.T) {
	A := mat.NewDense(4, 3, []float64{
		1, 2, 3,
		4, 5, 6,
		7, 8, 9,
		10, 11, 12,
	})
	r := 2
	U, sigma := matrix.SVDR(A, r)

	ur, uc := U.Dims()
	if ur != 4 || uc != r {
		t.Errorf("SVDR: U dimensions = (%d, %d), want (4, %d)", ur, uc, r)
	}
	sr, _ := sigma.Dims()
	if sr != r {
		t.Errorf("SVDR: Sigma size = %d, want %d", sr, r)
	}
}

func TestSVDR_UIsOrthonormal(t *testing.T) {
	A := mat.NewDense(5, 4, []float64{
		3, 1, 4, 1,
		5, 9, 2, 6,
		5, 3, 5, 8,
		9, 7, 9, 3,
		2, 3, 8, 4,
	})
	U, _ := matrix.SVDR(A, 3)
	isColumnOrthonormal(t, U, 1e-10)
}

func TestSVDR_SingularValuesDescending(t *testing.T) {
	A := mat.NewDense(4, 3, []float64{
		1, 0, 0,
		0, 2, 0,
		0, 0, 3,
		1, 1, 1,
	})
	_, sigma := matrix.SVDR(A, 3)
	data := sigma.RawBand().Data
	for i := 1; i < len(data); i++ {
		if data[i] > data[i-1]+1e-10 {
			t.Errorf("SVDR: singular values not descending: s[%d]=%.4f > s[%d]=%.4f", i, data[i], i-1, data[i-1])
		}
	}
}

func TestSVDR_PanicWhenRTooLarge(t *testing.T) {
	A := mat.NewDense(3, 2, nil)
	assertPanics(t, "SVDR r > min(rows,cols)", func() { matrix.SVDR(A, 3) })
}

// --- Concatenate ---

func TestConcatenate_Dimensions(t *testing.T) {
	A := mat.NewDense(3, 2, nil)
	B := mat.NewDense(3, 4, nil)
	C := matrix.Concatenate(A, B)
	cr, cc := C.Dims()
	if cr != 3 || cc != 6 {
		t.Errorf("Concatenate: dimensions = (%d, %d), want (3, 6)", cr, cc)
	}
}

func TestConcatenate_Values(t *testing.T) {
	A := mat.NewDense(2, 2, []float64{1, 2, 3, 4})
	B := mat.NewDense(2, 2, []float64{5, 6, 7, 8})
	C := matrix.Concatenate(A, B)

	want := mat.NewDense(2, 4, []float64{1, 2, 5, 6, 3, 4, 7, 8})
	if !mat.EqualApprox(C, want, 1e-12) {
		t.Errorf("Concatenate: got\n%v\nwant\n%v", mat.Formatted(C), mat.Formatted(want))
	}
}

func TestConcatenate_PanicOnRowMismatch(t *testing.T) {
	A := mat.NewDense(2, 2, nil)
	B := mat.NewDense(3, 2, nil)
	assertPanics(t, "Concatenate row mismatch", func() { matrix.Concatenate(A, B) })
}

// --- Merge ---

func TestMerge_Dimensions(t *testing.T) {
	A := mat.NewDense(4, 3, []float64{
		1, 2, 3,
		4, 5, 6,
		7, 8, 9,
		10, 11, 12,
	})
	r := 2
	U1, sigma1 := matrix.SVDR(A, r)

	B := mat.NewDense(4, 3, []float64{
		2, 0, 1,
		0, 3, 1,
		1, 1, 4,
		2, 2, 2,
	})
	U2, sigma2 := matrix.SVDR(B, r)

	U, sigma := matrix.Merge(U1, sigma1, U2, sigma2, r, 0.9, 1.1)

	ur, uc := U.Dims()
	if ur != 4 || uc != r {
		t.Errorf("Merge: U dimensions = (%d, %d), want (4, %d)", ur, uc, r)
	}
	sr, _ := sigma.Dims()
	if sr != r {
		t.Errorf("Merge: Sigma size = %d, want %d", sr, r)
	}
}

func TestMerge_UIsOrthonormal(t *testing.T) {
	A := mat.NewDense(4, 2, []float64{1, 0, 0, 1, 1, 1, 2, 3})
	U1, s1 := matrix.SVDR(A, 2)
	B := mat.NewDense(4, 2, []float64{2, 1, 1, 2, 0, 3, 3, 0})
	U2, s2 := matrix.SVDR(B, 2)

	U, _ := matrix.Merge(U1, s1, U2, s2, 2, 0.9, 1.1)
	isColumnOrthonormal(t, U, 1e-10)
}

// --- AggMerge ---

func TestAggMerge_Dimensions(t *testing.T) {
	A := mat.NewDense(4, 2, []float64{1, 2, 3, 4, 5, 6, 7, 8})
	B := mat.NewDense(4, 2, []float64{8, 7, 6, 5, 4, 3, 2, 1})
	U, sigma := matrix.AggMerge(A, B, 2)

	ur, uc := U.Dims()
	if ur != 4 || uc != 2 {
		t.Errorf("AggMerge: U dimensions = (%d, %d), want (4, 2)", ur, uc)
	}
	sr, _ := sigma.Dims()
	if sr != 2 {
		t.Errorf("AggMerge: Sigma size = %d, want 2", sr)
	}
}

func TestAggMerge_UIsOrthonormal(t *testing.T) {
	A := mat.NewDense(4, 2, []float64{1, 2, 3, 4, 5, 6, 7, 8})
	B := mat.NewDense(4, 2, []float64{8, 7, 6, 5, 4, 3, 2, 1})
	U, _ := matrix.AggMerge(A, B, 2)
	isColumnOrthonormal(t, U, 1e-10)
}

// --- ImpactOfRank ---

func TestImpactOfRank_KnownValues(t *testing.T) {
	tests := []struct {
		name   string
		data   []float64
		r      int
		want   float64
	}{
		// r=1: diagData[0] / diagData[0] = 1.0
		{"rank 1 of 2", []float64{6.0, 2.0}, 1, 1.0},
		// r=2: diagData[1] / (diagData[0]+diagData[1]) = 2.0/8.0 = 0.25
		{"rank 2 of 2", []float64{6.0, 2.0}, 2, 0.25},
		// r=2: 1.0 / (9.0 + 1.0) = 0.1
		{"rank 2 of 3", []float64{9.0, 1.0, 0.5}, 2, 0.1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sigma := mat.NewDiagDense(len(tt.data), tt.data)
			got := matrix.ImpactOfRank(sigma, tt.r)
			if math.Abs(got-tt.want) > 1e-10 {
				t.Errorf("ImpactOfRank = %.6f, want %.6f", got, tt.want)
			}
		})
	}
}

func TestImpactOfRank_PanicOnNilSigma(t *testing.T) {
	assertPanics(t, "nil sigma", func() { matrix.ImpactOfRank(nil, 1) })
}

func TestImpactOfRank_PanicOnROutOfBounds(t *testing.T) {
	sigma := mat.NewDiagDense(2, []float64{1.0, 2.0})
	assertPanics(t, "r=0", func() { matrix.ImpactOfRank(sigma, 0) })
	assertPanics(t, "r>size", func() { matrix.ImpactOfRank(sigma, 3) })
}

// --- Rank ---

func TestRank_Decrease(t *testing.T) {
	// sigma = diag(10, 0.5, 0.1), r=2
	// impact = ImpactOfRank(sigma, 2) = 0.5/10.5 ≈ 0.048
	// alpha=0.1, beta=0.5: impact < alpha → rank decreases to 1
	uData := []float64{
		0.1, 0.2, 0.3,
		0.4, 0.5, 0.6,
		0.7, 0.8, 0.9,
		1.0, 1.1, 1.2,
	}
	U := mat.NewDense(4, 3, uData)
	sigma := mat.NewDiagDense(3, []float64{10.0, 0.5, 0.1})

	outU, outSigma := matrix.Rank(U, sigma, 2, 0.1, 0.5)

	_, uc := outU.Dims()
	if uc != 1 {
		t.Errorf("Rank decrease: output U cols = %d, want 1", uc)
	}
	sr, _ := outSigma.Dims()
	if sr != 1 {
		t.Errorf("Rank decrease: output Sigma size = %d, want 1", sr)
	}
}

func TestRank_Stable(t *testing.T) {
	// sigma = diag(3, 1.5, 0.1), r=2
	// impact = ImpactOfRank(sigma, 2) = 1.5/4.5 = 0.333
	// alpha=0.1, beta=0.5: alpha <= impact <= beta → rank unchanged at 2
	uData := []float64{
		0.1, 0.2, 0.3,
		0.4, 0.5, 0.6,
		0.7, 0.8, 0.9,
		1.0, 1.1, 1.2,
	}
	U := mat.NewDense(4, 3, uData)
	sigma := mat.NewDiagDense(3, []float64{3.0, 1.5, 0.1})

	outU, outSigma := matrix.Rank(U, sigma, 2, 0.1, 0.5)

	_, uc := outU.Dims()
	if uc != 2 {
		t.Errorf("Rank stable: output U cols = %d, want 2", uc)
	}
	sr, _ := outSigma.Dims()
	if sr != 2 {
		t.Errorf("Rank stable: output Sigma size = %d, want 2", sr)
	}
}

func TestRank_PanicWhenRTooLarge(t *testing.T) {
	U := mat.NewDense(4, 2, nil)
	sigma := mat.NewDiagDense(2, []float64{1.0, 0.5})
	// r=2 with only 2 columns → uc <= r, should panic
	assertPanics(t, "Rank r >= cols", func() { matrix.Rank(U, sigma, 2, 0.1, 0.5) })
}
