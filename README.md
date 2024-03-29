# rtmp2hls
Simple rtmp server with hls output based on [joy4](https://github.com/nareix/joy4)

## Build
```
go build .
```

## Usage
```
./rtmp2hls [<path to config file>]
```
If config path not provided, config will be read from `config.toml` in the current directory.

`<streamname><segment index>.ts` segments and `<streamname>.m3u8` playlist are written to the current directory.

### Streaming
Using [ffmpeg](https://ffmpeg.org/)
```
ffmpeg -i <input file> -f flv "rtmp://localhost/stream01"
```

## Configuration
| key | type | default | description |
|-----|------|---------|-------------|
| Addr | `string` | `":1935"` | RTMP server listen address |
| Key | `string` | | Expected value of `key` query parameter for connecting clients (disabled if unset) |
| MsPerSegment | `int64` | `15000` | Milliseconds of video/audio written to one segment file |
| LogLevel | `log.Level` | `"info"` | Level for the internal logger |
| HLSDirectory | `string` | `"."` | Directory to write playlist/segment files into
| BaseURL | `string` | | Base URL to use as a prefix for segment files in playlist file |
