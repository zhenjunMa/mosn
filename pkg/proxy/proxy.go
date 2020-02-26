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

package proxy

import (
	"container/list"
	"context"
	"runtime"
	"sync"
	"sync/atomic"

	jsoniter "github.com/json-iterator/go"
	"mosn.io/api"
	v2 "mosn.io/mosn/pkg/config/v2"
	"mosn.io/mosn/pkg/configmanager"
	mosnctx "mosn.io/mosn/pkg/context"
	"mosn.io/mosn/pkg/log"
	"mosn.io/mosn/pkg/mtls"
	"mosn.io/mosn/pkg/protocol"
	"mosn.io/mosn/pkg/router"
	"mosn.io/mosn/pkg/stream"
	mosnsync "mosn.io/mosn/pkg/sync"
	"mosn.io/mosn/pkg/types"
	"mosn.io/mosn/pkg/upstream/cluster"
	"mosn.io/pkg/buffer"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

var (
	globalStats *Stats

	currProxyID uint32

	pool mosnsync.WorkerPool
)

func init() {
	// register init function with interest of P number
	configmanager.RegisterConfigParsedListener(configmanager.ParseCallbackKeyProcessor, initWorkerPool)
}

func initWorkerPool(data interface{}, endParsing bool) error {
	initGlobalStats()

	//线程池大小=核数*256
	poolSize := runtime.NumCPU() * 256

	// set poolSize equal to processor if it was specified
	//可以自定义修改poolSize
	if pNum, ok := data.(int); ok && pNum > 0 {
		poolSize = pNum * 256
	}

	pool = mosnsync.NewWorkerPool(poolSize)

	return nil
}

func initGlobalStats() {
	globalStats = newProxyStats(types.GlobalProxyName)
}

// types.ReadFilter
// types.ServerStreamConnectionEventListener
type proxy struct {
	config             *v2.Proxy
	clusterManager     types.ClusterManager
	readCallbacks      api.ReadFilterCallbacks
	upstreamConnection types.ClientConnection
	downstreamListener api.ConnectionEventListener
	clusterName        string
	routersWrapper     types.RouterWrapper // wrapper used to point to the routers instance
	serverStreamConn   types.ServerStreamConnection
	context            context.Context
	activeSteams       *list.List // downstream requests
	asMux              sync.RWMutex
	stats              *Stats
	listenerStats      *Stats
	accessLogs         []api.AccessLog
}

// NewProxy create proxy instance for given v2.Proxy config
func NewProxy(ctx context.Context, config *v2.Proxy) Proxy {
	proxy := &proxy{
		config:         config,
		clusterManager: cluster.GetClusterMngAdapterInstance().ClusterManager,
		activeSteams:   list.New(),
		stats:          globalStats,
		context:        ctx,
		accessLogs:     mosnctx.Get(ctx, types.ContextKeyAccessLogs).([]api.AccessLog),
	}

	extJSON, err := json.Marshal(proxy.config.ExtendConfig)
	if err == nil {
		log.DefaultLogger.Tracef("[proxy] extend config = %v", proxy.config.ExtendConfig)
		var xProxyExtendConfig v2.XProxyExtendConfig
		json.Unmarshal([]byte(extJSON), &xProxyExtendConfig)
		proxy.context = mosnctx.WithValue(proxy.context, types.ContextSubProtocol, xProxyExtendConfig.SubProtocol)
		log.DefaultLogger.Tracef("[proxy] extend config subprotocol = %v", xProxyExtendConfig.SubProtocol)
	} else {
		log.DefaultLogger.Errorf("[proxy] get proxy extend config fail = %v", err)
	}

	listenerName := mosnctx.Get(ctx, types.ContextKeyListenerName).(string)
	proxy.listenerStats = newListenerStats(listenerName)

	if routersWrapper := router.GetRoutersMangerInstance().GetRouterWrapperByName(proxy.config.RouterConfigName); routersWrapper != nil {
		proxy.routersWrapper = routersWrapper
	} else {
		log.DefaultLogger.Errorf("[proxy] RouterConfigName:%s doesn't exit", proxy.config.RouterConfigName)
	}

	proxy.downstreamListener = &downstreamCallbacks{
		proxy: proxy,
	}

	return proxy
}

func (p *proxy) OnData(buf buffer.IoBuffer) api.FilterStatus {
	if p.serverStreamConn == nil {
		var prot string
		if conn, ok := p.readCallbacks.Connection().RawConn().(*mtls.TLSConn); ok {
			prot = conn.ConnectionState().NegotiatedProtocol
		}
		protocol, err := stream.SelectStreamFactoryProtocol(p.context, prot, buf.Bytes())
		if err == stream.EAGAIN {
			return api.Stop
		} else if err == stream.FAILED {
			var size int
			if buf.Len() > 10 {
				size = 10
			} else {
				size = buf.Len()
			}
			log.DefaultLogger.Errorf("[proxy] Protocol Auto error magic :%v", buf.Bytes()[:size])
			p.readCallbacks.Connection().Close(api.NoFlush, api.OnReadErrClose)
			return api.Stop
		}
		log.DefaultLogger.Debugf("[proxy] Protoctol Auto: %v", protocol)
		p.serverStreamConn = stream.CreateServerStreamConnection(p.context, protocol, p.readCallbacks.Connection(), p)
	}
	//把数据分发到对应协议的的解码器
	p.serverStreamConn.Dispatch(buf)

	return api.Stop
}

//rpc realize upstream on event
func (p *proxy) onDownstreamEvent(event api.ConnectionEvent) {
	if event.IsClose() {
		p.stats.DownstreamConnectionDestroy.Inc(1)
		p.stats.DownstreamConnectionActive.Dec(1)
		p.listenerStats.DownstreamConnectionDestroy.Inc(1)
		p.listenerStats.DownstreamConnectionActive.Dec(1)
		var urEleNext *list.Element

		p.asMux.RLock()
		defer p.asMux.RUnlock()

		for urEle := p.activeSteams.Front(); urEle != nil; urEle = urEleNext {
			urEleNext = urEle.Next()

			ds := urEle.Value.(*downStream)
			ds.OnResetStream(types.StreamConnectionTermination)
		}
	}
}

func (p *proxy) ReadDisableUpstream(disable bool) {
	// TODO
}

func (p *proxy) ReadDisableDownstream(disable bool) {
	// TODO
}

func (p *proxy) InitializeReadFilterCallbacks(cb api.ReadFilterCallbacks) {
	p.readCallbacks = cb

	// bytes total adds all connections data together
	cb.Connection().SetCollector(p.stats.DownstreamBytesReadTotal, p.stats.DownstreamBytesWriteTotal)

	p.stats.DownstreamConnectionTotal.Inc(1)
	p.stats.DownstreamConnectionActive.Inc(1)
	p.listenerStats.DownstreamConnectionTotal.Inc(1)
	p.listenerStats.DownstreamConnectionActive.Inc(1)

	p.readCallbacks.Connection().AddConnectionEventListener(p.downstreamListener)
	if p.config.DownstreamProtocol != string(protocol.Auto) {
		p.serverStreamConn = stream.CreateServerStreamConnection(p.context, types.Protocol(p.config.DownstreamProtocol), p.readCallbacks.Connection(), p)
	}
}

func (p *proxy) OnGoAway() {}

func (p *proxy) NewStreamDetect(ctx context.Context, responseSender types.StreamSender, span types.Span) types.StreamReceiveListener {
	//每个请求对应一个stream
	stream := newActiveStream(ctx, p, responseSender, span)

	if value := mosnctx.Get(p.context, types.ContextKeyStreamFilterChainFactories); value != nil {
		ff := value.(*atomic.Value)
		ffs, ok := ff.Load().([]api.StreamFilterChainFactory)
		if ok {

			if log.Proxy.GetLogLevel() >= log.DEBUG {
				log.Proxy.Debugf(stream.context, "[proxy][downstream] %d stream filters in config", len(ffs))
			}

			for _, f := range ffs {
				f.CreateFilterChain(p.context, stream)
			}
		}
	}

	p.asMux.Lock()
	stream.element = p.activeSteams.PushBack(stream)
	p.asMux.Unlock()

	return stream
}

func (p *proxy) OnNewConnection() api.FilterStatus {
	return api.Continue
}

func (p *proxy) streamResetReasonToResponseFlag(reason types.StreamResetReason) api.ResponseFlag {
	switch reason {
	case types.StreamConnectionFailed:
		return api.UpstreamConnectionFailure
	case types.StreamConnectionTermination:
		return api.UpstreamConnectionTermination
	case types.StreamLocalReset:
		return api.UpstreamLocalReset
	case types.StreamOverflow:
		return api.UpstreamOverflow
	case types.StreamRemoteReset:
		return api.UpstreamRemoteReset
	}

	return 0
}

func (p *proxy) deleteActiveStream(s *downStream) {
	if s.element != nil {
		p.asMux.Lock()
		p.activeSteams.Remove(s.element)
		p.asMux.Unlock()
		s.element = nil
	}
}

// ConnectionEventListener
type downstreamCallbacks struct {
	proxy *proxy
}

func (dc *downstreamCallbacks) OnEvent(event api.ConnectionEvent) {
	dc.proxy.onDownstreamEvent(event)
}
