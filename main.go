package main

import (
	"bufio"
	"context"
	"fmt"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	hcm "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	"github.com/envoyproxy/go-control-plane/pkg/cache"
	xds "github.com/envoyproxy/go-control-plane/pkg/server"
	"github.com/envoyproxy/go-control-plane/pkg/util"
	"github.com/gogo/protobuf/types"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"net"

	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v2"
)

var (
	localhost = "127.0.0.1"
	port = ":9901"
	gatewayPort = ":9902"
	version int32
)

type logger struct{}

func (logger logger) Infof(format string, args ...interface{}) {
	logrus.Infof(format, args...)
}
func (logger logger) Errorf(format string, args ...interface{}) {
	logrus.Errorf(format, args...)
}

func (cb *callbacks) Report() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	logrus.WithFields(logrus.Fields{"fetches": cb.fetches, "requests": cb.requests}).Info("cb.Report()  callbacks")
}


// OnStreamOpen is called once an xDS stream is open with a stream ID and the type URL (or "" for ADS).
// Returning an error will end processing and close the stream. OnStreamClosed will still be called.
func (cb *callbacks) OnStreamOpen(ctx context.Context, id int64, typ string) error {
	logrus.Infof("OnStreamOpen %d open for %s", id, typ)
	return  nil
}
// OnStreamClosed is called immediately prior to closing an xDS stream with a stream ID.
func (cb *callbacks) OnStreamClosed(id int64) {
	logrus.Infof("OnStreamClosed %d closed", id)
}
// OnStreamRequest is called once a request is received on a stream.
// Returning an error will end processing and close the stream. OnStreamClosed will still be called.
func (cb *callbacks) OnStreamRequest(int64, *v2.DiscoveryRequest) error {
	logrus.Infof("OnStreamRequest")
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.requests++
	if cb.signal != nil {
		close(cb.signal)
		cb.signal = nil
	}
	return nil
}
// OnStreamResponse is called immediately prior to sending a response on a stream.
func (cb *callbacks) OnStreamResponse(int64, *v2.DiscoveryRequest, *v2.DiscoveryResponse) {
	logrus.Infof("OnStreamResponse...")
	cb.Report()
}
// OnFetchRequest is called for each Fetch request. Returning an error will end processing of the
// request and respond with an error.
func (cb *callbacks) OnFetchRequest(ctx context.Context, req *v2.DiscoveryRequest) error {
	logrus.Infof("OnFetchRequest...")
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.fetches++
	if cb.signal != nil {
		close(cb.signal)
		cb.signal = nil
	}
	return nil
}
// OnFetchResponse is called immediately prior to sending a response.
func (cb *callbacks) OnFetchResponse(*v2.DiscoveryRequest, *v2.DiscoveryResponse) {}

type callbacks struct {
	signal   chan struct{}
	fetches  int
	requests int
	mu       sync.Mutex
}

// Hasher returns node ID as an ID
type Hasher struct {
}

// ID function
func (h Hasher) ID(node *core.Node) string {
	if node == nil {
		return "unknown"
	}
	return node.Id
}

