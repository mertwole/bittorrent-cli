package serialize

import (
	"fmt"
	"io"
	"reflect"
)

func Serialize(writer io.Writer, value any) error {
	valueKind := reflect.TypeOf(value).Kind()
	valueValue := reflect.ValueOf(value)
	switch valueKind {
	case reflect.Pointer, reflect.Interface:
		deref := valueValue.Elem().Interface()
		return Serialize(writer, deref)
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
			Serialize(writer, valueValue.Index(i).Interface())
		}

		fmt.Fprint(writer, "e")
	case reflect.Struct:
		fmt.Fprint(writer, "d")

		// TODO
		// NOTE: When encoding dictionaries, keys must be sorted.

		fmt.Fprint(writer, "e")
	default:
		return fmt.Errorf("unserializable type: %v", valueKind)
	}

	return nil
}
