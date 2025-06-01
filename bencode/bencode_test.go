package bencode

import (
	"strings"
	"testing"
)

func TestIntDeserialize(t *testing.T) {
	testIntDeserialize("i10e", int8(10), t)
	testIntDeserialize("i-10e", int16(-10), t)
	testIntDeserialize("i10000000e", int32(10000000), t)
	testIntDeserialize("i0e", int64(0), t)
	testIntDeserialize("i-10000000e", int(-10000000), t)
}

func testIntDeserialize[I comparable](bencoded string, expectedValue I, t *testing.T) {
	var deserialized I
	err := Deserialize(strings.NewReader(bencoded), &deserialized)
	if err != nil {
		t.Errorf("failed to deserialize: %v", err)
	}

	if deserialized != expectedValue {
		t.Errorf("values don't match: expected %v, got %v", expectedValue, deserialized)
	}
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
