package bfv

import (
	"github.com/ldsec/lattigo/ring"
	"github.com/ldsec/lattigo/utils"
	"math/big"
)

// Evaluator is a struct holding the necessary elements to operates the homomorphic operations between ciphertext and/or plaintexts.
// It also holds a small memory pool used to store intermediate computations.
type Evaluator struct {
	params *Parameters

	bfvContext *bfvContext

	baseconverterQ1Q2 *ring.FastBasisExtender

	baseconverterQ1P *ring.FastBasisExtender
	decomposer       *ring.Decomposer

	pHalf *big.Int

	poolQ [][]*ring.Poly
	poolP [][]*ring.Poly

	polypool      [2]*ring.Poly
	keyswitchpool [5]*ring.Poly
}

// NewEvaluator creates a new Evaluator, that can be used to do homomorphic
// operations on the ciphertexts and/or plaintexts. It stores a small pool of polynomials
// and ciphertexts that will be used for intermediate values.
func NewEvaluator(params *Parameters) (evaluator *Evaluator) {

	evaluator = new(Evaluator)
	evaluator.params = params.Copy()
	evaluator.bfvContext = newBFVContext(params)

	evaluator.baseconverterQ1Q2 = ring.NewFastBasisExtender(evaluator.bfvContext.contextQ, evaluator.bfvContext.contextQMul)
	evaluator.baseconverterQ1P = ring.NewFastBasisExtender(evaluator.bfvContext.contextQ, evaluator.bfvContext.contextP)
	evaluator.decomposer = ring.NewDecomposer(evaluator.bfvContext.contextQ.Modulus, evaluator.bfvContext.contextP.Modulus)

	evaluator.pHalf = new(big.Int).Rsh(evaluator.bfvContext.contextQMul.ModulusBigint, 1)

	for i := 0; i < 2; i++ {
		evaluator.polypool[i] = evaluator.bfvContext.contextQ.NewPoly()
	}

	for i := 0; i < 5; i++ {
		evaluator.keyswitchpool[i] = evaluator.bfvContext.contextQP.NewPoly()
	}

	evaluator.poolQ = make([][]*ring.Poly, 4)
	evaluator.poolP = make([][]*ring.Poly, 4)
	for i := 0; i < 4; i++ {
		evaluator.poolQ[i] = make([]*ring.Poly, 6)
		evaluator.poolP[i] = make([]*ring.Poly, 6)
		for j := 0; j < 6; j++ {
			evaluator.poolQ[i][j] = evaluator.bfvContext.contextQ.NewPoly()
			evaluator.poolP[i][j] = evaluator.bfvContext.contextQMul.NewPoly()
		}
	}

	return evaluator
}

func (evaluator *Evaluator) getElemAndCheckBinary(op0, op1, opOut Operand, opOutMinDegree uint64) (el0, el1, elOut *bfvElement) {
	if op0 == nil || op1 == nil || opOut == nil {
		panic("operands cannot be nil")
	}

	if op0.Degree()+op1.Degree() == 0 {
		panic("operands cannot be both plaintext")
	}

	if opOut.Degree() < opOutMinDegree {
		panic("receiver operand degree is too small")
	}

	el0, el1, elOut = op0.Element(), op1.Element(), opOut.Element()
	return // TODO: more checks on elements
}

func (evaluator *Evaluator) getElemAndCheckUnary(op0, opOut Operand, opOutMinDegree uint64) (el0, elOut *bfvElement) {
	if op0 == nil || opOut == nil {
		panic("operand cannot be nil")
	}

	if op0.Degree() == 0 {
		panic("operand cannot be plaintext")
	}

	if opOut.Degree() < opOutMinDegree {
		panic("receiver operand degree is too small")
	}
	el0, elOut = op0.Element(), opOut.Element()
	return // TODO: more checks on elements
}

// evaluateInPlaceBinary applies the provided function in place on el0 and el1 and returns the result in elOut.
func evaluateInPlaceBinary(el0, el1, elOut *bfvElement, evaluate func(*ring.Poly, *ring.Poly, *ring.Poly)) {

	maxDegree := utils.MaxUint64(el0.Degree(), el1.Degree())
	minDegree := utils.MinUint64(el0.Degree(), el1.Degree())

	for i := uint64(0); i < minDegree+1; i++ {
		evaluate(el0.value[i], el1.value[i], elOut.value[i])
	}

	// If the inputs degree differ, copies the remaining degree on the receiver
	var largest *bfvElement
	if el0.Degree() > el1.Degree() {
		largest = el0
	} else if el1.Degree() > el0.Degree() {
		largest = el1
	}
	if largest != nil && largest != elOut { // checks to avoid unnecessary work.
		for i := minDegree + 1; i < maxDegree+1; i++ {
			elOut.value[i].Copy(largest.value[i])
		}
	}
}

