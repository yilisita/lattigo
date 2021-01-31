package ckks

import (
	"fmt"
	"math"
	"math/cmplx"

	"github.com/ldsec/lattigo/v2/ckks/bettersine"
	"github.com/ldsec/lattigo/v2/utils"
)

// Bootstrapper is a struct to stores a memory pool the plaintext matrices
// the polynomial approximation and the keys for the bootstrapping.
type Bootstrapper struct {
	BootstrappingParameters
	*BootstrappingKey
	params *Parameters

	dslots    uint64 // Number of plaintext slots after the re-encoding
	logdslots uint64

	encoder   Encoder   // Encoder
	evaluator Evaluator // Evaluator

	plaintextSize uint64 // Byte size of the plaintext DFT matrices

	repack      bool                    // If true then can repack the CoeffsToSlots into on ciphertext
	prescale    float64                 // Q[0]/(Q[0]/|m|)
	postscale   float64                 // Qi sineeval/(Q[0]/|m|)
	sinescale   float64                 // Qi sineeval
	chebycoeffs *ChebyshevInterpolation // Coefficients of the Chebyshev Interpolation of sin(2*pi*x) or cos(2*pi*x/r)

	coeffsToSlotsDiffScale complex128      // Matrice rescaling
	slotsToCoeffsDiffScale complex128      // Matrice rescaling
	pDFT                   []*PtDiagMatrix // Matrice vectors
	pDFTInv                []*PtDiagMatrix // Matrice vectors
	ctsLevel               []uint64        // index of the Qi used by CoeffsToSlots
	stcLevel               []uint64        // index of the Qi used by SlotsToCoeffs

	rotKeyIndex []uint64 // a list of the required rotation keys

	ctxpool *Ciphertext // Memory pool
}

func sin2pi2pi(x complex128) complex128 {
	return cmplx.Sin(6.283185307179586*x) / 6.283185307179586
}

func cos2pi(x complex128) complex128 {
	return cmplx.Cos(6.283185307179586 * x)
}

// NewBootstrapper creates a new Bootstrapper.
func NewBootstrapper(params *Parameters, btpParams *BootstrappingParameters, btpKey *BootstrappingKey) (btp *Bootstrapper, err error) {

	if btpParams.SinType == SinType(Sin) && btpParams.SinRescal != 0 {
		return nil, fmt.Errorf("cannot use double angle formul for SinType = Sin -> must use SinType = Cos")
	}

	btp = newBootstrapper(params, btpParams)

	btp.BootstrappingKey = btpKey
	if err = btp.CheckKeys(); err != nil {
		return nil, err
	}

	return btp, nil
}

// newBootstrapper is a constructor of "dummy" bootstrapper to enable the generation of bootstrapping-related constants
// without providing a bootstrapping key. To be replaced by a propper factorization of the bootstrapping pre-computations.
func newBootstrapper(params *Parameters, btpParams *BootstrappingParameters) (btp *Bootstrapper) {
	btp = new(Bootstrapper)

	btp.params = params.Copy()
	btp.BootstrappingParameters = *btpParams.Copy()

	btp.dslots = params.Slots()
	btp.logdslots = params.LogSlots()
	if params.logSlots < params.MaxLogSlots() {
		btp.repack = true
		btp.dslots <<= 1
		btp.logdslots++
	}

	btp.prescale = math.Exp2(math.Round(math.Log2(float64(params.qi[0]) / btp.MessageRatio)))
	btp.sinescale = math.Exp2(math.Round(math.Log2(btp.SineEvalModuli.ScalingFactor)))
	btp.postscale = btp.sinescale / btp.MessageRatio

	fmt.Println(btp.CtSDepth())
	fmt.Println(btp.StCDepth())

	btp.ctsLevel = make([]uint64, btp.CtSDepth())
	for i := range btp.ctsLevel {
		btp.ctsLevel[i] = btp.params.MaxLevel() - uint64(i)
	}

	btp.stcLevel = make([]uint64, btp.StCDepth())
	for i := range btp.stcLevel {
		btp.stcLevel[i] = btp.params.MaxLevel() - btp.CtSDepth() - btp.SineEvalDepth(true) - btp.ArcSineDepth() - uint64(i)
	}

	fmt.Println(btp.ctsLevel)
	fmt.Println(btp.stcLevel)

	fmt.Println(math.Log2(btp.prescale))
	fmt.Println(math.Log2(btp.sinescale))
	fmt.Println(math.Log2(btp.postscale))

	btp.encoder = NewEncoder(params)
	btp.evaluator = NewEvaluator(params)

	btp.genSinePoly()
	btp.genDFTMatrices()

	btp.ctxpool = NewCiphertext(params, 1, params.MaxLevel(), 0)

	return btp
}

