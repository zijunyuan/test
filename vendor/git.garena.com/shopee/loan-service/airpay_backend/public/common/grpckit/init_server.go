package grpckit

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	_ "net/http/pprof"

	"git.garena.com/shopee/loan-service/airpay_backend/public/common/lb"
	"git.garena.com/shopee/loan-service/airpay_backend/public/common/log"
	"git.garena.com/shopee/loan-service/airpay_backend/public/common/monitor"
	"git.garena.com/shopee/loan-service/airpay_backend/public/common/utils"
	"github.com/pkg/profile"
	"github.com/soheilhy/cmux"
	_ "go.uber.org/automaxprocs" // 会自动设置 maxprocs 数, 参考 https://mp.weixin.qq.com/s/H34xmtDIomaVSmZQO1JK-g
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

type RegistHandlerFunc func(s *grpc.Server)
type InitHttpServerHandler func() (httpServer *http.Server)

var (
	_etcdEndPoints []string
	_lbClient      *lb.LbClient
	prof           *profile.Profile
	_serviceNames  []string
	_grpcServer    *grpc.Server
	_serverConfig  *ServerConfig
)

type ServerConfig struct {
	LogLevel                        log.LogLevel
	GrpcPort                        int
	TracingAddress                  string
	AppName                         string //要具体一点, 避免有重复,AppName不会注册到etcd
	EtcdEndPoints                   []string
	NotRegist2Etcd                  bool                          //默认为false,如果在本地mac调试不想影响调用方可以设置为true
	DevMonitorEnable                bool                          //默认false 本地如果需要开启测试，传入true即可
	CustomerUnaryServerInterceptors []grpc.UnaryServerInterceptor //数组的顺序就是执行顺序
	CustomServerOption              []grpc.ServerOption           //自定义server option
}

//设置自定义拦截器
func (s *ServerConfig) SetCustomerUnaryServerInterceptors(interceptors []grpc.UnaryServerInterceptor) {
	s.CustomerUnaryServerInterceptors = interceptors
}
func initLbClient(etcdEndPoints []string) error {
	_etcdEndPoints = etcdEndPoints
	lbClient := lb.LbClient{}
	env := utils.GetEnv() //dev or test or product  从环境变量 ENV 去取

	err := lbClient.Init(_etcdEndPoints, env)
	if err != nil {
		log.Criticalf("lbclient init fail.%v", err) //打印错误日志到滚动日志
		//panic(err)
		return err
	}
	_lbClient = &lbClient
	return nil
}

//
func initLbClientV2(etcdEndPoints []string) error {
	_etcdEndPoints = etcdEndPoints
	lbClient := lb.LbClient{}
	env := utils.GetEnv() //dev or test or product  从环境变量 ENV 去取
	err := lbClient.Init(_etcdEndPoints, env)
	if err != nil {
		log.Criticalf("lbclient init fail.%v", err) //打印错误日志到滚动日志
		//panic(err)
		return err
	}
	_lbClient = &lbClient
	return nil
}

func GetLbClient() *lb.LbClient {
	return _lbClient
}

func InitConfig(serverConfig *ServerConfig) error {

	appName := serverConfig.AppName
	tracingAddress := serverConfig.TracingAddress //"http://10.12.77.215:14268/api/traces?format=jaeger.thrift" //从配置文件读,jeager的上报地址
	etcdEndPoints := serverConfig.EtcdEndPoints
	logLevel := serverConfig.LogLevel
	if utils.IsProduct() && logLevel == log.DebugLvl { //live环境不能打 debug 日志
		logLevel = log.InfoLvl
	}

	//1 初始化日志 initLogger
	_, err := log.InitLog(logLevel)
	if err != nil {
		panic(err)
		return err
	}

	err = initLbClient(etcdEndPoints)
	if err != nil {
		panic(err)
		return err
	}

	if utils.IsDev() && !serverConfig.DevMonitorEnable {

	} else {
		//非Dev环境或者dev手动开启使用
		monitor.Register(appName) //promethus 监控
	}

	// 初始化调用链跟踪 grpckit.InitTracing
	err = InitTracing(tracingAddress, appName)
	if err != nil {
		//panic(err)
		//return nil, nil, err
		log.Error("InitTracing fail.tracingAddress:"+tracingAddress, err)
	}

	// 从环境变量读取参数
	if !serverConfig.NotRegist2Etcd {
		notRegist2EtcdStrFromEnv, ok := utils.GetGeneralEnv("NotRegist2Etcd")
		if ok {
			if notRegist2EtcdFromEnv, err := strconv.ParseBool(notRegist2EtcdStrFromEnv); err == nil && notRegist2EtcdFromEnv != serverConfig.NotRegist2Etcd {
				log.Infof("load NotRegist2Etcd from env :%v", notRegist2EtcdFromEnv)
				serverConfig.NotRegist2Etcd = notRegist2EtcdFromEnv
			}
		}
	}
	return nil
}