// evaluateInPlaceUnary applies the provided function in place on el0 and returns the result in elOut.
func evaluateInPlaceUnary(el0, elOut *bfvElement, evaluate func(*ring.Poly, *ring.Poly)) {
	for i := range el0.value {
		evaluate(el0.value[i], elOut.value[i])
	}
}

// Add adds op0 to op1 and returns the result on ctOut.
func (evaluator *Evaluator) Add(op0, op1 Operand, ctOut *Ciphertext) {
	el0, el1, elOut := evaluator.getElemAndCheckBinary(op0, op1, ctOut, utils.MaxUint64(op0.Degree(), op1.Degree()))
	evaluateInPlaceBinary(el0, el1, elOut, evaluator.bfvContext.contextQ.Add)
}

// AddNew adds op0 to op1 and creates a new element ctOut to store the result.
func (evaluator *Evaluator) AddNew(op0, op1 Operand) (ctOut *Ciphertext) {
	ctOut = NewCiphertext(evaluator.params, utils.MaxUint64(op0.Degree(), op1.Degree()))
	evaluator.Add(op0, op1, ctOut)
	return
}

// AddNoMod adds op0 to op1 without modular reduction, and returns the result on cOut.
func (evaluator *Evaluator) AddNoMod(op0, op1 Operand, ctOut *Ciphertext) {
	el0, el1, elOut := evaluator.getElemAndCheckBinary(op0, op1, ctOut, utils.MaxUint64(op0.Degree(), op1.Degree()))
	evaluateInPlaceBinary(el0, el1, elOut, evaluator.bfvContext.contextQ.AddNoMod)
}

// AddNoModNew adds op0 to op1 without modular reduction and creates a new element ctOut to store the result.
func (evaluator *Evaluator) AddNoModNew(op0, op1 Operand) (ctOut *Ciphertext) {
	ctOut = NewCiphertext(evaluator.params, utils.MaxUint64(op0.Degree(), op1.Degree()))
	evaluator.AddNoMod(op0, op1, ctOut)
	return
}

// Sub subtracts op1 to op0 and returns the result on cOut.
func (evaluator *Evaluator) Sub(op0, op1 Operand, ctOut *Ciphertext) {
	el0, el1, elOut := evaluator.getElemAndCheckBinary(op0, op1, ctOut, utils.MaxUint64(op0.Degree(), op1.Degree()))
	evaluateInPlaceBinary(el0, el1, elOut, evaluator.bfvContext.contextQ.Sub)

	if el0.Degree() < el1.Degree() {
		for i := el0.Degree() + 1; i < el1.Degree()+1; i++ {
			evaluator.bfvContext.contextQ.Neg(ctOut.Value()[i], ctOut.Value()[i])
		}
	}
}

// SubNew subtracts op0 to op1 and creates a new element ctOut to store the result.
func (evaluator *Evaluator) SubNew(op0, op1 Operand) (ctOut *Ciphertext) {
	ctOut = NewCiphertext(evaluator.params, utils.MaxUint64(op0.Degree(), op1.Degree()))
	evaluator.Sub(op0, op1, ctOut)
	return
}

// SubNoMod subtracts op0 to op1 without modular reduction and returns the result on ctOut.
func (evaluator *Evaluator) SubNoMod(op0, op1 Operand, ctOut *Ciphertext) {
	el0, el1, elOut := evaluator.getElemAndCheckBinary(op0, op1, ctOut, utils.MaxUint64(op0.Degree(), op1.Degree()))

	evaluateInPlaceBinary(el0, el1, elOut, evaluator.bfvContext.contextQ.SubNoMod)

	if el0.Degree() < el1.Degree() {
		for i := el0.Degree() + 1; i < el1.Degree()+1; i++ {
			evaluator.bfvContext.contextQ.Neg(ctOut.Value()[i], ctOut.Value()[i])
		}
	}
}

