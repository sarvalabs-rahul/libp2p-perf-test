package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"sync"
	"sync/atomic"
	"time"

	manet "github.com/multiformats/go-multiaddr/net"

	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"

	"github.com/libp2p/go-libp2p"
	quic "github.com/libp2p/go-libp2p/p2p/transport/quic"
	"github.com/libp2p/go-libp2p/p2p/transport/tcp"

	"github.com/c2h5oh/datasize"
	ma "github.com/multiformats/go-multiaddr"
)

const TestProtocol = protocol.ID("/libp2p/test/data")

func main() {
	go func() {
		log.Println(http.ListenAndServe("0.0.0.0:6060", nil))
	}()

	streams := flag.Int("streams", 1, "number of parallel download streams")
	size := flag.String("size", "1 GB", "file size to download")
	flag.Parse()
	total := datasize.MustParseString(*size).Bytes()

	if len(flag.Args()) != 1 {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] peer\n", os.Args[0])
		flag.PrintDefaults()
		os.Exit(1)
	}

	a, err := ma.NewMultiaddr(flag.Args()[0])
	if err != nil {
		log.Fatal(err)
	}

	if _, err := a.ValueForProtocol(ma.P_P2P); err != nil {
		// raw TCP address
		downloadRawTCP(a, total)
		return
	}

	pi, err := peer.AddrInfoFromP2pAddr(a)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(pi)

	host, err := libp2p.New(
		libp2p.NoListenAddrs,
		libp2p.Transport(tcp.NewTCPTransport),
		libp2p.Transport(quic.NewTransport),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer host.Close()

	log.Printf("Connecting to %s", pi.ID.Pretty())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := host.Connect(ctx, *pi); err != nil {
		log.Fatal(err)
	}

	log.Printf("Connected; requesting data...")

	dataStreams := make([]network.Stream, 0, *streams)
	for i := 0; i < *streams; i++ {
		s, err := host.NewStream(ctx, pi.ID, TestProtocol)
		if err != nil {
			log.Fatal(err)
			return
		}
		dataStreams = append(dataStreams, s)
	}

	log.Printf("Transferring data in %d parallel streams", *streams)

	var wg sync.WaitGroup
	var end time.Time

	start := time.Now()
	var count atomic.Uint64
	var downloaded uint64 // the total amount of data downloaded once we've crossed the threshold
	var once sync.Once

	for i := 0; i < *streams; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			b := make([]byte, 1<<11)

			for {
				n, err := dataStreams[i].Read(b)
				if err != nil {
					return
				}
				t := count.Add(uint64(n))

				// We've downloaded enough data.
				// Record the result and reset all streams.
				if t >= total {
					once.Do(func() {
						end = time.Now()
						downloaded = count.Load()
						for _, str := range dataStreams {
							str.Reset()
						}
					})
				}
			}
		}(i)
	}
	wg.Wait()

	if downloaded < total {
		log.Fatal("Failed to download all the data.")
	}

	took := end.Sub(start)
	bandwidth := float64(downloaded) / took.Seconds()
	log.Printf("Received %d bytes in %s (%s/s)", downloaded, took, datasize.ByteSize(bandwidth).HumanReadable())
}

func downloadRawTCP(addr ma.Multiaddr, total uint64) {
	netw, host, err := manet.DialArgs(addr)
	if err != nil {
		log.Fatal(err)
	}
	raddr, err := net.ResolveTCPAddr(netw, host)
	if err != nil {
		log.Fatal(err)
	}
	conn, err := net.DialTCP(netw, nil, raddr)
	if err != nil {
		log.Fatal(err)
	}
	b := make([]byte, 1<<11)
	var count uint64
	start := time.Now()
	for {
		n, err := conn.Read(b)
		if err != nil {
			log.Fatal(err)
		}
		count += uint64(n)
		if count > total {
			break
		}
	}
	end := time.Now()

	took := end.Sub(start)
	bandwidth := float64(count) / took.Seconds()
	log.Printf("Received %d bytes in %s (%s/s)", count, took, datasize.ByteSize(bandwidth).HumanReadable())
}
