package grpckit

import (
	"fmt"
	"git.garena.com/shopee/loan-service/airpay_backend/public/common/config"
	"git.garena.com/shopee/loan-service/airpay_backend/public/common/grpckit/recovery"
	"git.garena.com/shopee/loan-service/airpay_backend/public/common/log"
	"git.garena.com/shopee/loan-service/airpay_backend/public/common/metadata"
	"git.garena.com/shopee/loan-service/airpay_backend/public/common/monitor"
	"git.garena.com/shopee/loan-service/airpay_backend/public/common/utils"
	"github.com/cihub/seelog"
	"github.com/grpc-ecosystem/go-grpc-middleware"
	"github.com/grpc-ecosystem/go-grpc-middleware/validator"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
	"runtime/debug"
	"sync"
)

var (
	customFunc recovery.RecoveryHandlerFunc
)

func init() {
	customFunc = ReturnRecoveryHandlerFunc()
}

func ReturnRecoveryHandlerFunc() recovery.RecoveryHandlerFunc {
	return func(pp interface{}) (err error) {
		recoverEntity, ok := pp.(recovery.RecoverEntity)
		p := pp
		traceId := ""
		uid := ""
		var onceLog *log.OnceLog
		onceLog = nil
		if ok {
			p = recoverEntity.RecoverRet
			onceLog = log.ExtractOnceLog(recoverEntity.Ctx)
			traceId = GetTraceID(recoverEntity.Ctx)
			uid = metadata.GetUidFromCtx(recoverEntity.Ctx)
		}
		strP := fmt.Sprintf("|%v", p) //返回给 client
		if nil != onceLog {
			onceLog.Error(strP + "|panic stack:" + string(debug.Stack()))
		} else {
			//onceLog 为空，只能手工保证日志符合rlog格式
			grpcMethod, _ := grpc.Method(recoverEntity.Ctx)

			prefixStr := fmt.Sprintf("|grpcMethod:%s|traceId:%s|uid:%s|",
				grpcMethod, traceId, uid)
			fileName, line, funcName := "not_print", 0, "not_print_when_panic"

			codeInfo := fmt.Sprintf("file:%s/%v|func:%s|", fileName, line, funcName)
			customerPrefix := ""

			fullPrefix := prefixStr + codeInfo + customerPrefix

			panicStack := "|panic stack:" + string(debug.Stack())

			fields := append([]interface{}{fullPrefix}, panicStack)
			seelog.Error(fields...)

			//sLog := "traceid:" + traceId + "|uid:" + uid + strP + "|panic stack:" + string(debug.Stack())
			//log.Errorf(sLog)
		}

		statusError := status.Error(config.ErrorMicroServPanic, strP) //生成 grpc 自定义的 Error  10012：panic错误码
		return statusError
	}
}

var interceptorSlice []grpc.UnaryServerInterceptor
var once sync.Once

//新增可选参数，不影响已经调用此方法的地方 直接取值args[0]即可
func NewGrpcServer(customopts []grpc.ServerOption, args ...[]grpc.UnaryServerInterceptor) *grpc.Server {
	opts := []recovery.Option{
		recovery.WithRecoveryHandler(customFunc),
	}
	once.Do(func() { //只初始化一次
		interceptorSlice = append(interceptorSlice, recovery.UnaryServerInterceptor(opts...)) //最上面加一层recover,防止代码崩溃
		interceptorSlice = append(interceptorSlice, TracingServerInterceptor())               //调用链跟踪的，提取 tracingid
		interceptorSlice = append(interceptorSlice, recovery.UnaryServerInterceptor(opts...)) //这一层 recover 可以打印 traceid
		interceptorSlice = append(interceptorSlice, monitor.MonitorServerInterceptor())       //加服务端监控
		interceptorSlice = append(interceptorSlice, flowLogUnaryInterceptor())                // 打印流水日志,必须要在 TracingServerInterceptor 之后才能获取tracingid
		interceptorSlice = append(interceptorSlice, grpc_validator.UnaryServerInterceptor())
		if len(args) > 0 { //传了拦截器，则初始化进去，否则，使用默认拦截器
			unaryServerInterceptors := args[0]
			for i := 0; i < len(args[0]); i++ {
				interceptorSlice = append(interceptorSlice, unaryServerInterceptors[i])
			}
		}
		interceptorSlice = append(interceptorSlice, utils.UnaryServerInterceptor())
	})
	serverOpts := make([]grpc.ServerOption, 0)
	serverOpts = append(serverOpts, grpc_middleware.WithUnaryServerChain(interceptorSlice...), grpc.MaxRecvMsgSize(50*1024*1024)) //支持文件上传，服务最大接受50M的请求
	if len(customopts) > 0 {
		serverOpts = append(serverOpts, customopts...)
	}
	return grpc.NewServer(serverOpts...)

}
