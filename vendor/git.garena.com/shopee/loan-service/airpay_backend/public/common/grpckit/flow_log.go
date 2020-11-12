package grpckit

import (
	"context"
	"fmt"
	commonlog "git.garena.com/shopee/loan-service/airpay_backend/public/common/log"
	"git.garena.com/shopee/loan-service/airpay_backend/public/common/metadata"
	"github.com/cihub/seelog"
	"github.com/opentracing/opentracing-go"
	"github.com/uber/jaeger-client-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
	"time"
)

type flowlogCustomKey struct{}
type printRespKey struct{}
type printRequestKey struct{}

func FlowLogAppend(ctx context.Context, content string) {
	val := ctx.Value(flowlogCustomKey{})
	if val == nil {
		return
	}
	strPtr := val.(*string)
	*strPtr = fmt.Sprintf("%s%s", *strPtr, content)
}
func SetPrintResp(ctx context.Context, printResp bool) {
	setPrint(ctx, printRespKey{}, printResp)
}
func SetPrintRequest(ctx context.Context, printRequest bool) {
	setPrint(ctx, printRequestKey{}, printRequest)
}
func setPrint(ctx context.Context, key interface{}, print bool) {
	val := ctx.Value(key)
	if val == nil {
		return
	}
	strPtr := val.(*bool)
	*strPtr = print
}

/***
 * 同时设置是否打印请求和是否打印响应
 * 默认请求是true：打印   默认响应是false：不打印
 */
func SetPrint(ctx context.Context, printRequest bool, printResp bool) {
	SetPrintRequest(ctx, printRequest)
	SetPrintResp(ctx, printResp)
}

func injectKey(ctx context.Context) context.Context {
	onceLog := commonlog.NewOnceLogFromContext(ctx)
	onceLogKeyCtx := context.WithValue(ctx, commonlog.OnceLogKey{}, onceLog)
	printRespCtx := context.WithValue(onceLogKeyCtx, printRespKey{}, new(bool))
	printRequestCtx := context.WithValue(printRespCtx, printRequestKey{}, new(bool))
	SetPrintRequest(printRequestCtx, true) //默认设置为true，与线上保持一致
	return context.WithValue(printRequestCtx, flowlogCustomKey{}, new(string))
}
func isPrintResp(ctx context.Context) bool {
	//默认不设置是（false）不打印响应的，保持和线上一致
	return isPrint(ctx, printRespKey{}, false)
}
func isPrintRequest(ctx context.Context) bool {
	//默认不设置是（true）打印请求的，保持和线上一致
	return isPrint(ctx, printRequestKey{}, true)
}
func isPrint(ctx context.Context, key interface{}, defaultValue bool) bool {
	val := ctx.Value(key)
	if val == nil {
		return defaultValue
	}
	strPtr := val.(*bool)
	return *strPtr
}

func extractContent(ctx context.Context) string {
	val := ctx.Value(flowlogCustomKey{})
	if val == nil {
		return ""
	}
	strPtr := val.(*string)
	return *strPtr
}

func flowLogUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{},
		info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {

		ctx = injectKey(ctx)
		start := time.Now()

		traceId := ""
		span := opentracing.SpanFromContext(ctx)
		if span != nil {
			if spanCtx, ok := span.Context().(jaeger.SpanContext); ok {
				traceId = spanCtx.TraceID().String()
			}
		}

		var err error
		var resp interface{}
		defer func() {
			content := extractContent(ctx)
			uid := metadata.GetUidFromCtx(ctx)
			retCode := status.Code(err)
			end := time.Now()
			ms := end.Sub(start) / time.Millisecond
			callFrom := metadata.GetCallFromInfoFromCtx(ctx)
			commonlog.FlowLogInfoWithConfig(traceId, int(retCode), ms, uid, info.FullMethod, content, req, resp, isPrintResp(ctx), isPrintRequest(ctx), callFrom)
		}()

		resp, err = handler(ctx, req) //调用业务接口

		return resp, err
	}
}

func FlowLogUnaryInterceptor(randDir string) grpc.UnaryServerInterceptor {
	commonlog.InitFlowLog(randDir)
	return flowLogUnaryInterceptor()
}

func Flush() {
	commonlog.FlushFlowLog()
	seelog.Flush()
}
