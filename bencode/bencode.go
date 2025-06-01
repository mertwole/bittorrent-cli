package bencode

import (
	"fmt"
	"io"
	"reflect"
	"strconv"
)

// string: <length>:<data>
// integer: i<data, base-10>e
// list: l<value><value><...>e
// dictionary: d<key><value><key><value><...>e // keys should be sorted

func Deserialize(reader io.Reader, value interface{}) error {
	firstChar, err := readOne(reader)
	if err != nil {
		return fmt.Errorf("failed to read first char: %w", err)
	}

	return deserializeInner(firstChar, reader, value)
}

func deserializeInner(firstChar byte, reader io.Reader, entity any) error {
	switch firstChar {
	case 'i':
		err := deserializeInt(reader, entity)
		if err != nil {
			return fmt.Errorf("failed to parse int: %w", err)
		}
	case 'l':
		// list
	case 'd':
		// dictionary
	default:
		if firstChar >= '0' && firstChar <= '9' {
			err := deserializeString(firstChar, reader, entity)
			if err != nil {
				return fmt.Errorf("failed to parse string: %w", err)
			}
		} else {
			return fmt.Errorf(
				"unexpected characted found: %s, expected one of `i`, `l`, `d`, `0-9`",
				string(firstChar),
			)
		}
	}

	return nil
}

func deserializeInt(reader io.Reader, entity any) error {
	digits := ""
	for {
		nextChar, err := readOne(reader)
		if err != nil {
			return fmt.Errorf("failed to read value: %w", err)
		}

		if nextChar == 'e' {
			break
		}

		digits += string(nextChar)
	}

	value, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		return fmt.Errorf("failed to parse value: %w", err)
	}

	entityKind := reflect.TypeOf(entity).Kind()
	if entityKind != reflect.Pointer && entityKind != reflect.Interface {
		return fmt.Errorf("wrong field type: expected pointer or interface, got %s", entityKind)
	}

	entityElem := reflect.ValueOf(entity).Elem()
	entityElemKind := entityElem.Kind()
	if entityElemKind != reflect.Int &&
		entityElemKind != reflect.Int8 &&
		entityElemKind != reflect.Int16 &&
		entityElemKind != reflect.Int32 &&
		entityElemKind != reflect.Int64 {
		return fmt.Errorf("wrong field type: expected integer, got %s", entityKind)
	}

	if !entityElem.CanSet() {
		return fmt.Errorf("cannot set integer value %s", entityElem)
	}

	entityElem.SetInt(value)

	return nil
}

func deserializeString(firstChar byte, reader io.Reader, entity any) error {
	value, err := readBencodedString(firstChar, reader)
	if err != nil {
		return fmt.Errorf("failed to read bencoded string: %w", err)
	}

	entityKind := reflect.TypeOf(entity).Kind()
	if entityKind != reflect.Pointer && entityKind != reflect.Interface {
		return fmt.Errorf("wrong field type: expected pointer or interface, got %s", entityKind)
	}

	entityElem := reflect.ValueOf(entity).Elem()
	entityElemKind := entityElem.Kind()
	if entityElemKind != reflect.String {
		return fmt.Errorf("wrong field type: expected string, got %s", entityKind)
	}

	if !entityElem.CanSet() {
		return fmt.Errorf("cannot set string value %s", entityElem)
	}

	entityElem.SetString(value)

	return nil
}

func deserializeDictionary(reader io.Reader, entity any) error {
	for {
		firstChar, err := readOne(reader)
		if err != nil {
			return fmt.Errorf("failed to read dictionary data: %w", err)
		}

		if firstChar == 'e' {
			break
		}

		key, err := readBencodedString(firstChar, reader)
		if err != nil {
			return fmt.Errorf("failed to read dictionary key: %w", err)
		}

		firstChar, err = readOne(reader)
		if err != nil {
			return fmt.Errorf("failed to read dictionary data: %w", err)
		}

		// TODO: Read tags.
		// TODO: Check if field is present.
		field, _ := reflect.TypeOf(entity).FieldByName(key)
		// TODO: Check if it's settable.
		fieldInterface := reflect.ValueOf(field).Interface()
		err = deserializeInner(firstChar, reader, fieldInterface)
		if err != nil {
			return fmt.Errorf("failed to deserialize dictionary value: %w", err)
		}
	}

	return nil
}

func readBencodedString(firstChar byte, reader io.Reader) (string, error) {
	lengthString := string(firstChar)
	for {
		nextChar, err := readOne(reader)
		if err != nil {
			return "", fmt.Errorf("failed to read value: %w", err)
		}

		if nextChar == ':' {
			break
		}

		lengthString += string(nextChar)
	}

	stringLength, err := strconv.ParseInt(lengthString, 10, 64)
	if err != nil {
		return "", fmt.Errorf("failed to parse a length: %w", err)
	}

	value := make([]byte, stringLength)
	_, err = io.ReadFull(reader, value)
	if err != nil {
		return "", fmt.Errorf("failed to read value: %w", err)
	}

	return string(value), nil
}

func readOne(reader io.Reader) (byte, error) {
	first := make([]byte, 1)

	_, err := io.ReadFull(reader, first)
	if err != nil {
		return 0, err
	}

	return first[0], nil
}

func Serialize(writer io.Writer, value interface{}) error {
	// TODO

	return nil
}
