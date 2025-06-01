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
		value, err := deserializeInt(reader)
		if err != nil {
			return fmt.Errorf("failed to parse int: %w", err)
		}

		// TODO: Support all int types.
		if reflect.TypeOf(entity).Kind() == reflect.Int {
			// TODO: Check CanSet.
			reflect.ValueOf(entity).SetInt(value)
		} else {
			// TODO: Throw error.
		}
	case 'l':
		// list
	case 'd':
		// dictionary
	default:
		if firstChar >= '0' && firstChar <= '9' {
			value, err := deserializeString(firstChar, reader)
			if err != nil {
				return fmt.Errorf("failed to parse string: %w", err)
			}

			if reflect.TypeOf(entity).Kind() == reflect.String {
				// TODO: Check CanSet.
				reflect.ValueOf(entity).SetString(value)
			} else {
				// TODO: Throw error.
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

func deserializeInt(reader io.Reader) (int64, error) {
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

	result, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse value: %w", err)
	}

	return result, nil
}

func deserializeString(firstChar byte, reader io.Reader) (string, error) {
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

	result := make([]byte, stringLength)
	_, err = io.ReadFull(reader, result)
	if err != nil {
		return "", fmt.Errorf("failed to read value: %w", err)
	}

	return string(result), nil
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
