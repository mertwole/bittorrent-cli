# Bittorrent-cli

Bittorrent-cli is a command-line interface Bittorrent client.

![Demo](./vhs/demo.gif)

## Getting binaries

### Precompiled binaries

Precompiled binaries are available for `Windows` and `Linux` on the releases page

### Compiling from source 

Instal golang: https://go.dev/doc/install

```bash
git clone https://github.com/mertwole/bittorrent-cli.git
cd bittorrent-cli
go build .
```

## Running application

### Interactive TUI mode

```bash
./bittorrent-cli
```

### Non-interactive mode

```bash
./bittorrent-cli --torrent [Path to torrent file] --download [Path to download folder] --interactive false
```

## License

[GNU General Public License](LICENSE)