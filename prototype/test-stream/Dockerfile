FROM jrottenberg/ffmpeg:4.4-alpine
COPY 142755-780943401_medium.mp4 /video.mp4
ENTRYPOINT ["sh", "-c"]
CMD ["while true; do ffmpeg -re -stream_loop -1 -i /video.mp4 -map 0:v -vf scale=-2:720 -c:v libx264 -preset ultrafast -tune zerolatency -g 30 -an -b:v 2500k -maxrate 2500k -bufsize 5000k -f flv \"${RTMP_TARGET:-rtmp://app:1935/livefeed}\"; sleep 2; done"]
