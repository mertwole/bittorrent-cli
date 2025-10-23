package bitfield

import "sync"

type Bitfield struct {
	data       []byte
	pieceCount int
}

func NewBitfield(data []byte, pieceCount int) Bitfield {
	return Bitfield{data: data, pieceCount: pieceCount}
}

func NewEmptyBitfield(pieceCount int) Bitfield {
	return Bitfield{data: make([]byte, (pieceCount+7)/8), pieceCount: pieceCount}
}

func (bitfield *Bitfield) PieceCount() int {
	return bitfield.pieceCount
}

func (bitfield *Bitfield) SetPiecesCount() int {
	setCount := 0
	for piece := range bitfield.pieceCount {
		if bitfield.ContainsPiece(piece) {
			setCount++
		}
	}

	return setCount
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

func (bitfield *Bitfield) AddPiece(piece uint64) {
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

func NewConcurrentBitfield(data []byte, pieceCount int) *ConcurrentBitfield {
	return &ConcurrentBitfield{inner: NewBitfield(data, pieceCount)}
}

func NewEmptyConcurrentBitfield(pieceCount int) *ConcurrentBitfield {
	return &ConcurrentBitfield{inner: NewEmptyBitfield(pieceCount)}
}

func (bitfield *ConcurrentBitfield) PieceCount() int {
	return bitfield.inner.pieceCount
}

func (bitfield *ConcurrentBitfield) GetBitfield() Bitfield {
	bitfield.mutex.RLock()
	defer bitfield.mutex.RUnlock()

	return bitfield.inner
}

func (bitfield *ConcurrentBitfield) SetBitfield(newInner Bitfield) {
	bitfield.mutex.Lock()
	defer bitfield.mutex.Unlock()

	bitfield.inner = newInner
}

func (bitfield *ConcurrentBitfield) AddPiece(piece uint64) {
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
