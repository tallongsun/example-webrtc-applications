package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
	"net"
	"net/http"
	"time"
)

//ffprobe -i rtp-forwarder.sdp -protocol_whitelist file,udp,rtp
//ffplay -i rtp-forwarder.sdp -protocol_whitelist file,udp,rtp -fflags nobuffer
//ffmpeg -protocol_whitelist file,udp,rtp -i rtp-forwarder.sdp -c:v libx264 -preset veryfast -b:v 3000k -maxrate 3000k -bufsize 6000k -pix_fmt yuv420p -g 50 -c:a aac -b:a 160k -ac 2 -ar 44100 -f flv rtmp://live.twitch.tv/app/live_679837561_IZ00s439FMb9En2TSdYrzJA6b71Yuz
//https://www.twitch.tv/tallongsun1
func main() {

	http.Handle("/", http.FileServer(http.Dir(".")))
	http.HandleFunc("/signal", signaling)
	http.ListenAndServe(":8080", nil)
}

type udpConn struct {
	conn        *net.UDPConn
	port        int
	payloadType uint8
}

func signaling(w http.ResponseWriter, r *http.Request) {
	peerConnection, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		panic(err)
	}

	udpConns := map[string]*udpConn{
		"audio": {port: 4000, payloadType: 111},
		"video": {port: 4002, payloadType: 96},
	}
	go func() {
		ctx, _ := context.WithCancel(context.Background())
		var laddr *net.UDPAddr
		if laddr, err = net.ResolveUDPAddr("udp", "127.0.0.1:"); err != nil {
			panic(err)
		}
		// Prepare udp conns
		// Also update incoming packets with expected PayloadType, the browser may use
		// a different value. We have to modify so our stream matches what rtp-forwarder.sdp expects
		for _, c := range udpConns {
			// Create remote addr
			var raddr *net.UDPAddr
			if raddr, err = net.ResolveUDPAddr("udp", fmt.Sprintf("127.0.0.1:%d", c.port)); err != nil {
				panic(err)
			}

			// Dial udp
			if c.conn, err = net.DialUDP("udp", laddr, raddr); err != nil {
				panic(err)
			}
			defer func(conn net.PacketConn) {
				if closeErr := conn.Close(); closeErr != nil {
					panic(closeErr)
				}
			}(c.conn)
		}
		<-ctx.Done()
	}()


	peerConnection.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		fmt.Println("on track.",track.ID(),track.StreamID(),track.Kind(),track.PayloadType(),track.Codec().MimeType)
		// Retrieve udp connection
		c, ok := udpConns[track.Kind().String()]
		if !ok {
			return
		}
		// Send a PLI on an interval so that the publisher is pushing a keyframe every rtcpPLIInterval
		// This is a temporary fix until we implement incoming RTCP events, then we would push a PLI only when a viewer requests it
		go func() {
			ticker := time.NewTicker(time.Second * 3)
			for range ticker.C {
				errSend := peerConnection.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: uint32(track.SSRC())}})
				if errSend != nil {
					fmt.Println(errSend)
				}
			}
		}()

		b := make([]byte, 1500)
		rtpPacket := &rtp.Packet{}
		for {
			// Read
			n, _, readErr := track.Read(b)
			if readErr != nil {
				panic(readErr)
			}

			// Unmarshal the packet and update the PayloadType
			if err = rtpPacket.Unmarshal(b[:n]); err != nil {
				panic(err)
			}
			rtpPacket.PayloadType = c.payloadType

			// Marshal into original buffer with updated PayloadType
			if n, err = rtpPacket.MarshalTo(b); err != nil {
				panic(err)
			}

			// Write
			if _, err = c.conn.Write(b[:n]); err != nil {
				// For this particular example, third party applications usually timeout after a short
				// amount of time during which the user doesn't have enough time to provide the answer
				// to the browser.
				// That's why, for this particular example, the user first needs to provide the answer
				// to the browser then open the third party application. Therefore we must not kill
				// the forward on "connection refused" errors
				if opError, ok := err.(*net.OpError); ok && opError.Err.Error() == "write: connection refused" {
					continue
				}
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


}
