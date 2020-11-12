package grpckit

import (
	"context"
	"git.garena.com/shopee/loan-service/airpay_backend/public/common/utils"
	log "github.com/cihub/seelog"
	otgrpc "github.com/grpc-ecosystem/go-grpc-middleware/tracing/opentracing"
	"github.com/opentracing/opentracing-go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/uber/jaeger-client-go"
	"github.com/uber/jaeger-client-go/config"
	jaeger_config "github.com/uber/jaeger-client-go/config"
	jaeger_prometheus "github.com/uber/jaeger-lib/metrics/prometheus"
	"google.golang.org/grpc"
	"io"
	"os"
	"strconv"
	"time"
)

var globalTracer opentracing.Tracer
var tracerCloser io.Closer

const (
	envSamplerType  = "JAEGER_SAMPLER_TYPE"
	envSamplerParam = "JAEGER_SAMPLER_PARAM"
)

func GlobalTracer() opentracing.Tracer {
	return globalTracer
}

func InitTracing(tracingAddress, serviceName string) error {
	traceType := jaeger.SamplerTypeConst
	var param float64 = 1
	if utils.IsProduct() { //live环境 随机采样60%样本
		traceType = jaeger.SamplerTypeProbabilistic
		param = 0.6
	}
	if e := os.Getenv(envSamplerType); e != "" {
		traceType = e
	}
	if e := os.Getenv(envSamplerParam); e != "" {
		if value, err := strconv.ParseFloat(e, 64); err == nil {
			param = value
		}
	}
	cfg := config.Configuration{
		Sampler: &config.SamplerConfig{
			Type:  traceType,
			Param: param,
		},
		Reporter: &config.ReporterConfig{
			LogSpans:            true,
			BufferFlushInterval: 1 * time.Second,
			CollectorEndpoint:   tracingAddress,
		},
	}
	metricsFactory := jaeger_prometheus.New(
		jaeger_prometheus.WithRegisterer(prometheus.DefaultRegisterer),
	)
	var err error
	globalTracer, tracerCloser, err = cfg.New(
		serviceName,
		jaeger_config.Metrics(metricsFactory),
	)
	if err != nil {
		log.Error("InitTracing err:", err)
		return err
	}
	opentracing.SetGlobalTracer(globalTracer)

	return nil
}

// GetTraceID 返回jaeger traceid，ctx是grpc server method的context
func GetTraceID(ctx context.Context) string {
	traceId := ""
	span := opentracing.SpanFromContext(ctx)
	if span != nil {
		if spanCtx, ok := span.Context().(jaeger.SpanContext); ok {
			traceId = spanCtx.TraceID().String()
		}
	}
	return traceId
}

func TracingClientInterceptor() grpc.UnaryClientInterceptor {
	return otgrpc.UnaryClientInterceptor()
}

func TracingServerInterceptor() grpc.UnaryServerInterceptor {
	return otgrpc.UnaryServerInterceptor()
}
