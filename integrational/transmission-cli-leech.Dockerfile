FROM ubuntu:latest

SHELL ["/bin/bash", "-c"]

RUN apt-get update && apt-get install -y transmission-daemon transmission-cli

COPY ./torrent/torrent.torrent /torrent/torrent.torrent
COPY ./transmission-config/settings.json /etc/transmission-daemon/settings.json
COPY ./transmission-config/startup.sh /startup.sh

RUN chmod +x /startup.sh

CMD "/startup.sh"