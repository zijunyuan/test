package lb

/*
go test -v lb_client_test.go lb_client.go etcd.go  strategy.go
*/

import (
	"context"
	"errors"
	"fmt"
	"git.garena.com/shopee/loan-service/airpay_backend/public/common/log"
	"git.garena.com/shopee/loan-service/airpay_backend/public/common/utils"
	"strconv"
	"strings"
	"time"

	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/etcdserver/api/v3rpc/rpctypes"
)

const (
	dialTimeout    = 5 * time.Second
	requestTimeout = 2 * time.Second

	sepFlag            = ":"
	leaseGrantTTL      = 10
	keepAliveHeartBeat = 3
	etcdPrefix         = "services"
)

//servicePrefix1:	       test-services:wallet_innerbalance.InnerBalanceGrpcService:100.98.25.87:31004
//servicePrefix2:     th-test-services:wallet_innerbalance.InnerBalanceGrpcService:100.98.25.87:31004
//servicePrefix3: apa-th-test-services:wallet_innerbalance.InnerBalanceGrpcService:100.98.25.87:31004
var (
	servicePrefix  string
	servicePrefix2 string
	servicePrefix3 string
	_stopHeartBeat bool
)

func newClientv3(endpoints []string) (c *clientv3.Client, err error) {
	return clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: dialTimeout,
	})
}

//NewETCDClientv3 return clientv3 of etcd
func NewETCDClientv3(endpoints []string, env string) (c *clientv3.Client, err error) {
	servicePrefix = fmt.Sprintf("%s-%s", env, etcdPrefix)
	servicePrefix, servicePrefix2, servicePrefix3 = conductServicePrefix()
	c, err = newClientv3(endpoints)
	return
}

//ServiceInfo info reported
type ServiceInfo struct {
	ServiceName string `json:"service_name"`
	IP          string `json:"ip"`
	Port        int    `json:"port"`
	Weight      int    `json:"weight"`
	StoreVal    int    `json:"store_val"`
}

//Service info for heartbeat
type Service struct {
	Info          *ServiceInfo
	StopHeartBeat chan struct{}
	LeaseID       clientv3.LeaseID
}

//GetServicePrefix return services prefix
func GetServicePrefix() string {
	return servicePrefix
}

//RegisterByEndpoints server register by etcd endpoints
func RegisterByEndpoints(endpoints []string, si *ServiceInfo) (*Service, error) {
	c, err := newClientv3(endpoints)
	if err != nil {
		return nil, err
	}

	leaseID, err := Grant(c)
	if err != nil {
		return nil, err
	}

	err = Put(c, si, leaseID)
	if err != nil {
		return nil, err
	}

	svr := &Service{
		Info:          si,
		StopHeartBeat: make(chan struct{}, 1),
		LeaseID:       leaseID,
	}

	go HeartBeat(c, svr)
	return svr, nil
}

func RegisterByClient(c *clientv3.Client, svr *Service) error {
	leaseID, err := Grant(c)
	if err != nil {
		return err
	}

	err = Put(c, svr.Info, leaseID)
	if err != nil {
		return err
	}
	fmt.Printf("[ETCD] grant new lease. new lease:%d, old lease:%d\n", leaseID, svr.LeaseID)
	svr.LeaseID = leaseID

	err = KeepAliveOnce(c, svr)
	return err
}

func getServiceByServicePrefix(c *clientv3.Client, identifier string, svcPrefix string) (resp []*ServiceInfo, err error) {
	result, err := getByServicePrefix(c, identifier, svcPrefix)
	if err != nil {
		return nil, err
	}
	resp = make([]*ServiceInfo, 0, len(result.Kvs))
	for _, ev := range result.Kvs {
		sarray := strings.Split(string(ev.Key), sepFlag)
		if 4 == len(sarray) {
			port, _ := strconv.Atoi(sarray[3])
			weight := 0
			storeVal, _ := strconv.Atoi(string(ev.Value))
			weight = storeVal
			s := &ServiceInfo{
				ServiceName: sarray[1],
				IP:          sarray[2],
				Port:        port,
				Weight:      weight,
				StoreVal:    storeVal,
			}
			resp = append(resp, s)
		}
	}
	return resp, nil
}

