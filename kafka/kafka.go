package kafka

import (
	"fmt"
	"github.com/Shopify/sarama"
)

//专门往kafka里面写日志的文件
var (
	client sarama.SyncProducer //声明一个全局的连接kafka的生产者客户端
)

// Init 初始化客户端
func Init(addr []string) (err error) {
	config := sarama.NewConfig()
	config.Producer.RequiredAcks = sarama.WaitForAll              //等待leader收到follower的ack，然后再收到leader的ack，这样很慢，需要follower从leader复制
	config.Producer.Partitioner = sarama.NewRoundRobinPartitioner //轮训选出分区
	config.Producer.Return.Successes = true                       //成功交付的消息将在success channel返回

	//连接kafka
	client, err = sarama.NewSyncProducer(addr, config)
	if err != nil {
		fmt.Println("producer closed,err:", err)
		return
	}
	return
}

func SendToKafka(topic, data string) {
	//构造一个消息
	msg := &sarama.ProducerMessage{}
	msg.Topic = topic
	msg.Value = sarama.StringEncoder(data)

	//发送消息
	pid, offset, err := client.SendMessage(msg)
	if err != nil {
		fmt.Println("send msg failed,err:", err)
		return
	}
	fmt.Printf("pid:%v offset:%v\n", pid, offset)
}
