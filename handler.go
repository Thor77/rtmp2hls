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

func removeOutdatedSegments(streamLogger *log.Entry, streamName string, playlist *m3u8.MediaPlaylist) error {
	// write all playlist segment URIs into map
	currentSegments := make(map[string]struct{}, len(playlist.Segments))
	for _, segment := range playlist.Segments {
		if segment != nil {
			currentSegments[segment.URI] = struct{}{}
		}
	}
	// find (probably) segment files in current directory
	segmentFiles, err := filepath.Glob(fmt.Sprintf("%s*.ts", streamName))
	if err != nil {
		return err
	}
	for _, segmentFile := range segmentFiles {
		// check if file belongs to a playlist segment
		if _, ok := currentSegments[segmentFile]; !ok {
			if err := os.Remove(segmentFile); err != nil {
				streamLogger.Errorln(err)
			} else {
				streamLogger.Debugf("Removed segment %s\n", segmentFile)
			}
		}
	}
	return nil
}

func publishHandler(conn *rtmp.Conn) {
	log.Debugf("Handling request %s\n", conn.URL.RequestURI())

	// verify key
	if config.Key != "" {
		if conn.URL.Query().Get("key") != config.Key {
			log.Errorln("Key mismatch, aborting request")
			return
		}
	}

	// verify stream has a name
	streamName := strings.ReplaceAll(conn.URL.Path, "/", "")
	if streamName == "" {
		log.Errorln("Invalid stream name")
		return
	}

	streamLogger := log.WithFields(log.Fields{"stream": streamName})

	streamLogger.Infoln("Client connected")

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
		// create new segment file
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

		// write packets
		var segmentLength time.Duration = 0
		var packetLength time.Duration = 0
		for segmentLength.Milliseconds() < config.MsPerSegment {
			// read packet from source
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
			// write packet to destination
			if err = tsMuxer.WritePacket(packet); err != nil {
				streamLogger.Errorln(err)
				return
			}

			// calculate segment length
			packetLength = packet.Time - lastPacketTime
			segmentLength += packetLength
			lastPacketTime = packet.Time
		}
		// write trailer
		if err := tsMuxer.WriteTrailer(); err != nil {
			streamLogger.Errorln(err)
			return
		}

		// close segment file
		if err := outFile.Close(); err != nil {
			streamLogger.Errorln(err)
			return
		}

		streamLogger.Debugf("Wrote segment %s\n", segmentName)

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

		// increase segment index
		i++
	}

	// remove all segments; this is probably a bad idea
	for _, segment := range playlist.Segments {
		if segment != nil {
			if err := os.Remove(segment.URI); err != nil {
				streamLogger.Errorln(err)
				return
			}
			streamLogger.Debugf("Removed segment %s\n", segment.URI)
		}
	}
	// remove playlist
	if err := os.Remove(playlistFileName); err != nil {
		streamLogger.Error(err)
		return
	}
}
