package monitor

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	//比如这个 metric 是上报某个服务的某个函数的成功数、失败数
	myMetric = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "myNamespace", Name: "metric_name"},
		[]string{"service_name", "func_name", "status", "code"}) //注意这里的label必须是收敛的,比如 uid、订单号不要用来做label

)

//注册指标
func InitMyMetric() {
	prometheus.MustRegister(myMetric)
}

//上报指标
func Report(serviceName, funcName, reportStatus, strCode string) {
	myMetric.WithLabelValues(serviceName, funcName, reportStatus, strCode).Inc()
}
