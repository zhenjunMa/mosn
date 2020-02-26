/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package sofarpc

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"mosn.io/api"
	mosnctx "mosn.io/mosn/pkg/context"
	"mosn.io/mosn/pkg/log"
	"mosn.io/mosn/pkg/network"
	"mosn.io/mosn/pkg/protocol"
	str "mosn.io/mosn/pkg/stream"
	"mosn.io/mosn/pkg/types"
	"mosn.io/pkg/utils"
)

const (
	Init = iota
	Connecting
	Connected
)

func init() {
	network.RegisterNewPoolFactory(protocol.SofaRPC, NewConnPool)
	types.RegisterConnPoolFactory(protocol.SofaRPC, true)
}

var defaultSubProtocol byte = 0x00

// types.ConnectionPool
// activeClient used as connected client
// host is the upstream
type connPool struct {
	activeClients sync.Map //sub protocol -> activeClient
	host          types.Host

	mux sync.Mutex
}

// NewConnPool
func NewConnPool(host types.Host) types.ConnectionPool {
	p := &connPool{
		host: host,
	}
	return p
}

func (p *connPool) SupportTLS() bool {
	return p.host.SupportTLS()
}

func (p *connPool) init(client *activeClient, sub byte) {
	utils.GoWithRecover(func() {
		if log.DefaultLogger.GetLogLevel() >= log.DEBUG {
			log.DefaultLogger.Debugf("[stream] [sofarpc] [connpool] init host %s", p.host.AddressString())
		}

		p.mux.Lock()
		defer p.mux.Unlock()
		client := newActiveClient(context.Background(), sub, p)
		if client != nil {
			client.state = Connected
			p.activeClients.Store(sub, client)
		} else {
			p.activeClients.Delete(sub)
		}
	}, nil)
}

func (p *connPool) CheckAndInit(ctx context.Context) bool {
	var client *activeClient

	subProtocol := getSubProtocol(ctx)

	v, ok := p.activeClients.Load(subProtocol)
	if !ok {
		fakeclient := &activeClient{}
		fakeclient.state = Init
		v, _ := p.activeClients.LoadOrStore(subProtocol, fakeclient)
		client = v.(*activeClient)
	} else {
		client = v.(*activeClient)
	}

	if atomic.LoadUint32(&client.state) == Connected {
		return true
	}

	if atomic.CompareAndSwapUint32(&client.state, Init, Connecting) {
		p.init(client, subProtocol)
	}

	return false
}

func (p *connPool) Protocol() types.Protocol {
	return protocol.SofaRPC
}

func (p *connPool) NewStream(ctx context.Context, responseDecoder types.StreamReceiveListener, listener types.PoolEventListener) {
	subProtocol := getSubProtocol(ctx)

	client, _ := p.activeClients.Load(subProtocol)

	if client == nil {
		listener.OnFailure(types.ConnectionFailure, p.host)
		return
	}

	activeClient := client.(*activeClient)
	if atomic.LoadUint32(&activeClient.state) != Connected {
		listener.OnFailure(types.ConnectionFailure, p.host)
		return
	}

	if !p.host.ClusterInfo().ResourceManager().Requests().CanCreate() {
		listener.OnFailure(types.Overflow, p.host)
		p.host.HostStats().UpstreamRequestPendingOverflow.Inc(1)
		p.host.ClusterInfo().Stats().UpstreamRequestPendingOverflow.Inc(1)
	} else {
		atomic.AddUint64(&activeClient.totalStream, 1)
		p.host.HostStats().UpstreamRequestTotal.Inc(1)
		p.host.ClusterInfo().Stats().UpstreamRequestTotal.Inc(1)

		var streamEncoder types.StreamSender
		// oneway
		if responseDecoder == nil {
			streamEncoder = activeClient.client.NewStream(ctx, nil)
		} else {
			//这里会把streamId对应的stream保存起来
			streamEncoder = activeClient.client.NewStream(ctx, responseDecoder)
			streamEncoder.GetStream().AddEventListener(activeClient)

			p.host.HostStats().UpstreamRequestActive.Inc(1)
			p.host.ClusterInfo().Stats().UpstreamRequestActive.Inc(1)
			p.host.ClusterInfo().ResourceManager().Requests().Increase()
		}

		listener.OnReady(streamEncoder, p.host)
	}

	return
}

