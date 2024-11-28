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

func AddReq(h http.Handler, w http.ResponseWriter, r *http.Request, done chan struct{}) {
	rate, _ := strconv.Atoi(r.Header.Get("X-Rate"))
	// fmt.Println("调用一次AddReq，rate为", rate)
	u := SchedulingUnit{Handler: h, Writer: w, Req: r, Done: done}

	QueueMutex.Lock()
	// fmt.Println("rate为", rate, "的AddReq已经获取队列锁")
	defer QueueMutex.Unlock()
	len := Queue.Len()
	if len > MaxQueueActualLen {
		MaxQueueActualLen = len
		fmt.Println("当前最大队列长度为", MaxQueueActualLen)
	}

	if len >= MaxQueueize {
		// 队列已满，返回服务器繁忙错误
		fmt.Println("队列已满")
		// http.Error(w, "服务器繁忙，请稍后再试。", http.StatusServiceUnavailable)
		close(done)
		return
	}

	D := float64(JoblenMap[rate]) - CalculateAvgExecTime()
	if float64(Lambda)*D < 1000 {
		u.Req.Header.Set("X-Last-Rate", "1")
		u.Timer = time.NewTimer(0)
		go serveRequest(u)
		return
	}

	u.Timer = time.NewTimer(time.Duration(1000000*math.Log(float64(Lambda)*D/1000)/D) * time.Millisecond)
	Queue.PushBack(u)
	QueueCond.Signal() // 让ManageQueue中该队列对应的goroutine解除阻塞
}

// 不停轮询，看到计时器到期的就发出去
func ManageQueue() {
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
	// fmt.Println("Serve一个请求")
	u.Handler.ServeHTTP(u.Writer, u.Req)
	close(u.Done)
}
