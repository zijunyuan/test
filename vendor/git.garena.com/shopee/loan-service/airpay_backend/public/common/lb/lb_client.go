package lb

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	log "github.com/cihub/seelog"
	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/mvcc/mvccpb"
	"github.com/gansidui/go-utils/safemap"
)

type LbClient struct {
	SvrInfo       map[string]map[string]int //first map for servername, second map for every ip
	regInfo       *safemap.SafeMap          //这两个map是有冗余的,由于历史原因暂不优化
	Strategy      LbIntf
	SvrLock       *sync.RWMutex
	etcdHandler   *clientv3.Client
	etcdEndPoints []string

	self *Service //解决同一个服务以相同的配置（serverName+ip+port 相同）先后进行启动导致会删除etcd中的注册信息
}

func (p *LbClient) GetEtcdClient() *clientv3.Client {
	return p.etcdHandler
}

func (p *LbClient) Init(endpoints []string, env string) error {
	p.SvrInfo = make(map[string]map[string]int, 0)
	var smooth SmoothLb
	p.Strategy = &smooth
	p.etcdEndPoints = endpoints
	p.SvrLock = new(sync.RWMutex)
	p.regInfo = safemap.New()
	var err error
	p.etcdHandler, err = NewETCDClientv3(endpoints, env)
	if err != nil {
		return err
	}

	p.syncEtcd("")
	p.Watch("")

	go p.syncPeriod()

	return nil
}

func (p *LbClient) UnRegister(serviceName string, port int) error {
	p.StopHeartBeat()

	c := p.etcdHandler

	ip, err := GetIntranetIp()
	if nil != err {
		return err
	}

	key1 := fmt.Sprintf("%s:%s:%s:%d", servicePrefix, serviceName, ip, port)
	key2 := fmt.Sprintf("%s:%s:%s:%d", servicePrefix2, serviceName, ip, port)
	//key3 := fmt.Sprintf("%s:%s", servicePrefix3, identifier)

	ctx1, cancel1 := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel1()

	_, err = c.Delete(ctx1, key1, clientv3.WithPrefix())
	if err != nil {
		log.Error("etcd Delete key1 error|key1:"+key1, err)
		//return err
	}

	ctx2, cancel2 := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel2()

	_, err = c.Delete(ctx2, key2, clientv3.WithPrefix())
	if err != nil {
		log.Error("etcd Delete key2 error|key2:"+key2, err)
		return err
	}

	return nil
}

func (p *LbClient) Register(serviceName string, srvPort int, weight int) error {
	srvIp, err := GetIntranetIp()
	if err != nil {
		log.Infof("[ETCD] get intranet ip error : %v", err)
		return err
	}

	s := new(ServiceInfo)
	s.ServiceName = serviceName
	s.IP = srvIp
	s.Port = srvPort
	s.Weight = weight

	service, err := p.register(p.etcdEndPoints, s)
	if err == nil {
		p.self = service
	}

	return err
}

func (p *LbClient) register(endPoints []string, svrInfo *ServiceInfo) (*Service, error) {
	_stopHeartBeat = false //这个要关掉
	service, err := RegisterByEndpoints(endPoints, svrInfo)
	if err != nil {
		log.Criticalf("[ETCD] etcd register error : %v", err)
		return nil, err
	}

	log.Infof("[ETCD] etcd end register success")
	return service, nil
}

