package monitor

import (
	"log"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"git.garena.com/shopee/loan-service/airpay_backend/public/common/config"
	comlog "git.garena.com/shopee/loan-service/airpay_backend/public/common/log"
	"git.garena.com/shopee/loan-service/airpay_backend/public/common/metadata"
	pbPublic "git.garena.com/shopee/loan-service/airpay_backend/public/public_proto"
	"github.com/cihub/seelog"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	STATUS_SUCESS    = "suc"
	STATUS_FAIL      = "fail"
	STATUS_EXCEPTION = "exception"
	STATUS_PANIC     = "panic"
)

var (
	promhttp_addr = ":30009"
)

var ( //客户端上报指标,一个是请求量，一个是请求耗时
	ReqPV = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "micro_service", Name: "request"},
		[]string{"from_service", "to_service", "to_func", "status", "code"})
	ReqCost = prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Namespace: "micro_service", Name: "timecost", MaxAge: time.Minute},
		[]string{"from_service", "to_service", "to_func", "status", "code"})
	ReqCostHist = prometheus.NewHistogramVec(prometheus.HistogramOpts{ //使用 Histogram 要比 Summary 好, 前者支持 分比
		Namespace: "micro_service", Name: "timecost_hist", Help: "The duration of the request"},
		[]string{"from_service", "to_service", "to_func", "status", "code"})
	fromAppName = ""
)

var ( //服务端上报的指标,一个是请求量，一个是请求耗时
	reqPVServer = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "micro_service", Subsystem: "svr_metric", Name: "requests_total"},
		[]string{"from_service", "to_service", "to_func", "status", "code"})

	reqCostHistServer = prometheus.NewHistogramVec(prometheus.HistogramOpts{ //使用 Histogram 要比 Summary 好, 前者支持 分比
		Namespace: "micro_service", Subsystem: "svr_metric", Name: "timecost_ms", Help: "The duration of the request from server in ms"},
		[]string{"from_service", "to_service", "to_func", "status", "code"})

	rlogCounter = prometheus.NewCounterVec(prometheus.CounterOpts{ //统计rlog的数量
		Namespace: "micro_service", Subsystem: "svr_metric", Name: "rlogs_total"},
		[]string{"from_service", "log_level", "grpc_method"})

	flogCounter = prometheus.NewCounterVec(prometheus.CounterOpts{ //统计flog的数量
		Namespace: "micro_service", Subsystem: "svr_metric", Name: "flogs_total"},
		[]string{"from_service", "grpc_method"})
)

func init() {
	val, ok := os.LookupEnv("MONITOR_PORT")
	if ok {
		promhttp_addr = ":" + val //如果环境变量有就取环境变量的端口
	}
	comlog.SetMonitorOnceLogHandler(returnMonitorOnceLogHandler())
	comlog.SetMonitorFLogHandler(returnMonitorFLogHandler())
}

func returnMonitorFLogHandler() comlog.MonitorFLogHandlerType {
	return func(grpcMethod string) {
		if index := strings.Index(grpcMethod, "?"); index > 0 {
			grpcMethod = grpcMethod[:index]
		}
		flogCounter.WithLabelValues(fromAppName, grpcMethod).Inc()
	}
}

func returnMonitorOnceLogHandler() comlog.MonitorOnceLogHandlerType {
	return func(level comlog.LogLevel, grpcMethod string) {
		strLevel := seelog.LogLevel(level).String()
		if index := strings.Index(grpcMethod, "?"); index > 0 {
			grpcMethod = grpcMethod[:index]
		}
		rlogCounter.WithLabelValues(fromAppName, strLevel, grpcMethod).Inc()
	}
}

func Register(appName string) {
	prometheus.MustRegister(ReqPV)
	prometheus.MustRegister(ReqCost)
	prometheus.MustRegister(ReqCostHist)
	fromAppName = appName
	prometheus.MustRegister(reqPVServer, reqCostHistServer)
	prometheus.MustRegister(rlogCounter, flogCounter)

	val, ok := os.LookupEnv("MONITOR_PORT")
	if ok {
		promhttp_addr = ":" + val
	}

	go func() {
		if err := http.ListenAndServe(promhttp_addr, promhttp.Handler()); err != nil {
			log.Fatalf("Init monitor err - %s", err.Error())
		}
	}()
}

func splitMethodName(fullMethodName string) (string, string) {
	fullMethodName = strings.TrimPrefix(fullMethodName, "/") // remove leading slash
	if i := strings.Index(fullMethodName, "/"); i >= 0 {
		return fullMethodName[:i], fullMethodName[i+1:]
	}
	return "unknown", "unknown"
}

func extractReportStatus(reply interface{}, err error) (reportStatus1 string, strCode1 string) {
	retStatus := status.Convert(err)
	reportStatus := ""
	if retStatus.Code() == codes.OK {
		reportStatus = STATUS_SUCESS
	} else if retStatus.Code() <= 20 {
		reportStatus = STATUS_EXCEPTION
	} else if retStatus.Code() == config.ErrorMicroServPanic {
		reportStatus = STATUS_PANIC
	} else {
		reportStatus = STATUS_FAIL
	}

	strCode := strconv.Itoa(int(retStatus.Code()))
	if retStatus.Code() == codes.OK && reply != nil {
		v := reflect.ValueOf(reply)
		if v.Kind() == reflect.Ptr && v.Elem().Kind() == reflect.Struct {
			f := v.Elem().FieldByName("Header")
			if f.IsValid() && f.CanInterface() {
				h, ok := f.Interface().(*pbPublic.Header)
				if ok && h != nil && h.GetErrcode() > 0 {
					strCode = strconv.Itoa(int(h.GetErrcode()))
				}
			}
		}
	}

	return reportStatus, strCode
}

func MonitorClientInterceptor() grpc.UnaryClientInterceptor {
	//o := evaluateOptions(opts)
	return func(parentCtx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		//请求量(业务失败tag(code>20)、成功tag(code=0)、异常失败(code<20))
		//总耗时
		//把调用方、被调方 也上报上去
		toServiceName, toFunc := splitMethodName(method)

		startTime := time.Now()

		err := invoker(parentCtx, method, req, reply, cc, opts...)
		timeCostMs := time.Now().Sub(startTime).Seconds() * 1000

		reportStatus, strCode := extractReportStatus(reply, err)

		ReqPV.WithLabelValues(fromAppName, toServiceName, toFunc, reportStatus, strCode).Inc()
		ReqCost.WithLabelValues(fromAppName, toServiceName, toFunc, reportStatus, strCode).Observe(timeCostMs)
		ReqCostHist.WithLabelValues(fromAppName, toServiceName, toFunc, reportStatus, strCode).Observe(timeCostMs / 1000)

		return err
	}
}

//server 端上报
func MonitorServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{},
		info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {

		startTime := time.Now()

		toServiceName, toFunc := splitMethodName(info.FullMethod)

		var err error
		var resp interface{}

		callFrom := metadata.GetCallFromInfoFromCtx(ctx)

		resp, err = handler(ctx, req) //==============调用业务接口==================

		timeCostMs := time.Now().Sub(startTime).Seconds() * 1000

		reportStatus, strCode := extractReportStatus(resp, err)

		reqPVServer.WithLabelValues(callFrom, toServiceName, toFunc, reportStatus, strCode).Inc()
		reqCostHistServer.WithLabelValues(callFrom, toServiceName, toFunc, reportStatus, strCode).Observe(timeCostMs)

		return resp, err
	}
}
