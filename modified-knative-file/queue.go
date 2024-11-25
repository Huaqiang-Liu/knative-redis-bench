package shared

import (
	"container/list"
	"fmt"
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
	// Timer   *time.Timer   // 新增字段，用于计时
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

func AddReq(h http.Handler, w http.ResponseWriter, r *http.Request, done chan struct{}) {
	rate, _ := strconv.Atoi(r.Header.Get("X-Rate"))
	// fmt.Println("调用一次AddReq，rate为", rate)
	u := SchedulingUnit{Handler: h, Writer: w, Req: r, Done: done}
	// u.Timer = time.NewTimer(time.Duration(MaxWaitingTime) * time.Millisecond)

	QueueMutex.Lock()
	// fmt.Println("rate为", rate, "的AddReq已经获取队列锁")
	defer QueueMutex.Unlock()

	if Queue.Len() >= MaxQueueize {
		// 队列已满，返回服务器繁忙错误
		fmt.Println("队列已满")
		http.Error(w, "服务器繁忙，请稍后再试。", http.StatusServiceUnavailable)
		close(done)
		return
	}

	// 检查队头元素（如果有）。exp2：如果rate比当前任务的rate大，则直接执行u；
	// exp3：如果队头任务预计执行时间-当前任务预计执行时间>当前任务到达时间-队头任务到达时间，则执行u
	if Queue.Len() > 0 {
		frontRateStr := Queue.Front().Value.(SchedulingUnit).Req.Header.Get("X-Rate")
		frontRate, _ := strconv.Atoi(frontRateStr)
		execTime := JoblenMap[rate]
		frontExecTime := JoblenMap[frontRate]
		arriveTime, _ := strconv.ParseFloat(r.Header.Get("X-Arrive-Timestamp"), 64)
		frontArriveTime, _ := strconv.ParseFloat(Queue.Front().Value.(SchedulingUnit).Req.Header.Get("X-Arrive-Timestamp"), 64)

		// if rate < frontRate {
		if float64(frontExecTime)-float64(execTime) > arriveTime-frontArriveTime {
			u.Req.Header.Set("X-Last-Rate", frontRateStr)
			// fmt.Println("rate为", rate, "的任务抢占了rate为", frontRate, "的任务")
			go serveRequest(u)
			return
		}
	}

	fmt.Println("rate为", rate, "的任务加入队列，当前队列长度为", Queue.Len())
	Queue.PushBack(u)
	QueueCond.Signal() // 让ManageQueue中该队列对应的goroutine解除阻塞
}

// rateIndex大的对应长任务，先看队头任务有没有到期，如果没有再从小到大地看小rate的队头有没有任务，如果有就work stealing
func ManageQueue() {
	for {
		QueueMutex.Lock()
		for Queue.Len() == 0 {
			// fmt.Println("已知rate为", rate, "的队列为空")
			QueueCond.Wait() // 队列为空，阻塞等待
		}

		e := Queue.Front()
		u := e.Value.(SchedulingUnit)
		Queue.Remove(e)
		QueueMutex.Unlock()

		// fmt.Println("从队列中serve rate为", u.Req.Header.Get("X-Rate"), "的请求")
		go serveRequest(u)

		// 睡1000/lambda ms
		time.Sleep(time.Duration(1000/Lambda) * time.Millisecond)

		// rate, _ := strconv.Atoi(u.Req.Header.Get("X-Rate"))

		// Outer:
		// 	for {
		// 		select {
		// 		case <-u.Timer.C:
		// 			break Outer
		// 		default:
		// 			QueueMutex.Lock()
		// 			if Queue.Len() == 0 {
		// 				QueueMutex.Unlock()
		// 				break
		// 			}
		// 			shorterE := Queue.Front()
		// 			shorterU := shorterE.Value.(SchedulingUnit)
		// 			frontRate, _ := strconv.Atoi(shorterU.Req.Header.Get("X-Rate"))
		// 			if frontRate < rate {
		// 				shorterU.Req.Header.Set("X-Last-Rate", strconv.Itoa(rate))
		// 				go serveRequest(shorterU)
		// 				Queue.Remove(shorterE)
		// 			} else {
		// 				QueueMutex.Unlock()
		// 				break Outer
		// 			}
		// 			QueueMutex.Unlock()
		// 		}
		// 	}
		// 	go serveRequest(u)
	}
}

func serveRequest(u SchedulingUnit) {
	// fmt.Println("Serve一个请求")
	u.Handler.ServeHTTP(u.Writer, u.Req)
	close(u.Done)
}
