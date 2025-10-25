package serialize

import (
	"bytes"
	"strings"
	"testing"
)

func TestIntSerialize(t *testing.T) {
	testSerialize(int8(10), "i10e", t)
	testSerialize(int16(-1000), "i-1000e", t)
	testSerialize(int32(123456789), "i123456789e", t)
	testSerialize(int64(0), "i0e", t)
	testSerialize(int(-123456789), "i-123456789e", t)
}

func TestUintSerialize(t *testing.T) {
	testSerialize(uint8(10), "i10e", t)
	testSerialize(uint16(1000), "i1000e", t)
	testSerialize(uint32(123456789), "i123456789e", t)
	testSerialize(uint64(0), "i0e", t)
	testSerialize(uint(123456789), "i123456789e", t)
}

func TestStringSerialize(t *testing.T) {
	testSerialize("test", "4:test", t)
	testSerialize("with the spaces", "15:with the spaces", t)
}

func TestListSerialize(t *testing.T) {
	testSerialize([]string{"test"}, "l4:teste", t)
	testSerialize([]string{"test 1", "test 2"}, "l6:test 16:test 2e", t)
}

func TestMapSerialize(t *testing.T) {
	value := make(map[string]int)
	value["test_1"] = 1
	value["test_2"] = 2

	expected := removeWhitespaces(`
		d
			6:test_1
				i1e
			6:test_2
				i2e
		e
	`)

	testSerialize(value, expected, t)
}

func TestDictionarySerialize(t *testing.T) {
	value := dictionaryStruct{
		IntField:    10,
		StringField: "test",
		DictField:   dictionaryStructInner{IntField: 20},
	}

	expectedString := removeWhitespaces(`
		d
			9:DictField
				d
					8:IntField
						i20e
				e
			9:int_field
				i10e
			12:string@field
				4:test
		e
	`)
	expectedString = strings.ReplaceAll(expectedString, "@", " ")

	testSerialize(value, expectedString, t)
}

func TestDictionarySerializeWithOptionalField(t *testing.T) {
	value := dictionaryStructWithOptionalField{
		OptionalField: &dictionaryStructInner{IntField: 10},
	}
	expected := removeWhitespaces(`
		d
			13:OptionalField
				d
					8:IntField
						i10e
				e
		e
	`)

	testSerialize(value, expected, t)

	value = dictionaryStructWithOptionalField{
		OptionalField: nil,
	}
	expected = "de"

	testSerialize(value, expected, t)
}

func removeWhitespaces(input string) string {
	input = strings.ReplaceAll(input, " ", "")
	input = strings.ReplaceAll(input, "\n", "")
	input = strings.ReplaceAll(input, "\r", "")
	input = strings.ReplaceAll(input, "\t", "")

	return input
}

type dictionaryStruct struct {
	IntField    int    `bencode:"int_field"`
	StringField string `bencode:"string field"`
	DictField   dictionaryStructInner
}

type dictionaryStructWithOptionalField struct {
	OptionalField *dictionaryStructInner
}

type dictionaryStructInner struct {
	IntField int
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