func appendServiceInfo(seriveArray []*ServiceInfo, etcdRsp *clientv3.GetResponse) []*ServiceInfo {
	for _, ev := range etcdRsp.Kvs {
		sarray := strings.Split(string(ev.Key), sepFlag)
		if 4 == len(sarray) {
			port, _ := strconv.Atoi(sarray[3])
			weight := 0
			storeVal, _ := strconv.Atoi(string(ev.Value))
			weight = storeVal
			s := &ServiceInfo{
				ServiceName: sarray[1],
				IP:          sarray[2],
				Port:        port,
				Weight:      weight,
				StoreVal:    storeVal,
			}
			seriveArray = append(seriveArray, s)
		}
	}
	return seriveArray
}

func GetService(c *clientv3.Client, identifier string) ([]*ServiceInfo, error) {
	etcdRsp1, err := getByServicePrefix(c, identifier, servicePrefix)
	if err != nil {
		log.Errorf("getServiceByServicePrefix error|identifier:%s|servicePrefix:%s|err:%s", identifier, servicePrefix, err.Error())
		return nil, err
	}

	etcdRsp2, err := getByServicePrefix(c, identifier, servicePrefix2)
	if err != nil {
		log.Errorf("getServiceByServicePrefix error|identifier:%s|servicePrefix:%s|err:%s", identifier, servicePrefix2, err.Error())
		return nil, err
	}
	retServiceArray := make([]*ServiceInfo, 0, len(etcdRsp1.Kvs)+len(etcdRsp2.Kvs))
	// 背景：不同国家的服务都会有一个servicePrefix，没有隔离的话就会被随机路由。
	// 非生产环境下，不同国家需要做服务隔离。etcdRsp2如果已经存在，直接使用本国家的，否则就使用etcdRsp1
	if !utils.IsProduct() && len(etcdRsp2.Kvs) > 0 {
		retServiceArray = appendServiceInfo(retServiceArray, etcdRsp2)
		return retServiceArray, nil
	}
	retServiceArray = appendServiceInfo(retServiceArray, etcdRsp1)
	retServiceArray = appendServiceInfo(retServiceArray, etcdRsp2)
	return retServiceArray, nil
}

//GetService return service by sid
func GetServiceOld(c *clientv3.Client, identifier string) (resp []*ServiceInfo, err error) {
	result, err := Get(c, identifier)
	if err != nil {
		return nil, err
	}
	resp = make([]*ServiceInfo, 0, len(result.Kvs))
	for _, ev := range result.Kvs {
		sarray := strings.Split(string(ev.Key), sepFlag)
		if 4 == len(sarray) {
			port, _ := strconv.Atoi(sarray[3])
			weight := 0
			storeVal, _ := strconv.Atoi(string(ev.Value))
			weight = storeVal
			s := &ServiceInfo{
				ServiceName: sarray[1],
				IP:          sarray[2],
				Port:        port,
				Weight:      weight,
				StoreVal:    storeVal,
			}
			resp = append(resp, s)
		}
	}
	return resp, nil
}

func Grant(c *clientv3.Client) (clientv3.LeaseID, error) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	resp, err := c.Grant(ctx, leaseGrantTTL)
	defer cancel()
	if err != nil {
		return 0, err
	}
	return resp.ID, nil
}

