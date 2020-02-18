#!/bin/sh
if ! getent group rtmp2hls > /dev/null 2>&1 ; then
    addgroup --system rtmp2hls --quiet
fi
if ! id rtmp2hls > /dev/null 2>&1 ; then
    adduser --system --no-create-home \
        --ingroup rtmp2hls --disabled-password --shell /bin/false \
        rtmp2hls
fi