// SubNoModNew subtracts op0 to op1 without modular reduction and creates a new element ctOut to store the result.
func (evaluator *Evaluator) SubNoModNew(op0, op1 Operand) (ctOut *Ciphertext) {
	ctOut = NewCiphertext(evaluator.params, utils.MaxUint64(op0.Degree(), op1.Degree()))
	evaluator.SubNoMod(op0, op1, ctOut)
	return
}

// Neg negates op and returns the result on ctOut.
func (evaluator *Evaluator) Neg(op Operand, ctOut *Ciphertext) {
	el0, elOut := evaluator.getElemAndCheckUnary(op, ctOut, op.Degree())
	evaluateInPlaceUnary(el0, elOut, evaluator.bfvContext.contextQ.Neg)
}

// NegNew negates op and creates a new element to store the result.
func (evaluator *Evaluator) NegNew(op Operand) (ctOut *Ciphertext) {
	ctOut = NewCiphertext(evaluator.params, op.Degree())
	evaluator.Neg(op, ctOut)
	return ctOut
}

// Reduce applies a modular reduction on op and returns the result on ctOut.
func (evaluator *Evaluator) Reduce(op Operand, ctOut *Ciphertext) {
	el0, elOut := evaluator.getElemAndCheckUnary(op, ctOut, op.Degree())
	evaluateInPlaceUnary(el0, elOut, evaluator.bfvContext.contextQ.Reduce)
}

// ReduceNew applies a modular reduction on op and creates a new element ctOut to store the result.
func (evaluator *Evaluator) ReduceNew(op Operand) (ctOut *Ciphertext) {
	ctOut = NewCiphertext(evaluator.params, op.Degree())
	evaluator.Reduce(op, ctOut)
	return ctOut
}

// MulScalar multiplies op by an uint64 scalar and returns the result on ctOut.
func (evaluator *Evaluator) MulScalar(op Operand, scalar uint64, ctOut *Ciphertext) {
	el0, elOut := evaluator.getElemAndCheckUnary(op, ctOut, op.Degree())
	fun := func(el, elOut *ring.Poly) { evaluator.bfvContext.contextQ.MulScalar(el, scalar, elOut) }
	evaluateInPlaceUnary(el0, elOut, fun)
}

// MulScalarNew multiplies op by an uint64 scalar and creates a new element ctOut to store the result.
func (evaluator *Evaluator) MulScalarNew(op Operand, scalar uint64) (ctOut *Ciphertext) {
	ctOut = NewCiphertext(evaluator.params, op.Degree())
	evaluator.MulScalar(op, scalar, ctOut)
	return
}

