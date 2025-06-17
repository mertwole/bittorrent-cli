package serialize

import (
	"cmp"
	"fmt"
	"io"
	"reflect"
	"slices"
)

const fieldTag = "bencode"

func Serialize(writer io.Writer, value any) error {
	valueKind := reflect.TypeOf(value).Kind()
	valueValue := reflect.ValueOf(value)
	switch valueKind {
	case reflect.Pointer, reflect.Interface:
		deref := valueValue.Elem()
		return Serialize(writer, deref.Interface())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		intValue := valueValue.Int()

		fmt.Fprintf(writer, "i%de", intValue)
	case reflect.String:
		stringValue := valueValue.String()
		length := len(stringValue)

		fmt.Fprintf(writer, "%d:%s", length, stringValue)
	case reflect.Array, reflect.Slice:
		fmt.Fprint(writer, "l")

		for i := range valueValue.Len() {
			err := Serialize(writer, valueValue.Index(i).Interface())
			if err != nil {
				return err
			}
		}

		fmt.Fprint(writer, "e")
	case reflect.Map:
		fmt.Fprintf(writer, "d")

		entries := valueValue.MapRange()
		mapKeyValues := make([]MapKeyValue, 0)
		for entries.Next() {
			mapKey := entries.Key()
			mapValue := entries.Value()

			mapKeyKind := reflect.TypeOf(mapKey.Interface()).Kind()
			if mapKeyKind != reflect.String {
				return fmt.Errorf("invalid map key type: %s, only string keys are supported", mapKeyKind)
			}

			keyValue := MapKeyValue{key: mapKey.String(), value: mapValue}
			mapKeyValues = append(mapKeyValues, keyValue)
		}

		slices.SortFunc(mapKeyValues, func(a, b MapKeyValue) int { return cmp.Compare(a.key, b.key) })

		for _, keyValue := range mapKeyValues {
			err := Serialize(writer, keyValue.key)
			if err != nil {
				return err
			}

			err = Serialize(writer, keyValue.value.Interface())
			if err != nil {
				return err
			}
		}

		fmt.Fprintf(writer, "e")
	case reflect.Struct:
		fmt.Fprint(writer, "d")

		fields := make(map[string]string, 0)
		for i := range valueValue.NumField() {
			fieldType := reflect.TypeOf(value).Field(i)
			fieldTagValue := fieldType.Tag.Get(fieldTag)

			if fieldTagValue == "" {
				fieldTagValue = fieldType.Name
			}

			if _, ok := fields[fieldTagValue]; ok {
				return fmt.Errorf("fields with duplicate name found: %s", fieldTagValue)
			}

			fields[fieldTagValue] = fieldType.Name
		}

		fieldKeys := make([]string, 0)
		for key := range fields {
			fieldKeys = append(fieldKeys, key)
		}

		slices.Sort(fieldKeys)

		for _, fieldKey := range fieldKeys {
			fieldName := fields[fieldKey]

			field := valueValue.FieldByName(fieldName)
			fieldInterface := field.Interface()

			fieldInterfaceKind := reflect.TypeOf(fieldInterface).Kind()
			if fieldInterfaceKind == reflect.Pointer && field.IsNil() {
				// Optional field.
				continue
			}

			err := Serialize(writer, fieldKey)
			if err != nil {
				return err
			}

			err = Serialize(writer, fieldInterface)
			if err != nil {
				return err
			}
		}

		fmt.Fprint(writer, "e")
	default:
		return fmt.Errorf("unserializable type: %v", valueKind)
	}

	return nil
}

type MapKeyValue struct {
	key   string
	value reflect.Value
}
