package grpckit

import (
	"context"
	"encoding/json"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"net/http"
)

const (
	GrpcRspCustomHeaderKey = "biz-custom-header-rsp"
)

type GrpcRspCustomHeader map[string][]string

// NewGrpcRspCustomHeader
func NewGrpcRspCustomHeader() GrpcRspCustomHeader {
	return GrpcRspCustomHeader{}
}

// AddHeader 增加自定义头部
func (c GrpcRspCustomHeader) AddHeader(k, v string) {
	c[k] = append(c[k], v)
}

// SendHeader 发送头部
// ctx: grpc方法的ctx参数
// 这个函数每个对象只有第一次调用生效
func (c GrpcRspCustomHeader) SendHeader(ctx context.Context) error {
	v, err := json.Marshal(&c)
	if err != nil {
		return err
	}
	header := metadata.Pairs(GrpcRspCustomHeaderKey, string(v))
	err = grpc.SendHeader(ctx, header)
	return err
}

// AppendHTTPHeader 从grpc的头部append到http头部
// hh: http Header
// gh: grpc header
func (c GrpcRspCustomHeader) AppendHTTPHeader(hh http.Header, gh metadata.MD) error {
	strSlice := gh.Get(GrpcRspCustomHeaderKey)
	if len(strSlice) == 0 {
		return nil
	}
	str := strSlice[0]
	err := json.Unmarshal([]byte(str), &c)
	if err != nil {
		return err
	} else {
		for k, v := range c {
			for _, vv := range v {
				hh.Add(k, vv)
			}
		}
	}
	return nil
}