//利用 net/http/pprof 库可以实时pprof go 进程
//登录到pod里面执行命令: curl http://127.0.0.1:4999/debug/pprof/goroutine
//详细看 https://docs.google.com/presentation/d/1-Ef3r0GVSGB0LOoZ_UENEix9_-y6_uxcCEucon-TddI/edit#slide=id.g743cc9ac3f_4_0
func initPprofHttpAsync(port int) {
	go func() {
		bindAddr := fmt.Sprintf(":%d", port) //前面加一个冒号,表示绑定所有ip
		if err := http.ListenAndServe(bindAddr, nil); err != nil {
			log.Criticalf("initPprofHttpAsync error:%v", err)
		}
	}()
}

func InitButNotRunServer(serverConfig *ServerConfig, registHandler RegistHandlerFunc) (*grpc.Server, net.Listener, error) {
	/*
			1 初始化日志 initLogger
		2 初始化调用链跟踪 grpckit.InitTracing
		3 注册服务 ServiceRegister→lb.LbClient.Init(endpoints,env)   // env:dev、test、product 三个不同环境
		4 启动 grpc 服务,注入切面监听器
	*/

	grpcPort := serverConfig.GrpcPort //从配置文件读,表示grpc提供服务的端口

	_serverConfig = serverConfig

	err := InitConfig(serverConfig)
	if err != nil {
		return nil, nil, err
	}

	bindAddr := fmt.Sprintf(":%d", grpcPort) //前面加一个冒号,表示绑定所有ip
	// 绑定端口
	lis, err := net.Listen("tcp", bindAddr)
	if err != nil {
		log.Criticalf("failed to listen: %v", err)
		panic(err)
		return nil, nil, err
	}

	httpProfPort := 4999
	initPprofHttpAsync(httpProfPort)

	//创建 grpc.Server 对象
	s := NewGrpcServer(serverConfig.CustomServerOption, serverConfig.CustomerUnaryServerInterceptors)

	registHandler(s) //要先注册 service

	for serviceName := range s.GetServiceInfo() {
		if serverConfig.NotRegist2Etcd {
			//不注册etcd
		} else {
			// 注册微服务到etcd
			_lbClient.Register(serviceName, grpcPort, 1)
		}
		_serviceNames = append(_serviceNames, serviceName)
	}

	if utils.IsTest() || utils.IsDev() {
		reflection.Register(s)
	}
	_grpcServer = s
	return s, lis, nil
}

// run server 的意思是这个函数会启动server并阻塞
func InitAndRunSync(serverConfig *ServerConfig, registHandler RegistHandlerFunc) (*grpc.Server, error) {
	s, lis, err := InitButNotRunServer(serverConfig, registHandler)
	if err != nil {
		return s, err
	}

	//  启动 grpc 服务
	if err := s.Serve(lis); err != nil {
		log.Criticalf("start grpc serve fail: %v", err)
		panic(err)
		return s, err
	}
	return s, nil
}

//初始化并异步启动 grpc server
func InitAndRunAsync(serverConfig *ServerConfig, registHandler RegistHandlerFunc) (*grpc.Server, error) {
	s, lis, err := InitButNotRunServer(serverConfig, registHandler)
	if err != nil {
		return s, err
	}

	go func() {
		//  启动 grpc 服务
		if err := s.Serve(lis); err != nil {
			log.Criticalf("start grpc serve fail: %v", err)
			panic(err)
		}
	}()

	return s, nil
}

