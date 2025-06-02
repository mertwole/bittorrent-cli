package bencode

import (
	"io"

	"github.com/mertwole/bittorrent-cli/bencode/deserialize"
	"github.com/mertwole/bittorrent-cli/bencode/serialize"
)

func Deserialize(reader io.Reader, value any) error {
	return deserialize.Deserialize(reader, value)
}

func Serialize(writer io.Writer, value any) error {
	return serialize.Serialize(writer, value)
}
