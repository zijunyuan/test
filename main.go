package main

import (
	"fmt"
	"gopkg.in/ini.v1"
	"test/conf"
	"test/kafka"
	"test/taillog"
	"time"
)

var (
	cfg *conf.AppConf=new(conf.AppConf)
)

func run() {
	//1.读取日志
	for {
		select {
		case line := <-taillog.ReadChan():
			//2/发送到kafka
			kafka.SendToKafka(cfg.KafkaConf.Topic, line.Text)
		default:
			time.Sleep(time.Second)
		}
	}
}

//logagent程序入口
func main() {
	//0.加载配置文件
	err := ini.MapTo(cfg, "./conf/config.ini")
	if err != nil {
		fmt.Printf("load ini failed,err:%v\n", err)
		return
	}

	//1.初始化kafka连接
	err = kafka.Init([]string{cfg.KafkaConf.Address})
	if err != nil {
		fmt.Println("init kafka failed,err:", err)
		return
	}
	fmt.Println("init kafka success")

	//2.打开日志文件准备收集日志
	err = taillog.Init(cfg.LogConf.FileName)
	if err != nil {
		fmt.Println("open file failed,err:", err)
		return
	}
	fmt.Println("init tail log success")

	run()
}
