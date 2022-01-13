package bootstrapping

import (
	"github.com/ldsec/lattigo/v2/ckks/advanced"
	"github.com/ldsec/lattigo/v2/utils"
)

// Parameters is a struct for the default bootstrapping parameters
type Parameters struct {
	SlotsToCoeffsParameters advanced.EncodingMatrixLiteral
	EvalModParameters       advanced.EvalModLiteral
	CoeffsToSlotsParameters advanced.EncodingMatrixLiteral
	MainSecretDensity       int // Hamming weight of the main secret
	EphemeralSecretDensity  int // Hamming weight of the ephemeral secret
}

// MarshalBinary encode the target Parameters on a slice of bytes.
func (p *Parameters) MarshalBinary() (data []byte, err error) {
	data = []byte{}
	tmp := []byte{}

	if tmp, err = p.SlotsToCoeffsParameters.MarshalBinary(); err != nil {
		return nil, err
	}

	data = append(data, uint8(len(tmp)))
	data = append(data, tmp...)

	if tmp, err = p.EvalModParameters.MarshalBinary(); err != nil {
		return nil, err
	}

	data = append(data, uint8(len(tmp)))
	data = append(data, tmp...)

	if tmp, err = p.CoeffsToSlotsParameters.MarshalBinary(); err != nil {
		return nil, err
	}

	data = append(data, uint8(len(tmp)))
	data = append(data, tmp...)

	tmp = make([]byte, 4)
	tmp[0] = uint8(p.MainSecretDensity >> 24)
	tmp[1] = uint8(p.MainSecretDensity >> 16)
	tmp[2] = uint8(p.MainSecretDensity >> 8)
	tmp[3] = uint8(p.MainSecretDensity >> 0)
	data = append(data, tmp...)

	tmp[0] = uint8(p.EphemeralSecretDensity >> 24)
	tmp[1] = uint8(p.EphemeralSecretDensity >> 16)
	tmp[2] = uint8(p.EphemeralSecretDensity >> 8)
	tmp[3] = uint8(p.EphemeralSecretDensity >> 0)
	data = append(data, tmp...)
	return
}

// UnmarshalBinary decodes a slice of bytes on the target Parameters.
func (p *Parameters) UnmarshalBinary(data []byte) (err error) {

	pt := 0
	dLen := int(data[pt])

	if err := p.SlotsToCoeffsParameters.UnmarshalBinary(data[pt+1 : pt+dLen+1]); err != nil {
		return err
	}

	pt += dLen
	pt++
	dLen = int(data[pt])

	if err := p.EvalModParameters.UnmarshalBinary(data[pt+1 : pt+dLen+1]); err != nil {
		return err
	}

	pt += dLen
	pt++
	dLen = int(data[pt])

	if err := p.CoeffsToSlotsParameters.UnmarshalBinary(data[pt+1 : pt+dLen+1]); err != nil {
		return err
	}

	pt += dLen
	pt++
	dLen = int(data[pt])

	p.MainSecretDensity = int(data[pt])<<24 | int(data[pt+1])<<16 | int(data[pt+2])<<8 | int(data[pt+3])

	pt += 4

	p.EphemeralSecretDensity = int(data[pt])<<24 | int(data[pt+1])<<16 | int(data[pt+2])<<8 | int(data[pt+3])

	return
}

// RotationsForBootstrapping returns the list of rotations performed during the Bootstrapping operation.
func (p *Parameters) RotationsForBootstrapping(LogN, LogSlots int) (rotations []int) {

	// List of the rotation key values to needed for the bootstrapp
	rotations = []int{}

	slots := 1 << LogSlots
	dslots := slots
	if LogSlots < LogN-1 {
		dslots <<= 1
	}

	//SubSum rotation needed X -> Y^slots rotations
	for i := LogSlots; i < LogN-1; i++ {
		if !utils.IsInSliceInt(1<<i, rotations) {
			rotations = append(rotations, 1<<i)
		}
	}

	rotations = append(rotations, p.CoeffsToSlotsParameters.Rotations(LogN, LogSlots)...)
	rotations = append(rotations, p.SlotsToCoeffsParameters.Rotations(LogN, LogSlots)...)

	return
}