// tensorAndRescales computes (ct0 x ct1) * (t/Q) and stores the result on ctOut.
func (evaluator *Evaluator) tensorAndRescale(ct0, ct1, ctOut *bfvElement) {

	contextQ := evaluator.bfvContext.contextQ
	contextQMul := evaluator.bfvContext.contextQMul

	level := uint64(len(contextQ.Modulus) - 1)

	// Prepares the ciphertexts for the Tensoring by extending their
	// basis from Q to QP and transforming them in NTT form

	c0Q1 := evaluator.poolQ[0]
	c0Q2 := evaluator.poolP[0]

	c1Q1 := evaluator.poolQ[1]
	c1Q2 := evaluator.poolP[1]

	c2Q1 := evaluator.poolQ[2]
	c2Q2 := evaluator.poolP[2]

	for i := range ct0.value {
		evaluator.baseconverterQ1Q2.ModUpSplitQP(level, ct0.value[i], c0Q2[i])

		contextQ.NTT(ct0.value[i], c0Q1[i])
		contextQMul.NTT(c0Q2[i], c0Q2[i])
	}

	if ct0 != ct1 {

		for i := range ct1.value {
			evaluator.baseconverterQ1Q2.ModUpSplitQP(level, ct1.value[i], c1Q2[i])

			contextQ.NTT(ct1.value[i], c1Q1[i])
			contextQMul.NTT(c1Q2[i], c1Q2[i])
		}
	}

	// Tensoring : multiplies each elements of the ciphertexts together
	// and adds them to their correspongint position in the new ciphertext
	// based on their respective degree

	// Case where both BfvElements are of degree 1
	if ct0.Degree() == 1 && ct1.Degree() == 1 {

		c00Q := evaluator.poolQ[3][0]
		c00Q2 := evaluator.poolP[3][0]
		c01Q := evaluator.poolQ[3][1]
		c01P := evaluator.poolP[3][1]

		contextQ.MForm(c0Q1[0], c00Q)
		contextQMul.MForm(c0Q2[0], c00Q2)

		contextQ.MForm(c0Q1[1], c01Q)
		contextQMul.MForm(c0Q2[1], c01P)

		// Squaring case
		if ct0 == ct1 {

			// c0 = c0[0]*c0[0]
			contextQ.MulCoeffsMontgomery(c00Q, c0Q1[0], c2Q1[0])
			contextQMul.MulCoeffsMontgomery(c00Q2, c0Q2[0], c2Q2[0])

			// c1 = 2*c0[0]*c0[1]
			contextQ.MulCoeffsMontgomery(c00Q, c0Q1[1], c2Q1[1])
			contextQMul.MulCoeffsMontgomery(c00Q2, c0Q2[1], c2Q2[1])

			contextQ.AddNoMod(c2Q1[1], c2Q1[1], c2Q1[1])
			contextQMul.AddNoMod(c2Q2[1], c2Q2[1], c2Q2[1])

			// c2 = c0[1]*c0[1]
			contextQ.MulCoeffsMontgomery(c01Q, c0Q1[1], c2Q1[2])
			contextQMul.MulCoeffsMontgomery(c01P, c0Q2[1], c2Q2[2])

			// Normal case
		} else {

			// c0 = c0[0]*c1[0]
			contextQ.MulCoeffsMontgomery(c00Q, c1Q1[0], c2Q1[0])
			contextQMul.MulCoeffsMontgomery(c00Q2, c1Q2[0], c2Q2[0])

			// c1 = c0[0]*c1[1] + c0[1]*c1[0]
			contextQ.MulCoeffsMontgomery(c00Q, c1Q1[1], c2Q1[1])
			contextQMul.MulCoeffsMontgomery(c00Q2, c1Q2[1], c2Q2[1])

			contextQ.MulCoeffsMontgomeryAndAddNoMod(c01Q, c1Q1[0], c2Q1[1])
			contextQMul.MulCoeffsMontgomeryAndAddNoMod(c01P, c1Q2[0], c2Q2[1])

			// c2 = c0[1]*c1[1]
			contextQ.MulCoeffsMontgomery(c01Q, c1Q1[1], c2Q1[2])
			contextQMul.MulCoeffsMontgomery(c01P, c1Q2[1], c2Q2[2])
		}

		// Case where both BfvElements are not of degree 1
	} else {

		for i := uint64(0); i < ctOut.Degree()+1; i++ {
			c2Q1[i].Zero()
			c2Q2[i].Zero()
		}

		// Squaring case
		if ct0 == ct1 {

			c00Q1 := evaluator.poolQ[3]
			c00Q2 := evaluator.poolP[3]

			for i := range ct0.value {
				contextQ.MForm(c0Q1[i], c00Q1[i])
				contextQMul.MForm(c0Q2[i], c00Q2[i])
			}

			for i := uint64(0); i < ct0.Degree()+1; i++ {
				for j := i + 1; j < ct0.Degree()+1; j++ {
					contextQ.MulCoeffsMontgomery(c00Q1[i], c0Q1[j], c2Q1[i+j])
					contextQMul.MulCoeffsMontgomery(c00Q2[i], c0Q2[j], c2Q2[i+j])

					contextQ.Add(c2Q1[i+j], c2Q1[i+j], c2Q1[i+j])
					contextQMul.Add(c2Q2[i+j], c2Q2[i+j], c2Q2[i+j])
				}
			}

			for i := uint64(0); i < ct0.Degree()+1; i++ {
				contextQ.MulCoeffsMontgomeryAndAdd(c00Q1[i], c0Q1[i], c2Q1[i<<1])
				contextQMul.MulCoeffsMontgomeryAndAdd(c00Q2[i], c0Q2[i], c2Q2[i<<1])
			}

			// Normal case
		} else {
			for i := range ct0.value {
				contextQ.MForm(c0Q1[i], c0Q1[i])
				contextQMul.MForm(c0Q2[i], c0Q2[i])
				for j := range ct1.value {
					contextQ.MulCoeffsMontgomeryAndAdd(c0Q1[i], c1Q1[j], c2Q1[i+j])
					contextQMul.MulCoeffsMontgomeryAndAdd(c0Q2[i], c1Q2[j], c2Q2[i+j])
				}
			}
		}
	}

	// Applies the inverse NTT to the ciphertext, scales the down ciphertext
	// by t/q and reduces its basis from QP to Q
	for i := range ctOut.value {
		contextQ.InvNTT(c2Q1[i], c2Q1[i])
		contextQMul.InvNTT(c2Q2[i], c2Q2[i])

		// Option 1) (ct(x) * T)/Q,  but doing so requires that Q*P > Q*Q*T, slower but smaller error.
		//contextQ.MulScalar(c2Q1[i], evaluator.bfvContext.contextT.Modulus[0], c2Q1[i])
		//contextQMul.MulScalar(c2Q2[i], evaluator.bfvContext.contextT.Modulus[0], c2Q2[i])

		// Extends the basis Q of ct(x) to the basis P and Divides (ct(x)Q -> P) by Q
		evaluator.baseconverterQ1Q2.ModDownSplitedQP(level, c2Q1[i], c2Q2[i], c2Q2[i])

		// Centers (ct(x)Q -> P)/Q by (P-1)/2 and extends ((ct(x)Q -> P)/Q) to the basis Q
		contextQMul.AddScalarBigint(c2Q2[i], evaluator.pHalf, c2Q2[i])
		evaluator.baseconverterQ1Q2.ModUpSplitPQ(level, c2Q2[i], ctOut.value[i])
		contextQ.SubScalarBigint(ctOut.value[i], evaluator.pHalf, ctOut.value[i])

		// Option 2) (ct(x)/Q)*T, doing so only requires that Q*P > Q*Q, faster but adds error ~|T|
		contextQ.MulScalar(ctOut.value[i], evaluator.bfvContext.contextT.Modulus[0], ctOut.value[i])
	}
}

