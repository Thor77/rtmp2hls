package main

import (
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/grafov/m3u8"
	"github.com/nareix/joy4/av"
	"github.com/nareix/joy4/format/rtmp"
	"github.com/nareix/joy4/format/ts"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
)

type connectionLock struct {
	mutex       sync.RWMutex
	connections map[string]uint8
}

func (c *connectionLock) Add(name string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.connections[name] = 1
}

func (c *connectionLock) Remove(name string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	delete(c.connections, name)
}

func (c *connectionLock) Exists(name string) bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	_, ok := c.connections[name]
	return ok
}

func removeOutdatedSegments(streamLogger *log.Entry, streamName string, playlist *m3u8.MediaPlaylist) error {
	// write all playlist segment URIs into map
	currentSegments := make(map[string]struct{}, len(playlist.Segments))
	for _, segment := range playlist.Segments {
		if segment != nil {
			currentSegments[path.Base(segment.URI)] = struct{}{}
		}
	}
	// find (probably) segment files in current directory
	segmentFiles, err := filepath.Glob(filepath.Join(config.HLSDirectory, fmt.Sprintf("%s*.ts", streamName)))
	if err != nil {
		return err
	}
	for _, segmentFile := range segmentFiles {
		// check if file belongs to a playlist segment
		if _, ok := currentSegments[path.Base(segmentFile)]; !ok {
			if err := os.Remove(segmentFile); err != nil {
				streamLogger.Errorln(err)
			} else {
				streamLogger.Debugf("Removed segment %s\n", segmentFile)
			}
		}
	}
	return nil
}

func handleErrorString(logger *log.Entry, conn *rtmp.Conn, err string) {
	logger.Errorln(err)
	if err := conn.Close(); err != nil {
		logger.Errorf("Error closing connection: %v", err)
	} else {
		logger.Infoln("Connection closed")
	}
}

func handleError(logger *log.Entry, conn *rtmp.Conn, err error) {
	handleErrorString(logger, conn, err.Error())
}

var connections = &connectionLock{connections: make(map[string]uint8)}

func publishHandler(conn *rtmp.Conn) {
	connLogger := log.WithField("remoteAddr", conn.NetConn().RemoteAddr().String())
	connLogger.Debugf("Handling request %s\n", conn.URL.RequestURI())

	// verify key
	if config.Key != "" {
		givenKey := conn.URL.Query().Get("key")
		if givenKey != config.Key {
			handleErrorString(connLogger.WithField("givenKey", givenKey), conn, "Key mismatch, aborting request")
			totalErrors.With(prometheus.Labels{"error": "key_mismatch"}).Inc()
			return
		}
	}

	// verify stream has a name
	streamName := strings.ReplaceAll(conn.URL.Path, "/", "")
	if streamName == "" {
		handleErrorString(connLogger.WithField("path", conn.URL.Path), conn, "Invalid stream name")
		totalErrors.With(prometheus.Labels{"error": "invalid_stream_name"}).Inc()
		return
	}

	streamLogger := connLogger.WithFields(log.Fields{"stream": streamName})

	if connections.Exists(streamName) {
		handleErrorString(streamLogger, conn, "client for this stream already exists")
		totalErrors.With(prometheus.Labels{"error": "duplicate_stream"}).Inc()
		return
	}

	// add stream to connections table
	connections.Add(streamName)

	// delete stream from connection table
	defer connections.Remove(streamName)

	streamLogger.Infoln("Client connected")
	totalConnections.Inc()
	currentConnections.Inc()
	defer currentConnections.Dec()

	// create hls playlist
	playlistFileName := filepath.Join(config.HLSDirectory, fmt.Sprintf("%s.m3u8", streamName))
	playlist, err := m3u8.NewMediaPlaylist(5, 10)
	if err != nil {
		handleError(streamLogger, conn, err)
		return
	}

	streams, err := conn.Streams()
	if err != nil {
		handleError(streamLogger, conn, err)
		return
	}

	var i uint8 = 1
	clientConnected := true
	var lastPacketTime time.Duration = 0
	for clientConnected {
		// create new segment file
		segmentName := filepath.Join(config.HLSDirectory, fmt.Sprintf("%s%04d.ts", streamName, i))
		outFile, err := os.Create(segmentName)
		if err != nil {
			handleError(streamLogger, conn, err)
			return
		}
		tsMuxer := ts.NewMuxer(outFile)

		// write header
		if err := tsMuxer.WriteHeader(streams); err != nil {
			handleError(streamLogger, conn, err)
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
				handleError(streamLogger, conn, err)
				return
			}
			// write packet to destination
			if err = tsMuxer.WritePacket(packet); err != nil {
				handleError(streamLogger, conn, err)
				return
			}

			// calculate segment length
			packetLength = packet.Time - lastPacketTime
			segmentLength += packetLength
			lastPacketTime = packet.Time
		}
		// write trailer
		if err := tsMuxer.WriteTrailer(); err != nil {
			handleError(streamLogger, conn, err)
			return
		}

		// close segment file
		if err := outFile.Close(); err != nil {
			handleError(streamLogger, conn, err)
			return
		}

		streamLogger.Debugf("Wrote segment %s\n", segmentName)

		// update playlist
		var playlistSegmentName string
		if config.BaseURL != "" {
			playlistSegmentName = path.Join(config.BaseURL, path.Base(segmentName))
		} else {
			playlistSegmentName = segmentName
		}
		playlist.Slide(playlistSegmentName, segmentLength.Seconds(), "")
		playlistFile, err := os.Create(playlistFileName)
		if err != nil {
			handleError(streamLogger, conn, err)
			return
		}
		playlistFile.Write(playlist.Encode().Bytes())
		playlistFile.Close()

		// cleanup segments
		if err := removeOutdatedSegments(streamLogger, streamName, playlist); err != nil {
			handleError(streamLogger, conn, err)
			return
		}

		// increase segment index
		if i == (math.MaxUint8 - 1) {
			i = 1
		} else {
			i++
		}
	}

	filesToRemove := make([]string, len(playlist.Segments)+1)

	// collect obsolete files
	for _, segment := range playlist.Segments {
		if segment != nil {
			filesToRemove = append(filesToRemove, path.Join(config.HLSDirectory, path.Base(segment.URI)))
		}
	}
	filesToRemove = append(filesToRemove, playlistFileName)

	// delete them later
	go func(logger *log.Entry, delay time.Duration, filesToRemove []string) {
		logger.Debugf("Files to be deleted after %v: %v", delay, filesToRemove)
		time.Sleep(delay)
		for _, file := range filesToRemove {
			if file != "" {
				if err := os.Remove(file); err != nil {
					logger.Errorln(err)
				} else {
					logger.Debugf("Successfully removed %s", file)
				}
			}
		}
	}(streamLogger, time.Duration(config.MsPerSegment*int64(playlist.Count()))*time.Millisecond, filesToRemove)
}
