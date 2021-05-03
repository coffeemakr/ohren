package main

import (
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"github.com/coffeemakr/ohren"
	"github.com/coffeemakr/ohren/websocket"
	"gopkg.in/yaml.v3"
	"log"
	"net"
	"net/http"
	"os"
	"path"
	"sync"
	"time"
)

type ResponderType string

const (
	ResponderTypeDns  ResponderType = "dns"
	ResponderTypeHttp ResponderType = "http"
)

type ResponderConfig struct {
	ListenPort int           `yaml:"port"`
	Type       ResponderType `yaml:"type"`
}

type WebsocketConfig struct {
	ListenPort int    `yaml:"port"`
	ListenHost string `yaml:"host"`
}

type ServerConfig struct {
	Hostname    string   `yaml:"hostname"`
	ListenHosts []string `yaml:"listen_hosts"`
	Dns         struct {
		PublicIPs []string `yaml:"public_ips"`
	} `yaml:"dns"`
	Http struct {
		Certificate string `yaml:"tls_certificate"`
		Key         string `json:"tls_key"`
	} `yaml:"http"`

	Responders []ResponderConfig `yaml:"responders"`

	Websocket WebsocketConfig `yaml:"websocket"`
}

var defaultConfig = &ServerConfig{
	ListenHosts: []string{
		"0.0.0.0",
	},
	Responders: []ResponderConfig{
		{
			ListenPort: 80,
			Type:       ResponderTypeHttp,
		},
		{
			ListenPort: 443,
			Type:       ResponderTypeHttp,
		},
		{
			ListenPort: 8080,
			Type:       ResponderTypeHttp,
		},
		{
			ListenPort: 53,
			Type:       ResponderTypeDns,
		},
	},
	Websocket: WebsocketConfig{
		ListenPort: 7853,
	},
}

func checkConfig(config *ServerConfig) (warnings []string, err error) {
	if config.Hostname == "" {
		err = errors.New("hostname is required")
		return
	}

	if len(config.Responders) == 0 {
		config.Responders = defaultConfig.Responders
	}

	if config.Websocket.ListenPort == 0 {
		config.Websocket.ListenPort = defaultConfig.Websocket.ListenPort
	}

	var hasDnsResponder bool
	var hasHttpResponder bool
	for _, responder := range config.Responders {
		switch responder.Type {
		case ResponderTypeDns:
			hasDnsResponder = true
		case ResponderTypeHttp:
			hasHttpResponder = true
		}
	}
	if hasHttpResponder {
		if config.Http.Key == "" || config.Http.Certificate == "" {
			warnings = append(warnings, "No tls certificates provided, unable to use TLS")
		}
	}
	if hasDnsResponder {
		if len(config.Dns.PublicIPs) == 0 {
			var publicIps []net.IP
			warnings = append(warnings, "No public_ips configured, detecting automatically")
			publicIps, err = getIpAddresses()
			if err != nil {
				err = fmt.Errorf("error getting ips: %s", err)
				return
			}
			for _, ip := range publicIps {
				config.Dns.PublicIPs = append(config.Dns.PublicIPs, ip.String())
			}
		}
	}
	for _, responder := range config.Responders {
		if responder.ListenPort == 0 {
			err = errors.New("responder has no port configured")
			return
		}
	}
	return
}

