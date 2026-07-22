package service

import (
	"net/http"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetHttpClientWithProxyConcurrentRequestsShareClient(t *testing.T) {
	ResetProxyClientCache()
	t.Cleanup(ResetProxyClientCache)

	const goroutines = 16
	clients := make([]*http.Client, goroutines)
	errors := make([]error, goroutines)
	var waitGroup sync.WaitGroup
	waitGroup.Add(goroutines)
	for index := range goroutines {
		go func(index int) {
			defer waitGroup.Done()
			clients[index], errors[index] = GetHttpClientWithProxy("http://proxy.example:8080")
		}(index)
	}
	waitGroup.Wait()

	for index := range goroutines {
		require.NoError(t, errors[index])
		assert.Same(t, clients[0], clients[index])
	}
}

func TestProxyClientCacheCanonicalizationAndInvalidation(t *testing.T) {
	ResetProxyClientCache()
	t.Cleanup(ResetProxyClientCache)

	first, err := GetHttpClientWithProxy("http://proxy.example:8080/")
	require.NoError(t, err)
	canonical, err := GetHttpClientWithProxy("http://proxy.example:8080")
	require.NoError(t, err)
	other, err := GetHttpClientWithProxy("http://other.example:8080")
	require.NoError(t, err)
	assert.Same(t, first, canonical)

	InvalidateProxyClient("http://proxy.example:8080/legacy")
	recreated, err := GetHttpClientWithProxy("http://proxy.example:8080")
	require.NoError(t, err)
	otherAfter, err := GetHttpClientWithProxy("http://other.example:8080")
	require.NoError(t, err)
	assert.NotSame(t, first, recreated)
	assert.Same(t, other, otherAfter)
}

func TestProxyClientHonorsZeroRelayTimeout(t *testing.T) {
	ResetProxyClientCache()
	t.Cleanup(ResetProxyClientCache)
	originalRelayTimeout := common.RelayTimeout
	common.RelayTimeout = 0
	t.Cleanup(func() { common.RelayTimeout = originalRelayTimeout })

	client, err := GetHttpClientWithProxy("socks5://proxy.example")

	require.NoError(t, err)
	assert.Zero(t, client.Timeout)
}
