FROM jrottenberg/ffmpeg:4.4-alpine
COPY 142755-780943401_medium.mp4 /video.mp4
# RTMP_TARGET defaults to the docker-compose service (app); the k8s publisher
# overrides it to the in-cluster ingest Service. The source mp4 is already
# H.264/yuv420p, so -c copy (no re-encode) — trivial CPU/RAM, no OOM, and
# browser-decodable as-is. -bsf:v h264_mp4toannexb repackages for FLV/RTMP.
ENV RTMP_TARGET=rtmp://app:1935/DL1TEST_1
ENTRYPOINT ["sh", "-c"]
CMD ["while true; do ffmpeg -re -stream_loop -1 -i /video.mp4 -c copy -bsf:v h264_mp4toannexb -f flv \"$RTMP_TARGET\"; sleep 2; done"]
