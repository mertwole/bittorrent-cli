package deserialize

import (
	"reflect"
	"strings"
	"testing"
)

func TestIntDeserialize(t *testing.T) {
	testDeepEqualDeserailize("i10e", int8(10), t)
	testDeepEqualDeserailize("i-10e", int16(-10), t)
	testDeepEqualDeserailize("i10000000e", int32(10000000), t)
	testDeepEqualDeserailize("i0e", int64(0), t)
	testDeepEqualDeserailize("i-10000000e", int(-10000000), t)
}

func TestStringDeserialize(t *testing.T) {
	testStringDeserialize("0:", "", t)
	testStringDeserialize("4:test", "test", t)
}

func TestDictionaryDeserialize(t *testing.T) {
	bencoded := removeWhitespaces(`
		d
			11:StringField
				4:test

			9:DictField
				d
					8:IntField
						i10e
				e
		e
	`)

	expected := dictionaryStruct{
		StringField: "test",
		DictField: dictionaryStructInner{
			IntField: 10,
		},
	}

	testDeepEqualDeserailize(bencoded, expected, t)
}

func TestDictionaryDeserializeToMap(t *testing.T) {
	bencoded := removeWhitespaces(`
		d
			1:a
				i10e
			1:b
				i20e
		e
	`)

	expected := make(map[string]int)
	expected["a"] = 10
	expected["b"] = 20

	testDeepEqualDeserailize(bencoded, expected, t)
}

func TestListDeserialize(t *testing.T) {
	bencoded := "l4:test4:liste"
	expected := []string{"test", "list"}

	testDeepEqualDeserailize(bencoded, expected, t)

	bencoded = removeWhitespaces(`
		l
			d
				8:IntField
					i10e
			e
			d
				8:IntField
					i20e
			e
			d
				8:IntField
					i30e
			e
		e
	`)
	expectedValue := []dictionaryStructInner{
		{IntField: 10}, {IntField: 20}, {IntField: 30},
	}

	testDeepEqualDeserailize(bencoded, expectedValue, t)
}

func TestOptionalDeserialize(t *testing.T) {
	bencoded := removeWhitespaces(`
		d
			8:IntField
				i15e
			16:OptionalIntField
				i10e
		e
	`)
	optionalValue := 10
	expected := dictionaryStructWithOptional{
		IntField:         15,
		OptionalIntField: &optionalValue,
	}

	testDeepEqualDeserailize(bencoded, expected, t)

	bencoded = removeWhitespaces(`
		d
			8:IntField
				i15e
		e
	`)
	expected = dictionaryStructWithOptional{
		IntField:         15,
		OptionalIntField: nil,
	}

	testDeepEqualDeserailize(bencoded, expected, t)
}

func TestTaggedFields(t *testing.T) {
	bencoded := removeWhitespaces(`
		d
			9:int_field
				i15e
			12:string@field
				4:test
		e
	`)
	bencoded = strings.ReplaceAll(bencoded, "@", " ")

	expected := dictionaryStructWithTags{
		IntField:    15,
		StringField: "test",
	}

	testDeepEqualDeserailize(bencoded, expected, t)
}

func TestExtraFields(t *testing.T) {
	bencoded := removeWhitespaces(`
		d
			11:StringField
				4:test

			10:ExtraField
				d
					3:key
					5:value
				e

			9:DictField
				d
					8:IntField
						i10e
					10:ExtraField
						l
							4:this
							6:should
							3:not
							2:be
							6:parsed
						e
				e
		e
	`)

	expected := dictionaryStruct{
		StringField: "test",
		DictField: dictionaryStructInner{
			IntField: 10,
		},
	}

	testDeepEqualDeserailize(bencoded, expected, t)
}

type dictionaryStruct struct {
	StringField string
	DictField   dictionaryStructInner
}

type dictionaryStructInner struct {
	IntField int
}

type dictionaryStructWithOptional struct {
	IntField         int
	OptionalIntField *int
}

type dictionaryStructWithTags struct {
	IntField    int    `bencode:"int_field"`
	StringField string `bencode:"string field"`
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

func testDeepEqualDeserailize[I any](bencoded string, expectedValue I, t *testing.T) {
	var deserialized I
	err := Deserialize(strings.NewReader(bencoded), &deserialized)
	if err != nil {
		t.Errorf("failed to deserialize: %v", err)
	}

	if !reflect.DeepEqual(deserialized, expectedValue) {
		t.Errorf("values don't match: expected %v, got %v", expectedValue, deserialized)
	}
}

func removeWhitespaces(input string) string {
	input = strings.ReplaceAll(input, " ", "")
	input = strings.ReplaceAll(input, "\n", "")
	input = strings.ReplaceAll(input, "\r", "")
	input = strings.ReplaceAll(input, "\t", "")

	return input
}
