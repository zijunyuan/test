package grpckit

import (
	"errors"
	"git.garena.com/shopee/loan-service/airpay_backend/public/common/utils"
)

type RegisterServiceConfig struct {
	Port           int      //应用服务的端口
	ServiceName    string   //应用服务名字
	EtcdEndPoints  []string //如果etcd已经初始化完成，可以为空
	NotRegist2Etcd bool     //默认为false,如果在本地mac调试不想影响调用方可以设置为true
}

//初始化lbClient
func InitLbClient(etcdEndPoints []string) error {
	if _lbClient == nil { //_lbClient为空，需要初始化
		err := initLbClient(etcdEndPoints)
		if err != nil {
			return err
		}
	}
	return nil
}

//注册服务：需要保证已经初始化过_lbClient
func RegisterService(serviceName string, port int) error {
	if _lbClient == nil { //_lbClient为空，需要初始化
		return errors.New("_lbClient is nil ,please init it")
	}
	// 注册微服务到etcd
	_lbClient.Register(serviceName, port, 1)
	//追加_serviceName,成员变量赋值，保证后续调用监听方法能使用成员变量
	_serviceNames = append(_serviceNames, serviceName)
	return nil
}

//初始化etcd并且注册服务
func InitLbClientAndRegisterService(registerConfig *RegisterServiceConfig) error {
	err := InitLbClient(registerConfig.EtcdEndPoints)
	if err != nil {
		return err
	}
	if registerConfig.NotRegist2Etcd && utils.IsDev() {
		//不注册etcd
	} else {
		RegisterService(registerConfig.ServiceName, registerConfig.Port)
	}
	return nil
}