// Mul multiplies op0 by op1 and returns the result on ctOut.
func (evaluator *Evaluator) Mul(op0 *Ciphertext, op1 Operand, ctOut *Ciphertext) {
	el0, el1, elOut := evaluator.getElemAndCheckBinary(op0, op1, ctOut, op0.Degree()+op1.Degree())
	evaluator.tensorAndRescale(el0, el1, elOut)
}

// MulNew multiplies op0 by op1 and creates a new element ctOut to store the result.
func (evaluator *Evaluator) MulNew(op0 *Ciphertext, op1 Operand) (ctOut *Ciphertext) {
	ctOut = NewCiphertext(evaluator.params, op0.Degree()+op1.Degree())
	evaluator.Mul(op0, op1, ctOut)
	return
}

// relinearize is a method common to Relinearize and RelinearizeNew. It switches ct0 out in the NTT domain, applies the keyswitch, and returns the result out of the NTT domain.
func (evaluator *Evaluator) relinearize(ct0 *Ciphertext, evakey *EvaluationKey, ctOut *Ciphertext) {

	if ctOut != ct0 {
		evaluator.bfvContext.contextQ.Copy(ct0.value[0], ctOut.value[0])
		evaluator.bfvContext.contextQ.Copy(ct0.value[1], ctOut.value[1])
	}

	for deg := uint64(ct0.Degree()); deg > 1; deg-- {
		evaluator.switchKeys(ct0.value[deg], evakey.evakey[deg-2], ctOut)
	}

	ctOut.SetValue(ctOut.value[:2])
}

// Relinearize relinearizes the ciphertext ct0 of degree > 1 until it is of degree 1 and returns the result on cOut.
//
// Requires a correct evaluation key as additional input :
//
// - it must match the secret-key that was used to create the public key under which the current ct0 is encrypted.
//
// - it must be of degree high enough to relinearize the input ciphertext to degree 1 (ex. a ciphertext
// of degree 3 will require that the evaluation key stores the keys for both degree 3 and 2 ciphertexts).
func (evaluator *Evaluator) Relinearize(ct0 *Ciphertext, evakey *EvaluationKey, ctOut *Ciphertext) {

	if int(ct0.Degree()-1) > len(evakey.evakey) {
		panic("cannot relinearize -> input ciphertext degree too large to allow relinearization")
	}

	if ct0.Degree() < 2 {
		if ct0 != ctOut {
			ctOut.Copy(ct0.Element())
		}
	} else {
		evaluator.relinearize(ct0, evakey, ctOut)
	}
}

