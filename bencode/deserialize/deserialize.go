package deserialize

import (
	"fmt"
	"io"
	"reflect"
	"strconv"
)

const fieldTag = "bencode"

func Deserialize(reader io.Reader, value any) error {
	firstChar, err := readOne(reader)
	if err != nil {
		return fmt.Errorf("failed to read first char: %w", err)
	}

	return deserialize(firstChar, reader, value)
}

func deserialize(firstChar byte, reader io.Reader, entity any) error {
	switch firstChar {
	case 'i':
		err := deserializeInt(reader, entity)
		if err != nil {
			return fmt.Errorf("failed to parse int: %w", err)
		}
	case 'l':
		err := deserializeList(reader, entity)
		if err != nil {
			return fmt.Errorf("failed to parse list: %w", err)
		}
	case 'd':
		err := deserializeDictionary(reader, entity)
		if err != nil {
			return fmt.Errorf("failed to parse dictionary: %w", err)
		}
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

func deserializeAndDrop(firstChar byte, reader io.Reader) error {
	switch firstChar {
	case 'i':
		_, err := readInt(reader)
		if err != nil {
			return fmt.Errorf("failed to parse int: %w", err)
		}
	case 'l':
		err := deserializeAndDropList(reader)
		if err != nil {
			return fmt.Errorf("failed to parse list: %w", err)
		}
	case 'd':
		err := deserializeAndDropDictionary(reader)
		if err != nil {
			return fmt.Errorf("failed to parse dictionary: %w", err)
		}
	default:
		if firstChar >= '0' && firstChar <= '9' {
			_, err := readString(firstChar, reader)
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
	value, err := readInt(reader)
	if err != nil {
		return err
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
	value, err := readString(firstChar, reader)
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
	entityKind := reflect.TypeOf(entity).Kind()
	if entityKind != reflect.Pointer && entityKind != reflect.Interface {
		return fmt.Errorf("wrong field type: expected pointer or interface, got %s", entityKind)
	}

	entityElem := reflect.ValueOf(entity).Elem()
	entityElemKind := entityElem.Kind()
	if entityElemKind != reflect.Struct {
		return fmt.Errorf("wrong field type: expected struct, got %s", entityKind)
	}

	nameMapping := make(map[string]string)
	for i := range entityElem.NumField() {
		field := entityElem.Type().Field(i)
		tag := field.Tag.Get(fieldTag)

		var mapKey string
		if tag != "" {
			mapKey = tag
		} else {
			mapKey = field.Name
		}

		_, keyAlreadyExists := nameMapping[mapKey]
		if keyAlreadyExists {
			return fmt.Errorf("duplicate field names in a dictionary: %s", mapKey)
		}
		nameMapping[mapKey] = field.Name
	}

	for {
		firstChar, err := readOne(reader)
		if err != nil {
			return fmt.Errorf("failed to read dictionary data: %w", err)
		}

		if firstChar == 'e' {
			break
		}

		key, err := readString(firstChar, reader)
		if err != nil {
			return fmt.Errorf("failed to read dictionary key: %w", err)
		}

		firstChar, err = readOne(reader)
		if err != nil {
			return fmt.Errorf("failed to read dictionary data: %w", err)
		}

		fieldName, fieldPresentInMapping := nameMapping[key]
		_, fieldPresent := entityElem.Type().FieldByName(fieldName)

		if !fieldPresentInMapping || !fieldPresent {
			err = deserializeAndDrop(firstChar, reader)
			if err != nil {
				return fmt.Errorf("failed to deserialize dictionary value: %w", err)
			}

			continue
		}

		field := entityElem.FieldByName(fieldName)

		var fieldInterface any
		if field.Type().Kind() == reflect.Pointer {
			newField := reflect.New(field.Type().Elem())
			field.Set(newField)

			fieldInterface = newField.Interface()
		} else {
			if !field.CanAddr() {
				return fmt.Errorf("unaddressable struct field: %s", key)
			}

			fieldInterface = field.Addr().Interface()
		}

		err = deserialize(firstChar, reader, fieldInterface)
		if err != nil {
			return fmt.Errorf("failed to deserialize dictionary value: %w", err)
		}
	}

	return nil
}

func deserializeList(reader io.Reader, entity any) error {
	entityKind := reflect.TypeOf(entity).Kind()
	if entityKind != reflect.Pointer && entityKind != reflect.Interface {
		return fmt.Errorf("wrong field type: expected pointer or interface, got %s", entityKind)
	}

	entityElem := reflect.ValueOf(entity).Elem()
	entityElemKind := entityElem.Kind()
	if entityElemKind != reflect.Slice {
		return fmt.Errorf("wrong field type: expected slice, got %s", entityKind)
	}

	if !entityElem.CanSet() {
		return fmt.Errorf("cannot set slice value")
	}

	list := reflect.MakeSlice(entityElem.Type(), 0, 0)
	entityElem.Set(list)

	for {
		firstChar, err := readOne(reader)
		if err != nil {
			return fmt.Errorf("failed to read list data: %w", err)
		}

		if firstChar == 'e' {
			break
		}

		listElementType := entityElem.Type().Elem()
		newListElement := reflect.New(listElementType)

		err = deserialize(firstChar, reader, newListElement.Interface())
		if err != nil {
			return fmt.Errorf("failed to deserialize list element: %w", err)
		}

		appendedElement := newListElement.Elem()
		newList := reflect.Append(entityElem, appendedElement)
		entityElem.Set(newList)
	}

	return nil
}

func deserializeAndDropList(reader io.Reader) error {
	for {
		firstChar, err := readOne(reader)
		if err != nil {
			return fmt.Errorf("failed to read list data: %w", err)
		}

		if firstChar == 'e' {
			break
		}

		err = deserializeAndDrop(firstChar, reader)
		if err != nil {
			return fmt.Errorf("failed to deserialize list element: %w", err)
		}
	}

	return nil
}

func deserializeAndDropDictionary(reader io.Reader) error {
	for {
		firstChar, err := readOne(reader)
		if err != nil {
			return fmt.Errorf("failed to read dictionary data: %w", err)
		}

		if firstChar == 'e' {
			break
		}

		_, err = readString(firstChar, reader)
		if err != nil {
			return fmt.Errorf("failed to read dictionary key: %w", err)
		}

		firstChar, err = readOne(reader)
		if err != nil {
			return fmt.Errorf("failed to read dictionary data: %w", err)
		}

		err = deserializeAndDrop(firstChar, reader)
		if err != nil {
			return fmt.Errorf("failed to deserialize dictionary value: %w", err)
		}
	}

	return nil
}

func readInt(reader io.Reader) (int64, error) {
	digits := ""
	for {
		nextChar, err := readOne(reader)
		if err != nil {
			return 0, fmt.Errorf("failed to read value: %w", err)
		}

		if nextChar == 'e' {
			break
		}

		digits += string(nextChar)
	}

	value, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse value: %w", err)
	}

	return value, nil
}

func readString(firstChar byte, reader io.Reader) (string, error) {
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
