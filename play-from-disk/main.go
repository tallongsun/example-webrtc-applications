package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media"
	"github.com/pion/webrtc/v3/pkg/media/ivfreader"
	"github.com/pion/webrtc/v3/pkg/media/oggreader"
	"io"
	"net/http"
	"os"
	"time"
)

const (
	audioFileName = "output.ogg"
	videoFileName = "output.ivf"
)

//ffmpeg -i big_buck_bunny.mp4 -g 30 output.ivf
//ffmpeg -i big_buck_bunny.mp4 -c:a libopus -page_duration 20000 -vn output.ogg
func main() {
	http.Handle("/", http.FileServer(http.Dir(".")))
	http.HandleFunc("/signal", signaling)
	http.ListenAndServe(":8080", nil)
}

func signaling(w http.ResponseWriter, r *http.Request) {
	_, err := os.Stat(videoFileName)
	haveVideoFile := !os.IsNotExist(err)

	_, err = os.Stat(audioFileName)
	haveAudioFile := !os.IsNotExist(err)
	if !haveAudioFile && !haveVideoFile {
		panic("Could not find `" + audioFileName + "` or `" + videoFileName + "`")
	}

	peerConnection, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		panic(err)
	}

	iceConnectedCtx, iceConnectedCtxCancel := context.WithCancel(context.Background())

	if haveVideoFile {
		videoTrack, err := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: "video/vp8"}, "video", "pion")
		if err != nil {
			panic(err)
		}

		// Add this newly created track to the PeerConnection
		_, err = peerConnection.AddTrack(videoTrack)
		if err != nil {
			panic(err)
		}

		go func() {
			// Open a IVF file and start reading using our IVFReader
			file, ivfErr := os.Open(videoFileName)
			if ivfErr != nil {
				panic(ivfErr)
			}

			ivf, header, ivfErr := ivfreader.NewWith(file)
			if ivfErr != nil {
				panic(ivfErr)
			}

			// Wait for connection established
			<-iceConnectedCtx.Done()

			// Send our video file frame at a time. Pace our sending so we send it at the same speed it should be played back as.
			// This isn't required since the video is timestamped, but we will such much higher loss if we send all at once.
			sleepTime := time.Millisecond * time.Duration((float32(header.TimebaseNumerator)/float32(header.TimebaseDenominator))*1000)
			for {
				frame, _, ivfErr := ivf.ParseNextFrame()
				if ivfErr == io.EOF {
					fmt.Printf("All video frames parsed and sent")
					os.Exit(0)
				}

				if ivfErr != nil {
					panic(ivfErr)
				}

				time.Sleep(sleepTime)
				if ivfErr = videoTrack.WriteSample(media.Sample{Data: frame, Duration: time.Second}); ivfErr != nil {
					panic(ivfErr)
				}
			}
		}()

	}

	if haveAudioFile {
		// Create a audio track
		audioTrack, audioTrackErr := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: "audio/opus"}, "audio", "pion")
		if audioTrackErr != nil {
			panic(audioTrackErr)
		}

		_, audioTrackErr = peerConnection.AddTrack(audioTrack)
		if audioTrackErr != nil {
			panic(audioTrackErr)
		}

		go func() {
			// Open a IVF file and start reading using our IVFReader
			file, oggErr := os.Open(audioFileName)
			if oggErr != nil {
				panic(oggErr)
			}

			// Open on oggfile in non-checksum mode.
			ogg, _, oggErr := oggreader.NewWith(file)
			if oggErr != nil {
				panic(oggErr)
			}

			// Wait for connection established
			<-iceConnectedCtx.Done()

			// Keep track of last granule, the difference is the amount of samples in the buffer
			var lastGranule uint64
			for {
				pageData, pageHeader, oggErr := ogg.ParseNextPage()
				if oggErr == io.EOF {
					fmt.Printf("All audio pages parsed and sent")
					os.Exit(0)
				}

				if oggErr != nil {
					panic(oggErr)
				}

				// The amount of samples is the difference between the last and current timestamp
				sampleCount := float64(pageHeader.GranulePosition - lastGranule)
				lastGranule = pageHeader.GranulePosition
				sampleDuration := time.Duration((sampleCount/48000)*1000) * time.Millisecond

				if oggErr = audioTrack.WriteSample(media.Sample{Data: pageData, Duration: sampleDuration}); oggErr != nil {
					panic(oggErr)
				}

				time.Sleep(sampleDuration)
			}
		}()
	}



	peerConnection.OnICECandidate(func(i *webrtc.ICECandidate) {
		fmt.Println("on ice candidate.")
		if i != nil {
			fmt.Println(i.String())
		}
	})

	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		fmt.Printf("ICE Connection State has changed: %s\n", connectionState.String())
		if connectionState == webrtc.ICEConnectionStateConnected {
			iceConnectedCtxCancel()
		}
	})
	peerConnection.OnConnectionStateChange(func(p webrtc.PeerConnectionState) {
		fmt.Printf("Connection State has changed: %s\n",p.String())
	})

	var offer webrtc.SessionDescription
	if err = json.NewDecoder(r.Body).Decode(&offer); err != nil {
		panic(err)
	}
	fmt.Println(offer.Type)
	if err = peerConnection.SetRemoteDescription(offer); err != nil {
		panic(err)
	}
	answer, err := peerConnection.CreateAnswer(nil)
	if err != nil {
		panic(err)
	}
	gatherComplete := webrtc.GatheringCompletePromise(peerConnection)
	if err = peerConnection.SetLocalDescription(answer); err != nil {
		panic(err)
	}
	<-gatherComplete
	response, err := json.Marshal(*peerConnection.LocalDescription())
	if err != nil {
		panic(err)
	}
	fmt.Println(answer.Type)
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(response); err != nil {
		panic(err)
	}
}
