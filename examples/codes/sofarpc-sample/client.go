package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"strconv"
	"sync/atomic"
	"time"

	"mosn.io/mosn/pkg/log"
	"mosn.io/mosn/pkg/network"
	"mosn.io/mosn/pkg/protocol"
	"mosn.io/mosn/pkg/protocol/rpc/sofarpc"
	_ "mosn.io/mosn/pkg/protocol/rpc/sofarpc/codec"
	"mosn.io/mosn/pkg/protocol/serialize"
	"mosn.io/mosn/pkg/stream"
	_ "mosn.io/mosn/pkg/stream/sofarpc"
	"mosn.io/mosn/pkg/types"
	"mosn.io/pkg/buffer"
)

type Client struct {
	Client stream.Client
	conn   types.ClientConnection
	Id     uint64
}

func NewClient(addr string) *Client {
	c := &Client{}
	stopChan := make(chan struct{})
	remoteAddr, _ := net.ResolveTCPAddr("tcp", addr)
	//网络连接+过滤器链
	conn := network.NewClientConnection(nil, 0, nil, remoteAddr, stopChan)
	//连接
	if err := conn.Connect(); err != nil {
		fmt.Println(err)
		return nil
	}
	c.Client = stream.NewStreamClient(context.Background(), protocol.SofaRPC, conn, nil)
	c.conn = conn
	return c
}

func (c *Client) OnReceive(ctx context.Context, headers types.HeaderMap, data types.IoBuffer, trailers types.HeaderMap) {
	//fmt.Printf("[RPC Client] Receive Data:")
	//if cmd, ok := headers.(sofarpc.SofaRpcCmd); ok {
	//	streamID := protocol.StreamIDConv(cmd.RequestID())
	//
	//	if resp, ok := cmd.(rpc.RespStatus); ok {
	//		fmt.Println("stream:", streamID, " status:", resp.RespStatus())
	//	}
	//}
}

func (c *Client) OnDecodeError(context context.Context, err error, headers types.HeaderMap) {}

func (c *Client) Request() {
	c.Id++
	requestEncoder := c.Client.NewStream(context.Background(), c)
	headers := buildBoltV1Request(c.Id)
	requestEncoder.AppendHeaders(context.Background(), headers, true)
}

func buildBoltV1Request(requestID uint64) *sofarpc.BoltRequest {
	request := &sofarpc.BoltRequest{
		Protocol: sofarpc.PROTOCOL_CODE_V1,
		CmdType:  sofarpc.REQUEST,
		CmdCode:  sofarpc.RPC_REQUEST,
		Version:  1,
		ReqID:    uint32(requestID),
		Codec:    sofarpc.HESSIAN2_SERIALIZE, //todo: read default codec from config
		Timeout:  -1,
	}

	headers := map[string]string{"service": "testSofa"} // used for sofa routing

	buf := buffer.NewIoBuffer(100)
	if err := serialize.Instance.SerializeMap(headers, buf); err != nil {
		panic("serialize headers error")
	} else {
		request.HeaderMap = buf.Bytes()
		request.HeaderLen = int16(buf.Len())
	}

	return request
}

func main() {
	log.InitDefaultLogger("", log.DEBUG)
	//t := flag.Bool("t", true, "-t")
	flag.Parse()
	var sumReq, sumCost int64

	d := time.Duration(time.Second)
	t := time.NewTicker(d)
	go func() {
		for  {
			<- t.C
			if sumCost > 0 {
				fmt.Println("req:" + strconv.FormatInt(sumReq, 10) + ", time:" + strconv.FormatInt(sumCost, 10) + ", avg:" + strconv.FormatInt((sumCost / sumReq), 10))
			}
			//atomic.SwapInt64(&sumReq, 0)
			//atomic.SwapInt64(&sumCost, 0)
		}
	}()
	//for i:=0;i<10;i++ {
	//	go func() {
	//		if client := NewClient("127.0.0.1:2045"); client != nil {
	//			for  {
	//				start := time.Now().Nanosecond()
	//				client.Request()
	//				atomic.AddInt64(&sumReq, 1)
	//				atomic.AddInt64(&sumCost, int64(time.Now().Nanosecond() - start))
	//				time.Sleep(100 * time.Millisecond)
	//				//if !*t {
	//				//	time.Sleep(3 * time.Second)
	//				//	return
	//				//}
	//			}
	//		}
	//	}()
	//}
	if client := NewClient("127.0.0.1:2045"); client != nil {
		for  {
			start := time.Now().Nanosecond()
			client.Request()
			atomic.AddInt64(&sumReq, 1)
			atomic.AddInt64(&sumCost, int64(time.Now().Nanosecond() - start))
			time.Sleep(1000 * time.Millisecond)
			//if !*t {
			//	time.Sleep(3 * time.Second)
			//	return
			//}
		}
	}
	time.Sleep(time.Duration(time.Hour))

}
