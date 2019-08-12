package ring

import (
	"encoding/binary"
	"errors"
	"math/bits"
)

// Poly is the structure containing the coefficients of a ring
type Poly struct {
	Coeffs [][]uint64 //Coefficients in CRT representation
	//Context *Context might be added back at a later time
}

// GetDegree returns the number of coefficients (degree) of the ring
func (Pol *Poly) GetDegree() int {
	return len(Pol.Coeffs[0])
}

// CetLenModulies returns the number of modulies
func (Pol *Poly) GetLenModulies() int {
	return len(Pol.Coeffs)
}

func (Pol *Poly) ExtendModulies() {

}

func (Pol *Poly) Zero() {
	for i := range Pol.Coeffs {
		for j := range Pol.Coeffs[0] {
			Pol.Coeffs[i][j] = 0
		}
	}
}

// Copy creates a new ring with the same coefficients
func (Pol *Poly) CopyNew() (p1 *Poly) {
	p1 = new(Poly)
	p1.Coeffs = make([][]uint64, len(Pol.Coeffs))
	for i := range Pol.Coeffs {
		p1.Coeffs[i] = make([]uint64, len(Pol.Coeffs[i]))
		for j := range Pol.Coeffs[i] {
			p1.Coeffs[i][j] = Pol.Coeffs[i][j]
		}
	}

	return p1
}

func (context *Context) Copy(p0, p1 *Poly) error {
	if len(p1.Coeffs) < len(context.Modulus) || uint64(len(p1.Coeffs[0])) < context.N {
		return errors.New("error : copy Poly, receiver poly is invalide")
	}
	for i := range context.Modulus {
		for j := uint64(0); j < context.N; j++ {
			p1.Coeffs[i][j] = p0.Coeffs[i][j]
		}
	}
	return nil
}

// Copies the coefficient on a receiver ring
func (Pol *Poly) Copy(p1 *Poly) error {
	if len(Pol.Coeffs) > len(p1.Coeffs) || len(Pol.Coeffs[0]) > len(p1.Coeffs[0]) {
		return errors.New("error : copy Poly, receiver poly is invalide")
	}
	for i := range Pol.Coeffs {
		for j := range Pol.Coeffs[i] {
			p1.Coeffs[i][j] = Pol.Coeffs[i][j]
		}
	}
	return nil
}

// SetCoefficients sets the coefficients of ring directly from a CRT format (double slice)
func (Pol *Poly) SetCoefficients(coeffs [][]uint64) error {

	if len(coeffs) > len(Pol.Coeffs) {
		return errors.New("error : len(coeffs) > len(Pol.Coeffs")
	}

	if len(coeffs[0]) > len(Pol.Coeffs[0]) {
		return errors.New("error : len(coeffs[0]) > len(Pol.Coeffs[0]")
	}

	for i := range coeffs {
		for j := range coeffs[0] {
			Pol.Coeffs[i][j] = coeffs[i][j]
		}
	}

	return nil
}

// GetCoefficients returns a double slice containing the coefficients of the ring
func (Pol *Poly) GetCoefficients() [][]uint64 {
	coeffs := make([][]uint64, len(Pol.Coeffs))

	for i := range Pol.Coeffs {
		coeffs[i] = make([]uint64, len(Pol.Coeffs[i]))
		for j := range Pol.Coeffs[i] {
			coeffs[i][j] = Pol.Coeffs[i][j]
		}
	}

	return coeffs
}

// WriteCoeffsTo converts a matrix of coefficients to a byte array
func WriteCoeffsTo(pointer, N, numberModuli uint64, coeffs [][]uint64, data []byte) (uint64, error) {
	tmp := N << 3
	for i := uint64(0); i < numberModuli; i++ {
		for j := uint64(0); j < N; j++ {
			binary.BigEndian.PutUint64(data[pointer+(j<<3):pointer+((j+1)<<3)], coeffs[i][j])
		}
		pointer += tmp
	}

	return pointer, nil
}

// DecodeCoeffs converts a byte array to a matrix of coefficients
func DecodeCoeffs(pointer, N, numberModuli uint64, coeffs [][]uint64, data []byte) (uint64, error) {
	tmp := N << 3
	for i := uint64(0); i < numberModuli; i++ {
		coeffs[i] = make([]uint64, N)
		for j := uint64(0); j < N; j++ {
			coeffs[i][j] = binary.BigEndian.Uint64(data[pointer+(j<<3) : pointer+((j+1)<<3)])
		}
		pointer += tmp
	}

	return pointer, nil
}

func (Pol *Poly) MarshalBinary() ([]byte, error) {

	N := uint64(len(Pol.Coeffs[0]))
	numberModulies := uint64(len(Pol.Coeffs))

	data := make([]byte, 2+((N*numberModulies)<<3))

	if numberModulies > 0xFF {
		return nil, errors.New("error : poly max modulies uint16 overflow")
	}

	data[0] = uint8(bits.Len64(uint64(N)) - 1)
	data[1] = uint8(numberModulies)

	var pointer uint64

	pointer = 2

	if _, err := WriteCoeffsTo(pointer, N, numberModulies, Pol.Coeffs, data); err != nil {
		return nil, err
	}

	return data, nil
}

func (Pol *Poly) UnMarshalBinary(data []byte) (*Poly, error) {

	N := uint64(int(1 << data[0]))
	numberModulies := uint64(int(data[1]))

	Coeffs := make([][]uint64, numberModulies)

	var pointer uint64

	pointer = 2

	if ((uint64(len(data)) - pointer) >> 3) != N*numberModulies {
		return nil, errors.New("error : invalid ring encoding")
	}

	if _, err := DecodeCoeffs(pointer, N, numberModulies, Coeffs, data); err != nil {
		return nil, err
	}

	Pol = &Poly{Coeffs}

	return Pol, nil
}
