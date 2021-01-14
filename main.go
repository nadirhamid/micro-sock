package main

import (
	"bufio"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
	"encoding/json"

	log "github.com/Sirupsen/logrus"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	connectionFailedCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "num_failed_connections",
		Help: "Number of failed connections.",
	})
	dataRecievedCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "num_msgs_recieved",
		Help: "Number of messages recieved.",
	})
	dataSentCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "num_msgs_sent",
		Help: "Number of messages sent.",
	})
)

type config struct {
	timeout       time.Duration
	retryInterval time.Duration
	sendInterval  time.Duration
}
type JSONStruct struct {
	Value1 int `json:"value1"`
	Value2 int `json:"value2"`
	Value3 int `json:"value3"`
}

func main() {
	log.Infof("Starting server..")
	log.SetOutput(os.Stderr)
	prometheus.MustRegister(connectionFailedCounter)
	prometheus.MustRegister(dataRecievedCounter)
	prometheus.MustRegister(dataSentCounter)

	var (
		listen bool
		laddr  string
		c      config
	)

	flag.BoolVar(&listen, "l", false, "listen")
	flag.StringVar(&laddr, "a", "", "listening address")
	flag.DurationVar(&c.timeout, "t", 5*time.Second, "connect/send/recv timeout")
	flag.DurationVar(&c.retryInterval, "r", 1*time.Second, "connection retry interval")
	flag.DurationVar(&c.sendInterval, "i", 2*time.Second, "message sending interval")
	flag.Parse()
	log.Infof("will try to listen on %s", laddr)

	if listen {
		laddr = normalizeAddr(laddr)
		http.HandleFunc("/test", test)
		http.Handle("/metrics", promhttp.Handler())
		log.Fatal(http.ListenAndServe(laddr, nil))
	}
}

func test(rw http.ResponseWriter, req *http.Request) {
    //log.Infof("Received JSON body: %s", req.Body)
    decoder := json.NewDecoder(req.Body)
    var t JSONStruct
    err := decoder.Decode(&t)
    if err != nil {
        panic(err)
	return
    }
    log.Printf("Value1 = %d, Value2 = %d, Value3 = %d", t.Value1, t.Value2, t.Value3)
}

func normalizeAddr(addr string) string {
	parts := strings.Split(addr, ":")
	if len(parts) == 1 {
		return addr + ":8080"
	}
	return addr
}

func accept(ln net.Listener, c config) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Errorf("accept failed: %s", err)
			connectionFailedCounter.Inc()
			continue
		}
		go recv(conn, c)
	}
}

func recv(conn net.Conn, c config) {
	log.Infof("accepted connection from %s", remoteAddr(conn))
	r := bufio.NewReader(conn)
	for {
		conn.SetDeadline(time.Now().Add(c.timeout))
		s, err := r.ReadString('\n')
		parts := strings.Split(s, "\n")
		msg := parts[0]
		if err != nil {
			log.Errorln(err)
			break
		}
		if _, err := conn.Write([]byte("\n")); err != nil {
			log.Errorln(err)
			break
		}
		fmt.Printf("%s <-- %s\n", msg, remoteAddr(conn))
		dataRecievedCounter.Inc()
	}
	conn.Close()
}

func send(addr string, c config) {
	i := 1
	for {
		conn, err := net.DialTimeout("tcp", addr, c.timeout)
		if err != nil {
			log.Errorln(err)
			time.Sleep(c.retryInterval)
			continue
		}
		log.Infof("connected to %s", remoteAddr(conn))
		r := bufio.NewReader(conn)
		for {
			conn.SetDeadline(time.Now().Add(c.timeout))
			msg := fmt.Sprintf("%d", i)
			if _, err := conn.Write([]byte(msg + "\n")); err != nil {
				log.Errorln(err)
				break
			}
			_, err := r.ReadString('\n')
			if err != nil {
				log.Errorln(err)
				break
			}
			fmt.Printf("%s --> %s\n", msg, remoteAddr(conn))
			i++
			time.Sleep(c.sendInterval)
			dataSentCounter.Inc()
		}
		conn.Close()
	}
}

func remoteAddr(conn net.Conn) string {
	ip := conn.RemoteAddr().(*net.TCPAddr).IP.String()
	names, err := net.LookupAddr(ip)
	if err == nil && len(names) > 0 {
		parts := strings.Split(names[0], ".")
		return parts[0] + " (" + ip + ")"
	}
	return ip
}
