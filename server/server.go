package main

import (
	"crypto/tls"
	"fmt"
	"github.com/coffeemakr/ohren"
	"github.com/coffeemakr/ohren/websocket"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

func main() {
	//var stopChan = make(chan os.Signal, 2)
	//signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	var wg sync.WaitGroup
	host := "0.0.0.0"

	recordChannel := make(chan ohren.Record)

	cert, err := tls.LoadX509KeyPair("server.crt", "server.key")
	if err != nil {
		log.Fatalln(err)
	}
	if cert.PrivateKey == nil {
		log.Fatalln("no private key loaded")
	}

	websocket := websocket.NewWebsocketHandler(recordChannel)
	go websocket.RunBroadcast()

	mux := http.NewServeMux()
	mux.Handle("/ws", websocket)
	mux.Handle("/", http.FileServer(http.Dir("./static")))

	adminServer := &http.Server{
		Handler: mux,
		Addr:    "127.0.0.1:45987",
	}
	go func() {
		err := adminServer.ListenAndServe()
		if err != nil {
			log.Fatalln(err)
		}
	}()

	var handlers []ohren.Listener
	var ipAddresses []net.IP
	interfaces, err := net.Interfaces()
	if err != nil {
		log.Fatalln(err)
	}
	for _, iface := range interfaces {
		addrs, err := iface.Addrs()
		if err != nil {
			log.Println(err)
			continue
		}
		for _, addr := range addrs {
			ip, _, err := net.ParseCIDR(addr.String())
			if err != nil {
				log.Println(err)
				continue
			}
			if ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				continue
			}
			if len(ip) == net.IPv6len && ip[0] == 0xfe && ip[1] == 0x80 {
				continue
			}
			ipAddresses = append(ipAddresses, ip)
		}
	}
	var ipv4Count = 0
	var ipv6Count = 0
	for i, addr := range ipAddresses {
		ipv4Addr := addr.To4()
		if ipv4Addr != nil {
			ipv4Count++
			ipAddresses[i] = ipv4Addr
		} else {
			ipv6Count++
		}
	}
	if ipv4Count == 0 && ipv6Count == 0 {
		log.Fatalln("no ip count be detected")
	}
	if ipv4Count > 1 {
		log.Println("Warning got mulitple IPv4 addresses: ", ipAddresses)
	} else if ipv6Count == 0 {
		log.Println("Warning got no IPv4 address")
	}
	if ipv6Count > 1 {
		log.Println("Warning got mulitple IPv6 addresses: ", ipAddresses)
	} else if ipv6Count == 0 {
		log.Println("Warning got no IPv6 address")
	}

	dnsResponser := &ohren.DnsResponder{
		Addresses: ipAddresses,
		TTL:       0,
	}

	httpResponder := ohren.DefaultHttpResponder.WithTlsConfig(&tls.Config{
		Certificates: []tls.Certificate{cert},
	})

	tcpResponders := map[int]ohren.Responder{
		80:   httpResponder,
		443:  httpResponder,
		8080: httpResponder,
		8443: httpResponder,
		53:   dnsResponser,
		5353: dnsResponser,
	}

	for port, responder := range tcpResponders {
		l, err := net.Listen("tcp", fmt.Sprintf("%s:%d", host, port))
		if err != nil {
			log.Printf("listener on port %d failed: %s\n", port, err)
		} else {
			log.Printf("listening on port %d\n", port)
			handlers = append(handlers, ohren.TcpListener{
				Listener:    l,
				Responder:   responder,
				Timeout:     1 * time.Second,
				WorkerCount: 5,
			})
		}
	}

	for _, handler := range handlers {
		wg.Add(1)
		handler := handler
		go func() {
			err := handler.Record(recordChannel)
			log.Printf("recording failed: %s on %s", err, handler)
		}()
	}

	log.Println("started.")
	wg.Wait()
	close(recordChannel)
	log.Println("done")
	time.Sleep(time.Second)
}