func (p *LbClient) GetAddress(serverName string) (string, error) {
	var currentWeight map[string]int

	p.SvrLock.RLock()
	if _, ok := p.SvrInfo[serverName]; ok {
		//深拷贝
		currentWeight = make(map[string]int)
		for k, v := range p.SvrInfo[serverName] {
			currentWeight[k] = v
		}
	}
	p.SvrLock.RUnlock()

	if currentWeight == nil {
		p.SvrLock.Lock()
		currentWeight = make(map[string]int, 0)
		p.SvrInfo[serverName] = make(map[string]int, 0)
		p.SvrLock.Unlock()
	}

	confWeight := make(map[string]int, 0)
	val, ok := p.regInfo.Get(serverName)
	if ok {
		items := val.([]*ServiceInfo)
		for _, item := range items {
			svrAddress := fmt.Sprintf("%s:%d", item.IP, item.Port)
			confWeight[svrAddress] = item.Weight
		}
	} else {
		log.Errorf("[ETCD] get conf form mem fail. invalid serverName:%s", serverName)

		items := p.regInfo.Items()

		for k, v := range items {

			log.Warnf("[ETCD] print item k=%v,v=%v", k, v)

			fmt.Printf("%s=%d;", k, v)
		}

		for k := range currentWeight {
			confWeight[k] = 0
		}
	}

	if currentWeight == nil || len(currentWeight) == 0 {
		log.Warnf("[ETCD] currentWeight is nil or len is zero.currentWeight:%v|serverName:%s", currentWeight, serverName)
	}

	if confWeight == nil || len(confWeight) == 0 {
		log.Warnf("[ETCD] confWeight is nil or len is zero.confWeight:%v|serverName:%s", confWeight, serverName)
	}

	newWeight, address := p.Strategy.GetAddress(currentWeight, confWeight)
	p.SvrLock.Lock()
	p.SvrInfo[serverName] = newWeight
	p.SvrLock.Unlock()

	if address == "" {
		log.Errorf("[ETCD] no instance found, please register server first.currentWeight:%v,confWeight:%v|serverName:%s", currentWeight, confWeight, serverName)
		return "", errors.New("[ETCD] no instance found, please register server first.serverName:" + serverName)
	}

	return address, nil
}

// RemoveAddress 提供一个将缓存在内存中的指定serviceName以及ip地址的地址删除的功能，可以在发现目标地址不可用时调用。
func (p *LbClient) RemoveAddress(serviceName string, addr string) {
	p.SvrLock.Lock()
	defer p.SvrLock.Unlock()
	if addrMap, ok := p.SvrInfo[serviceName]; ok {
		delete(addrMap, addr)
	}

	if val, ok := p.regInfo.Get(serviceName); ok {
		existedServices := val.([]*ServiceInfo)
		newServices := make([]*ServiceInfo, 0, len(existedServices))
		for _, s := range existedServices {
			if fmt.Sprintf("%s:%d", s.IP, s.Port) != addr {
				newServices = append(newServices, s)
			}
		}
		p.regInfo.Set(serviceName, newServices)
	}

}

func (p *LbClient) GetServerInfo(serverName string) ([]*ServiceInfo, error) {
	return GetService(p.etcdHandler, serverName)
}

func IsIntranetIP(IP net.IP) bool {
	if IP.IsLoopback() {
		return false
	}

	intranetCIDRs := []string{
		"192.168.0.0/16",
		"172.16.0.0/12",
		"100.64.0.0/10", // This is preserverd for carrier NAT
		"10.0.0.0/8",
	}

	for _, cidr := range intranetCIDRs {
		_, ipNet, _ := net.ParseCIDR(cidr)
		if ipNet.Contains(IP) {
			return true
		}
	}

	return false
}

func GetIntranetIp() (string, error) {
	ip := ""
	addrs, err := net.InterfaceAddrs()

	if err != nil {
		return ip, err
	}

	for _, address := range addrs {

		// 检查ip地址判断是否回环地址
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if IsIntranetIP(ipnet.IP) {
				ip = ipnet.IP.String()
				break
			}
		}
	}

	return ip, nil
}

