package config

import (
	cc "git.garena.com/shopee/loan-service/airpay_backend/airpay_counter/public/common/config"
	"git.garena.com/shopee/loan-service/airpay_backend/public/common/log"
)

var (
	MyConfig *Config
)

type Config struct {
	S3 struct {
		Addr       string
		AuthKey    string
		ExpireTime uint32
		Path       string
	}
}

func Init() {
	if MyConfig == nil {
		tmp := Config{}
		log.Infof("parse config with result: %v", cc.Parse(&tmp))
		MyConfig = &tmp
	}
}
