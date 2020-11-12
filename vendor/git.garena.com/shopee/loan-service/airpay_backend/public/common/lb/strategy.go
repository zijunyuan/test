package lb

const (
	MIN_WEIGHT = -10000
)

type SvrInfo struct {
	svrMap map[string]([]*AddressInfo)
}

type AddressInfo struct {
	addr          string
	currentWeight int
	configWeight  int
}

type LbIntf interface {
	GetAddress(current map[string]int, conf map[string]int) (map[string]int, string)
}

type SmoothLb struct {
}

func (p *SmoothLb) GetAddress(current map[string]int, conf map[string]int) (map[string]int, string) {
	total := 0
	for _, v := range conf {
		total += v
	}
	curMap := make(map[string]int, 0)
	maxAddress := ""
	maxWeight := MIN_WEIGHT
	for k, v := range current {
		v += conf[k]
		if v > maxWeight {
			maxAddress = k
			maxWeight = v
		}
		curMap[k] = v
	}
	curMap[maxAddress] -= total
	//log.Infof("maxAddress is %s, maxWeight is %d", maxAddress, curMap[maxAddress])
	return curMap, maxAddress
}
