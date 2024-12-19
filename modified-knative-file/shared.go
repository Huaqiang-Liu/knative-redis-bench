// 将决定算法的全局数据结构及其暴露接口放在这个包下，由main.go添加或删除数据项，lb_policy.go中的函数读取
// 该数据结构以实现新的负载均衡算法

package shared

import (
	"math/rand"
	"strconv"
	"sync"
	"time"
)

// 对于ALU模拟服务
var JoblenALU = []int{8000, 4000, 2000, 1000, 700, 500, 350, 250, 200, 150, 100, 50, 40, 30, 25, 15, 5, 3, 2, 1}
var JoblenMapALU = map[int]int{
	8000: 32000,
	4000: 16000,
	2000: 8000,
	1000: 4000,
	700:  2800,
	500:  2000,
	350:  1400,
	250:  1000,
	200:  800,
	150:  600,
	100:  400,
	50:   200,
	40:   160,
	30:   120,
	25:   100,
	15:   60,
	5:    20,
	3:    12,
	2:    8,
	1:    4,
}

// 对于ServerlessBench种使用默认Azure数据集的真实场景模拟服务
var JoblenEdge = []int{20, 80, 180, 365, 670, 1155, 2125, 4835, 16555, 1571585} // 每一组的最长任务
var Joblen = []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}                                // 其实是任务长度的分组，下面对应的是每组的预计任务执行时间的数学期望
var JoblenMap = map[int]float64{0: 9.009700000000002, 1: 48.189249999999994, 2: 126.42465000000001, 3: 258.3718, 4: 506.4466499999997, 5: 898.9176499999991, 6: 1565.905400000001, 7: 3235.254600000007, 8: 8975.372949999984, 9: 83970.61725000001}

// 用于计算平均任务执行时间。TODO: 后面要改成对每个长短组分别统计
var TotalJobNum = 0
var TotalExecTime = 0.0
var MaxExecTime = 0.0
var GlobalVarMutex sync.RWMutex

type PodInfo struct {
	reqs    [10]int // pod上每个长短组的任务的数量
	ratesum int
	jobnum  int
}

type RequestStatic struct {
	sync.RWMutex
	Data map[string]PodInfo // key是pod的ip，value是PodInfo
}

var requestStatic = &RequestStatic{
	Data: make(map[string]PodInfo),
}

var Lambda = 10                             // 每秒任务数的数学期望
var MaxWaitingTime = 1000 / float64(Lambda) // 1000/lambda
var MaxQueueActualLen = 0                   // 更新出队列的最大长度

func AddJobToGlobalVar(joblen float64) {
	GlobalVarMutex.Lock()
	defer GlobalVarMutex.Unlock()
	TotalJobNum += 1
	TotalExecTime += joblen
	if joblen > MaxExecTime {
		MaxExecTime = joblen
	}
}

func CalculateAvgAndMaxExecTime() (float64, float64) {
	GlobalVarMutex.RLock()
	defer GlobalVarMutex.RUnlock()
	if TotalJobNum == 0 {
		return 0, 0
	}
	return TotalExecTime / float64(TotalJobNum), MaxExecTime
}

// 当一个任务调度成功时，更新requestStatic：将该任务的rate加入到对应pod的rates中（RS指的是Request Static）
func AddReqToRS(podip string, rate int) {
	requestStatic.Lock()
	defer requestStatic.Unlock()
	if _, ok := requestStatic.Data[podip]; !ok {
		requestStatic.Data[podip] = PodInfo{
			reqs:    [10]int{}, // 数组的元素默认值就是0
			ratesum: 0,
			jobnum:  0,
		}
	}
	// rate的值是Joblen中的某个值，取index为这个值对应的下标
	index := GetGroupIndex(rate)
	if index == -1 {
		return
	}
	podInfo := requestStatic.Data[podip]
	podInfo.reqs[index]++
	podInfo.ratesum += rate
	podInfo.jobnum++
	requestStatic.Data[podip] = podInfo
}

// 当一个任务执行完返回报文到activator时，更新requestStatic：减一次该pod上这个rate相应的请求数，以及ratesum
func DelReqFromRS(podip string, rate int) {
	requestStatic.Lock()
	defer requestStatic.Unlock()
	if _, ok := requestStatic.Data[podip]; !ok {
		return // 按理说这不可能发生——难道能虚空执行一个任务吗？
	}
	index := GetGroupIndex(rate)
	if index == -1 {
		return
	}
	podInfo := requestStatic.Data[podip]
	if podInfo.reqs[index] > 0 {
		podInfo.reqs[index]--
		podInfo.ratesum -= rate
		podInfo.jobnum--
	}
	requestStatic.Data[podip] = podInfo
}

// 选择两个pod，根据rate选择其中一个
func ChoosePodByRate(podip1 string, podip2 string) string {
	podInfo1 := requestStatic.Data[podip1]
	podInfo2 := requestStatic.Data[podip2]
	// fmt.Println("两个pod上的总rate数分别为：", podInfo1.ratesum, podInfo2.ratesum)
	if podInfo1.ratesum > podInfo2.ratesum {
		return podip2
	} else {
		return podip1
	}
}

func ChoosePodByNumOfJobs(podip1 string, podip2 string) string {
	podInfo1 := requestStatic.Data[podip1]
	podInfo2 := requestStatic.Data[podip2]

	if podInfo1.jobnum > podInfo2.jobnum {
		return podip2
	} else {
		return podip1
	}
}

func CheckPodBusy(podip string) bool { // 占用则返回true
	podInfo := requestStatic.Data[podip]
	return podInfo.ratesum != 0
}

func ChooseIdlePod(podip1 string, podip2 string) string {
	podInfo1 := requestStatic.Data[podip1]
	podInfo2 := requestStatic.Data[podip2]
	if podInfo1.ratesum == 0 {
		// fmt.Println("选择空闲pod1", podip1)
		return podip1
	} else if podInfo2.ratesum == 0 {
		// fmt.Println("选择空闲pod2", podip2)
		return podip2
	} else {
		return ""
	}
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
	// index := rng.Intn(10)
	// index += 10
	return strconv.Itoa(Joblen[index])
}
