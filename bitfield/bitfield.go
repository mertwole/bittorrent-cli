package bitfield

import "sync"

type Bitfield struct {
	data []byte
	// TODO: Rename to pieceCount
	length int
}

func NewBitfield(data []byte, length int) Bitfield {
	return Bitfield{data: data, length: length}
}

func NewEmptyBitfield(pieceCount int) Bitfield {
	return Bitfield{data: make([]byte, (pieceCount+7)/8), length: pieceCount}
}

// TODO: Rename to PieceCount
func (bitfield *Bitfield) Length() int {
	return bitfield.length
}

func (bitfield *Bitfield) IsEmpty() bool {
	for _, byte_ := range bitfield.data {
		if byte_ != 0 {
			return false
		}
	}

	return true
}

func (bitfield *Bitfield) ToBytes() []byte {
	return bitfield.data
}

func (bitfield *Bitfield) AddPiece(piece int) {
	byteIdx := piece / 8
	bitIdx := piece % 8

	bitfield.data[byteIdx] |= 1 << (7 - bitIdx)
}

func (bitfield *Bitfield) RemovePiece(piece int) {
	byteIdx := piece / 8
	bitIdx := piece % 8

	bitfield.data[byteIdx] &= ^(1 << (7 - bitIdx))
}

func (bitfield *Bitfield) ContainsPiece(piece int) bool {
	byteIdx := piece / 8
	bitIdx := piece % 8

	return bitfield.data[byteIdx]&(1<<(7-bitIdx)) != 0
}

func (bitfield *Bitfield) Subtract(other *Bitfield) Bitfield {
	result := *bitfield
	for i := range len(bitfield.data) {
		result.data[i] = bitfield.data[i] & (^other.data[i])
	}
	return result
}

type ConcurrentBitfield struct {
	inner Bitfield
	mutex sync.RWMutex
}

func NewConcurrentBitfield(data []byte, length int) *ConcurrentBitfield {
	return &ConcurrentBitfield{inner: NewBitfield(data, length)}
}

func NewEmptyConcurrentBitfield(pieceCount int) *ConcurrentBitfield {
	return &ConcurrentBitfield{inner: NewEmptyBitfield(pieceCount)}
}

func (bitfield *ConcurrentBitfield) GetBitfield() Bitfield {
	bitfield.mutex.RLock()
	defer bitfield.mutex.RUnlock()

	return bitfield.inner
}

func (bitfield *ConcurrentBitfield) AddPiece(piece int) {
	bitfield.mutex.Lock()
	defer bitfield.mutex.Unlock()

	bitfield.inner.AddPiece(piece)
}

func (bitfield *ConcurrentBitfield) ContainsPiece(piece int) bool {
	bitfield.mutex.RLock()
	defer bitfield.mutex.RUnlock()

	return bitfield.inner.ContainsPiece(piece)
}

func (bitfield *ConcurrentBitfield) IsEmpty() bool {
	bitfield.mutex.RLock()
	defer bitfield.mutex.RUnlock()

	return bitfield.inner.IsEmpty()
}