func RunManagementServer(server xds.Server) {
	lis, err := net.Listen("tcp", port)
	if err != nil {
		logrus.Fatalf("failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()

	logrus.Info("running management server")


	discovery.RegisterAggregatedDiscoveryServiceServer(grpcServer, server)
	v2.RegisterEndpointDiscoveryServiceServer(grpcServer, server)
	v2.RegisterClusterDiscoveryServiceServer(grpcServer, server)
	v2.RegisterRouteDiscoveryServiceServer(grpcServer, server)
	v2.RegisterListenerDiscoveryServiceServer(grpcServer, server)

	//gateway
	//go proxy.Call()

	if err := grpcServer.Serve(lis); err != nil {
		logrus.Fatalf("failed to serve: %v", err)
	}
}

func RunManagementGateway(srv xds.Server) {

	logrus.Info("running gateway")
	err := http.ListenAndServe(gatewayPort, &xds.HTTPGateway{Server: srv})
	if err != nil {
	}
}

func main() {

	//ctx := context.Background()
	config := cache.NewSnapshotCache(true, Hasher{}, logger{})
	//config := cache.NewSnapshotCache(false, hash{}, logger{})
	signal := make(chan struct{})
	srv := xds.NewServer(config, &callbacks{
		signal:   signal,
		fetches:  0,
		requests: 0,
	})



	go RunManagementServer(srv)
	go RunManagementGateway(srv)



	for {
		var clusterName= "service_google"
		var remoteHost= "www.google.com"
		var sni= "www.google.com"
		logrus.Infof(">>>>>>>>>>>>>>>>>>> creating cluster " + clusterName)

		//c := []cache.Resource{resource.MakeCluster(resource.Ads, clusterName)}
		/*h := &core.Address{Address: &core.Address_SocketAddress{
		SocketAddress: &core.SocketAddress{
			Address:  remoteHost,
			Protocol: core.TCP,
			PortSpecifier: &core.SocketAddress_PortValue{
				PortValue: uint32(443),
			},
		},
	}}*/

		//create cluster
		c := []cache.Resource{
			&v2.Cluster{
				Name:           clusterName,
				ConnectTimeout: 1 * time.Second,
				ClusterDiscoveryType: &v2.Cluster_Type{
					Type: v2.Cluster_LOGICAL_DNS,
				},
				DnsLookupFamily: v2.Cluster_V4_ONLY,
				LbPolicy:        v2.Cluster_ROUND_ROBIN,
				LoadAssignment: &v2.ClusterLoadAssignment{
					ClusterName: clusterName,
					Endpoints: []endpoint.LocalityLbEndpoints{
						{
							LbEndpoints: []endpoint.LbEndpoint{
								{
									HostIdentifier: &endpoint.LbEndpoint_Endpoint{
										Endpoint: &endpoint.Endpoint{
											Address: &core.Address{
												Address: &core.Address_SocketAddress{
													SocketAddress: &core.SocketAddress{
														Address: remoteHost,
														Protocol: core.TCP,
														PortSpecifier: &core.SocketAddress_PortValue{
															PortValue: uint32(443),
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
				TlsContext: &auth.UpstreamTlsContext{
					Sni: sni,
				},
			},
		}

		var listenerName= "listener_0"
		var targetHost= "www.google.com"
		var targetPrefix= "/"
		var virtualHostName= "local_service"
		var routeConfigName= "local_route"
		logrus.Infof(">>>>>>>>>>>>>>>>>>> creating listener " + listenerName)
		//virtual host (inside http filter)
		v := route.VirtualHost{
			Name:    virtualHostName,
			Domains: []string{"*"},
			Routes: []route.Route{{
				Match: route.RouteMatch{
					PathSpecifier: &route.RouteMatch_Prefix{
						Prefix: targetPrefix,
					},
				},
				Action: &route.Route_Route{
					Route: &route.RouteAction{
						HostRewriteSpecifier: &route.RouteAction_HostRewrite{
							HostRewrite: targetHost,
						},
						ClusterSpecifier: &route.RouteAction_Cluster{
							Cluster: clusterName,
						},
					},
				},
			}},
		}

		//http filter (inside listener)
		manager := &hcm.HttpConnectionManager{
			StatPrefix: "ingress_http",
			RouteSpecifier: &hcm.HttpConnectionManager_RouteConfig{
				RouteConfig: &v2.RouteConfiguration{
					Name:         routeConfigName,
					VirtualHosts: []route.VirtualHost{v},
				},
			},
			HttpFilters: []*hcm.HttpFilter{{
				Name: util.Router,
			}},
		}
		pbst, err := types.MarshalAny(manager)
		if err != nil {
			fmt.Println("yellow")
			panic(err)
		}
		fmt.Println(pbst.TypeUrl)

		//create listener
		var l = []cache.Resource{
			&v2.Listener{
				Name: listenerName,
				Address: core.Address{
					Address: &core.Address_SocketAddress{
						SocketAddress: &core.SocketAddress{
							Protocol: core.TCP,
							Address:  localhost,
							PortSpecifier: &core.SocketAddress_PortValue{
								PortValue: 10000,
							},
						},
					},
				},
				FilterChains: []listener.FilterChain{{
					Filters: []listener.Filter{{
						Name: util.HTTPConnectionManager,
						ConfigType: &listener.Filter_TypedConfig{
							TypedConfig: pbst,
						},
					}},
				}},
			}}

		atomic.AddInt32(&version, 1)
		//nodeId := config.GetStatusKeys()[1]

		logrus.Infof(">>>>>>>>>>>>>>>>>>> creating snapshot Version " + fmt.Sprint(version))
		snap := cache.NewSnapshot(fmt.Sprint(version), nil, c, nil, l)

		_ = config.SetSnapshot("jenny", snap)

		reader := bufio.NewReader(os.Stdin)
		_, _ = reader.ReadString('\n')

	}

}