func main() {
	//var stopChan = make(chan os.Signal, 2)
	//signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	var err error
	var configFile string
	flag.StringVar(&configFile, "config", "", "the configuration file")
	flag.Parse()

	config, err := readConfigFile(configFile)
	warnings, err := checkConfig(config)
	if err != nil {
		log.Fatalln(err)
	}
	for _, warning := range warnings {
		log.Printf("Warning: %s\n", warning)
	}

	if len(config.ListenHosts) == 0 {
		config.ListenHosts = defaultConfig.ListenHosts
	}

	var wg sync.WaitGroup

	recordChannel := make(chan ohren.Record)

	ws := websocket.NewWebsocketHandler(recordChannel)
	go ws.RunBroadcast()

	mux := http.NewServeMux()
	mux.Handle("/ws", ws)
	mux.Handle("/", http.FileServer(http.Dir("./static")))

	adminServer := &http.Server{
		Handler: mux,
		Addr:    fmt.Sprintf("%s:%d", config.Websocket.ListenHost, config.Websocket.ListenPort),
	}
	go func() {
		err := adminServer.ListenAndServe()
		if err != nil {
			log.Fatalln(err)
		}
	}()

	var handlers []ohren.Listener

	dnsResponder := getDnsResponder(config)
	httpResponder := getHttpResponder(config)

	for _, host := range config.ListenHosts {
		for _, responder := range config.Responders {
			port := responder.ListenPort
			switch responder.Type {
			case ResponderTypeHttp:
				l, err := net.Listen("tcp", fmt.Sprintf("%s:%d", host, port))
				if err != nil {
					log.Fatalf("listener on port %d failed: %s\n\n", port, err)
				} else {
					log.Printf("listening on port %d\n", port)
					handlers = append(handlers, ohren.TcpListener{
						Listener:    l,
						Responder:   httpResponder,
						Timeout:     1 * time.Second,
						WorkerCount: 5,
					})
				}
			case ResponderTypeDns:
				hostIp := net.ParseIP(host)
				if hostIp == nil {
					log.Fatalf("not a valid listen ip: %s\n", host)
				}
				log.Printf("listening on port %d\n", port)
				handlers = append(handlers, ohren.UdpListener{
					Addr: &net.UDPAddr{
						IP:   net.IPv4(127, 0, 0, 1),
						Port: port,
					},
					Responder:   dnsResponder,
					Timeout:     1 * time.Second,
					WorkerCount: 5,
				})
			}

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

func readConfigFile(file string) (*ServerConfig, error) {
	var err error
	var configFp *os.File
	if file == "" {
		userConfigDir, err := os.UserConfigDir()
		userConfigFilePath := ""
		if err != nil {
			userConfigFilePath = path.Join(userConfigDir, "ohren", "ohren.yml")
		}
		filePaths := []string{
			"ohren.yml",
			userConfigFilePath,
			"/etc/ohren/ohren.yml",
		}
		for _, filePath := range filePaths {
			configFp, err = os.Open(filePath)
			if err == nil {
				break
			}
		}
		if err != nil {
			return nil, errors.New("could not find config file")
		}
	} else {
		configFp, err = os.Open(file)
		if err != nil {
			return nil, err
		}
	}

	var config ServerConfig
	decoder := yaml.NewDecoder(configFp)
	err = decoder.Decode(&config)
	if err != nil {
		return nil, err
	}
	err = configFp.Close()
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func getHttpResponder(config *ServerConfig) *ohren.MultiHttpResponder {
	httpResponder := ohren.DefaultHttpResponder
	if config.Http.Key != "" && config.Http.Certificate != "" {
		cert, err := tls.LoadX509KeyPair(config.Http.Certificate, config.Http.Key)
		if err != nil {
			log.Fatalln(err)
		}
		if cert.PrivateKey == nil {
			log.Fatalln("no private key loaded")
		}
		httpResponder = httpResponder.WithTlsConfig(&tls.Config{
			Certificates: []tls.Certificate{cert},
		})
	}
	return httpResponder
}

func getDnsResponder(config *ServerConfig) *ohren.DnsResponder {
	publicIps := make([]net.IP, len(config.Dns.PublicIPs))
	for i, rawIp := range config.Dns.PublicIPs {
		ip := net.ParseIP(rawIp)
		if ip == nil {
			log.Fatalf("invalid ip in public_ips: %s\n", rawIp)
		}
		publicIps[i] = ip
	}
	return &ohren.DnsResponder{
		Addresses: publicIps,
		TTL:       0,
	}
}

func getIpAddresses() ([]net.IP, error) {
	var ipAddresses []net.IP
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
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

			ipv4 := ip.To4()
			if ipv4 != nil {
				ip = ipv4
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
		return nil, errors.New("no ip count be detected")
	}
	if ipv4Count > 1 {
		return nil, fmt.Errorf("got mulitple IPv4 addresses: %d", ipv4Count)
	} else if ipv4Count == 0 {
		log.Println("Warning got no IPv4 address")
	}
	if ipv6Count > 1 {
		return nil, fmt.Errorf("got mulitple IPv6 addresses: %d", ipv6Count)
	} else if ipv6Count == 0 {
		log.Println("Warning got no IPv6 address")
	}
	if len(ipAddresses) == 0 {
		return nil, errors.New("no ip addresses found")
	}
	return ipAddresses, nil
}
