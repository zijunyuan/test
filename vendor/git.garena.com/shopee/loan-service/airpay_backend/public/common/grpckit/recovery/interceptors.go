// Copyright 2017 David Ackroyd. All Rights Reserved.
// See LICENSE for licensing terms.

//整份代码都是从 grpc_recovery 拷贝过来的,为了能在回调的地方把 context 传递过去

package recovery

import (
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

type RecoverEntity struct {
	RecoverRet interface{}
	Ctx        context.Context
}

// RecoveryHandlerFunc is a function that recovers from the panic `p` by returning an `error`.
type RecoveryHandlerFunc func(p interface{}) (err error)

// UnaryServerInterceptor returns a new unary server interceptor for panic recovery.
func UnaryServerInterceptor(opts ...Option) grpc.UnaryServerInterceptor {
	o := evaluateOptions(opts)
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (_ interface{}, err error) {
		defer func() {
			if r := recover(); r != nil {
				recoverEntity := RecoverEntity{}
				recoverEntity.RecoverRet = r
				recoverEntity.Ctx = ctx
				err = recoverFrom(recoverEntity, o.recoveryHandlerFunc)
			}
		}()

		return handler(ctx, req)
	}
}

// StreamServerInterceptor returns a new streaming server interceptor for panic recovery.
func StreamServerInterceptor(opts ...Option) grpc.StreamServerInterceptor {
	o := evaluateOptions(opts)
	return func(srv interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
		defer func() {
			if r := recover(); r != nil {
				err = recoverFrom(r, o.recoveryHandlerFunc)
			}
		}()

		return handler(srv, stream)
	}
}

func recoverFrom(p interface{}, r RecoveryHandlerFunc) error {
	if r == nil {
		return grpc.Errorf(codes.Internal, "%s", p)
	}
	return r(p)
}