// RelinearizeNew relinearizes the ciphertext ct0 of degree > 1 until it is of degree 1 and creates a new ciphertext to store the result.
//
// Requires a correct evaluation key as additional input :
//
// - it must match the secret-key that was used to create the public key under which the current ct0 is encrypted
//
// - it must be of degree high enough to relinearize the input ciphertext to degree 1 (ex. a ciphertext
// of degree 3 will require that the evaluation key stores the keys for both degree 3 and 2 ciphertexts).
func (evaluator *Evaluator) RelinearizeNew(ct0 *Ciphertext, evakey *EvaluationKey) (ctOut *Ciphertext) {
	ctOut = NewCiphertext(evaluator.params, 1)
	evaluator.Relinearize(ct0, evakey, ctOut)
	return
}

// SwitchKeys applies the key-switching procedure to the ciphertext ct0 and returns the result on ctOut. It requires as an additional input a valide switching-key :
// it must encrypt the target key under the public key under which ct0 is currently encrypted.
func (evaluator *Evaluator) SwitchKeys(ct0 *Ciphertext, switchKey *SwitchingKey, ctOut *Ciphertext) {

	if ct0.Degree() != 1 || ctOut.Degree() != 1 {
		panic("cannot switchkeys -> input and output must be of degree 1 to allow key switching")
	}

	if ct0 != ctOut {
		evaluator.bfvContext.contextQ.Copy(ct0.value[0], ctOut.value[0])
		evaluator.bfvContext.contextQ.Copy(ct0.value[1], ctOut.value[1])
	}

	evaluator.switchKeys(ct0.value[1], switchKey, ctOut)
}

// SwitchKeysNew applies the key-switching procedure to the ciphertext ct0 and creates a new ciphertext to store the result. It requires as an additional input a valide switching-key :
// it must encrypt the target key under the public key under which ct0 is currently encrypted.
func (evaluator *Evaluator) SwitchKeysNew(ct0 *Ciphertext, switchkey *SwitchingKey) (ctOut *Ciphertext) {
	ctOut = NewCiphertext(evaluator.params, 1)
	evaluator.SwitchKeys(ct0, switchkey, ctOut)
	return
}

// RotateColumnsNew applies RotateColumns and returns the result on a new Ciphertext.
func (evaluator *Evaluator) RotateColumnsNew(ct0 *Ciphertext, k uint64, evakey *RotationKeys) (ctOut *Ciphertext) {
	ctOut = NewCiphertext(evaluator.params, 1)
	evaluator.RotateColumns(ct0, k, evakey, ctOut)
	return
}

// RotateColumns rotates the columns of ct0 by k position to the left and returns the result on ctOut. As an additional input it requires a rotationkeys :
//
// - it must either store all the left and right power of 2 rotations or the specific rotation that is asked.
//
// If only the power of two rotations are stored, the numbers k and n/2-k will be decomposed in base 2 and the rotation with the least
// hamming weight will be chosen, then the specific rotation will be computed as a sum of powers of two rotations.
func (evaluator *Evaluator) RotateColumns(ct0 *Ciphertext, k uint64, evakey *RotationKeys, ctOut *Ciphertext) {

	if ct0.Degree() != 1 || ctOut.Degree() != 1 {
		panic("cannot rotate -> input and or output must be of degree 1")
	}

	k &= ((evaluator.bfvContext.n >> 1) - 1)

	if k == 0 {

		ctOut.Copy(ct0.Element())

	} else {

		// Looks in the rotationkey if the corresponding rotation has been generated or if the input is a plaintext
		if evakey.evakeyRotColLeft[k] != nil {

			evaluator.permute(ct0, evaluator.bfvContext.galElRotColLeft[k], evakey.evakeyRotColLeft[k], ctOut)

		} else {

			// If not looks if the left and right pow2 rotations have been generated
			hasPow2Rotations := true
			for i := uint64(1); i < evaluator.bfvContext.n>>1; i <<= 1 {
				if evakey.evakeyRotColLeft[i] == nil || evakey.evakeyRotColRight[i] == nil {
					hasPow2Rotations = false
					break
				}
			}

			// If yes, computes the least amount of rotation between k to the left and n/2 -k to the right required to apply the demanded rotation
			if hasPow2Rotations {

				if utils.HammingWeight64(k) <= utils.HammingWeight64((evaluator.bfvContext.n>>1)-k) {
					evaluator.rotateColumnsLPow2(ct0, k, evakey, ctOut)
				} else {
					evaluator.rotateColumnsRPow2(ct0, (evaluator.bfvContext.n>>1)-k, evakey, ctOut)
				}

				// Else returns an error indicating that the keys have not been generated
			} else {
				panic("cannot rotate -> specific rotation and pow2 rotations have not been generated")
			}
		}
	}
}

