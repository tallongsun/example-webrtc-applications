package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"
	"io"
	"net/http"
	"time"
)


var localTrackChan *webrtc.TrackLocalStaticRTP

func main() {
	http.Handle("/", http.FileServer(http.Dir(".")))
	http.HandleFunc("/signal", signaling)
	http.ListenAndServe(":8080", nil)
}



func signaling(w http.ResponseWriter, r *http.Request) {
	keys, _ := r.URL.Query()["publisher"]

	if keys[0] == "true" {
		peerConnection, err := webrtc.NewPeerConnection(webrtc.Configuration{})
		if err != nil {
			panic(err)
		}

		peerConnection.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
			fmt.Println("on track.",track.ID(),track.StreamID(),track.Kind(),track.RID())

			// Send a PLI on an interval so that the publisher is pushing a keyframe every rtcpPLIInterval
			// This can be less wasteful by processing incoming RTCP events, then we would emit a NACK/PLI when a viewer requests it
			go func() {
				ticker := time.NewTicker(time.Second * 3)
				for range ticker.C {
					if rtcpSendErr := peerConnection.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: uint32(track.SSRC())}}); rtcpSendErr != nil {
						fmt.Println(rtcpSendErr)
					}
				}
			}()

			localTrack, newTrackErr := webrtc.NewTrackLocalStaticRTP(track.Codec().RTPCodecCapability, "video", "pion")
			if newTrackErr != nil {
				panic(newTrackErr)
			}
			localTrackChan = localTrack

			rtpBuf := make([]byte, 1400)
			for {
				i, _, readErr := track.Read(rtpBuf)
				if readErr != nil {
					panic(readErr)
				}
				if _, err = localTrack.Write(rtpBuf[:i]); err != nil && !errors.Is(err, io.ErrClosedPipe) {
					panic(err)
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
	}else{

		peerConnection, err := webrtc.NewPeerConnection(webrtc.Configuration{})
		if err != nil {
			panic(err)
		}

		_, err = peerConnection.AddTrack(localTrackChan)
		if err != nil {
			panic(err)
		}
		//go func() {
		//	rtcpBuf := make([]byte, 1500)
		//	for {
		//		if _, _, rtcpErr := rtpSender.Read(rtcpBuf); rtcpErr != nil {
		//			return
		//		}
		//	}
		//}()

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


}
