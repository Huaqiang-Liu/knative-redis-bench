package shared

import (
	"container/list"
	"fmt"
	"net/http"
	"strconv"
	"sync"
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
	fmt.Println("调用一次AddReq，rate为", rate)
	u := SchedulingUnit{Handler: h, Writer: w, Req: r, Done: done}
	// u.Timer = time.NewTimer(time.Duration(MaxWaitingTime) * time.Millisecond)

	QueueMutex.Lock()
	// fmt.Println("rate为", rate, "的AddReq已经获取队列锁")
	defer QueueMutex.Unlock()

	if Queue.Len() >= MaxQueueize {
		// 队列已满，返回服务器繁忙错误
		http.Error(w, "服务器繁忙，请稍后再试。", http.StatusServiceUnavailable)
		close(done)
		return
	}

	// 检查队头元素（如果有），如果rate比当前任务的rate大，则直接执行u
	// if Queue.Len() > 0 {
	// 	frontRateStr := Queue.Front().Value.(SchedulingUnit).Req.Header.Get("X-Rate")
	// 	frontRate, _ := strconv.Atoi(frontRateStr)
	// 	if rate < frontRate {
	// 		u.Req.Header.Set("X-Last-Rate", frontRateStr)
	// 		go serveRequest(u)
	// 		return
	// 	}
	// }

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

		go serveRequest(u)

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
	fmt.Println("Serve一个请求")
	u.Handler.ServeHTTP(u.Writer, u.Req)
	close(u.Done)
}