func Put(c *clientv3.Client, si *ServiceInfo, leaseID clientv3.LeaseID) error {
	if si == nil {
		return errors.New("[ETCD] invalid server info")
	}

	//key := fmt.Sprintf("%s:%s:%s:%d:%d", servicePrefix, si.ServiceName, si.IP, si.Port, si.Weight)
	//例如: test-services:wallet_innerbalance.InnerBalanceGrpcService:100.98.25.87:31004
	key1 := fmt.Sprintf("%s:%s:%s:%d", servicePrefix, si.ServiceName, si.IP, si.Port)
	key2 := fmt.Sprintf("%s:%s:%s:%d", servicePrefix2, si.ServiceName, si.IP, si.Port)
	key3 := fmt.Sprintf("%s:%s:%s:%d", servicePrefix3, si.ServiceName, si.IP, si.Port)

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	_, err := c.Put(ctx, key1, strconv.Itoa(si.Weight), clientv3.WithLease(leaseID))
	if err != nil {
		fmt.Printf("etcd put err|key1:"+key1+"\n", err.Error())
		log.Error("etcd put err|key1:"+key1, err)
	}

	ctx2, cancel2 := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel2()
	_, err = c.Put(ctx2, key2, strconv.Itoa(si.Weight), clientv3.WithLease(leaseID))
	if err != nil {
		fmt.Printf("etcd put err|key2:"+key2+"\n", err.Error())
		log.Error("etcd put err|key2:"+key2, err)

	}

	ctx3, cancel3 := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel3()
	_, err = c.Put(ctx3, key3, strconv.Itoa(si.Weight), clientv3.WithLease(leaseID))
	if err != nil {
		fmt.Printf("etcd put err|key3:"+key3+"\n", err.Error())
		log.Error("etcd put err|key3:"+key3, err)
	}

	return err
}

func KeepAliveOnce(c *clientv3.Client, s *Service) error {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	_, err := c.KeepAliveOnce(ctx, s.LeaseID)

	return err
}

//servicePrefix1:	       test-services:wallet_innerbalance.InnerBalanceGrpcService:100.98.25.87:31004
//servicePrefix2:     th-test-services:wallet_innerbalance.InnerBalanceGrpcService:100.98.25.87:31004
//servicePrefix3: apa-th-test-services:wallet_innerbalance.InnerBalanceGrpcService:100.98.25.87:31004
func conductServicePrefix() (servicePrefix1 string, servicePrefix2 string, servicePrefix3 string) {

	//key := fmt.Sprintf("%s:%s", servicePrefix, identifier)
	env := strings.ToLower(utils.GetEnv())       //dev or test or product  从环境变量 ENV 去取
	region := strings.ToLower(utils.GetRegion()) //从环境变量  去取

	appType := utils.GetAppType() //从环境变量  去取

	if env != "" {
		servicePrefix1 = fmt.Sprintf("%s-%s", env, etcdPrefix)
	}

	if region != "" && env != "" {
		servicePrefix2 = fmt.Sprintf("%s-%s-%s", region, env, etcdPrefix)

	}

	if appType != "" && region != "" && env != "" {
		servicePrefix3 = fmt.Sprintf("%s-%s-%s-%s", appType, region, env, etcdPrefix)
	}

	return
}

func getByServicePrefix(c *clientv3.Client, identifier string, svcPrefix string) (*clientv3.GetResponse, error) {
	key := fmt.Sprintf("%s:%s", svcPrefix, identifier)

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	result, err := c.Get(ctx, key, clientv3.WithPrefix())
	if err != nil {
		log.Errorf("etcd get key error|key:%s|%v", key, err)
		return nil, err
	}

	return result, nil
}

func Get(c *clientv3.Client, identifier string) (*clientv3.GetResponse, error) {
	//key := fmt.Sprintf("%s:%s", servicePrefix, identifier)
	key1 := fmt.Sprintf("%s:%s", servicePrefix, identifier)
	key2 := fmt.Sprintf("%s:%s", servicePrefix2, identifier)
	//key3 := fmt.Sprintf("%s:%s", servicePrefix3, identifier)

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	result, err := c.Get(ctx, key2, clientv3.WithPrefix()) //先读 key2的，key2 有就直接返回
	if err != nil {
		fmt.Printf("etcd try to get key2 error,then try to get key1|key2:"+key2+"\n", err)

		log.Warn("etcd try to get key2 error,then try to get key1|key2:"+key2, err)
	} else if result.Count > 0 {
		return result, nil
	}

	ctx2, cancel2 := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel2()

	result, err = c.Get(ctx2, key1, clientv3.WithPrefix())
	if err != nil {
		log.Error("etcd get key1 error|key1:"+key1, err)
		fmt.Printf("etcd get key1 error|key1:"+key1+"\n", err)
		return nil, err
	}

	return result, nil
}

