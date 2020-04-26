package cluster

import (
	"github.com/stretchr/testify/assert"
	"mosn.io/mosn/pkg/types"
	"testing"
)

func TestNewLARBalancer(t *testing.T) {
	balancer := NewLoadBalancer(&clusterInfo{lbType: types.LeastActiveRequest}, &hostSet{})
	assert.NotNil(t, balancer)
	assert.IsType(t, &leastActiveRequestLoadBalancer{}, balancer)
}

func TestLARChooseHost(t *testing.T) {
	hosts := createHostsetWithStats(exampleHostConfigs(), "test")
	balancer := NewLoadBalancer(&clusterInfo{lbType: types.LeastActiveRequest}, hosts)
	host := balancer.ChooseHost(newMockLbContext(nil))
	assert.NotNil(t, host)

	for _, host := range hosts.Hosts() {
		mockRequest(host, true, 10)
	}
	// new lb to refresh edf
	balancer = NewLoadBalancer(&clusterInfo{lbType: types.LeastActiveRequest}, hosts)
	actual := balancer.ChooseHost(newMockLbContext(nil))
	assert.Equal(t, hosts.allHosts[6], actual)
	actual = balancer.ChooseHost(newMockLbContext(nil))

	// test only one host
	h := exampleHostConfigs()[0:1]
	hosts = createHostsetWithStats(h, "test")
	balancer = NewLoadBalancer(&clusterInfo{lbType: types.LeastActiveRequest}, hosts)
	actual = balancer.ChooseHost(nil)
	assert.Equal(t, hosts.allHosts[0], actual)

	// test no host
	h = exampleHostConfigs()[0:0]
	hosts = createHostsetWithStats(h, "test")
	balancer = NewLoadBalancer(&clusterInfo{lbType: types.LeastActiveRequest}, hosts)
	actual = balancer.ChooseHost(nil)
	assert.Nil(t, actual)

}

func mockRequest(host types.Host, active bool, times int) {
	for i := 0; i < times; i++ {
		if active {
			host.HostStats().UpstreamRequestActive.Inc(1)
		}
	}
}
