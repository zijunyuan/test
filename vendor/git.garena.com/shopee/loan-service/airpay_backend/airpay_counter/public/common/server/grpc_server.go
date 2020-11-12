/*
@Time : 2019-06-11 11:57
@Author : siminliao
*/
package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"git.garena.com/shopee/loan-service/airpay_backend/airpay_counter/public/common/config"
	"git.garena.com/shopee/loan-service/airpay_backend/airpay_counter/public/common/lbclient"
	"git.garena.com/shopee/loan-service/airpay_backend/public/common/grpckit"
	"git.garena.com/shopee/loan-service/airpay_backend/public/common/grpckit/recovery"
	"git.garena.com/shopee/loan-service/airpay_backend/public/common/log"
	"git.garena.com/shopee/loan-service/airpay_backend/public/common/monitor"
	"git.garena.com/shopee/loan-service/airpay_backend/public/common/utils"
	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_validator "github.com/grpc-ecosystem/go-grpc-middleware/validator"
	"google.golang.org/grpc"
	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

var (
	grpcServer *grpc.Server
)
var conf = struct {
	Server struct {
		Port              int
		Tracing           string
		Etcds             []string
		Env               string          `default:"dev"`
		MaxConnectionIdle config.Duration `default:"1m"`
		OpenMonitor       bool            `default:"true"`
		RegistEtcd        bool            `default:"true"` //开发环境可以设置为false不注册etcd
	}
	Log struct {
		Level string `default:"debug"`
	}
}{}

func GetInstance() *grpc.Server {
	if grpcServer == nil {
		err := config.Parse(&conf)
		if err != nil {
			panic(err)
		}
		randDir, err := log.InitLog(getLogLevel(conf.Log.Level))
		if err != nil {
			panic(err)
		}

		if conf.Server.OpenMonitor {
			monitor.Register(config.AppName) //promethus 监控
		}
		_ = grpckit.InitTracing(conf.Server.Tracing, config.AppName) // 初始化调用链，下面才可用.

		opts := []recovery.Option{
			recovery.WithRecoveryHandler(grpckit.ReturnRecoveryHandlerFunc()),
		}

		//注册json解码器。支持web
		encoding.RegisterCodec(JSON{
			Marshaler: jsonpb.Marshaler{
				EmitDefaults: false,
				OrigName:     true,
				EnumsAsInts:  true,
			},
			Unmarshaler: jsonpb.Unmarshaler{
				AllowUnknownFields: true,
			},
		})

		grpcServer = grpc.NewServer(
			grpc_middleware.WithUnaryServerChain(
				recovery.UnaryServerInterceptor(opts...),
				grpckit.TracingServerInterceptor(),       // XXX: 调用链要放在最前面，后面的才能取到traceid
				grpckit.FlowLogUnaryInterceptor(randDir), // 流水日志
				grpc_validator.UnaryServerInterceptor(),
				utils.UnaryServerInterceptor(),
				debugLogInterceptor(),
			), grpc.KeepaliveParams(keepalive.ServerParameters{
				MaxConnectionIdle: conf.Server.MaxConnectionIdle.Duration(),
			}))
	}
	return grpcServer
}

func Run() {
	address := fmt.Sprintf(":%d", conf.Server.Port)
	lis, err := net.Listen("tcp", address)
	if err != nil {
		log.Criticalf("failed to listen: %v", err)
		panic(err)
	}

	err = lbclient.GetInstance().Init(conf.Server.Etcds, conf.Server.Env)
	if err != nil {
		log.Errorf("etct init fail,err:%s", err)
		panic(err)
	}

	if conf.Server.RegistEtcd {
		etcdRegist()
	}

	reflection.Register(grpcServer) //用于grpcui

	//优雅退出。等正在处理的请求都处理完了再退出
	stopChan := make(chan os.Signal)
	signal.Notify(stopChan, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)
	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			log.Criticalf("failed to serve: %v", err)
			panic(err)
		}
	}()
	<-stopChan
	if conf.Server.RegistEtcd {
		etcdUnRegist()
	}
	grpcServer.GracefulStop()
}

func etcdRegist() {
	for serviceName := range GetInstance().GetServiceInfo() {
		fmt.Println("etcd regist serviceName:", serviceName)
		err := lbclient.GetInstance().Register(serviceName, conf.Server.Port, 1)
		if err != nil {
			log.Errorf("etct register fail,err:%s", err)
			panic(err)
		}
	}
}

func etcdUnRegist() {
	for serviceName := range GetInstance().GetServiceInfo() {
		if strings.Contains(serviceName, "grpc") { //reflection.Register(grpcServer) 也会注册一个serviceName
			continue
		}
		fmt.Println("etcd unregist serviceName:", serviceName)
		err := lbclient.GetInstance().UnRegister(serviceName, conf.Server.Port)
		if err != nil {
			log.Errorf("etct unregister fail,err:%s", err)
		}
	}
}

func getLogLevel(logLvl string) log.LogLevel {
	switch logLvl {
	case "trace":
		return log.TraceLvl
	case "debug":
		return log.DebugLvl
	case "info":
		return log.InfoLvl
	case "warn":
		return log.WarnLvl
	case "error":
		return log.ErrorLvl
	case "critical":
		return log.CriticalLvl
	case "off":
		return log.Off
	default:
		fmt.Printf("unknow log level:%s,use debug as default", logLvl)
		return log.DebugLvl
	}
}

//要用jsonpb，可以自动将uint64转成string。web的js没有long类型
type JSON struct {
	jsonpb.Marshaler
	jsonpb.Unmarshaler
}

func (_ JSON) Name() string {
	return "json"
}

func (j JSON) Marshal(v interface{}) (out []byte, err error) {
	if pm, ok := v.(proto.Message); ok {
		b := new(bytes.Buffer)
		err := j.Marshaler.Marshal(b, pm)
		if err != nil {
			return nil, err
		}
		return b.Bytes(), nil
	}
	return json.Marshal(v)
}

func (j JSON) Unmarshal(data []byte, v interface{}) (err error) {
	if len(data) == 0 {
		data = []byte("{}")
	}
	if pm, ok := v.(proto.Message); ok {
		b := bytes.NewBuffer(data)
		err = j.Unmarshaler.Unmarshal(b, pm)
		return
	}
	return json.Unmarshal(data, v)
}
