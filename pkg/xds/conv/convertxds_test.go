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

package conv

import (
	"reflect"
	"testing"
	"time"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	xdstype "github.com/envoyproxy/go-control-plane/envoy/type"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	v1 "istio.io/api/mixer/v1"
	"istio.io/api/mixer/v1/config/client"

	v2 "mosn.io/mosn/pkg/config/v2"
	"mosn.io/mosn/pkg/filter/stream/faultinject"
	"mosn.io/mosn/pkg/router"
	"mosn.io/mosn/pkg/server"
	"mosn.io/mosn/pkg/upstream/cluster"

	xdsapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	xdscore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	xdsendpoint "github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	xdslistener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	xdsroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	xdsaccesslog "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v2"
	xdsfal "github.com/envoyproxy/go-control-plane/envoy/config/filter/accesslog/v2"
	xdsfault "github.com/envoyproxy/go-control-plane/envoy/config/filter/fault/v2"
	xdshttpfault "github.com/envoyproxy/go-control-plane/envoy/config/filter/http/fault/v2"
	xdshttp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	xdstcp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/tcp_proxy/v2"
	xdsutil "github.com/envoyproxy/go-control-plane/pkg/util"
	google_protobuf1 "github.com/gogo/protobuf/types"
)

func TestMain(m *testing.M) {
	// init
	router.NewRouterManager()
	cm := cluster.NewClusterManagerSingleton(nil, nil)
	sc := server.NewConfig(&v2.ServerConfig{
		ServerName:      "test_xds_server",
		DefaultLogPath:  "stdout",
		DefaultLogLevel: "FATAL",
	})
	server.NewServer(sc, &mockCMF{}, cm)
	m.Run()
}

// messageToAny converts from proto message to proto Any
func messageToAny(msg proto.Message) *types.Any {
	s, err := types.MarshalAny(msg)
	if err != nil {
		return nil
	}
	return s
}

// mmessageToStruct converts from proto message to proto Struct
//func messageToStruct(msg proto.Message) *types.Struct {
//	s, err := xdsutil.MessageToStruct(msg)
//	if err != nil {
//		return nil
//	}
//	return s
//}