func genHeartBeatMsg(what string, serviceName string, err interface{}) string {
	msg := fmt.Sprintf("[ETCD][heartbeat]|what:%v|serviceName:%v|err:%v", what, serviceName, err)
	return msg
}

//HeartBeat server heart beat
func HeartBeat(c *clientv3.Client, s *Service) {
	defer func() {
		if err := recover(); err != nil {
			msg := genHeartBeatMsg("panic", s.Info.ServiceName, err)
			fmt.Println(msg)
			log.Errorf(msg)
			// panic 导致的才需要重启这个函数
			go HeartBeat(c, s)
		}
	}()
	ticker := time.NewTicker(time.Duration(keepAliveHeartBeat) * time.Second)
	defer ticker.Stop()
	for {
		if _stopHeartBeat {
			c.Close()
			msg := genHeartBeatMsg("StopHeartBeat by Flag", s.Info.ServiceName, _stopHeartBeat)
			fmt.Println(msg)
			log.Warnf(msg)
			return
		}
		select {
		//case <-time.After(time.Duration(keepAliveHeartBeat) * time.Second):
		case <-ticker.C:
			err := KeepAliveOnce(c, s)
			if err != nil {
				msg := genHeartBeatMsg("KeepAliveOnce error", s.Info.ServiceName, err)
				fmt.Println(msg)
				log.Error(msg)

				if err.Error() == rpctypes.ErrorDesc(rpctypes.ErrLeaseNotFound) {
					msg := genHeartBeatMsg("ErrLeaseNotFound and do RegisterByClient again", s.Info.ServiceName, err)
					fmt.Println(msg)
					log.Warn(msg)

					RegisterByClient(c, s)
				} else if err.Error() == rpctypes.ErrorDesc(rpctypes.ErrTimeoutDueToConnectionLost) {
					newClient, err := newClientv3(c.Endpoints())
					if err == nil {
						err = c.Close()
						if err != nil {
							msg := genHeartBeatMsg("close old etcd client error", s.Info.ServiceName, err)
							fmt.Println(msg)
							log.Warn(msg)
						}
						c = newClient
						RegisterByClient(c, s)
					} else {
						msg := genHeartBeatMsg("new etcd clientv3 error", s.Info.ServiceName, err)
						fmt.Println(msg)
						log.Error(msg)
					}
				} else {
					msg := genHeartBeatMsg("KeepAliveOnce other error", s.Info.ServiceName, err)
					fmt.Println(msg)
					log.Error(msg)

				}
			}
		case <-s.StopHeartBeat:
			c.Close()
			msg := genHeartBeatMsg("StopHeartBeat by channel", s.Info.ServiceName, "")
			fmt.Println(msg)
			log.Error(msg)

			return
		}
	}
}

//WatchServices watch server to detect changes, non break, you need to rewriter watch services
func WatchServices(c *clientv3.Client, serviceNames ...string) {
	if len(serviceNames) == 0 {
		return
	}
	for _, sn := range serviceNames {
		rch := c.Watch(context.Background(), fmt.Sprintf("%s:", sn), clientv3.WithPrefix())
		for wresp := range rch {
			for _, ev := range wresp.Events {
				//mostly, you should do your own logicals. below just a example to handler the changes
				fmt.Printf("[ETCD] %s %q : %q\n", ev.Type, ev.Kv.Key, ev.Kv.Value)
			}
		}
	}
}
