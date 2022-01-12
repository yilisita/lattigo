package dbfv

import (
	"github.com/ldsec/lattigo/v2/bfv"
	"github.com/ldsec/lattigo/v2/drlwe"
	"github.com/ldsec/lattigo/v2/rlwe"
)

// RefreshProtocol is a struct storing the relevant parameters for the Refresh protocol.
type RefreshProtocol struct {
	MaskedTransformProtocol
}

// ShallowCopy creates a shallow copy of RefreshProtocol in which all the read-only data-structures are
// shared with the receiver and the temporary buffers are reallocated. The receiver and the returned
// RefreshProtocol can be used concurrently.
func (rfp *RefreshProtocol) ShallowCopy() *RefreshProtocol {
	return &RefreshProtocol{*rfp.MaskedTransformProtocol.ShallowCopy()}
}

// RefreshShare is a struct storing a party's share in the Refresh protocol.
type RefreshShare struct {
	MaskedTransformShare
}

// NewRefreshProtocol creates a new Refresh protocol instance.
func NewRefreshProtocol(params bfv.Parameters, sigmaSmudging float64) (rfp *RefreshProtocol) {
	rfp = new(RefreshProtocol)
	rfp.MaskedTransformProtocol = *NewMaskedTransformProtocol(params, sigmaSmudging)
	return
}

// AllocateShare allocates the shares of the PermuteProtocol
func (rfp *RefreshProtocol) AllocateShare() *RefreshShare {
	share := rfp.MaskedTransformProtocol.AllocateShare()
	return &RefreshShare{*share}
}

// GenShares generates a share for the Refresh protocol.
func (rfp *RefreshProtocol) GenShares(sk *rlwe.SecretKey, ciphertext *bfv.Ciphertext, crp drlwe.CKSCRP, shareOut *RefreshShare) {
	rfp.MaskedTransformProtocol.GenShares(sk, ciphertext, crp, nil, &shareOut.MaskedTransformShare)
}

// Aggregate aggregates two parties' shares in the Refresh protocol.
func (rfp *RefreshProtocol) Aggregate(share1, share2, shareOut *RefreshShare) {
	rfp.MaskedTransformProtocol.Aggregate(&share1.MaskedTransformShare, &share2.MaskedTransformShare, &shareOut.MaskedTransformShare)
}

// Finalize applies Decrypt, Recode and Recrypt on the input ciphertext.
func (rfp *RefreshProtocol) Finalize(ciphertext *bfv.Ciphertext, crp drlwe.CKSCRP, share *RefreshShare, ciphertextOut *bfv.Ciphertext) {
	rfp.MaskedTransformProtocol.Transform(ciphertext, nil, crp, &share.MaskedTransformShare, ciphertextOut)
}
