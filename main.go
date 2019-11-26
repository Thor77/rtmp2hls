package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/grafov/m3u8"
	"github.com/nareix/joy4/av"
	"github.com/nareix/joy4/format/rtmp"
	"github.com/nareix/joy4/format/ts"
)

const addr = ":1935"
const key = "test"
const msPerSegment = 15000

// TODO: replace failln with println / switch to logrus to include stream name in msg

func removeOutdatedSegments(streamName string, playlist *m3u8.MediaPlaylist) error {
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
			os.Remove(segmentFile)
		}
	}
	return nil
}

func main() {
	server := &rtmp.Server{Addr: addr}

	server.HandlePublish = func(conn *rtmp.Conn) {
		log.Printf("Handling request %s", conn.URL.RequestURI())
		if conn.URL.Query().Get("key") != key {
			log.Println("Key mismatch, aborting request")
			return
		}

		streamName := strings.ReplaceAll(conn.URL.Path, "/", "")
		if streamName == "" {
			log.Println("Invalid stream name")
			return
		}

		// create hls playlist
		playlist, err := m3u8.NewMediaPlaylist(5, 10)
		if err != nil {
			log.Fatal(err)
		}

		streams, err := conn.Streams()
		if err != nil {
			log.Fatalln(err)
		}

		i := 0
		clientConnected := true
		var lastPacketTime time.Duration = 0
		for clientConnected {
			// create new segment
			segmentName := fmt.Sprintf("%s%04d.ts", streamName, i)
			outFile, err := os.Create(segmentName)
			if err != nil {
				log.Fatalln(err)
			}
			tsMuxer := ts.NewMuxer(outFile)

			// write header
			if err := tsMuxer.WriteHeader(streams); err != nil {
				log.Fatalln(err)
			}
			// write some data
			var segmentLength time.Duration = 0
			//var lastPacketTime time.Duration = 0
			var packetLength time.Duration = 0
			for segmentLength.Milliseconds() < msPerSegment {
				var packet av.Packet
				if packet, err = conn.ReadPacket(); err != nil {
					if err == io.EOF {
						log.Println("Client disconnected")
						clientConnected = false
						break
					}
					log.Fatalln(err)
				}
				if err = tsMuxer.WritePacket(packet); err != nil {
					log.Fatalln(err)
				}
				packetLength = packet.Time - lastPacketTime
				segmentLength += packetLength
				lastPacketTime = packet.Time
			}
			// write trailer
			if err := tsMuxer.WriteTrailer(); err != nil {
				log.Fatalln(err)
			}
			log.Printf("Successfully wrote segment %s\n", segmentName)

			// update playlist
			playlist.Slide(segmentName, segmentLength.Seconds(), "")
			playlistFile, err := os.Create(fmt.Sprintf("%s.m3u8", streamName))
			if err != nil {
				log.Fatalln(err)
			}
			playlistFile.Write(playlist.Encode().Bytes())
			playlistFile.Close()

			// cleanup segments
			if err := removeOutdatedSegments(streamName, playlist); err != nil {
				log.Println(err)
			}

			// increase counter
			i++
		}

		// remove all segments
		for _, segment := range playlist.Segments {
			if segment != nil {
				if err := os.Remove(segment.URI); err != nil {
					log.Println(err)
				}
			}
		}
	}

	log.Printf("Listening on %s", server.Addr)
	server.ListenAndServe()
}
