/*
@Time : 2019-07-08 16:22
@Author : siminliao
*/
package lbclient

import (
"git.garena.com/shopee/loan-service/airpay_backend/public/common/lb"
)

var lbClient *lb.LbClient

func GetAddress(serverName string) (string, error) {
	return GetInstance().GetAddress(serverName)
}


func GetInstance() *lb.LbClient {
	return lbClient
}

func init() {
	if lbClient == nil {
		lbClient = &lb.LbClient{}
	}
}