// InitAndRunMultiplexingServer 将复用同一个端口同时提供grpc以及http服务，
// grpc server, http server在此函数中以异步的方式各自运行于一个单独的goroutine中。
// 请求将使用http版本号区分，1.x版本号的请求将被路由到http server，其余情况将被路由到grpc server。
// 需要注意由于共用listener，此时关闭2个server中的任意一个另一个都会随之关闭，包括如下函数：
//		grpcServer.Stop()
//		grpcServer.GracefulStop()
//		httpServer.Shutdown()
//		httpServer.Close()
//deprecated: 这个函数不支持把tracer对象嵌入到ginContext,推荐使用 InitAndRunMultiplexingServer2
func InitAndRunMultiplexingServer(serverConfig *ServerConfig, grpcHandler RegistHandlerFunc, httpServer *http.Server) (*grpc.Server, error) {
	// init grpc server
	var grpcServer, lis, err = InitButNotRunServer(serverConfig, grpcHandler)
	if err != nil {
		return nil, err
	}

	runMultiplexingServer(grpcServer, httpServer, lis)

	return grpcServer, nil
}

//初始化prof收集内存、cpu性能信息生成火焰图
//deprecated: 改用 ListenStopSignalSync 监听stop信号,改用 initPprofHttpAsync 函数支持动态输出火焰图
func InitProfSync(mode string) {
	dir := "./log/" + strconv.FormatInt(time.Now().Unix(), 10)
	switch mode {
	case "cpu":
		prof = profile.Start(profile.CPUProfile, profile.ProfilePath(dir), profile.NoShutdownHook).(*profile.Profile)
	case "mem":
		prof = profile.Start(profile.MemProfile, profile.ProfilePath(dir), profile.NoShutdownHook).(*profile.Profile)
	case "mutex":
		prof = profile.Start(profile.MutexProfile, profile.ProfilePath(dir), profile.NoShutdownHook).(*profile.Profile)
	case "block":
		prof = profile.Start(profile.BlockProfile, profile.ProfilePath(dir), profile.NoShutdownHook).(*profile.Profile)
	default:
		// do nothing
	}

	listenSignal()

}

func stopPProf() {
	if prof != nil {
		prof.Stop()
	}
}

func listenSignal() {
	sigs := make(chan os.Signal, 1)
	//ctrl+c 或 kill -3 pid
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGKILL)
	s := <-sigs
	log.Criticalf("receive signal, stopPProf, signal:" + s.String())
	for _, serviceName := range _serviceNames {
		// 在etcd 删掉本服务的key
		_lbClient.UnRegister(serviceName, _serverConfig.GrpcPort)
	}
	stopPProf()
	// to ensure server request already timeout
	time.Sleep(45 * time.Second) //睡眠一下,保证调用方已经把ip摘除了
	if _grpcServer != nil {
		_grpcServer.GracefulStop()
	}
}

//监听关停信号,实现优雅关闭,一般放在main函数末尾调用,会阻塞住当前线程
func ListenStopSignalSync() {
	listenSignal()
}

func runMultiplexingServer(grpcServer *grpc.Server, httpServer *http.Server, lis net.Listener) {
	// config listener multiplexing
	var mux = cmux.New(lis)
	var httpLis = mux.Match(cmux.HTTP1())
	var grpcLis = mux.Match(cmux.Any())

	// run http server, grpc server and cmux
	// 主动关闭grpcServer或httpServer当中的任何一个时，另一个server的Serve函数将收到cmux.ErrListenerClosed。
	go func() {
		err := grpcServer.Serve(grpcLis)
		if err != nil && err != cmux.ErrListenerClosed {
			log.Criticalf("start grpc serve fail: %v", err)
			os.Exit(1)
		}
	}()
	go func() {
		err := httpServer.Serve(httpLis)
		if err != nil && err != cmux.ErrListenerClosed && err != http.ErrServerClosed {
			log.Criticalf("start http serve fail: %v", err)
			os.Exit(2)
		}
	}()
	go func() {
		err := mux.Serve()
		if err != nil {
			log.Criticalf("start cmux serve fail: %v", err)
			os.Exit(1)
		}
	}()
}

// 参考 TestMultiplexing 的使用例子
func InitAndRunMultiplexingServer2(serverConfig *ServerConfig, grpcHandler RegistHandlerFunc, initHttpServerHandler InitHttpServerHandler) (*grpc.Server, error) {
	// init grpc server
	var grpcServer, lis, err = InitButNotRunServer(serverConfig, grpcHandler)
	if err != nil {
		return nil, err
	}

	httpServer := initHttpServerHandler()

	runMultiplexingServer(grpcServer, httpServer, lis)

	return grpcServer, nil
}
