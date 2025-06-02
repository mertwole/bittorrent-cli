package bencode

import (
	"io"

	"github.com/mertwole/bittorrent-cli/bencode/deserialize"
)

func Deserialize(reader io.Reader, value any) error {
	return deserialize.Deserialize(reader, value)
}