// todo fill the unit test
func Test_convertEndpointsConfig(t *testing.T) {
	type args struct {
		xdsEndpoint *xdsendpoint.LocalityLbEndpoints
	}
	tests := []struct {
		name string
		args args
		want []v2.Host
	}{
		{
			name: "case1",
			args: args{
				xdsEndpoint: &xdsendpoint.LocalityLbEndpoints{
					Priority: 1,
				},
			},
			want: []v2.Host{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ConvertEndpointsConfig(tt.args.xdsEndpoint); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("convertEndpointsConfig() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_convertHeaders(t *testing.T) {
	type args struct {
		xdsHeaders []*xdsroute.HeaderMatcher
	}
	tests := []struct {
		name string
		args args
		want []v2.HeaderMatcher
	}{
		{
			name: "case1",
			args: args{
				xdsHeaders: []*xdsroute.HeaderMatcher{
					{
						Name: "end-user",
						HeaderMatchSpecifier: &xdsroute.HeaderMatcher_ExactMatch{
							ExactMatch: "jason",
						},
						InvertMatch: false,
					},
				},
			},
			want: []v2.HeaderMatcher{
				{
					Name:  "end-user",
					Value: "jason",
					Regex: false,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := convertHeaders(tt.args.xdsHeaders); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("convertHeaders(xdsHeaders []*xdsroute.HeaderMatcher) = %v, want %v", got, tt.want)
			}
		})
	}
}

func NewBoolValue(val bool) *types.BoolValue {
	return &types.BoolValue{
		Value:                val,
		XXX_NoUnkeyedLiteral: struct{}{},
		XXX_unrecognized:     nil,
		XXX_sizecache:        0,
	}
}

func Test_convertListenerConfig(t *testing.T) {
	type args struct {
		xdsListener  *xdsapi.Listener
		address      core.Address
		filterName   string
		filterConfig *xdshttp.HttpConnectionManager
	}
	// TODO use Any
	//accessLogFilterConfig := messageToAny(&xdsaccesslog.FileAccessLog{
	accessLogFilterConfig := messageToStruct(t, &xdsaccesslog.FileAccessLog{
		Path: "/dev/stdout",
	})

	zeroSecond := new(time.Duration)
	*zeroSecond = 0

	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "0.0.0.0_80",
			args: args{
				filterConfig: &xdshttp.HttpConnectionManager{
					CodecType:  xdshttp.AUTO,
					StatPrefix: "0.0.0.0_80",
					RouteSpecifier: &xdshttp.HttpConnectionManager_RouteConfig{
						RouteConfig: &xdsapi.RouteConfiguration{
							Name: "80",
							VirtualHosts: []xdsroute.VirtualHost{
								{
									Name: "istio-egressgateway.istio-system.svc.cluster.local:80",
									Domains: []string{
										"istio-egressgateway.istio-system.svc.cluster.local",
										"istio-egressgateway.istio-system.svc.cluster.local:80",
										"istio-egressgateway.istio-system",
										"istio-egressgateway.istio-system:80",
										"istio-egressgateway.istio-system.svc.cluster",
										"istio-egressgateway.istio-system.svc.cluster:80",
										"istio-egressgateway.istio-system.svc",
										"istio-egressgateway.istio-system.svc:80",
										"172.19.3.204",
										"172.19.3.204:80",
									},
									Routes: []xdsroute.Route{
										{
											Match: xdsroute.RouteMatch{
												PathSpecifier: &xdsroute.RouteMatch_Prefix{
													Prefix: "/",
												},
											},
											Action: &xdsroute.Route_Route{
												Route: &xdsroute.RouteAction{
													ClusterSpecifier: &xdsroute.RouteAction_Cluster{
														Cluster: "outbound|80||istio-egressgateway.istio-system.svc.cluster.local",
													},
													ClusterNotFoundResponseCode: xdsroute.RouteAction_SERVICE_UNAVAILABLE,
													MetadataMatch:               nil,
													PrefixRewrite:               "",
													HostRewriteSpecifier:        nil,
													Timeout:                     zeroSecond,
													RetryPolicy:                 nil,
													RequestMirrorPolicy:         nil,
													Priority:                    core.RoutingPriority_DEFAULT,
													MaxGrpcTimeout:              new(time.Duration),
												},
											},
											Metadata: nil,
											Decorator: &xdsroute.Decorator{
												Operation: "istio-egressgateway.istio-system.svc.cluster.local:80/*",
											},
											PerFilterConfig: nil,
										},
									},
									RequireTls: xdsroute.VirtualHost_NONE,
								},
								{
									Name: "istio-ingressgateway.istio-system.svc.cluster.local:80",
									Domains: []string{
										"istio-ingressgateway.istio-system.svc.cluster.local",
										"istio-ingressgateway.istio-system.svc.cluster.local:80",
										"istio-ingressgateway.istio-system",
										"istio-ingressgateway.istio-system:80",
										"istio-ingressgateway.istio-system.svc.cluster",
										"istio-ingressgateway.istio-system.svc.cluster:80",
										"istio-ingressgateway.istio-system.svc",
										"istio-ingressgateway.istio-system.svc:80",
										"172.19.8.101",
										"172.19.8.101:80",
									},
									Routes: []xdsroute.Route{
										{
											Match: xdsroute.RouteMatch{
												PathSpecifier: &xdsroute.RouteMatch_Prefix{
													Prefix: "/",
												},
											},
											Action: &xdsroute.Route_Route{
												Route: &xdsroute.RouteAction{
													ClusterSpecifier: &xdsroute.RouteAction_Cluster{
														Cluster: "outbound|80||istio-ingressgateway.istio-system.svc.cluster.local",
													},
													ClusterNotFoundResponseCode: xdsroute.RouteAction_SERVICE_UNAVAILABLE,
													PrefixRewrite:               "",
													Timeout:                     zeroSecond,
													Priority:                    core.RoutingPriority_DEFAULT,
													MaxGrpcTimeout:              new(time.Duration),
												},
											},
											Decorator: &xdsroute.Decorator{
												Operation: "istio-ingressgateway.istio-system.svc.cluster.local:80/*",
											},
											PerFilterConfig: nil,
										},
									},
									RequireTls: xdsroute.VirtualHost_NONE,
								},
								{
									Name: "nginx-ingress-lb.kube-system.svc.cluster.local:80",
									Domains: []string{
										"nginx-ingress-lb.kube-system.svc.cluster.local",
										"nginx-ingress-lb.kube-system.svc.cluster.local:80",
										"nginx-ingress-lb.kube-system",
										"nginx-ingress-lb.kube-system:80",
										"nginx-ingress-lb.kube-system.svc.cluster",
										"nginx-ingress-lb.kube-system.svc.cluster:80",
										"nginx-ingress-lb.kube-system.svc",
										"nginx-ingress-lb.kube-system.svc:80",
										"172.19.6.192:80",
										"172.19.8.101:80",
									},
									Routes: []xdsroute.Route{
										{
											Match: xdsroute.RouteMatch{
												PathSpecifier: &xdsroute.RouteMatch_Prefix{
													Prefix: "/",
												},
											},
											Action: &xdsroute.Route_Route{
												Route: &xdsroute.RouteAction{
													ClusterSpecifier: &xdsroute.RouteAction_Cluster{
														Cluster: "outbound|80||nginx-ingress-lb.kube-system.svc.cluster.local",
													},
													ClusterNotFoundResponseCode: xdsroute.RouteAction_SERVICE_UNAVAILABLE,
													PrefixRewrite:               "",
													Timeout:                     zeroSecond,
													Priority:                    core.RoutingPriority_DEFAULT,
													MaxGrpcTimeout:              new(time.Duration),
												},
											},
											Decorator: &xdsroute.Decorator{
												Operation: "nginx-ingress-lb.kube-system.svc.cluster.local:80/*",
											},
											PerFilterConfig: nil,
										},
									},
									RequireTls: xdsroute.VirtualHost_NONE,
								},
							},
							ValidateClusters: NewBoolValue(false),
						},
					},
					ServerName: "",
					AccessLog: []*xdsfal.AccessLog{{
						Name:   "envoy.file_access_log",
						Filter: nil,
						// TODO use Any
						//ConfigType: &xdsfal.AccessLog_TypedConfig{
						ConfigType: &xdsfal.AccessLog_Config{
							accessLogFilterConfig,
						},
					}},
					UseRemoteAddress:            NewBoolValue(false),
					XffNumTrustedHops:           0,
					SkipXffAppend:               false,
					Via:                         "",
					GenerateRequestId:           NewBoolValue(true),
					ForwardClientCertDetails:    xdshttp.SANITIZE,
					SetCurrentClientCertDetails: nil,
					Proxy_100Continue:           false,
					RepresentIpv4RemoteAddressAsIpv4MappedIpv6: false,
				},
				filterName: "envoy.http_connection_manager",
				address: core.Address{
					Address: &core.Address_SocketAddress{
						SocketAddress: &core.SocketAddress{
							Protocol: core.TCP,
							Address:  "0.0.0.0",
							PortSpecifier: &core.SocketAddress_PortValue{
								PortValue: 80,
							},
							ResolverName: "",
							Ipv4Compat:   false,
						},
					},
				},
			},
			want: `{"name":"0.0.0.0_80","address":"","bind_port":false,"handoff_restoreddestination":false,"log_path":"stdout","access_logs":[{"log_path":"/dev/stdout"}],"filter_chains":[{"match":"\u003cnil\u003e","tls_context":{"status":false,"type":"","extend_verify":null},"filters":[{"type":"proxy","config":{"downstream_protocol":"Http1","extend_config":null,"name":"","support_dynamic_route":true,"upstream_protocol":"Http1","validate_clusters":false,"virtual_hosts":[{"domains":["istio-egressgateway.istio-system.svc.cluster.local","istio-egressgateway.istio-system.svc.cluster.local:80","istio-egressgateway.istio-system","istio-egressgateway.istio-system:80","istio-egressgateway.istio-system.svc.cluster","istio-egressgateway.istio-system.svc.cluster:80","istio-egressgateway.istio-system.svc","istio-egressgateway.istio-system.svc:80","172.19.3.204","172.19.3.204:80"],"name":"istio-egressgateway.istio-system.svc.cluster.local:80","require_tls":"NONE","routers":[{"decorator":"operation:\"istio-egressgateway.istio-system.svc.cluster.local:80/*\" ","match":{"case_sensitive":false,"headers":null,"path":"","prefix":"/","regex":"","runtime":{"default_value":0,"runtime_key":""}},"metadata":{"filter_metadata":{"mosn.lb":null}},"redirect":{"host_redirect":"","path_redirect":"","response_code":0},"route":{"cluster_header":"","cluster_name":"outbound|80||istio-egressgateway.istio-system.svc.cluster.local","metadata_match":{"filter_metadata":{"mosn.lb":null}},"retry_policy":{"num_retries":0,"retry_on":false,"retry_timeout":"0s"},"timeout":"0s","weighted_clusters":null}}],"virtual_clusters":null},{"domains":["istio-ingressgateway.istio-system.svc.cluster.local","istio-ingressgateway.istio-system.svc.cluster.local:80","istio-ingressgateway.istio-system","istio-ingressgateway.istio-system:80","istio-ingressgateway.istio-system.svc.cluster","istio-ingressgateway.istio-system.svc.cluster:80","istio-ingressgateway.istio-system.svc","istio-ingressgateway.istio-system.svc:80","172.19.8.101","172.19.8.101:80"],"name":"istio-ingressgateway.istio-system.svc.cluster.local:80","require_tls":"NONE","routers":[{"decorator":"operation:\"istio-ingressgateway.istio-system.svc.cluster.local:80/*\" ","match":{"case_sensitive":false,"headers":null,"path":"","prefix":"/","regex":"","runtime":{"default_value":0,"runtime_key":""}},"metadata":{"filter_metadata":{"mosn.lb":null}},"redirect":{"host_redirect":"","path_redirect":"","response_code":0},"route":{"cluster_header":"","cluster_name":"outbound|80||istio-ingressgateway.istio-system.svc.cluster.local","metadata_match":{"filter_metadata":{"mosn.lb":null}},"retry_policy":{"num_retries":0,"retry_on":false,"retry_timeout":"0s"},"timeout":"0s","weighted_clusters":null}}],"virtual_clusters":null},{"domains":["nginx-ingress-lb.kube-system.svc.cluster.local","nginx-ingress-lb.kube-system.svc.cluster.local:80","nginx-ingress-lb.kube-system","nginx-ingress-lb.kube-system:80","nginx-ingress-lb.kube-system.svc.cluster","nginx-ingress-lb.kube-system.svc.cluster:80","nginx-ingress-lb.kube-system.svc","nginx-ingress-lb.kube-system.svc:80","172.19.6.192:80","172.19.8.101:80"],"name":"nginx-ingress-lb.kube-system.svc.cluster.local:80","require_tls":"NONE","routers":[{"decorator":"operation:\"nginx-ingress-lb.kube-system.svc.cluster.local:80/*\" ","match":{"case_sensitive":false,"headers":null,"path":"","prefix":"/","regex":"","runtime":{"default_value":0,"runtime_key":""}},"metadata":{"filter_metadata":{"mosn.lb":null}},"redirect":{"host_redirect":"","path_redirect":"","response_code":0},"route":{"cluster_header":"","cluster_name":"outbound|80||nginx-ingress-lb.kube-system.svc.cluster.local","metadata_match":{"filter_metadata":{"mosn.lb":null}},"retry_policy":{"num_retries":0,"retry_on":false,"retry_timeout":"0s"},"timeout":"0s","weighted_clusters":null}}],"virtual_clusters":null}]}}]}],"inspector":true}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// TODO use Any
			//conf := messageToAny(tt.args.filterConfig)
			conf := messageToStruct(t, tt.args.filterConfig)
			listenerConfig := &xdsapi.Listener{
				Name:    "0.0.0.0_80",
				Address: tt.args.address,
				FilterChains: []xdslistener.FilterChain{
					{
						FilterChainMatch: nil,
						TlsContext:       nil,
						Filters: []xdslistener.Filter{
							{
								Name: tt.args.filterName,
								// TODO use Any
								//ConfigType: &xdslistener.Filter_TypedConfig{
								ConfigType: &xdslistener.Filter_Config{
									conf,
								},
							},
						},
					},
				},
				DeprecatedV1: &xdsapi.Listener_DeprecatedV1{
					BindToPort: NewBoolValue(false),
				},
				DrainType: xdsapi.Listener_DEFAULT,
				ListenerFilters: []xdslistener.ListenerFilter{
					{
						Name:       "original_dst",
						ConfigType: &xdslistener.ListenerFilter_TypedConfig{},
					},
				},
			}

			tt.want = `{"name":"0.0.0.0_80","access_logs":[{"log_path":"/dev/stdout"}],"listener_filters":[{"type":"original_dst"}],"filter_chains":[{"match":"\u003cnil\u003e","tls_context_set":[{}],"filters":[{"type":"proxy","config":{"downstream_protocol":"Http1","router_config_name":"80","upstream_protocol":"Http1"}},{"type":"connection_manager","config":{"router_config_name":"80","virtual_hosts":[{"domains":["istio-egressgateway.istio-system.svc.cluster.local","istio-egressgateway.istio-system.svc.cluster.local:80","istio-egressgateway.istio-system","istio-egressgateway.istio-system:80","istio-egressgateway.istio-system.svc.cluster","istio-egressgateway.istio-system.svc.cluster:80","istio-egressgateway.istio-system.svc","istio-egressgateway.istio-system.svc:80","172.19.3.204","172.19.3.204:80"],"name":"istio-egressgateway.istio-system.svc.cluster.local:80","routers":[{"match":{"prefix":"/"},"route":{"cluster_name":"outbound|80||istio-egressgateway.istio-system.svc.cluster.local","retry_policy":{"retry_timeout":"0s"},"timeout":"0s"}}]},{"domains":["istio-ingressgateway.istio-system.svc.cluster.local","istio-ingressgateway.istio-system.svc.cluster.local:80","istio-ingressgateway.istio-system","istio-ingressgateway.istio-system:80","istio-ingressgateway.istio-system.svc.cluster","istio-ingressgateway.istio-system.svc.cluster:80","istio-ingressgateway.istio-system.svc","istio-ingressgateway.istio-system.svc:80","172.19.8.101","172.19.8.101:80"],"name":"istio-ingressgateway.istio-system.svc.cluster.local:80","routers":[{"match":{"prefix":"/"},"route":{"cluster_name":"outbound|80||istio-ingressgateway.istio-system.svc.cluster.local","retry_policy":{"retry_timeout":"0s"},"timeout":"0s"}}]},{"domains":["nginx-ingress-lb.kube-system.svc.cluster.local","nginx-ingress-lb.kube-system.svc.cluster.local:80","nginx-ingress-lb.kube-system","nginx-ingress-lb.kube-system:80","nginx-ingress-lb.kube-system.svc.cluster","nginx-ingress-lb.kube-system.svc.cluster:80","nginx-ingress-lb.kube-system.svc","nginx-ingress-lb.kube-system.svc:80","172.19.6.192:80","172.19.8.101:80"],"name":"nginx-ingress-lb.kube-system.svc.cluster.local:80","routers":[{"match":{"prefix":"/"},"route":{"cluster_name":"outbound|80||nginx-ingress-lb.kube-system.svc.cluster.local","retry_policy":{"retry_timeout":"0s"},"timeout":"0s"}}]}]}}]}],"inspector":true}`
			got := ConvertListenerConfig(listenerConfig)
			want := &v2.Listener{}
			err := json.Unmarshal([]byte(tt.want), want)
			if err != nil {
				t.Errorf("json.Unmarshal([]byte(tt.want) got error: %v", err)
			}

			if !reflect.DeepEqual(want.ListenerConfig.ListenerFilters, got.ListenerConfig.ListenerFilters) {
				t.Errorf("convertListenerConfig(xdsListener *xdsapi.Listener)\ngot=%+v\nwant=%+v\n", got.ListenerConfig.ListenerFilters, want.ListenerConfig.ListenerFilters)
			}
		})
	}

}

func Test_convertCidrRange(t *testing.T) {
	type args struct {
		cidr []*xdscore.CidrRange
	}
	tests := []struct {
		name string
		args args
		want []v2.CidrRange
	}{
		{
			name: "case1",
			args: args{
				cidr: []*xdscore.CidrRange{
					{
						AddressPrefix: "192.168.1.1",
						PrefixLen:     &google_protobuf1.UInt32Value{Value: 32},
					},
				},
			},
			want: []v2.CidrRange{
				{
					Address: "192.168.1.1",
					Length:  32,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := convertCidrRange(tt.args.cidr); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("convertCidrRange(cidr []*xdscore.CidrRange) = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_convertTCPRoute(t *testing.T) {
	type args struct {
		deprecatedV1 *xdstcp.TcpProxy_DeprecatedV1
	}
	tests := []struct {
		name string
		args args
		want []*v2.TCPRoute
	}{
		{
			name: "case1",
			args: args{
				deprecatedV1: &xdstcp.TcpProxy_DeprecatedV1{
					Routes: []*xdstcp.TcpProxy_DeprecatedV1_TCPRoute{
						{
							Cluster: "tcp",
							DestinationIpList: []*xdscore.CidrRange{
								{
									AddressPrefix: "192.168.1.1",
									PrefixLen:     &google_protobuf1.UInt32Value{Value: 32},
								},
							},
							DestinationPorts: "50",
							SourceIpList: []*xdscore.CidrRange{
								{
									AddressPrefix: "192.168.1.2",
									PrefixLen:     &google_protobuf1.UInt32Value{Value: 32},
								},
							},
							SourcePorts: "40",
						},
					},
				},
			},
			want: []*v2.TCPRoute{
				{
					Cluster: "tcp",
					DestinationAddrs: []v2.CidrRange{
						{
							Address: "192.168.1.1",
							Length:  32,
						},
					},
					DestinationPort: "50",
					SourceAddrs: []v2.CidrRange{
						{
							Address: "192.168.1.2",
							Length:  32,
						},
					},
					SourcePort: "40",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := convertTCPRoute(tt.args.deprecatedV1); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("convertTCPRoute(deprecatedV1 *xdstcp.TcpProxy_DeprecatedV1) = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_convertHeadersToAdd(t *testing.T) {
	type args struct {
		headerValueOption []*xdscore.HeaderValueOption
	}

	FALSE := false

	tests := []struct {
		name string
		args args
		want []*v2.HeaderValueOption
	}{
		{
			name: "case1",
			args: args{
				headerValueOption: []*xdscore.HeaderValueOption{
					{
						Header: &xdscore.HeaderValue{
							Key:   "namespace",
							Value: "demo",
						},
						Append: &google_protobuf1.BoolValue{Value: false},
					},
				},
			},
			want: []*v2.HeaderValueOption{
				{
					Header: &v2.HeaderValue{
						Key:   "namespace",
						Value: "demo",
					},
					Append: &FALSE,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := convertHeadersToAdd(tt.args.headerValueOption); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("convertHeadersToAdd(headerValueOption []*xdscore.HeaderValueOption) = %v, want %v", got, tt.want)
			}
		})
	}
}

// Test stream filters convert for envoy.fault
func Test_convertStreamFilter_IsitoFault(t *testing.T) {
	faultInjectConfig := &xdshttpfault.HTTPFault{
		Delay: &xdsfault.FaultDelay{
			Percentage: &xdstype.FractionalPercent{
				Numerator:   100,
				Denominator: xdstype.FractionalPercent_HUNDRED,
			},
			FaultDelaySecifier: &xdsfault.FaultDelay_FixedDelay{},
		},
		Abort: &xdshttpfault.FaultAbort{
			Percentage: &xdstype.FractionalPercent{
				Numerator:   100,
				Denominator: xdstype.FractionalPercent_HUNDRED,
			},
			ErrorType: &xdshttpfault.FaultAbort_HttpStatus{
				HttpStatus: 500,
			},
		},
		UpstreamCluster: "testupstream",
		Headers: []*xdsroute.HeaderMatcher{
			{
				Name: "end-user",
				HeaderMatchSpecifier: &xdsroute.HeaderMatcher_ExactMatch{
					ExactMatch: "jason",
				},
				InvertMatch: false,
			},
		},
	}
	faultStruct, err := xdsutil.MessageToStruct(faultInjectConfig)
	if err != nil {
		t.Fatal("make fault inject struct failed")
	}
	// empty types.Struct will makes a default empty filter
	testCases := []struct {
		config   *types.Struct
		expected *v2.StreamFaultInject
	}{
		{
			config: faultStruct,
			expected: &v2.StreamFaultInject{
				Delay: &v2.DelayInject{
					Delay: 0,
					DelayInjectConfig: v2.DelayInjectConfig{
						Percent: 100,
					},
				},
				Abort: &v2.AbortInject{
					Status:  500,
					Percent: 100,
				},
				Headers: []v2.HeaderMatcher{
					{
						Name:  "end-user",
						Value: "jason",
						Regex: false,
					},
				},
				UpstreamCluster: "testupstream",
			},
		},
		{
			config:   nil,
			expected: &v2.StreamFaultInject{},
		},
	}
	for i, tc := range testCases {
		convertFilter := convertStreamFilter(IstioFault, tc.config)
		if convertFilter.Type != v2.FaultStream {
			t.Errorf("#%d convert to mosn stream filter not expected, want %s, got %s", i, v2.FaultStream, convertFilter.Type)
			continue
		}
		rawFault := &v2.StreamFaultInject{}
		b, _ := json.Marshal(convertFilter.Config)
		if err := json.Unmarshal(b, rawFault); err != nil {
			t.Errorf("#%d unexpected config for fault", i)
			continue
		}
		if tc.expected.Abort == nil {
			if rawFault.Abort != nil {
				t.Errorf("#%d abort check unexpected", i)
			}
		} else {
			if rawFault.Abort.Status != tc.expected.Abort.Status || rawFault.Abort.Percent != tc.expected.Abort.Percent {
				t.Errorf("#%d abort check unexpected", i)
			}
		}
		if tc.expected.Delay == nil {
			if rawFault.Delay != nil {
				t.Errorf("#%d delay check unexpected", i)
			}
		} else {
			if rawFault.Delay.Delay != tc.expected.Delay.Delay || rawFault.Delay.Percent != tc.expected.Delay.Percent {
				t.Errorf("#%d delay check unexpected", i)
			}
		}
		if rawFault.UpstreamCluster != tc.expected.UpstreamCluster || !reflect.DeepEqual(rawFault.Headers, tc.expected.Headers) {
			t.Errorf("#%d fault config is not expected, %v", i, rawFault)
		}

	}
}

func Test_convertPerRouteConfig(t *testing.T) {
	mixerFilterConfig := &client.ServiceConfig{
		DisableReportCalls: false,
		DisableCheckCalls:  true,
		MixerAttributes: &v1.Attributes{
			Attributes: map[string]*v1.Attributes_AttributeValue{
				"test": &v1.Attributes_AttributeValue{
					Value: &v1.Attributes_AttributeValue_StringValue{
						StringValue: "test_value",
					},
				},
			},
		},
	}
	mixerStruct, err := xdsutil.MessageToStruct(mixerFilterConfig)
	if err != nil {
		t.Fatal("make mixer struct failed")
	}
	fixedDelay := time.Second
	faultInjectConfig := &xdshttpfault.HTTPFault{
		Delay: &xdsfault.FaultDelay{
			Percentage: &xdstype.FractionalPercent{
				Numerator:   100,
				Denominator: xdstype.FractionalPercent_HUNDRED,
			},
			FaultDelaySecifier: &xdsfault.FaultDelay_FixedDelay{
				FixedDelay: &fixedDelay,
			},
		},
		Abort: &xdshttpfault.FaultAbort{
			Percentage: &xdstype.FractionalPercent{
				Numerator:   100,
				Denominator: xdstype.FractionalPercent_HUNDRED,
			},
			ErrorType: &xdshttpfault.FaultAbort_HttpStatus{
				HttpStatus: 500,
			},
		},
		UpstreamCluster: "testupstream",
		Headers: []*xdsroute.HeaderMatcher{
			{
				Name: "end-user",
				HeaderMatchSpecifier: &xdsroute.HeaderMatcher_ExactMatch{
					ExactMatch: "jason",
				},
				InvertMatch: false,
			},
		},
	}
	faultStruct, err := xdsutil.MessageToStruct(faultInjectConfig)
	if err != nil {
		t.Fatal("make fault inject struct failed")
	}
	configs := map[string]*types.Struct{
		v2.MIXER:       mixerStruct,
		v2.FaultStream: faultStruct,
	}
	perRouteConfig := convertPerRouteConfig(configs)
	if len(perRouteConfig) != 2 {
		t.Fatalf("want to get %d configs, but got %d", 2, len(perRouteConfig))
	}
	// verify
	if mixerPer, ok := perRouteConfig[v2.MIXER]; !ok {
		t.Error("no mixer config found")
	} else {
		// TODO: mixer config needs to fix
		if rawMixer, ok := mixerPer.(client.ServiceConfig); !ok {
			t.Error("mixer config is not expected")
		} else {
			if !reflect.DeepEqual(&rawMixer, mixerFilterConfig) {
				t.Error("mixer config is not expected")
			}
		}
	}
	if faultPer, ok := perRouteConfig[v2.FaultStream]; !ok {
		t.Error("no fault inject config found")
	} else {
		b, err := json.Marshal(faultPer)
		if err != nil {
			t.Fatal("marshal fault inject config failed")
		}
		conf := make(map[string]interface{})
		json.Unmarshal(b, &conf)
		rawFault, err := faultinject.ParseStreamFaultInjectFilter(conf)
		if err != nil {
			t.Fatal("fault config is not expected")
		}
		expectedHeader := v2.HeaderMatcher{
			Name:  "end-user",
			Value: "jason",
			Regex: false,
		}
		if !(rawFault.Abort.Status == 500 &&
			rawFault.Abort.Percent == 100 &&
			rawFault.Delay.Delay == time.Second &&
			rawFault.Delay.Percent == 100 &&
			rawFault.UpstreamCluster == "testupstream" &&
			len(rawFault.Headers) == 1 &&
			reflect.DeepEqual(rawFault.Headers[0], expectedHeader)) {
			t.Errorf("fault config is not expected, %v", rawFault)
		}

	}

}