// rotateColumnsLPow2 applies the Galois Automorphism on the element, rotating the element by k positions to the left, returns the result on ctOut.
func (evaluator *Evaluator) rotateColumnsLPow2(ct0 *Ciphertext, k uint64, evakey *RotationKeys, ctOut *Ciphertext) {
	evaluator.rotateColumnsPow2(ct0, GaloisGen, k, evakey.evakeyRotColLeft, ctOut)
}

// rotateColumnsRPow2 applies the Galois Endomorphism on the element, rotating the element by k positions to the right, returns the result on ctOut.
func (evaluator *Evaluator) rotateColumnsRPow2(ct0 *Ciphertext, k uint64, evakey *RotationKeys, ctOut *Ciphertext) {
	genInv := ring.ModExp(GaloisGen, 2*evaluator.bfvContext.n-1, 2*evaluator.bfvContext.n)
	evaluator.rotateColumnsPow2(ct0, genInv, k, evakey.evakeyRotColRight, ctOut)
}

// rotateColumnsPow2 rotates ct0 by k position (left or right depending on the input), decomposing k as a sum of power of 2 rotations, and returns the result on ctOut.
func (evaluator *Evaluator) rotateColumnsPow2(ct0 *Ciphertext, generator, k uint64, evakeyRotCol map[uint64]*SwitchingKey, ctOut *Ciphertext) {

	var mask, evakeyIndex uint64

	context := evaluator.bfvContext.contextQ

	mask = (evaluator.bfvContext.n << 1) - 1

	evakeyIndex = 1

	if ct0 != ctOut {
		context.Copy(ct0.value[0], ctOut.value[0])
		context.Copy(ct0.value[1], ctOut.value[1])
	}

	// Applies the galois automorphism and the switching-key process
	for k > 0 {

		if k&1 == 1 {

			evaluator.permute(ctOut, generator, evakeyRotCol[evakeyIndex], ctOut)
		}

		generator *= generator
		generator &= mask

		evakeyIndex <<= 1
		k >>= 1
	}
}

// RotateRows swaps the rows of ct0 and returns the result on ctOut.
func (evaluator *Evaluator) RotateRows(ct0 *Ciphertext, evakey *RotationKeys, ctOut *Ciphertext) {

	if ct0.Degree() != 1 || ctOut.Degree() != 1 {
		panic("cannot rotate -> input and or output degree must be of degree 1")
	}

	if evakey.evakeyRotRow == nil {
		panic("cannot rotate -> rotation key not generated")
	}

	evaluator.permute(ct0, evaluator.bfvContext.galElRotRow, evakey.evakeyRotRow, ctOut)
}

// RotateRowsNew swaps the rows of ct0 and returns the result a new Ciphertext.
func (evaluator *Evaluator) RotateRowsNew(ct0 *Ciphertext, evakey *RotationKeys) (ctOut *Ciphertext) {
	ctOut = NewCiphertext(evaluator.params, 1)
	evaluator.RotateRows(ct0, evakey, ctOut)
	return
}

// InnerSum computs the inner sum of ct0 and returns the result on ctOut. It requires a rotation key storing all the left power of two rotations.
// The resulting vector will be of the form [sum, sum, .., sum, sum ].
func (evaluator *Evaluator) InnerSum(ct0 *Ciphertext, evakey *RotationKeys, ctOut *Ciphertext) {

	if ct0.Degree() != 1 || ctOut.Degree() != 1 {
		panic("cannot inner sum -> input and output must be of degree 1")
	}

	cTmp := NewCiphertext(evaluator.params, 1)

	ctOut.Copy(ct0.Element())

	for i := uint64(1); i < evaluator.bfvContext.n>>1; i <<= 1 {
		evaluator.RotateColumns(ctOut, i, evakey, cTmp)
		evaluator.Add(cTmp.bfvElement, ctOut, ctOut.Ciphertext())
	}

	evaluator.RotateRows(ctOut, evakey, cTmp)
	evaluator.Add(ctOut, cTmp.bfvElement, ctOut)
}

