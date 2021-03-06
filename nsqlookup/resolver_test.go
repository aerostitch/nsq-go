package nsqlookup

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestResolveServers(t *testing.T) {
	tests := []struct {
		servers Servers
		results []string
	}{
		{
			servers: nil,
			results: nil,
		},
		{
			servers: Servers{},
			results: nil,
		},
		{
			servers: Servers{"A"},
			results: []string{"A"},
		},
		{
			servers: Servers{"A", "B"},
			results: []string{"A", "B"},
		},
		{
			servers: Servers{"A", "B", "C"},
			results: []string{"A", "B", "C"},
		},
	}

	for _, test := range tests {
		t.Run(strings.Join(test.results, ","), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			res, err := test.servers.Resolve(ctx)

			if err != nil {
				t.Error(err)
			}

			if !reflect.DeepEqual(res, test.results) {
				t.Error(res)
			}

			cancel()
			_, err = test.servers.Resolve(ctx)
			if err == nil {
				t.Error("bad error after the context was canceled:", err)
			}
		})
	}
}

func TestResolveCached(t *testing.T) {
	servers := Servers{
		"A",
		"B",
		"C",
	}

	miss := 0
	rslv := &CachedResolver{
		Resolver: ResolverFunc(func(ctx context.Context) ([]string, error) {
			miss++
			return servers.Resolve(ctx)
		}),
		Timeout: 10 * time.Millisecond,
	}

	ctx, cancel := context.WithCancel(context.Background())

	for i := 0; i != 3; i++ {
		for j := 0; j != 10; j++ {
			res, err := rslv.Resolve(ctx)

			if err != nil {
				t.Error(err)
			}

			if !reflect.DeepEqual(res, ([]string)(servers)) {
				t.Error(res)
			}
		}

		if miss != (i + 1) {
			t.Error("too many cache misses:", miss)
		}

		// Sleep for a little while so the cache entry expires.
		time.Sleep(20 * time.Millisecond)
	}

	cancel()
	_, err := rslv.Resolve(ctx)
	if err == nil {
		t.Error("bad error after the context was canceled:", err)
	}
}

func TestResolveConsul(t *testing.T) {
	type ServiceResultNode struct {
		Node    string
		Address string
	}

	type ServiceResultService struct {
		Address string
		Port    int
	}

	type ServiceResult struct {
		Node    ServiceResultNode
		Service ServiceResultService
	}

	server := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/v1/health/service/nsqlookupd" {
			json.NewEncoder(res).Encode([]ServiceResult{
				{
					Node: ServiceResultNode{
						Node:    "A",
						Address: "127.0.0.1",
					},
					Service: ServiceResultService{
						Address: "",
						Port:    4242,
					},
				},
				{
					Node: ServiceResultNode{
						Node:    "B",
						Address: "192.168.0.20",
					},
					Service: ServiceResultService{
						Address: "192.168.0.1",
						Port:    4161,
					},
				},
				{
					Node: ServiceResultNode{
						Node:    "C",
						Address: "192.168.0.2",
					},
					Service: ServiceResultService{
						Address: "",
						Port:    4161,
					},
				},
			})
		} else {
			t.Error("bad URL path:", req.URL.Path)
		}
		res.Header().Set("Content-Type", "application/json; charset=utf-8")
	}))
	defer server.Close()

	rslv := &ConsulResolver{
		Address: server.URL,
	}

	ctx, cancel := context.WithCancel(context.Background())

	res, err := rslv.Resolve(ctx)
	if err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(res, []string{
		"127.0.0.1:4242",
		"192.168.0.1:4161",
		"192.168.0.2:4161",
	}) {
		t.Error(res)
	}

	cancel()
	_, err = rslv.Resolve(ctx)
	if err == nil {
		t.Error("bad error after the context was canceled:", err)
	}
}

func TestResolveMulti(t *testing.T) {
	rslv := MultiResolver(
		Servers{},
		Servers{"A"},
		Servers{"B", "C"},
	)

	res, err := rslv.Resolve(nil)
	sort.Strings(res)

	if err != nil {
		t.Error(err)
	}

	if !reflect.DeepEqual(res, []string{"A", "B", "C"}) {
		t.Error(res)
	}
}
