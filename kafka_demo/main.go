package main

import (
	"fmt"
	"github.com/Shopify/sarama"
)

func main() {
	config := sarama.NewConfig()
	config.Producer.RequiredAcks = sarama.WaitForAll              //等待leader收到follower的ack，然后再收到leader的ack，这样很慢，需要follower从leader复制
	config.Producer.Partitioner = sarama.NewRoundRobinPartitioner //轮训选出分区
	config.Producer.Return.Successes = true                       //成功交付的消息将在success channel返回

	//构造一个消息
	msg := &sarama.ProducerMessage{}
	msg.Topic = "yzj"
	msg.Value = sarama.StringEncoder("大帅哥")

	//连接kafka
	client, err := sarama.NewSyncProducer([]string{"127.0.0.1:9092"}, config)
	if err != nil {
		fmt.Println("producer closed,err:", err)
		return
	}
	defer client.Close()

	//发送消息
	pid, offset, err := client.SendMessage(msg)
	if err != nil {
		fmt.Println("send msg failed,err:", err)
		return
	}
	fmt.Printf("pid:%v offset:%v", pid, offset)
}
