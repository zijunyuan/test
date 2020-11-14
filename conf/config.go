package conf

type AppConf struct {
	KafkaConf `ini:"kafka"`
	LogConf   `ini:"taillog"`
}

type KafkaConf struct {
	Address string `ini:"address"`
	Topic   string `ini:"topic"`
}

type LogConf struct {
	FileName string `ini:"path"`
}