// CheckKeys checks if all the necessary keys are present
func (btp *Bootstrapper) CheckKeys() (err error) {

	if btp.relinkey == nil || btp.rotkeys == nil {
		return fmt.Errorf("empty relinkkey and/or rotkeys")
	}

	if btp.rotkeys.evakeyConjugate == nil {
		return fmt.Errorf("missing conjugate key")
	}

	rotMissing := []uint64{}
	for _, i := range btp.rotKeyIndex {
		if btp.rotkeys.evakeyRotColLeft[i] == nil || btp.rotkeys.permuteNTTLeftIndex[i] == nil {
			rotMissing = append(rotMissing, i)
		}
	}

	if len(rotMissing) != 0 {
		return fmt.Errorf("missing rotation keys : %d", rotMissing)
	}

	return nil
}

func (btp *Bootstrapper) addMatrixRotToList(pVec *PtDiagMatrix, rotations []uint64, slots uint64, repack bool) {

	var index uint64
	for j := range pVec.Vec {

		N1 := pVec.N1

		index = ((j / N1) * N1)

		if repack {
			// Sparse repacking, occurring during the first DFT matrix of the CoeffsToSlots.
			index &= (2*slots - 1)
		} else {
			// Other cases
			index &= (slots - 1)
		}

		if index != 0 && !utils.IsInSliceUint64(index, rotations) {
			rotations = append(rotations, index)
		}

		index = j & (N1 - 1)

		if index != 0 && !utils.IsInSliceUint64(index, rotations) {
			rotations = append(rotations, index)
		}
	}
}

func (btp *Bootstrapper) genDFTMatrices() {

	a := real(btp.chebycoeffs.a)
	b := real(btp.chebycoeffs.b)
	n := float64(btp.params.N())
	scFac := float64(int(1 << btp.SinRescal))
	qDiff := float64(btp.params.qi[0]) / math.Exp2(math.Round(math.Log2(float64(btp.params.qi[0]))))

	// Change of variable for the evaluation of the Chebyshev polynomial + cancelling factor for the DFT and SubSum + evantual scaling factor for the double angle formula
	btp.coeffsToSlotsDiffScale = complex(math.Pow(2.0/((b-a)*n*scFac*qDiff), 1.0/float64(btp.CtSDepth())), 0)

	// Rescaling factor to set the final ciphertext to the desired scale
	btp.slotsToCoeffsDiffScale = complex(math.Pow((qDiff*btp.params.scale)/btp.postscale, 1.0/float64(btp.StCDepth())), 0)

	// Computation and encoding of the matrices for CoeffsToSlots and SlotsToCoeffs.
	btp.computePlaintextVectors()

	// List of the rotation key values to needed for the bootstrapp
	btp.rotKeyIndex = []uint64{}

	//SubSum rotation needed X -> Y^slots rotations
	for i := btp.params.logSlots; i < btp.params.MaxLogSlots(); i++ {
		if !utils.IsInSliceUint64(1<<i, btp.rotKeyIndex) {
			btp.rotKeyIndex = append(btp.rotKeyIndex, 1<<i)
		}
	}

	// Coeffs to Slots rotations
	for _, pVec := range btp.pDFTInv {
		btp.addMatrixRotToList(pVec, btp.rotKeyIndex, btp.params.Slots(), false)
	}

	// Slots to Coeffs rotations
	for i, pVec := range btp.pDFT {
		if i == 0 {
			btp.addMatrixRotToList(pVec, btp.rotKeyIndex, btp.params.Slots(), btp.repack)
		} else {
			btp.addMatrixRotToList(pVec, btp.rotKeyIndex, btp.params.Slots(), false)
		}

	}
}

