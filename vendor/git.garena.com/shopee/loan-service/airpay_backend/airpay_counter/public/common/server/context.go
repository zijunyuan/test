/*
@Time : 2019-12-04 10:51
@Author : siminliao
*/
package server

import (
	"context"
	"git.garena.com/shopee/loan-service/airpay_backend/public/common/log"
	"git.garena.com/shopee/loan-service/airpay_backend/public/common/metadata"
	grpc_metadata "google.golang.org/grpc/metadata"
	"strconv"
)

type MitraContext struct {
	context.Context
	*log.OnceLog
	ClientIP   string
	UID        uint64
	Lang       string
	OS         string
	Operator   string //admin
	AppType    string // agent/
	AppVersion string
}

func NewMitraContext(ctx context.Context) *MitraContext {
	log.Debug(grpc_metadata.FromIncomingContext(ctx))
	uid, _ := strconv.ParseInt(metadata.GetUidFromCtx(ctx), 0, 64)
	return &MitraContext{
		Context:    ctx,
		OnceLog:    log.NewOnceLogFromContext(ctx),
		ClientIP:   metadata.GetRemoteIP(ctx),
		UID:        uint64(uid),
		OS:         metadata.GetStringFromCtx(ctx, "A-OS"),
		Lang:       metadata.GetStringFromCtx(ctx, "A-Lang"),
		Operator:   metadata.GetStringFromCtx(ctx, "operator"),
		AppType:    metadata.GetStringFromCtx(ctx, "A-AppType"),
		AppVersion: metadata.GetStringFromCtx(ctx, "A-AppVer"),
	}
}

func (m *MitraContext) FromIOS() bool {
	return m.OS == "ios"
}
func (m *MitraContext) FromWeb() bool {
	return m.OS == "web"
}
func (m *MitraContext) FromAndroid() bool {
	return m.OS == "adr"
}
