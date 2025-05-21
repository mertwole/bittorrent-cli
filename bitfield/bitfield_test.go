package bitfield

import (
	"testing"
)

func TestBitfieldSubtract(t *testing.T) {
	lhs := NewBitfield([]byte{0b1100_0000}, 4)
	rhs := NewBitfield([]byte{0b0101_0000}, 4)

	result := lhs.Subtract(&rhs)
	expectedResult := NewBitfield([]byte{0b1000_0000}, 4)

	for i := range 4 {
		resultBit := result.ContainsPiece(i)
		expectedBit := expectedResult.ContainsPiece(i)

		if resultBit != expectedBit {
			t.Errorf("unexpected value of result bit #%d: expected %t, got %t", i, expectedBit, resultBit)
		}
	}
}
