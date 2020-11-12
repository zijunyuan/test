## 功能说明

lb包实现了服务发现，负载均衡的功能。其对外提供了两个简易接口供调用：
1.服务注册；2.均衡获取某服务的地址。

## 使用示例

1.注册服务

```javascript
var lbClient lb.LbClient
// Init参数含义：etcd集群列表
err = lbClient.Init([]string{"10.12.77.132:2379"})
if err != nil {
    //do something
}   
// Register参数含义：服务名；端口号；权重
err = lbClient.Register("test_svr", 27072, 10) 
if err != nil {
    //do something
}  
```

2.获取服务地址（即ip:port)

```javascript
var lbClient lb.LbClient
// Init参数含义：etcd集群列表
err = lbClient.Init([]string{"10.12.77.132:2379"})
if err != nil {
    //do something
}  
// GetAddress参数含义：服务名
address, err := lbClient.GetAddress("test_svr")
if err != nil {
    //do something
} 
```
## 使用建议

1. 在全局使用lb.LbClient对象，不要每次都重新生成；
2. 维护两个lb.LbClient对象，一个用于注册，一个用于获取其他服务地址；
3. 权重值不要过大，1-100即可，如果一开始权重一样，那么可以都填1。