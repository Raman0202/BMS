FROM jrottenberg/ffmpeg:4.4-alpine
COPY 142755-780943401_medium.mp4 /video.mp4
ENTRYPOINT ["sh", "-c"]
CMD ["while true; do ffmpeg -re -stream_loop -1 -i /video.mp4 -c:v libx264 -preset veryfast -f flv rtmp://mediamtx:1935/DL1TEST_1; sleep 2; done"]
