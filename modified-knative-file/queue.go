package shared

import (
	"container/list"
	"context"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"
)

type SchedulingUnit struct {
	Handler http.Handler
	Writer  http.ResponseWriter
	Req     *http.Request
	Done    chan struct{} // 用于通知请求执行完成的通道，执行完了HandlerFunc才能关闭，否则会丢失上下文
	Timer   *time.Timer   // 新增字段，用于计时
}

// 用于存储请求的线程安全队列
var (
	Queue      list.List
	QueueMutex sync.Mutex
	QueueCond  sync.Cond // 队列空阻塞，有任务唤醒
)

func init() {
	QueueCond.L = &QueueMutex
}

const MaxQueueize = 10000 // 定义队列的最大容量

// 延迟绑定需要在上下文中放一个通道，以便调度完成后关闭之，再取下一个任务。这样队列长度会累积得很多，可以说明控制节点内存瓶颈问题
type ContextKey string

const SchedulingDoneKey ContextKey = "schedulingDone"

// 增加varx，就只有更短的任务能直接发送，所以减少抢占率。也增大了D，减少了每个任务等待的时间
// 增加vary就是让每个任务等待时间变长，应该是能增大抢占率的
var varmapForExp3 = map[int]float64{
	30: 3400 + 1000/float64(30),
	40: 3400 + 1000/float64(40),
	50: 3400 + 1000/float64(50),
}
var varmapForExp4 = map[int]float64{
	30: 4250 + 1000/30,
	40: 4270 + 1000/40,
	50: 4380 + 1000/50,
}
var varx = varmapForExp3[Lambda]
var vary = 0.5

func AddReq0(h http.Handler, w http.ResponseWriter, r *http.Request, done chan struct{}) { // 早期绑定和延迟绑定：直接加入队列
	schedulingDone := make(chan struct{})
	ctx := context.WithValue(r.Context(), SchedulingDoneKey, schedulingDone)
	u := SchedulingUnit{Handler: h, Writer: w, Req: r.WithContext(ctx), Done: done}

	QueueMutex.Lock()
	defer QueueMutex.Unlock()
	len := Queue.Len()

	if len >= MaxQueueize {
		fmt.Println("队列已满")
		close(done)
		return
	}

	if len > MaxQueueActualLen {
		MaxQueueActualLen = len
	}
	fmt.Println("当前队列长度和最大队列长度分别为", len, MaxQueueActualLen)
	Queue.PushBack(u)
	QueueCond.Signal()
}
func ManageQueueEarly() { // 早期绑定：不断取队头元素然后serve
	for {
		QueueMutex.Lock()
		for Queue.Len() == 0 {
			QueueCond.Wait()
		}
		e := Queue.Front()
		u := e.Value.(SchedulingUnit)
		Queue.Remove(e)
		QueueMutex.Unlock()

		go serveRequest(u)
	}
}
func ManageQueueLate() { // 延迟绑定：取队头元素，等schedulingDone返回（或者超过20秒），再取下一个任务
	for {
		QueueMutex.Lock()
		for Queue.Len() == 0 {
			QueueCond.Wait() // 队列为空，阻塞等待
		}
		e := Queue.Front()
		u := e.Value.(SchedulingUnit)
		Queue.Remove(e)
		QueueMutex.Unlock()

		go serveRequest(u)

		// 等待调度完成，立即处理下一个请求
		select {
		case <-u.Req.Context().Value(SchedulingDoneKey).(chan struct{}):
		case <-time.After(20 * time.Second):
		}
	}

}

func AddReq12(h http.Handler, w http.ResponseWriter, r *http.Request, done chan struct{}) { // 实验1，2：简单抢占
	rate, _ := strconv.Atoi(r.Header.Get("X-Rate"))
	u := SchedulingUnit{Handler: h, Writer: w, Req: r, Done: done}
	u.Timer = time.NewTimer(time.Duration(MaxWaitingTime) * time.Millisecond)

	QueueMutex.Lock()
	defer QueueMutex.Unlock()

	if Queue.Len() >= MaxQueueize {
		fmt.Println("队列已满")
		close(done)
		return
	}

	// 检查队头元素（如果有），如果rate比当前任务的rate大，则直接执行u
	if Queue.Len() > 0 {
		frontRateStr := Queue.Front().Value.(SchedulingUnit).Req.Header.Get("X-Rate")
		frontRate, _ := strconv.Atoi(frontRateStr)
		if rate < frontRate {
			u.Req.Header.Set("X-Last-Rate", "1")
			u.Timer = time.NewTimer(0)
			go serveRequest(u)
			return
		}
	}

	Queue.PushBack(u)
	QueueCond.Signal() // 让ManageQueue中该队列对应的goroutine解除阻塞
}

