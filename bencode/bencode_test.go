package bencode

import (
	"strings"
	"testing"
)

func TestIntDeserialize(t *testing.T) {
	bencoded := strings.NewReader("i10e")
	expected := 10

	var deserialized int
	err := Deserialize(bencoded, &deserialized)
	if err != nil {
		t.Errorf("failed to deserialize: %v", err)
	}

	if deserialized != expected {
		t.Errorf("values don't match: expected %d, got %d", expected, deserialized)
	}
}
