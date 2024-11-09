// 将决定算法的全局数据结构及其暴露接口放在这个包下，由main.go添加或删除数据项，lb_policy.go中的函数读取
// 该数据结构以实现新的负载均衡算法

package shared

import (
	"math/rand"
	"strconv"
	"sync"
	"time"
)

var Joblen = []int{8000, 4000, 2000, 1000, 700, 500, 350, 250, 200, 150, 100, 50, 40, 30, 25, 15, 5, 3, 2, 1}

type PodInfo struct {
	reqs    [20]int // rates[i]表示rate为job_len[i]的请求数
	ratesum int
}

type RequestStatic struct {
	sync.RWMutex
	Data map[string]PodInfo // key是pod的HOSTNAME，value是PodInfo
}

var requestStatic = &RequestStatic{
	Data: make(map[string]PodInfo),
}

var MaxWaitingTime = 500

func GetRequestStatic() *map[string]PodInfo {
	requestStatic.RLock()
	defer requestStatic.RUnlock()
	return &requestStatic.Data
}

// 当一个任务调度成功时，更新requestStatic：将该任务的rate加入到对应pod的rates中（RS指的是Request Static）
func AddReqToRS(podname string, rate int) {
	requestStatic.Lock()
	defer requestStatic.Unlock()
	if _, ok := requestStatic.Data[podname]; !ok {
		requestStatic.Data[podname] = PodInfo{
			reqs:    [20]int{}, // 数组的元素默认值就是0
			ratesum: 0,
		}
	}
	// rate的值是Joblen中的某个值，取index为这个值对应的下标
	index := -1
	for i, v := range Joblen {
		if v == rate {
			index = i
			break
		}
	}
	if index == -1 {
		return
	}
	podInfo := requestStatic.Data[podname]
	podInfo.reqs[index]++
	podInfo.ratesum += rate
	requestStatic.Data[podname] = podInfo
}

// 当一个任务执行完返回报文到activator时，更新requestStatic：减一次该pod上这个rate相应的请求数，以及ratesum
func DelReqFromRS(podname string, rate int) {
	requestStatic.Lock()
	defer requestStatic.Unlock()
	if _, ok := requestStatic.Data[podname]; !ok {
		return // 按理说这不可能发生——难道能虚空执行一个任务吗？
	}
	index := -1
	for i, v := range Joblen {
		if v == rate {
			index = i
			break
		}
	}
	if index == -1 {
		return
	}
	podInfo := requestStatic.Data[podname]
	if podInfo.reqs[index] > 0 {
		podInfo.reqs[index]--
		podInfo.ratesum -= rate
	}
	requestStatic.Data[podname] = podInfo
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

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	index := rng.Intn(20)
	return strconv.Itoa(Joblen[index])
}
