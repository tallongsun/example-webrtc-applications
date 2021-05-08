package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/pion/interceptor"
	"github.com/pion/webrtc/v3"
	"io"
	"net/http"
)

func main() {

	http.Handle("/", http.FileServer(http.Dir(".")))
	http.HandleFunc("/signal", signaling)
	http.ListenAndServe(":8080", nil)
}

func signaling(w http.ResponseWriter, r *http.Request) {
	// Enable Extension Headers needed for Simulcast
	m := &webrtc.MediaEngine{}
	if err := m.RegisterDefaultCodecs(); err != nil {
		panic(err)
	}
	for _, extension := range []string{
		"urn:ietf:params:rtp-hdrext:sdes:mid",
		"urn:ietf:params:rtp-hdrext:sdes:rtp-stream-id",
		"urn:ietf:params:rtp-hdrext:sdes:repaired-rtp-stream-id",
	} {
		if err := m.RegisterHeaderExtension(webrtc.RTPHeaderExtensionCapability{URI: extension}, webrtc.RTPCodecTypeVideo); err != nil {
			panic(err)
		}
	}
	i := &interceptor.Registry{}
	if err := webrtc.RegisterDefaultInterceptors(m, i); err != nil {
		panic(err)
	}
	peerConnection, err := webrtc.NewAPI(webrtc.WithMediaEngine(m), webrtc.WithInterceptorRegistry(i)).NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		panic(err)
	}

	outputTracks := map[string]*webrtc.TrackLocalStaticRTP{}
	outputTrack, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: "video/vp8"}, "video_q", "pion_q")
	if err != nil {
		panic(err)
	}
	outputTracks["q"] = outputTrack

	outputTrack, err = webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: "video/vp8"}, "video_h", "pion_h")
	if err != nil {
		panic(err)
	}
	outputTracks["h"] = outputTrack

	outputTrack, err = webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: "video/vp8"}, "video_f", "pion_f")
	if err != nil {
		panic(err)
	}
	outputTracks["f"] = outputTrack
	if _, err = peerConnection.AddTrack(outputTracks["q"]); err != nil {
		panic(err)
	}
	if _, err = peerConnection.AddTrack(outputTracks["h"]); err != nil {
		panic(err)
	}
	if _, err = peerConnection.AddTrack(outputTracks["f"]); err != nil {
		panic(err)
	}

	peerConnection.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		fmt.Println("on track.",track.ID(),track.StreamID(),track.Kind(),track.RID())
		for {
			packet, _, readErr := track.ReadRTP()
			if readErr != nil {
				panic(readErr)
			}
			if writeErr := outputTracks[track.RID()].WriteRTP(packet); writeErr != nil && !errors.Is(writeErr, io.ErrClosedPipe) {
				panic(writeErr)
			}

		}
	})

	peerConnection.OnICECandidate(func(i *webrtc.ICECandidate) {
		fmt.Println("on ice candidate.")
		if i != nil {
			fmt.Println(i.String())
		}
	})

	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		fmt.Printf("ICE Connection State has changed: %s\n", connectionState.String())
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
