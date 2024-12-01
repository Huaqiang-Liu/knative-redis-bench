package shared

import (
	"container/list"
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
var varx = 3380 + 1000/float64(Lambda)
var vary = 0.5

func AddReq0(h http.Handler, w http.ResponseWriter, r *http.Request, done chan struct{}) { // Baseline：直接加入队列
	u := SchedulingUnit{Handler: h, Writer: w, Req: r, Done: done}

	QueueMutex.Lock()
	defer QueueMutex.Unlock()
	len := Queue.Len()

	if len >= MaxQueueize {
		fmt.Println("队列已满")
		close(done)
		return
	}

	Queue.PushBack(u)
	QueueCond.Signal()
}
func ManageQueue0() { // Baseline：不断取队头元素然后serve
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

func AddReq(h http.Handler, w http.ResponseWriter, r *http.Request, done chan struct{}) { // 实验2：简单抢占
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

func AddReq3(h http.Handler, w http.ResponseWriter, r *http.Request, done chan struct{}) { // 实验3
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

	D := float64(JoblenMap[rate]) - CalculateAvgExecTime() + varx
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
func ManageQueue() { // 实验2，3
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
		TimeoutJobNumMutex.Lock()
		TimeoutJobNum += 1
		fmt.Println("超时任务数量为", TimeoutJobNum)
		TimeoutJobNumMutex.Unlock()
		// 让HandlerFunc超时处理程序来关闭通道，避免重复关闭
	default:
		close(u.Done)

	}
}
