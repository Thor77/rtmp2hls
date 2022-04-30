FROM scratch
COPY rtmp2hls /
ENTRYPOINT ["/rtmp2hls", "/etc/rtmp2hls.toml"]
