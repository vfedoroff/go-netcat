// go-nc project main.go
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strconv"
)

// Handles TC connection and perform synchorinization:
// TCP -> Stdout and Stdin -> TCP
func tcp_con_handle(con net.Conn) {
	chan_to_stdout := stream_copy(con, os.Stdout)
	chan_to_remote := stream_copy(os.Stdin, con)
	select {
	case <-chan_to_stdout:
		log.Println("Remote connection is closed")
	case <-chan_to_remote:
		log.Println("Local program is terminated")
	}
}

// Performs copy operation between streams: os and tcp streams
func stream_copy(src io.Reader, dst io.Writer) <-chan int {
	buf := make([]byte, 1024)
	sync_channel := make(chan int)
	go func() {
		defer func() {
			if con, ok := dst.(net.Conn); ok {
				con.Close()
				log.Printf("Connection from %v is closed\n", con.RemoteAddr())
			}
			sync_channel <- 0 // Notify that processing is finished
		}()
		for {
			var nBytes int
			var err error
			nBytes, err = src.Read(buf)
			if err != nil {
				if err != io.EOF {
					log.Printf("Read error: %s\n", err)
				}
				break
			}
			_, err = dst.Write(buf[0:nBytes])
			if err != nil {
				log.Fatalf("Write error: %s\n", err)
			}
		}
	}()
	return sync_channel
}

//Accept data from UPD connection and copy it to the stream
func accept_from_udp_to_stream(src net.Conn, dst io.Writer) <-chan net.Addr {
	buf := make([]byte, 1024)
	sync_channel := make(chan net.Addr)
	con, ok := src.(*net.UDPConn)
	if !ok {
		log.Printf("Input must be UDP connection")
		return sync_channel
	}
	go func() {
		var remoteAddr net.Addr
		for {
			var nBytes int
			var err error
			var addr net.Addr
			nBytes, addr, err = con.ReadFromUDP(buf)
			if err != nil {
				if err != io.EOF {
					log.Printf("Read error: %s\n", err)
				}
				break
			}
			if remoteAddr == nil && remoteAddr != addr {
				remoteAddr = addr
				sync_channel <- remoteAddr
			}
			_, err = dst.Write(buf[0:nBytes])
			if err != nil {
				log.Fatalf("Write error: %s\n", err)
			}
		}
	}()
	log.Println("Exit write_from_udp_to_stream")
	return sync_channel
}

// Put input date from the stream to UDP connection
func put_from_stream_to_udp(src io.Reader, dst net.Conn, remoteAddr net.Addr) <-chan net.Addr {
	buf := make([]byte, 1024)
	sync_channel := make(chan net.Addr)
	go func() {
		for {
			var nBytes int
			var err error
			nBytes, err = src.Read(buf)
			if err != nil {
				if err != io.EOF {
					log.Printf("Read error: %s\n", err)
				}
				break
			}
			log.Println("Write to the remote address:", remoteAddr)
			if con, ok := dst.(*net.UDPConn); ok && remoteAddr != nil {
				_, err = con.WriteTo(buf[0:nBytes], remoteAddr)
			}
			if err != nil {
				log.Fatalf("Write error: %s\n", err)
			}
		}
	}()
	return sync_channel
}

// Handle UDP connection
func udp_con_handle(con net.Conn) {
	in_channel := accept_from_udp_to_stream(con, os.Stdout)
	log.Println("Waiting for remote connection")
	remoteAddr := <-in_channel
	log.Println("Connected from", remoteAddr)
	out_channel := put_from_stream_to_udp(os.Stdin, con, remoteAddr)
	select {
	case <-in_channel:
		log.Println("Remote connection is closed")
	case <-out_channel:
		log.Println("Local program is terminated")
	}
}

func main() {
	var sourcePort string
	var destinationPort string
	var isUdp bool
	var isListen bool
	var host string
	flag.StringVar(&sourcePort, "p", "", "Specifies the source port netcat should use, subject to privilege restrictions and availability.")
	flag.BoolVar(&isUdp, "u", false, "Use UDP instead of the default option of TCP.")
	flag.BoolVar(&isListen, "l", false, "Used to specify that netcat should listen for an incoming connection rather than initiate a connection to a remote host.")
	flag.Parse()
	if flag.NFlag() == 0 && flag.NArg() == 0 {
		fmt.Println("go-nc [-lu] [-p source port ] [-s source ip address ] [hostname ] [port[s]]")
		flag.Usage()
		os.Exit(1)
	}
	log.Println("Source port:", sourcePort)
	if flag.Lookup("u") != nil {
		log.Println("Protocol:", "udp")
		isUdp = true
	} else {
		log.Println("Protocol:", "tcp")
	}
	if sourcePort != "" {
		if _, err := strconv.Atoi(sourcePort); err != nil {
			log.Println("Source port shall be not empty and have integer value")
			os.Exit(1)
		}
	}

	if !isListen {
		if flag.NArg() < 2 {
			log.Println("[hostname ] [port] are mandatory arguments")
			os.Exit(1)
		}

		if _, err := strconv.Atoi(flag.Arg(1)); err != nil {
			log.Println("Destination port shall be not empty and have integer value")
			os.Exit(1)
		}
		host = flag.Arg(0)
		destinationPort = fmt.Sprintf(":%v", flag.Arg(1))

	} else {
		if flag.NArg() < 1 {
			fmt.Println("when you use -l option [port] is mandatory argument")
			os.Exit(1)
		}
		if _, err := strconv.Atoi(flag.Arg(0)); err != nil {
			log.Println("Destination port shall be not empty and have integer value")
			os.Exit(1)
		}
		destinationPort = fmt.Sprintf(":%v", flag.Arg(0))
	}

	log.Println("Hostname:", host)
	log.Println("Port:", destinationPort)
	if !isUdp {
		log.Println("Work with TCP protocol")
		if isListen {
			listener, err := net.Listen("tcp", destinationPort)
			if err != nil {
				log.Fatalln(err)
			}
			log.Println("Listening on", destinationPort)
			con, err := listener.Accept()
			if err != nil {
				log.Fatalln(err)
			}
			log.Println("Connect from", con.RemoteAddr())
			tcp_con_handle(con)

		} else if host != "" {
			con, err := net.Dial("tcp", host+destinationPort)
			if err != nil {
				log.Fatalln(err)
			}
			log.Println("Connected to", host+destinationPort)
			tcp_con_handle(con)
		} else {
			flag.Usage()
		}
	} else {
		log.Println("Work with UDP protocol")
		if isListen {
			addr, err := net.ResolveUDPAddr("udp", destinationPort)
			if err != nil {
				log.Fatalln(err)
			}
			con, err := net.ListenUDP("udp", addr)
			if err != nil {
				log.Fatalln(err)
			}
			log.Println("Has been resolved UDP address:", addr)
			log.Println("Listening on", destinationPort)
			udp_con_handle(con)
		} else if host != "" {
			addr, err := net.ResolveUDPAddr("udp", host+destinationPort)
			if err != nil {
				log.Fatalln(err)
			}
			log.Println("Has been resolved UDP address:", addr)
			con, err := net.DialUDP("udp", nil, addr)
			if err != nil {
				log.Fatalln(err)
			}
			udp_con_handle(con)
		}
	}
}
