FROM debian:12-slim

SHELL ["/bin/bash", "-c"]

RUN apt-get update && apt-get install -y transmission-cli

COPY ./torrent/torrent.torrent /torrent/torrent.torrent

CMD "transmission-cli" "--download-dir=/torrent" "--config-dir=/tmp" "/torrent/torrent.torrent"
