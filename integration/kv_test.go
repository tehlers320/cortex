// +build requires_docker

package main

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cortexproject/cortex/integration/e2e"
	e2edb "github.com/cortexproject/cortex/integration/e2e/db"
	"github.com/cortexproject/cortex/pkg/ring/kv"
	"github.com/cortexproject/cortex/pkg/ring/kv/consul"
	"github.com/cortexproject/cortex/pkg/ring/kv/etcd"
)

func TestKV_List_Delete(t *testing.T) {
	s, err := e2e.NewScenario(networkName)
	require.NoError(t, err)
	defer s.Close()

	// Start dependencies
	etcdSvc := e2edb.NewETCD()
	consulSvc := e2edb.NewConsul()

	require.NoError(t, s.StartAndWaitReady(etcdSvc, consulSvc))

	etcdKv, err := kv.NewClient(kv.Config{
		Store:  "etcd",
		Prefix: "keys/",
		StoreConfig: kv.StoreConfig{
			Etcd: etcd.Config{
				Endpoints:   []string{etcdSvc.HTTPEndpoint()},
				DialTimeout: time.Minute,
				MaxRetries:  5,
			},
		},
	}, stringCodec{})
	require.NoError(t, err)

	consulKv, err := kv.NewClient(kv.Config{
		Store:  "consul",
		Prefix: "keys/",
		StoreConfig: kv.StoreConfig{
			Consul: consul.Config{
				Host:              consulSvc.HTTPEndpoint(),
				HTTPClientTimeout: time.Minute,
				WatchKeyBurstSize: 5,
				WatchKeyRateLimit: 1,
			},
		},
	}, stringCodec{})
	require.NoError(t, err)

	kvs := []struct {
		name string
		kv   kv.Client
	}{
		{"etcd", etcdKv},
		{"consul", consulKv},
	}

	for _, kv := range kvs {
		t.Run(kv.name+"_list", func(t *testing.T) {
			// Create keys to list back
			keysToCreate := []string{"key-a", "key-b", "key-c"}
			for _, key := range keysToCreate {
				err := kv.kv.CAS(context.Background(), key, func(in interface{}) (out interface{}, retry bool, err error) {
					return key, false, nil
				})
				require.NoError(t, err, "could not create key")
			}

			// Get list of keys and sort them
			keys, err := kv.kv.List(context.Background(), "")
			require.NoError(t, err, "could not list keys")
			sort.Strings(keys)
			require.Equal(t, keysToCreate, keys, "returned key paths did not match created paths")
		})

		t.Run(kv.name+"_delete", func(t *testing.T) {
			// Create a key
			err = kv.kv.CAS(context.Background(), "key-to-delete", func(in interface{}) (out interface{}, retry bool, err error) {
				return "key-to-delete", false, nil
			})
			require.NoError(t, err, "object could not be created")

			// Now delete it
			err = kv.kv.Delete(context.Background(), "key-to-delete")
			require.NoError(t, err)

			// Get it back
			v, err := kv.kv.Get(context.Background(), "key-to-delete")
			require.NoError(t, err, "unexpected error")
			require.Nil(t, v, "object was not deleted")
		})
	}
}

type stringCodec struct{}

func (c stringCodec) Decode(bb []byte) (interface{}, error) { return string(bb), nil }
func (c stringCodec) Encode(v interface{}) ([]byte, error)  { return []byte(v.(string)), nil }
func (c stringCodec) CodecID() string                       { return "stringCodec" }