// permute operates a column rotation on ct0 and returns the result on ctOut
func (evaluator *Evaluator) permute(ct0 *Ciphertext, generator uint64, switchKey *SwitchingKey, ctOut *Ciphertext) {

	context := evaluator.bfvContext.contextQ

	var el0, el1 *ring.Poly

	if ct0 != ctOut {
		el0, el1 = ctOut.value[0], ctOut.value[1]
	} else {
		el0, el1 = evaluator.polypool[0], evaluator.polypool[1]
	}

	context.Permute(ct0.value[0], generator, el0)
	context.Permute(ct0.value[1], generator, el1)

	if el0 != ctOut.value[0] || el1 != ctOut.value[1] {
		context.Copy(el0, ctOut.value[0])
		context.Copy(el1, ctOut.value[1])
	}

	evaluator.switchKeys(el1, switchKey, ctOut)
}

// Applies the general keyswitching procedure of the form [c0 + cx*evakey[0], c1 + cx*evakey[1]]
func (evaluator *Evaluator) switchKeys(cx *ring.Poly, evakey *SwitchingKey, ctOut *Ciphertext) {

	var level, reduce uint64

	level = uint64(len(ctOut.value[0].Coeffs)) - 1
	context := evaluator.bfvContext.contextQ
	contextKeys := evaluator.bfvContext.contextQP

	for i := range evaluator.keyswitchpool {
		evaluator.keyswitchpool[i].Zero()
	}

	c2Qi := evaluator.keyswitchpool[0]
	c2 := evaluator.keyswitchpool[1]

	// We switch the element on which the switching key operation will be conducted out of the NTT domain
	context.NTT(cx, c2)

	reduce = 0

	N := contextKeys.N
	c2QiNtt := make([]uint64, N)

	// Key switching with crt decomposition for the Qi
	for i := uint64(0); i < evaluator.params.Beta; i++ {

		p0idxst := i * evaluator.params.Alpha
		p0idxed := p0idxst + evaluator.decomposer.Xalpha()[i]

		// c2Qi = cx mod qi
		evaluator.decomposer.Decompose(level, i, cx, c2Qi)

		for x, qi := range contextKeys.Modulus {

			nttPsi := contextKeys.GetNttPsi()[x]
			bredParams := contextKeys.GetBredParams()[x]
			mredParams := contextKeys.GetMredParams()[x]

			if p0idxst <= uint64(x) && uint64(x) < p0idxed {
				p2tmp := c2.Coeffs[x]
				for j := uint64(0); j < N; j++ {
					c2QiNtt[j] = p2tmp[j]
				}
			} else {
				ring.NTT(c2Qi.Coeffs[x], c2QiNtt, N, nttPsi, qi, mredParams, bredParams)
			}

			key0 := evakey.evakey[i][0].Coeffs[x]
			key1 := evakey.evakey[i][1].Coeffs[x]
			p2tmp := evaluator.keyswitchpool[2].Coeffs[x]
			p3tmp := evaluator.keyswitchpool[3].Coeffs[x]

			for y := uint64(0); y < context.N; y++ {
				p2tmp[y] += ring.MRed(key0[y], c2QiNtt[y], qi, mredParams)
				p3tmp[y] += ring.MRed(key1[y], c2QiNtt[y], qi, mredParams)
			}
		}

		if reduce&7 == 7 {
			contextKeys.Reduce(evaluator.keyswitchpool[2], evaluator.keyswitchpool[2])
			contextKeys.Reduce(evaluator.keyswitchpool[3], evaluator.keyswitchpool[3])
		}

		reduce++
	}

	if (reduce-1)&7 != 7 {
		contextKeys.Reduce(evaluator.keyswitchpool[2], evaluator.keyswitchpool[2])
		contextKeys.Reduce(evaluator.keyswitchpool[3], evaluator.keyswitchpool[3])
	}

	contextKeys.InvNTT(evaluator.keyswitchpool[2], evaluator.keyswitchpool[2])
	contextKeys.InvNTT(evaluator.keyswitchpool[3], evaluator.keyswitchpool[3])

	evaluator.baseconverterQ1P.ModDownPQ(level, evaluator.keyswitchpool[2], evaluator.keyswitchpool[2])
	evaluator.baseconverterQ1P.ModDownPQ(level, evaluator.keyswitchpool[3], evaluator.keyswitchpool[3])

	context.Add(ctOut.value[0], evaluator.keyswitchpool[2], ctOut.value[0])
	context.Add(ctOut.value[1], evaluator.keyswitchpool[3], ctOut.value[1])
}
