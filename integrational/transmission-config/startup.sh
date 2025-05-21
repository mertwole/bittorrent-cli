service transmission-daemon start

sleep 2

transmission-remote --add /torrent/torrent.torrent --download-dir /download
transmission-remote --torrent 1 --start

while true; do
    transmission-remote -l
    sleep 2
done