func (btp *Bootstrapper) genSinePoly() {

	if btp.SinType == Sin {

		K := complex(float64(btp.SinRange), 0)
		btp.chebycoeffs = Approximate(sin2pi2pi, -K, K, int(btp.SinDeg))

	} else if btp.SinType == Cos1 {

		K := int(btp.SinRange)
		deg := int(btp.SinDeg)
		scFac := complex(float64(int(1<<btp.SinRescal)), 0)

		cheby := new(ChebyshevInterpolation)

		cheby.coeffs = bettersine.Approximate(K, deg, btp.MessageRatio, int(btp.SinRescal))

		var sqrt2pi float64
		if btp.ArcSineDeg > 0 {
			sqrt2pi = math.Pow(1, 1.0/real(scFac))
		} else {
			sqrt2pi = math.Pow(0.15915494309189535, 1.0/real(scFac))
		}

		for i := range cheby.coeffs {
			cheby.coeffs[i] *= complex(sqrt2pi, 0)
		}

		cheby.maxDeg = cheby.Degree()
		cheby.a = complex(float64(-K), 0) / scFac
		cheby.b = complex(float64(K), 0) / scFac
		cheby.lead = true

		btp.chebycoeffs = cheby

	} else if btp.SinType == Cos2 {

		K := int(btp.SinRange)
		deg := int(btp.SinDeg)
		scFac := complex(float64(int(1<<btp.SinRescal)), 0)

		cheby := Approximate(cos2pi, -complex(float64(K), 0)/scFac, complex(float64(K), 0)/scFac, deg)

		var sqrt2pi float64
		if btp.ArcSineDeg > 0 {
			sqrt2pi = math.Pow(1, 1.0/real(scFac))
		} else {
			sqrt2pi = math.Pow(0.15915494309189535, 1.0/real(scFac))
		}

		for i := range cheby.coeffs {
			cheby.coeffs[i] *= complex(sqrt2pi, 0)
		}

		btp.chebycoeffs = cheby

	} else {
		panic("Bootstrapper -> invalid sineType")
	}
}

func computeRoots(N uint64) (roots []complex128) {

	var angle float64

	m := N << 1

	roots = make([]complex128, m)

	roots[0] = 1

	for i := uint64(1); i < m; i++ {
		angle = 6.283185307179586 * float64(i) / float64(m)
		roots[i] = complex(math.Cos(angle), math.Sin(angle))
	}

	return
}

func fftPlainVec(logN, dslots uint64, roots []complex128, pow5 []uint64) (a, b, c [][]complex128) {

	var N, m, index, tt, gap, k, mask, idx1, idx2 uint64

	N = 1 << logN

	a = make([][]complex128, logN)
	b = make([][]complex128, logN)
	c = make([][]complex128, logN)

	var size uint64
	if 2*N == dslots {
		size = 2
	} else {
		size = 1
	}

	index = 0
	for m = 2; m <= N; m <<= 1 {

		a[index] = make([]complex128, dslots)
		b[index] = make([]complex128, dslots)
		c[index] = make([]complex128, dslots)

		tt = m >> 1

		for i := uint64(0); i < N; i += m {

			gap = N / m
			mask = (m << 2) - 1

			for j := uint64(0); j < m>>1; j++ {

				k = (pow5[j] & mask) * gap

				idx1 = i + j
				idx2 = i + j + tt

				for u := uint64(0); u < size; u++ {
					a[index][idx1+u*N] = 1
					a[index][idx2+u*N] = -roots[k]
					b[index][idx1+u*N] = roots[k]
					c[index][idx2+u*N] = 1
				}
			}
		}

		index++
	}

	return
}

func fftInvPlainVec(logN, dslots uint64, roots []complex128, pow5 []uint64) (a, b, c [][]complex128) {

	var N, m, index, tt, gap, k, mask, idx1, idx2 uint64

	N = 1 << logN

	a = make([][]complex128, logN)
	b = make([][]complex128, logN)
	c = make([][]complex128, logN)

	var size uint64
	if 2*N == dslots {
		size = 2
	} else {
		size = 1
	}

	index = 0
	for m = N; m >= 2; m >>= 1 {

		a[index] = make([]complex128, dslots)
		b[index] = make([]complex128, dslots)
		c[index] = make([]complex128, dslots)

		tt = m >> 1

		for i := uint64(0); i < N; i += m {

			gap = N / m
			mask = (m << 2) - 1

			for j := uint64(0); j < m>>1; j++ {

				k = ((m << 2) - (pow5[j] & mask)) * gap

				idx1 = i + j
				idx2 = i + j + tt

				for u := uint64(0); u < size; u++ {

					a[index][idx1+u*N] = 1
					a[index][idx2+u*N] = -roots[k]
					b[index][idx1+u*N] = 1
					c[index][idx2+u*N] = roots[k]
				}
			}
		}

		index++
	}

	return
}

