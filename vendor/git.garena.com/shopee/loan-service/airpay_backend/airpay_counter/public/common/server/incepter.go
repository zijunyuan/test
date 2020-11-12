/*
@Time : 2020-01-09 20:01
@Author : siminliao
*/
package server

import (
	"context"
	"git.garena.com/shopee/loan-service/airpay_backend/public/common/log"
	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc"
)

func debugLogInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		onceLog := log.NewOnceLogFromContext(ctx)
		if req2, ok := req.(proto.Message); ok {
			onceLog.Debugf("Req: %s", req2.String())
		}
		resp, err = handler(ctx, req) //调用业务接口
		if err != nil {
			onceLog.Debug("err:", err)
		} else if resp2, ok := resp.(proto.Message); ok {
			onceLog.Debug("Resp:", resp2.String())
		}
		return
	}
}
