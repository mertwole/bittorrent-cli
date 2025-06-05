package serialize

import (
	"bytes"
	"testing"
)

func TestIntSerialize(t *testing.T) {
	testSerialize(int8(10), "i10e", t)
	testSerialize(int16(-1000), "i-1000e", t)
	testSerialize(int32(123456789), "i123456789e", t)
	testSerialize(int64(0), "i0e", t)
	testSerialize(int(-123456789), "i-123456789e", t)
}

func TestStringSerialize(t *testing.T) {
	testSerialize("test", "4:test", t)
	testSerialize("with the spaces", "15:with the spaces", t)
}

func TestListSerialize(t *testing.T) {
	testSerialize([]string{"test"}, "l4:teste", t)
	testSerialize([]string{"test 1", "test 2"}, "l6:test 16:test 2e", t)
}

func testSerialize(value any, expected string, t *testing.T) {
	result := bytes.NewBufferString("")

	err := Serialize(result, value)
	if err != nil {
		t.Error(err)
	}

	if expected != result.String() {
		t.Errorf("got unexpected value. expected %s, got %s", expected, result.String())
	}
}