func (btp *Bootstrapper) computePlaintextVectors() {

	slots := btp.params.Slots()

	ctsLevel := btp.ctsLevel
	stcLevel := btp.stcLevel

	roots := computeRoots(slots << 1)
	pow5 := make([]uint64, (slots<<1)+1)
	pow5[0] = 1
	for i := uint64(1); i < (slots<<1)+1; i++ {
		pow5[i] = pow5[i-1] * 5
		pow5[i] &= (slots << 2) - 1
	}

	// CoeffsToSlots vectors
	btp.pDFTInv = make([]*PtDiagMatrix, len(ctsLevel))
	pVecDFTInv := btp.computeDFTMatrices(roots, pow5, btp.coeffsToSlotsDiffScale, true)
	for i, lvl := range ctsLevel {
		btp.pDFTInv[i] = btp.encoder.EncodeDiagMatrixAtLvl(lvl, pVecDFTInv[i], btp.CoeffsToSlotsModuli.ScalingFactor[i], btp.MaxN1N2Ratio, btp.logdslots)
	}

	// SlotsToCoeffs vectors
	btp.pDFT = make([]*PtDiagMatrix, len(stcLevel))
	pVecDFT := btp.computeDFTMatrices(roots, pow5, btp.slotsToCoeffsDiffScale, false)
	for i, lvl := range stcLevel {
		btp.pDFT[i] = btp.encoder.EncodeDiagMatrixAtLvl(lvl, pVecDFT[i], btp.SlotsToCoeffsModuli.ScalingFactor[i], btp.MaxN1N2Ratio, btp.logdslots)
	}
}

func (btp *Bootstrapper) computeDFTMatrices(roots []complex128, pow5 []uint64, diffscale complex128, forward bool) (plainVector []map[uint64][]complex128) {

	var level, depth, nextLevel, logSlots uint64

	logSlots = btp.params.logSlots
	level = logSlots

	var a, b, c [][]complex128
	var maxDepth uint64

	if forward {
		maxDepth = btp.CtSDepth()
		a, b, c = fftInvPlainVec(btp.params.logSlots, btp.dslots, roots, pow5)
	} else {
		maxDepth = btp.StCDepth()
		a, b, c = fftPlainVec(btp.params.logSlots, btp.dslots, roots, pow5)
	}

	plainVector = make([]map[uint64][]complex128, maxDepth)

	// We compute the chain of merge in order or reverse order depending if its DFT or InvDFT because
	// the way the levels are collapsed has an inpact on the total number of rotations and keys to be
	// stored. Ex. instead of using 255 + 64 plaintext vectors, we can use 127 + 128 plaintext vectors
	// by reversing the order of the merging.
	merge := make([]uint64, maxDepth)
	for i := uint64(0); i < maxDepth; i++ {

		depth = uint64(math.Ceil(float64(level) / float64(maxDepth-i)))

		if forward {
			merge[i] = depth
		} else {
			merge[uint64(len(merge))-i-1] = depth

		}

		level -= depth
	}

	level = logSlots
	for i := uint64(0); i < maxDepth; i++ {

		if btp.repack && !forward && i == 0 {

			// Special initial matrix for the repacking before SlotsToCoeffs
			plainVector[i] = genWfftRepack(logSlots, level)

			// Merges this special initial matrix with the first layer of SlotsToCoeffs DFT
			plainVector[i] = nextLevelfft(plainVector[i], logSlots, 2<<logSlots, level, a[logSlots-level], b[logSlots-level], c[logSlots-level], forward)

			// Continues the merging with the next layers if the total depth requires it.
			nextLevel = level - 1
			for j := uint64(0); j < merge[i]-1; j++ {
				plainVector[i] = nextLevelfft(plainVector[i], logSlots, 2<<logSlots, nextLevel, a[logSlots-nextLevel], b[logSlots-nextLevel], c[logSlots-nextLevel], forward)
				nextLevel--
			}

		} else {
			// First layer of the i-th level of the DFT
			plainVector[i] = genWfft(logSlots, level, a[logSlots-level], b[logSlots-level], c[logSlots-level], forward)

			// Merges the layer with the next levels of the DFT if the total depth requires it.
			nextLevel = level - 1
			for j := uint64(0); j < merge[i]-1; j++ {
				plainVector[i] = nextLevelfft(plainVector[i], logSlots, 1<<logSlots, nextLevel, a[logSlots-nextLevel], b[logSlots-nextLevel], c[logSlots-nextLevel], forward)
				nextLevel--
			}
		}

		level -= merge[i]
	}

	// Repacking after the CoeffsToSlots (we multiply the last DFT matrix with the vector [1, 1, ..., 1, 1, 0, 0, ..., 0, 0]).
	if btp.repack && forward {
		for j := range plainVector[maxDepth-1] {
			for x := uint64(0); x < btp.params.Slots(); x++ {
				plainVector[maxDepth-1][j][x+btp.params.Slots()] = complex(0, 0)
			}
		}
	}

	// Rescaling of the DFT matrix of the SlotsToCoeffs/CoeffsToSlots
	for j := range plainVector {
		for x := range plainVector[j] {
			for i := range plainVector[j][x] {
				plainVector[j][x][i] *= diffscale
			}
		}
	}

	return
}