//以下部分用于监听机制的处理
func (p *LbClient) Watch(serverName string) {
	serverName = strings.TrimSpace(serverName)
	var key string
	var key2 string
	if serverName == "" {
		key = servicePrefix + ":"
	} else {
		key = servicePrefix + ":" + serverName
	}

	if serverName == "" {
		key2 = servicePrefix2 + ":"
	} else {
		key2 = servicePrefix2 + ":" + serverName
	}

	// ctx, cancel := context.WithCancel(context.Background())
	rch := p.etcdHandler.Watch(context.Background(), key, clientv3.WithPrefix())
	// cancel()
	go p.watchEvents(rch)

	rch2 := p.etcdHandler.Watch(context.Background(), key2, clientv3.WithPrefix())
	go p.watchEvents(rch2)
}

func (p *LbClient) watchEvents(rch clientv3.WatchChan) {
	for wresp := range rch {
		for _, ev := range wresp.Events {
			info := strings.Split(string(ev.Kv.Key), ":")
			if len(info) < 4 {
				log.Warnf("[ETCD] invalid change. key:%s", ev.Kv.Key)
				continue
			}
			s := &ServiceInfo{
				ServiceName: info[1],
				IP:          info[2],
			}
			s.Port, _ = strconv.Atoi(info[3])
			s.StoreVal, _ = strconv.Atoi(string(ev.Kv.Value))
			s.Weight = s.StoreVal
			log.Infof("[ETCD] etcd-watchEvents. type:%v, serviceName:%s, ip:%s, port:%d", ev.Type, s.ServiceName, s.IP, s.Port)
			switch ev.Type {
			case mvccpb.PUT:
				p.put(s)
			case mvccpb.DELETE:
				parts := strings.Split(info[0], "-")
				p.del(s)
				if len(parts) == 2 {
					//只有 "test-services" 这种前缀的才走 reRegisterInWatch 流程 by dahe.lai @2019-07-02
					//否则会产生 StopHeartBeat channel 的竞争导致崩溃
					p.reRegisterInWatch(s)
				}
			}
		}
	}
}

//called when self info deleted in etcd, only take effect when server has registered in etcd
func (p *LbClient) reRegisterInWatch(delItem *ServiceInfo) {
	if p.self != nil {
		if !equalServerInfo(delItem, p.self.Info) {
			return
		}

		if _stopHeartBeat { //如果已经停止心跳了就不注册自己了
			return
		}

		log.Infof(fmt.Sprintf("[ETCD] register server(%s) when delete in watcher", delItem.ServiceName))
		p.reRegister()
	}
}

func (p *LbClient) StopHeartBeat() {
	if _stopHeartBeat {
		// 若已经关闭心跳则直接返回，否则将造成多次关闭channel而引发崩溃，
		// 典型的场景是一个grpc server包含多个service，依次UnRegister的时候将多次调用StopHeartBeat。
		// Tips: 此处的stopHeartBeat变量在多个goroutine中未进行同步，目前使用暂时没问题。
		return
	}
	_stopHeartBeat = true //先标记为stop,下面再发信号,两者顺序不能变
	p.self.StopHeartBeat <- struct{}{}
	close(p.self.StopHeartBeat)
}

//called when self info deleted in etcd by watcher
func (p *LbClient) reRegister() {
	//shut done previous heartBeat goroutines
	p.self.StopHeartBeat <- struct{}{}
	close(p.self.StopHeartBeat)

	//register self in etcd
	for retry := 0; retry < 3; retry++ {
		service, err := p.register(p.etcdEndPoints, p.self.Info)
		if err == nil {
			p.self = service
			break
		}
	}
}

func equalServerInfo(delItem, self *ServiceInfo) bool {
	return delItem.ServiceName == self.ServiceName && delItem.IP == self.IP && delItem.Port == self.Port
}