func (p *connPool) Close() {
	f := func(k, v interface{}) bool {
		ac, _ := v.(*activeClient)
		if ac.client != nil {
			ac.client.Close()
		}
		return true
	}

	p.activeClients.Range(f)
}

// Shutdown stop the keepalive, so the connection will be idle after requests finished
func (p *connPool) Shutdown() {
	f := func(k, v interface{}) bool {
		ac, _ := v.(*activeClient)
		if ac.keepAlive != nil {
			ac.keepAlive.keepAlive.Stop()
		}
		return true
	}
	p.activeClients.Range(f)
}

func (p *connPool) onConnectionEvent(client *activeClient, event api.ConnectionEvent) {
	// event.ConnectFailure() contains types.ConnectTimeout and types.ConnectTimeout
	if event.IsClose() {
		p.host.HostStats().UpstreamConnectionClose.Inc(1)
		p.host.HostStats().UpstreamConnectionActive.Dec(1)

		p.host.ClusterInfo().Stats().UpstreamConnectionClose.Inc(1)
		p.host.ClusterInfo().Stats().UpstreamConnectionActive.Dec(1)

		switch event {
		case api.LocalClose:
			p.host.HostStats().UpstreamConnectionLocalClose.Inc(1)
			p.host.ClusterInfo().Stats().UpstreamConnectionLocalClose.Inc(1)

			if client.closeWithActiveReq {
				p.host.HostStats().UpstreamConnectionLocalCloseWithActiveRequest.Inc(1)
				p.host.ClusterInfo().Stats().UpstreamConnectionLocalCloseWithActiveRequest.Inc(1)
			}

		case api.RemoteClose:
			p.host.HostStats().UpstreamConnectionRemoteClose.Inc(1)
			p.host.ClusterInfo().Stats().UpstreamConnectionRemoteClose.Inc(1)

			if client.closeWithActiveReq {
				p.host.HostStats().UpstreamConnectionRemoteCloseWithActiveRequest.Inc(1)
				p.host.ClusterInfo().Stats().UpstreamConnectionRemoteCloseWithActiveRequest.Inc(1)

			}
		default:
			// do nothing
		}
		p.mux.Lock()
		p.activeClients.Delete(client.subProtocol)
		p.mux.Unlock()
	} else if event == api.ConnectTimeout {
		p.host.HostStats().UpstreamRequestTimeout.Inc(1)
		p.host.ClusterInfo().Stats().UpstreamRequestTimeout.Inc(1)
		client.client.Close()
	} else if event == api.ConnectFailed {
		p.host.HostStats().UpstreamConnectionConFail.Inc(1)
		p.host.ClusterInfo().Stats().UpstreamConnectionConFail.Inc(1)
	}
}

func (p *connPool) onStreamDestroy(client *activeClient) {
	p.host.HostStats().UpstreamRequestActive.Dec(1)
	p.host.ClusterInfo().Stats().UpstreamRequestActive.Dec(1)
	p.host.ClusterInfo().ResourceManager().Requests().Decrease()
}