func genWfft(logL, level uint64, a, b, c []complex128, forward bool) (vectors map[uint64][]complex128) {

	var rot uint64

	if forward {
		rot = 1 << (level - 1)
	} else {
		rot = 1 << (logL - level)
	}

	vectors = make(map[uint64][]complex128)

	addToDicVector(vectors, 0, a)
	addToDicVector(vectors, rot, b)
	addToDicVector(vectors, (1<<logL)-rot, c)

	return
}

func genWfftRepack(logL, level uint64) (vectors map[uint64][]complex128) {

	vectors = make(map[uint64][]complex128)

	a := make([]complex128, 2<<logL)
	b := make([]complex128, 2<<logL)

	for i := uint64(0); i < 1<<logL; i++ {
		a[i] = complex(1, 0)
		a[i+(1<<logL)] = complex(0, 1)

		b[i] = complex(0, 1)
		b[i+(1<<logL)] = complex(1, 0)
	}

	addToDicVector(vectors, 0, a)
	addToDicVector(vectors, (1 << logL), b)

	return
}

func nextLevelfft(vec map[uint64][]complex128, logL, N, nextLevel uint64, a, b, c []complex128, forward bool) (newVec map[uint64][]complex128) {

	var rot uint64

	newVec = make(map[uint64][]complex128)

	if forward {
		rot = (1 << (nextLevel - 1)) & (N - 1)
	} else {
		rot = (1 << (logL - nextLevel)) & (N - 1)
	}

	for i := range vec {
		addToDicVector(newVec, i, mul(vec[i], a))
		addToDicVector(newVec, (i+rot)&(N-1), mul(rotate(vec[i], rot), b))
		addToDicVector(newVec, (i+N-rot)&(N-1), mul(rotate(vec[i], N-rot), c))
	}

	return
}

func addToDicVector(dic map[uint64][]complex128, index uint64, vec []complex128) {
	if dic[index] == nil {
		dic[index] = vec
	} else {
		dic[index] = add(dic[index], vec)
	}
}

func rotate(x []complex128, n uint64) (y []complex128) {

	y = make([]complex128, len(x))

	mask := uint64(len(x) - 1)

	// Rotates to the left
	for i := uint64(0); i < uint64(len(x)); i++ {
		y[i] = x[(i+n)&mask]
	}

	return
}

func mul(a, b []complex128) (res []complex128) {

	res = make([]complex128, len(a))

	for i := 0; i < len(a); i++ {
		res[i] = a[i] * b[i]
	}

	return
}

func add(a, b []complex128) (res []complex128) {

	res = make([]complex128, len(a))

	for i := 0; i < len(a); i++ {
		res[i] = a[i] + b[i]
	}

	return
}
