package bitfield

type Bitfield struct {
	data   []byte
	length int
}

func New(data []byte, length int) Bitfield {
	return Bitfield{data: data, length: length}
}

func NewEmpty(pieceCount int) Bitfield {
	return Bitfield{data: make([]byte, (pieceCount+7)/8), length: pieceCount}
}

func (bitfield *Bitfield) Length() int {
	return bitfield.length
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
