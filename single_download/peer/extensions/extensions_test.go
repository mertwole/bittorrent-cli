package extensions

import (
	"testing"
)

func TestExtensionsInsert(t *testing.T) {
	extensions := Empty()

	insertExtension("test_0", 1, &extensions, t)
	assertExtensionPresent("test_0", 1, &extensions, t)

	insertExtensionShouldFail("test_0", 2, &extensions, t)

	insertExtensionShouldFail("test_1", 1, &extensions, t)

	insertExtension("test_1", 2, &extensions, t)
	insertExtension("test_1", 2, &extensions, t)
	assertExtensionPresent("test_1", 2, &extensions, t)

	insertExtension("test_1", 0, &extensions, t)
	assertExtensionNotPresent("test_1", &extensions, t)
}

func insertExtension(name string, id int, extensions *Extensions, t *testing.T) {
	err := extensions.Insert(name, id)
	if err != nil {
		t.Errorf("failed to insert extension ID: %v", err)
	}
}

func insertExtensionShouldFail(name string, id int, extensions *Extensions, t *testing.T) {
	err := extensions.Insert(name, id)
	if err == nil {
		t.Errorf("expected error inserting extension, got success")
	}
}

func assertExtensionPresent(name string, expectedID int, extensions *Extensions, t *testing.T) {
	id, ok := extensions.GetID(name)

	if !ok {
		t.Errorf("failed to fetch extension ID for extension %s", name)
	}

	if id != expectedID {
		t.Errorf("unexpected extension ID. expected %d, got %d", expectedID, id)
	}
}

func assertExtensionNotPresent(name string, extensions *Extensions, t *testing.T) {
	id, ok := extensions.GetID(name)
	if ok {
		t.Errorf("unexpected extension present '%s' with ID %d", name, id)
	}
}
