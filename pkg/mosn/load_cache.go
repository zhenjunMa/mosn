package mosn

import (
	"encoding/json"
	"net"

	"mosn.io/mosn/pkg/admin/store"
	"mosn.io/mosn/pkg/api/v2"
	"mosn.io/mosn/pkg/config"
	"mosn.io/mosn/pkg/log"
	"mosn.io/mosn/pkg/router"
	"mosn.io/mosn/pkg/server"
	"mosn.io/mosn/pkg/types"
	"mosn.io/mosn/pkg/upstream/cluster"
)

func LoadCacheConfig(inheritListeners []net.Listener) {
	b, err := store.ReadCacheConfig()
	if err != nil {
		log.DefaultLogger.Infof("load cache file failed, error: %v", err)
		return
	}
	ecfg := &store.EffectiveConfig{}
	if err := json.Unmarshal(b, ecfg); err != nil {
		log.DefaultLogger.Errorf("unmarshal cache config failed, error: %v", err)
		return
	}
	// add listener
	for _, listenerConfig := range ecfg.Listener {
		lc := config.ParseListenerConfig(&listenerConfig, inheritListeners)
		var nfcf []types.NetworkFilterChainFactory
		var sfcf []types.StreamFilterChainFactory

		// Note: as we use fasthttp and net/http2.0, the IO we created in mosn should be disabled
		// network filters
		if !lc.UseOriginalDst {
			// network and stream filters
			nfcf = config.GetNetworkFilters(&lc.FilterChains[0])
			sfcf = config.GetStreamFilters(lc.StreamFilters)
		}

		if err := server.GetListenerAdapterInstance().AddOrUpdateListener("", lc, nfcf, sfcf); err != nil {
			log.DefaultLogger.Fatalf("[mosn] [NewMosn] AddListener error:%s", err.Error())
		}
	}
	// add router
	mng := router.GetRoutersMangerInstance()
	for _, rc := range ecfg.Routers {
		if err := mng.AddOrUpdateRouters(&rc); err != nil {
			log.DefaultLogger.Errorf("add router config failed, error: %v", err)
		}
	}
	var clusters []v2.Cluster
	for _, c := range ecfg.Cluster {
		clusters = append(clusters, c)
	}
	cs, cMap := config.ParseClusterConfig(clusters)
	adapter := cluster.GetClusterMngAdapterInstance()
	// Add Cluster
	for _, cluster := range cs {
		if err := adapter.AddOrUpdatePrimaryCluster(cluster); err != nil {
			log.DefaultLogger.Errorf("AddOrUpdatePrimaryCluster failure, cluster name = %s, error: %v", cluster.Name, err)
		}
	}
	// Add Host
	for clusterName, hosts := range cMap {
		if err := adapter.UpdateClusterHosts(clusterName, hosts); err != nil {
			log.DefaultLogger.Errorf("adapter.UpdateClusterHosts failure, cluster name = %s, error: %v", clusterName, err)
		}
	}
}