func ManageQueue1() { // 实验1，不轮询等待，每次取队头元素并serve，然后sleep 2000/Lambda毫秒
	for {
		QueueMutex.Lock()
		for Queue.Len() == 0 {
			QueueCond.Wait()
		}
		e := Queue.Back()
		u := e.Value.(SchedulingUnit)
		Queue.Remove(e)
		QueueMutex.Unlock()

		go serveRequest(u)
		time.Sleep(time.Duration(2000/float64(Lambda)) * time.Millisecond)
	}
}

func AddReq(h http.Handler, w http.ResponseWriter, r *http.Request, done chan struct{}) { // 实验3，4
	rate, _ := strconv.Atoi(r.Header.Get("X-Rate"))
	u := SchedulingUnit{Handler: h, Writer: w, Req: r, Done: done}

	QueueMutex.Lock()
	defer QueueMutex.Unlock()
	len := Queue.Len()
	if len > MaxQueueActualLen {
		MaxQueueActualLen = len
		fmt.Println("当前最大队列长度为", MaxQueueActualLen)
	}

	if len >= MaxQueueize {
		fmt.Println("队列已满")
		close(done)
		return
	}

	avgExecTime, maxExecTime := CalculateAvgAndMaxExecTime() // 因为改成了实际情况而非预测情况，这个变长，D变小，抢占变多。所以要增加varx来达到原来的效果
	fmt.Println("平均和最大执行时间：", avgExecTime, ' ', maxExecTime)
	D := float64(JoblenMap[rate]) - avgExecTime + varx // 实验4一阶段，这样计算D
	// 实验4二阶段，D改为二重积分，\int_{0}^{avgExecTime}y\int_{0}^{maxExecTime}f(x)f(y-x)dxdy，再加上varx
	// TODO: f(x)是任务执行时间的概率密度函数，还不知道是什么，后面再说
	if float64(Lambda)*D < 1000 {
		u.Req.Header.Set("X-Last-Rate", "1")
		u.Timer = time.NewTimer(0)
		go serveRequest(u)
		return
	}

	u.Timer = time.NewTimer(time.Duration(vary*1000000*math.Log(float64(Lambda)*D/1000)/D) * time.Millisecond)
	Queue.PushBack(u)
	QueueCond.Signal() // 让ManageQueue中该队列对应的goroutine解除阻塞
}

// 不停轮询，看到计时器到期的就发出去
func ManageQueue() { // 实验2，3，4(包括一阶段和二阶段)
	for {
		QueueMutex.Lock()
		for Queue.Len() == 0 {
			QueueCond.Wait()
		}
		e := Queue.Back()
		QueueMutex.Unlock()

		for e != nil {
			QueueMutex.Lock()
			u := e.Value.(SchedulingUnit)
			prev := e.Prev()
			select {
			case <-u.Timer.C:
				Queue.Remove(e)
				go serveRequest(u)
			default:
			}
			QueueMutex.Unlock()
			e = prev
		}
	}
}

func serveRequest(u SchedulingUnit) {
	timer := time.NewTimer(time.Duration(100) * time.Second)
	u.Handler.ServeHTTP(u.Writer, u.Req)
	select {
	case <-timer.C:
		// TimeoutJobNumMutex.Lock()
		// TimeoutJobNum += 1
		// fmt.Println("超时任务数量为", TimeoutJobNum)
		// TimeoutJobNumMutex.Unlock()
		// 让HandlerFunc超时处理程序来关闭通道，避免重复关闭
	default:
		close(u.Done)

	}
}
