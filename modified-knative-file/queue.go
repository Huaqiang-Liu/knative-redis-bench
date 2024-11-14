package shared

import (
	"container/list"
	"context"
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
}

// 用于存储请求的线程安全队列
var (
	ActivatorQueue = list.New()
	QueueMutex     sync.Mutex
	QueueCond      = sync.NewCond(&QueueMutex) // 添加条件变量
)

const MaxQueueSize = 10000 // 定义队列的最大容量

func (u *SchedulingUnit) GetRate() int {
	ratestr := u.Req.Header.Get("X-Rate")
	rate, _ := strconv.Atoi(ratestr)
	return rate
}

func AddReq(h http.Handler, w http.ResponseWriter, r *http.Request, done chan struct{}) {
	u := SchedulingUnit{Handler: h, Writer: w, Req: r, Done: done}
	QueueMutex.Lock()
	if ActivatorQueue.Len() >= MaxQueueSize {
		QueueMutex.Unlock()
		// 队列已满，返回服务器繁忙错误
		http.Error(w, "服务器繁忙，请稍后再试。", http.StatusServiceUnavailable)
		return
	}
	ActivatorQueue.PushBack(u) // 将请求加入队列
	QueueCond.Signal()         // 通知等待的协程
	QueueMutex.Unlock()
}

// 定义一个公共的上下文键类型，不能直接用string（静态检查为了防止混淆不让用内置基本类型）
type ContextKey string

const SchedulingDoneKey ContextKey = "schedulingDone"

// 管理调度策略的主函数，在main.go中作为goroutine运行
func ManageQueue() {
	for {
		QueueMutex.Lock()
		for ActivatorQueue.Len() == 0 {
			QueueCond.Wait() // 队列为空，阻塞等待
		}
		e := ActivatorQueue.Front()
		ActivatorQueue.Remove(e)
		QueueMutex.Unlock()

		u := e.Value.(SchedulingUnit)
		fmt.Println("现在队列中有", ActivatorQueue.Len(), "个任务")

		schedulingDone := make(chan struct{})
		// 将通道添加到请求的上下文中
		ctx := context.WithValue(u.Req.Context(), SchedulingDoneKey, schedulingDone)
		u.Req = u.Req.WithContext(ctx)

		// 在新的 goroutine 中执行 ServeHTTP
		go func(u SchedulingUnit) {
			u.Handler.ServeHTTP(u.Writer, u.Req)
			fmt.Println("_______当前任务执行完毕，准备关闭Done通道_______")
			close(u.Done) // 请求处理完成，关闭 done 通道
		}(u)

		// 等待调度完成，立即处理下一个请求
		select {
		case <-schedulingDone:
			fmt.Println("_______当前任务调度完毕，准备处理下一个请求_______")
		case <-time.After(5 * time.Second):
			fmt.Println("_______调度阻塞超过5秒，放弃当前任务_______")
			// ClearActivatorQueue()
			close(schedulingDone)
			close(u.Done)
		}
	}
}

// 添加PeekQueueHead函数，用于查看队首元素但不移除
func PeekQueueHead() (SchedulingUnit, bool) {
	QueueMutex.Lock()
	defer QueueMutex.Unlock()
	if ActivatorQueue.Len() > 0 {
		e := ActivatorQueue.Front()
		u := e.Value.(SchedulingUnit)
		return u, true
	}
	return SchedulingUnit{}, false
}

// 添加清空 ActivatorQueue 的函数
// func ClearActivatorQueue() {
// 	for len(ActivatorQueue) > 0 {
// 		<-ActivatorQueue
// 	}
// }
