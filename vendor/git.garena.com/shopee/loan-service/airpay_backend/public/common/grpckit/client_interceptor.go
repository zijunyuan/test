package grpckit

import (
	"fmt"
	"git.garena.com/shopee/loan-service/airpay_backend/public/common/config"
	cmetadata "git.garena.com/shopee/loan-service/airpay_backend/public/common/metadata"
	"git.garena.com/shopee/loan-service/airpay_backend/public/common/utils"
	"github.com/golang/protobuf/proto"
	"github.com/grpc-ecosystem/go-grpc-middleware/util/metautils"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func PassMetaDataClientInterceptor() grpc.UnaryClientInterceptor {
	return func(parentCtx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		var newCtx context.Context
		if cmetadata.IsRequestFromStaff(parentCtx) {
			newCtx = handleStaffMetaData(parentCtx)
		} else {
			newCtx = handleCustomerMetaData(parentCtx)
		}
		return invoker(newCtx, method, req, reply, cc, opts...)
	}

}

func handleCustomerMetaData(parentCtx context.Context) context.Context {
	outGoingMd := metautils.ExtractOutgoing(parentCtx).Clone()
	var headerBytes []byte

	airpayHeader, err := cmetadata.GetAirpayHeader(parentCtx)
	if err == nil && airpayHeader != nil {
		headerBytes, err = proto.Marshal(airpayHeader)
		if err == nil {
			//这个方法有问题，所以注释了
			//outGoingMd.Add("airpay-header-bin", string(headerBytes))
		} else {
			fmt.Println("PassMetaDataClientInterceptor proto.Marshal error=====", err)
		}
	} else {
		fmt.Println("PassMetaDataClientInterceptor GetAirpayHeader error=====", err)
	}

	strUid := cmetadata.GetUidFromCtx(parentCtx)
	strRemoteIp := cmetadata.GetRemoteIP(parentCtx)
	strOperatorEmail := cmetadata.GetOperatorEmailFromCtx(parentCtx)
	sdkVer := cmetadata.GetSdkVersionFromCtx(parentCtx)
	outGoingMd.Add("uid", strUid)
	outGoingMd.Add("remote_ip", strRemoteIp)
	outGoingMd.Add("call_from", utils.GetAppName())
	outGoingMd.Add("a-operator-email", strOperatorEmail)
	outGoingMd.Add(config.AppVer, sdkVer)

	newCtx := outGoingMd.ToOutgoing(parentCtx)
	if len(headerBytes) > 0 {
		newCtx = metadata.AppendToOutgoingContext(newCtx,
			//参考 https://github.com/grpc/grpc-go/blob/master/Documentation/grpc-metadata.md#storing-binary-data-in-metadata
			//传二进制流的时候,key 要有 bin 后缀
			"airpay-header-bin", (string)(headerBytes),
		)
	}

	return newCtx
}

func handleStaffMetaData(parentCtx context.Context) context.Context {
	outGoingMd := metautils.ExtractOutgoing(parentCtx).Clone()

	staffId := cmetadata.GetStaffIdFromCtx(parentCtx)
	strRemoteIp := cmetadata.GetRemoteIP(parentCtx)

	outGoingMd.Add("staff_id", staffId)
	outGoingMd.Add("staff_device_id", cmetadata.GetStaffDeviceIdFromCtx(parentCtx))
	outGoingMd.Add("remote_ip", strRemoteIp)
	outGoingMd.Add("call_from", utils.GetAppName())
	outGoingMd.Add(config.Lang, cmetadata.GetLangFromCtx(parentCtx))

	return outGoingMd.ToOutgoing(parentCtx)
}
