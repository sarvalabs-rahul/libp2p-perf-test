package main

import (
	"bytes"
	"crypto/rand"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/p2p/muxer/mplex"
	"github.com/libp2p/go-libp2p/p2p/muxer/yamux"
	quic "github.com/libp2p/go-libp2p/p2p/transport/quic"
	"github.com/libp2p/go-libp2p/p2p/transport/tcp"

	_ "net/http/pprof"
)

const TestProtocol = protocol.ID("/libp2p/test/data")

var randomData []byte

func init() {
	randomData = make([]byte, 1<<20)
	rand.Read(randomData)
}

func main() {
	go func() {
		log.Println(http.ListenAndServe("0.0.0.0:6060", nil))
	}()

	port := flag.Int("port", 4001, "server listen port (raw TCP prot will be +1)")
	flag.Parse()

	privKey, _, err := crypto.GenerateECDSAKeyPair(bytes.NewReader(bytes.Repeat([]byte{1}, 100)))
	if err != nil {
		log.Fatal(err)
	}
	host, err := libp2p.New(
		libp2p.ListenAddrStrings(
			fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", *port),
			fmt.Sprintf("/ip4/0.0.0.0/udp/%d/quic", *port),
		),
		libp2p.Transport(tcp.NewTCPTransport),
		libp2p.Transport(quic.NewTransport),
		libp2p.Muxer("/mplex/6.7.0", mplex.DefaultTransport),
		libp2p.Muxer("/yamux/1.0.0", yamux.DefaultTransport),
		libp2p.Identity(privKey),
		libp2p.ResourceManager(network.NullResourceManager),
	)
	if err != nil {
		log.Fatal(err)
	}
	host.SetStreamHandler(TestProtocol, handleStream)

	ln, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4zero, Port: *port + 1})
	if err != nil {
		log.Fatal(err)
	}
	defer ln.Close()
	go listenTCP(ln)

	for _, addr := range host.Addrs() {
		fmt.Printf("I am %s/p2p/%s\n", addr, host.ID())
	}
	fmt.Printf("Raw TCP on port %d\n", ln.Addr().(*net.TCPAddr).Port)

	select {}
}

func handleStream(s network.Stream) {
	defer s.Close()

	log.Printf("Incoming connection from %s", s.Conn().RemoteMultiaddr())
	for {
		if _, err := s.Write(randomData); err != nil {
			return
		}
	}
}

func listenTCP(ln *net.TCPListener) {
	for {
		conn, err := ln.AcceptTCP()
		if err != nil {
			return
		}
		log.Printf("Incoming connection from %s", conn.RemoteAddr())
		go func() {
			for {
				if _, err := conn.Write(randomData); err != nil {
					return
				}
			}
		}()
	}
}
