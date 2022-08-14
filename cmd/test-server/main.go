package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/protocol"

	"github.com/libp2p/go-libp2p"
	quic "github.com/libp2p/go-libp2p/p2p/transport/quic"
	"github.com/libp2p/go-libp2p/p2p/transport/tcp"

	_ "net/http/pprof"
)

const TestProtocol = protocol.ID("/libp2p/test/data")

var testFilePath string

func main() {
	go func() {
		log.Println(http.ListenAndServe("0.0.0.0:6060", nil))
	}()

	port := flag.Int("port", 4001, "server listen port")
	testFile := flag.String("file", "data", "data file to serve")

	flag.Parse()

	if _, err := os.Stat(*testFile); err != nil {
		log.Fatal(err)
	}
	testFilePath = *testFile

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
		libp2p.Identity(privKey),
	)
	if err != nil {
		log.Fatal(err)
	}

	for _, addr := range host.Addrs() {
		fmt.Printf("I am %s/p2p/%s\n", addr, host.ID())
	}

	host.SetStreamHandler(TestProtocol, handleStream)

	select {}
}

func handleStream(s network.Stream) {
	defer s.Close()

	log.Printf("Incoming connection from %s", s.Conn().RemoteMultiaddr())

	file, err := os.Open(testFilePath)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	start := time.Now().UnixNano()
	n, err := io.Copy(s, file)
	if err != nil {
		log.Printf("Error transmiting file: %s", err)
	}
	end := time.Now().UnixNano()
	log.Printf("Transmitted %d bytes in %s", n, time.Duration(end-start))
}