func (p *connPool) onStreamReset(client *activeClient, reason types.StreamResetReason) {
	if reason == types.StreamConnectionTermination || reason == types.StreamConnectionFailed {
		p.host.HostStats().UpstreamRequestFailureEject.Inc(1)
		p.host.ClusterInfo().Stats().UpstreamRequestFailureEject.Inc(1)
		client.closeWithActiveReq = true
	} else if reason == types.StreamLocalReset {
		p.host.HostStats().UpstreamRequestLocalReset.Inc(1)
		p.host.ClusterInfo().Stats().UpstreamRequestLocalReset.Inc(1)
	} else if reason == types.StreamRemoteReset {
		p.host.HostStats().UpstreamRequestRemoteReset.Inc(1)
		p.host.ClusterInfo().Stats().UpstreamRequestRemoteReset.Inc(1)
	}
}

func (p *connPool) createStreamClient(context context.Context, connData types.CreateConnectionData) str.Client {
	return str.NewStreamClient(context, protocol.SofaRPC, connData.Connection, connData.Host)
}

// keepAliveListener is a types.ConnectionEventListener
type keepAliveListener struct {
	keepAlive types.KeepAlive
}

func (l *keepAliveListener) OnEvent(event api.ConnectionEvent) {
	if event == api.OnReadTimeout {
		l.keepAlive.SendKeepAlive()
	}
}

// types.StreamEventListener
// types.ConnectionEventListener
// types.StreamConnectionEventListener
type activeClient struct {
	subProtocol        byte
	pool               *connPool
	keepAlive          *keepAliveListener
	client             str.Client
	host               types.CreateConnectionData
	closeWithActiveReq bool
	totalStream        uint64
	state              uint32
}

func newActiveClient(ctx context.Context, subProtocol byte, pool *connPool) *activeClient {
	ac := &activeClient{
		subProtocol: subProtocol,
		pool:        pool,
	}

	data := pool.host.CreateConnection(ctx)
	connCtx := mosnctx.WithValue(ctx, types.ContextKeyConnectionID, data.Connection.ID())
	codecClient := pool.createStreamClient(connCtx, data)
	codecClient.AddConnectionEventListener(ac)
	codecClient.SetStreamConnectionEventListener(ac)

	ac.client = codecClient
	ac.host = data

	// Add Keep Alive
	// protocol is from onNewDetectStream
	// TODO: support protocol convert

	// TODO: support config
	if subProtocol != defaultSubProtocol {
		rpcKeepAlive := NewSofaRPCKeepAlive(codecClient, subProtocol, time.Second, 6)
		rpcKeepAlive.StartIdleTimeout()
		ac.keepAlive = &keepAliveListener{
			keepAlive: rpcKeepAlive,
		}
		ac.client.AddConnectionEventListener(ac.keepAlive)
	}

	if err := ac.client.Connect(); err != nil {
		return nil
	}

	// stats
	pool.host.HostStats().UpstreamConnectionTotal.Inc(1)
	pool.host.HostStats().UpstreamConnectionActive.Inc(1)
	pool.host.ClusterInfo().Stats().UpstreamConnectionTotal.Inc(1)
	pool.host.ClusterInfo().Stats().UpstreamConnectionActive.Inc(1)

	// bytes total adds all connections data together
	codecClient.SetConnectionCollector(pool.host.ClusterInfo().Stats().UpstreamBytesReadTotal, pool.host.ClusterInfo().Stats().UpstreamBytesWriteTotal)

	return ac
}

func (ac *activeClient) OnEvent(event api.ConnectionEvent) {
	ac.pool.onConnectionEvent(ac, event)
}

// types.StreamEventListener
func (ac *activeClient) OnDestroyStream() {
	ac.pool.onStreamDestroy(ac)
}

func (ac *activeClient) OnResetStream(reason types.StreamResetReason) {
	ac.pool.onStreamReset(ac, reason)
}

// types.StreamConnectionEventListener
func (ac *activeClient) OnGoAway() {}

func getSubProtocol(ctx context.Context) byte {
	if ctx != nil {
		if val := mosnctx.Get(ctx, types.ContextSubProtocol); val != nil {
			if code, ok := val.(byte); ok {
				return code
			}
		}
	}
	return defaultSubProtocol
}
