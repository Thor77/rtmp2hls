package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/grafov/m3u8"
	"github.com/nareix/joy4/av"
	"github.com/nareix/joy4/format/rtmp"
	"github.com/nareix/joy4/format/ts"

	log "github.com/sirupsen/logrus"
)

const addr = ":1935"
const key = "test"
const msPerSegment = 15000

// TODO: replace failln with println / switch to logrus to include stream name in msg

func removeOutdatedSegments(streamLogger *log.Entry, streamName string, playlist *m3u8.MediaPlaylist) error {
	currentSegments := make(map[string]struct{}, len(playlist.Segments))
	for _, segment := range playlist.Segments {
		if segment != nil {
			currentSegments[segment.URI] = struct{}{}
		}
	}
	segmentFiles, err := filepath.Glob(fmt.Sprintf("%s*.ts", streamName))
	if err != nil {
		return err
	}
	for _, segmentFile := range segmentFiles {
		if _, ok := currentSegments[segmentFile]; !ok {
			if err := os.Remove(segmentFile); err != nil {
				streamLogger.Errorln(err)
			} else {
				streamLogger.Infof("Removed segment %s\n", segmentFile)
			}
		}
	}
	return nil
}

func main() {
	server := &rtmp.Server{Addr: addr}

	server.HandlePublish = func(conn *rtmp.Conn) {
		log.Infof("Handling request %s\n", conn.URL.RequestURI())
		if conn.URL.Query().Get("key") != key {
			log.Infoln("Key mismatch, aborting request")
			return
		}

		streamName := strings.ReplaceAll(conn.URL.Path, "/", "")
		if streamName == "" {
			log.Errorln("Invalid stream name")
			return
		}

		streamLogger := log.WithFields(log.Fields{"stream": streamName})

		// create hls playlist
		playlistFileName := fmt.Sprintf("%s.m3u8", streamName)
		playlist, err := m3u8.NewMediaPlaylist(5, 10)
		if err != nil {
			streamLogger.Errorln(err)
			return
		}

		streams, err := conn.Streams()
		if err != nil {
			streamLogger.Errorln(err)
			return
		}

		i := 0
		clientConnected := true
		var lastPacketTime time.Duration = 0
		for clientConnected {
			// create new segment
			segmentName := fmt.Sprintf("%s%04d.ts", streamName, i)
			outFile, err := os.Create(segmentName)
			if err != nil {
				streamLogger.Errorln(err)
				return
			}
			tsMuxer := ts.NewMuxer(outFile)

			// write header
			if err := tsMuxer.WriteHeader(streams); err != nil {
				streamLogger.Errorln(err)
				return
			}
			// write some data
			var segmentLength time.Duration = 0
			//var lastPacketTime time.Duration = 0
			var packetLength time.Duration = 0
			for segmentLength.Milliseconds() < msPerSegment {
				var packet av.Packet
				if packet, err = conn.ReadPacket(); err != nil {
					if err == io.EOF {
						streamLogger.Infoln("Client disconnected")
						clientConnected = false
						break
					}
					streamLogger.Errorln(err)
					return
				}
				if err = tsMuxer.WritePacket(packet); err != nil {
					streamLogger.Errorln(err)
					return
				}
				packetLength = packet.Time - lastPacketTime
				segmentLength += packetLength
				lastPacketTime = packet.Time
			}
			// write trailer
			if err := tsMuxer.WriteTrailer(); err != nil {
				streamLogger.Errorln(err)
				return
			}
			streamLogger.Infof("Wrote segment %s\n", segmentName)

			// update playlist
			playlist.Slide(segmentName, segmentLength.Seconds(), "")
			playlistFile, err := os.Create(playlistFileName)
			if err != nil {
				streamLogger.Errorln(err)
				return
			}
			playlistFile.Write(playlist.Encode().Bytes())
			playlistFile.Close()

			// cleanup segments
			if err := removeOutdatedSegments(streamLogger, streamName, playlist); err != nil {
				streamLogger.Errorln(err)
				return
			}

			// increase counter
			i++
		}

		// remove all segments; this is probably not a good idea
		for _, segment := range playlist.Segments {
			if segment != nil {
				if err := os.Remove(segment.URI); err != nil {
					streamLogger.Errorln(err)
					return
				}
			}
		}
		// remove playlist
		if err := os.Remove(playlistFileName); err != nil {
			streamLogger.Error(err)
			return
		}
	}

	log.Printf("Listening on %s", server.Addr)
	server.ListenAndServe()
}