func (p *LbClient) put(s *ServiceInfo) {
	if s == nil {
		return
	}

	defer p.SvrLock.Unlock()
	p.SvrLock.Lock()

	if _, ok := p.SvrInfo[s.ServiceName]; !ok {
		p.SvrInfo[s.ServiceName] = make(map[string]int, 0)
	}
	svrAddress := fmt.Sprintf("%s:%d", s.IP, s.Port)
	p.SvrInfo[s.ServiceName][svrAddress] = s.Weight

	var regs []*ServiceInfo
	if val, ok := p.regInfo.Get(s.ServiceName); ok {
		regs = val.([]*ServiceInfo)
	} else {
		regs = make([]*ServiceInfo, 0, 10)
	}
	regs = append(regs, s)
	p.regInfo.Set(s.ServiceName, regs)
}

func (p *LbClient) del(s *ServiceInfo) {
	if s == nil {
		return
	}

	defer p.SvrLock.Unlock()
	p.SvrLock.Lock()

	if _, ok := p.SvrInfo[s.ServiceName]; ok {
		svrAddress := fmt.Sprintf("%s:%d", s.IP, s.Port)
		delete(p.SvrInfo[s.ServiceName], svrAddress)
	}

	var regs []*ServiceInfo
	if val, ok := p.regInfo.Get(s.ServiceName); ok {
		regs = val.([]*ServiceInfo)
		regs = delFromSlice(regs, s.IP, s.Port)
		p.regInfo.Set(s.ServiceName, regs) //解决 p.regInfo.Get(s.ServiceName) 里面的数组删不掉的问题
	}
}

func delFromSlice(old []*ServiceInfo, ip string, port int) []*ServiceInfo {
	index := -1
	for i, dt := range old {
		if dt.IP == ip && dt.Port == port {
			index = i
			break
		}
	}

	if index == -1 {
		return old
	}

	if index == 0 {
		return old[1:]
	}

	length := len(old)
	if index == length {
		return old[:length-1]
	}

	old[index] = old[length-1]
	new := old[0 : length-1]
	return new

	/*
		new := make([]*ServiceInfo, 0, length-1)
		new = append(new, old[:index]...)
		new = append(new, old[index+1:]...)
		return new
	*/
}

func (p *LbClient) syncEtcd(serverName string) {
	resp, err := GetService(p.etcdHandler, serverName)
	if err != nil {
		log.Criticalf("[ETCD] sync etcd info error : %v", err)
		return
	}

	svrAddress := ""
	svrs := make(map[string]map[string]int)
	regs := safemap.New()

	for _, s := range resp {
		//fmt.Println("syncEtcd======", s.ServiceName, s.IP)
		//log.Warnf("syncEtcd======%s,%s", s.ServiceName, s.IP)

		svrAddress = fmt.Sprintf("%s:%d", s.IP, s.Port)
		if _, ok := svrs[s.ServiceName]; !ok {
			svrs[s.ServiceName] = make(map[string]int)
			regs.Set(s.ServiceName, make([]*ServiceInfo, 0, 5))
		}

		svrs[s.ServiceName][svrAddress] = s.Weight
		val, _ := regs.Get(s.ServiceName)
		infos := val.([]*ServiceInfo)
		infos = append(infos, s)
		regs.Set(s.ServiceName, infos)
	}

	// //同步发现服务器本机的数据不存在
	// //在注册时同步， 如果服务不进行注册， 也不需要进行监测本机
	// if p.self != nil {
	// 	if _, ok := regs.Get(p.self.Info.ServiceName); !ok {
	// 		log.Infof(fmt.Sprintf("register server(%s) when delete in sync", p.self.Info.ServiceName))
	// 		p.reRegister()
	// 	}
	// }

	p.SvrLock.Lock()
	p.SvrInfo = svrs
	p.regInfo = regs
	p.SvrLock.Unlock()
}

func (p *LbClient) syncPeriod() {
	defer func() {
		if err := recover(); err != nil {
			log.Criticalf("[ETCD] panic etcd syncPeriod. err:%v", err)

			go p.syncPeriod()
		}
	}()

	ticker := time.Tick(time.Second * 6) //调成6秒
	for {
		select {
		case <-ticker:
			p.syncEtcd("")
		}
	}
}
