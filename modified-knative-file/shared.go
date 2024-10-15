// 将决定算法的全局数据结构及其暴露接口放在这个包下，由main.go添加或删除数据项，lb_policy.go中的函数读取
// 该数据结构以实现新的负载均衡算法

package shared

import (
	"math/rand"
	"strconv"
	"sync"
	"time"
)

type RequestStatic struct {
	sync.RWMutex
	Data map[string]string
}

var requestStatic = &RequestStatic{
	Data: make(map[string]string),
}

func GetRequestStatic() *RequestStatic {
	requestStatic.RLock()
	defer requestStatic.RUnlock()
	return requestStatic
}

func SetRequestStatic(key string, value string) {
	requestStatic.Lock()
	defer requestStatic.Unlock()
	requestStatic.Data[key] = value
}

// 全局变量，记录上一次的rate和到达时间戳（字符串，默认为空）
var (
	lastRateMutex sync.RWMutex
	lastRate      = ""

	lastArriveTimeMutex sync.RWMutex
	lastArriveTime      = ""
)

func GetLastRate() string {
	lastRateMutex.RLock()
	defer lastRateMutex.RUnlock()
	return lastRate
}

func SetLastRate(rate string) {
	lastRateMutex.Lock()
	defer lastRateMutex.Unlock()
	lastRate = rate
}

func GetlastArriveTime() string {
	lastArriveTimeMutex.RLock()
	defer lastArriveTimeMutex.RUnlock()
	return lastArriveTime
}

func SetlastArriveTime(time string) {
	lastArriveTimeMutex.Lock()
	defer lastArriveTimeMutex.Unlock()
	lastArriveTime = time
}

// 摇随机数决定rate
func GenRate() string {
	job_len := []int{8000, 4000, 2000, 1000, 700, 500, 350, 250, 200, 150, 100, 50, 40, 30, 25, 15, 5, 3, 2, 1}
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	index := rng.Intn(20)
	return strconv.Itoa(job_len[index])
}
