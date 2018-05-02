# kvl - K/V Store Layer

It's an abstract layer for common operations of K/V stores like:

1. consul
2. etcd

TODO to support more backends could be a scheduling plan.

## LICENSE

MIT for free.

## Usages

Main interface is lied in [`store.KVStore`](store/store.go#L35)  

We make new store.KVStore with `kvl.New()` call:

```go

store := kvl.New(&consul_util.ConsulConfig{
    Scheme: "http",
    Addr: "127.0.0.1:8500",
    Insecure: true,
    CertFile: "", KeyFile: "", CACertFile: "",
    Username: "", Password: "",
    Root: "",
    DeregisterCriticalServiceAfter: "30s",
})
defer store.Close() // optional

// or
storeEtcd := kvl.New(
    &etcd.Etcdtool{
        Peers: "127.0.0.1:2379",
        Cert: "",
        Key: "", CA: "", User: "",
        Timeout: time.Second * 10,
        CommandTimeout: time.Second * 5,
        Routes: []etcd.Route{},
        PasswordFilePath: "",
        Root: "",
    }
)
defer storeEtcd.Close() // optional

```

Once `store` got, we can get/set key/value pair:

```go
store.Put("x", "yz")
log.Info(store.Get("x"))

// Or, with yaml encode/decode transparently
state := map[string]string{
    "ab": "111",
    "cd": "222",
}
store.PutYaml("state", state)
state1 := store.GetYaml("state")
log.Infof("state1: %v", state1)

// And, with path separatly
store.Put("config/gwapi/enabled", "true")

// And, delete them:
store.Delete("x")
store.Delete("state")
store.Delete("config/gwapi")
```

For more usages, see also `consul/core_test.go` and `etcd/i_test.go`, ....


## About origin of `kvl`

`kvl` 本是 gwkool 的一部分，由于通用性还算是好，所以
拆分出来做一次纯粹的分享。不过目前仅仅是简单发布一下，
并未最优化相应的代码，在工程实践中可以使用它来简化
kvstore 的访问，但如果你并没有通用支撑的需求的话，或
许直接使用 consul 和 etcd 的 api 才是简单的。

`kvl` 暂未考虑专门做一个完整全面的 kvstore 抽象层。
一方面是因为足够全面的抽象比较困难，每个 kvstore 的实
现都有它们的特点，难于通用化地抽象出来。另一方面是因为
多数工程上的需求，似乎 `kvl` 马马虎虎可以满足了。
或许我会考虑使用 `kvlayer` 来发布一个折中后的全面的版本。

