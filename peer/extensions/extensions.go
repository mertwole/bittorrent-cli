package extensions

import "fmt"

type Extensions struct {
	extensions map[string]int
}

func New() Extensions {
	return Extensions{extensions: make(map[string]int)}
}

func (extensions *Extensions) Insert(name string, id int) error {
	if id == 0 {
		delete(extensions.extensions, name)
	} else {
		presentId, ok := extensions.extensions[name]
		if ok && presentId != id {
			return fmt.Errorf(
				"conflicting extensions discovered: extension '%s' corresponds to message ids %d and %d",
				name,
				presentId,
				id,
			)
		}

		for key, value := range extensions.extensions {
			if value == id && key != name {
				return fmt.Errorf(
					"conflicting extensions discovered: message id %d corresponds to extensions '%s' and '%s'",
					id,
					name,
					key,
				)
			}
		}

		extensions.extensions[name] = id
	}

	return nil
}

func (extensions *Extensions) GetID(name string) (int, bool) {
	id, ok := extensions.extensions[name]
	return id, ok
}
