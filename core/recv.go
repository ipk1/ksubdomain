package core

import (
	"bufio"
	"fmt"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"log"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

func Recv(device string, options *Options, flagID uint16) {
	var (
		snapshotLen int32         = 1024
		promiscuous bool          = false
		timeout     time.Duration = -1 * time.Second
	)
	handle, _ := pcap.OpenLive(device, snapshotLen, promiscuous, timeout)
	err := handle.SetBPFFilter("udp and port 53")
	if err != nil {
		log.Fatal(err)
	}
	packetSource := gopacket.NewPacketSource(handle, handle.LinkType())
	defer handle.Close()

	var udp layers.UDP
	var dns layers.DNS
	var eth layers.Ethernet
	var ipv4 layers.IPv4

	parser := gopacket.NewDecodingLayerParser(
		layers.LayerTypeEthernet, &eth, &ipv4, &udp, &dns)
	var isWrite bool = false
	var isttl bool = options.TTL
	var issilent bool = options.Silent
	if options.Output != "" {
		isWrite = true
	}
	var foutput *os.File
	if isWrite {
		foutput, err = os.OpenFile(options.Output, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0664)
		if err != nil {
			log.Panicln(err)
		}
	}
	for {
		packet, err := packetSource.NextPacket()
		if err != nil {
			continue
		}
		var decoded []gopacket.LayerType
		err = parser.DecodeLayers(packet.Data(), &decoded)
		if err != nil {
			continue
		}
		if !dns.QR {
			continue
		}
		if dns.ID/100 == flagID {
			atomic.AddUint64(&RecvIndex, 1)
			udp, _ := packet.Layer(layers.LayerTypeUDP).(*layers.UDP)
			index := GenerateMapIndex(dns.ID%100, uint16(udp.DstPort))
			if _data, ok := LocalStauts.Load(uint32(index)); ok {
				data := _data.(StatusTable)
				dnsName := data.Dns
				if dnsnum, ok2 := DnsChoice.Load(dnsName); !ok2 {
					DnsChoice.Store(dnsName, 1)
				} else {
					DnsChoice.Store(dnsName, dnsnum.(int)+1)
				}
				if LocalStack.Len() <= 50000 {
					LocalStack.Push(uint32(index))
				}
				LocalStauts.Delete(uint32(index))
			}
			if dns.ANCount > 0 {
				atomic.AddUint64(&SuccessIndex, 1)
				msg := ""
				for _, v := range dns.Questions {
					msg += string(v.Name) + " => "
				}
				for _, v := range dns.Answers {
					msg += v.String()
					if isttl {
						msg += " ttl:" + strconv.Itoa(int(v.TTL))
					}
					msg += " => "
				}
				msg = strings.Trim(msg, " => ")
				if !issilent {
					fmt.Println("\r" + msg)
				}
				if isWrite {
					w := bufio.NewWriter(foutput)
					_, err = w.WriteString(msg + "\n")
					if err != nil {
						fmt.Println(err)
					}
					w.Flush()
				}
			}
		}
	}
}
