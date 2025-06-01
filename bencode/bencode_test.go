package bencode

import (
	"strings"
	"testing"
)

func TestIntDeserialize(t *testing.T) {
	testComparableDeserialize("i10e", int8(10), t)
	testComparableDeserialize("i-10e", int16(-10), t)
	testComparableDeserialize("i10000000e", int32(10000000), t)
	testComparableDeserialize("i0e", int64(0), t)
	testComparableDeserialize("i-10000000e", int(-10000000), t)
}

func TestStringDeserialize(t *testing.T) {
	testStringDeserialize("0:", "", t)
	testStringDeserialize("4:test", "test", t)
}

func testStringDeserialize(bencoded string, expectedValue string, t *testing.T) {
	deserialized := "garbage"
	err := Deserialize(strings.NewReader(bencoded), &deserialized)
	if err != nil {
		t.Errorf("failed to deserialize: %v", err)
	}

	if deserialized != expectedValue {
		t.Errorf("values don't match: expected %v, got %v", expectedValue, deserialized)
	}
}

func TestDictionaryDeserialize(t *testing.T) {
	bencoded := `
		d
			11:StringField
				4:test

			9:DictField
				d
					8:IntField
						i10e
				e
		e
	`
	bencoded = strings.ReplaceAll(bencoded, " ", "")
	bencoded = strings.ReplaceAll(bencoded, "\n", "")
	bencoded = strings.ReplaceAll(bencoded, "\r", "")
	bencoded = strings.ReplaceAll(bencoded, "\t", "")

	expected := dictionaryStruct{
		StringField: "test",
		DictField: dictionaryStructInner{
			IntField: 10,
		},
	}

	testComparableDeserialize(bencoded, expected, t)
}

type dictionaryStruct struct {
	StringField string
	DictField   dictionaryStructInner
}

type dictionaryStructInner struct {
	IntField int
}

func testComparableDeserialize[I comparable](bencoded string, expectedValue I, t *testing.T) {
	var deserialized I
	err := Deserialize(strings.NewReader(bencoded), &deserialized)
	if err != nil {
		t.Errorf("failed to deserialize: %v", err)
	}

	if deserialized != expectedValue {
		t.Errorf("values don't match: expected %v, got %v", expectedValue, deserialized)
	}
}
