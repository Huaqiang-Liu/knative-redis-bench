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
	Timer   *time.Timer   // 新增字段，用于计时
}

// 用于存储请求的线程安全队列，都是用rate索引，而非rateIndex
var (
	Queues     map[int]*list.List
	QueueMutex map[int]*sync.Mutex
	QueueConds map[int]*sync.Cond // 队列空阻塞，有任务唤醒
)

const MaxQueueSize = 10000 // 定义队列的最大容量

func init() { // 包加载的时候就会自动调用init函数，所以初始化什么的都可以放这里面搞
	Queues = make(map[int]*list.List)
	QueueMutex = make(map[int]*sync.Mutex)
	QueueConds = make(map[int]*sync.Cond)

	for rateIndex := 19; rateIndex >= 0; rateIndex-- {
		rate := Joblen[rateIndex]
		Queues[rate] = list.New()
		QueueMutex[rate] = &sync.Mutex{}
		QueueConds[rate] = sync.NewCond(QueueMutex[rate])
	}
}

func (u *SchedulingUnit) GetRate() int {
	ratestr := u.Req.Header.Get("X-Rate")
	rate, _ := strconv.Atoi(ratestr)
	return rate
}

func AddReq(h http.Handler, w http.ResponseWriter, r *http.Request, done chan struct{}) {
	rate, _ := strconv.Atoi(r.Header.Get("X-Rate"))
	fmt.Println("调用一次AddReq，rate为", rate)
	u := SchedulingUnit{Handler: h, Writer: w, Req: r, Done: done}
	u.Timer = time.NewTimer(time.Duration(MaxWaitingTime) * time.Millisecond)

	QueueMutex[rate].Lock() // 看来是在这一句卡住了，肯定是哪个傻逼没释放
	fmt.Println("rate为", rate, "的AddReq已经获取队列锁")
	defer QueueMutex[rate].Unlock()
	queue := Queues[rate]
	if queue.Len() >= MaxQueueSize {
		// 队列已满，返回服务器繁忙错误
		http.Error(w, "服务器繁忙，请稍后再试。", http.StatusServiceUnavailable)
		close(done)
		return
	}
	queue.PushBack(u)
	fmt.Println("加入队列成功，现在rate为", rate, "的队列长度为", queue.Len())
	QueueConds[rate].Signal() // 让ManageQueue中该队列对应的goroutine解除阻塞
	fmt.Println("唤醒rate为", rate, "的队列的信号已经发出")
}

// rateIndex大的对应长任务，先看队头任务有没有到期，如果没有再从小到大地看小rate的队头有没有任务，如果有就work stealing
func ManageQueue(rateIndex int) {
	rate := Joblen[rateIndex]
	for {
		// fmt.Println("rate为", rate, "的队列的ManageQueue开始执行")
		QueueMutex[rate].Lock()
		// fmt.Println("rate为", rate, "的队列已经获得队列锁，队列长度为", Queues[rate].Len())
		for Queues[rate].Len() == 0 {
			// fmt.Println("已知rate为", rate, "的队列为空")
			QueueConds[rate].Wait() // 队列为空，阻塞等待
		}

		e := Queues[rate].Front()
		u := e.Value.(SchedulingUnit)
		Queues[rate].Remove(e)
		QueueMutex[rate].Unlock()
		// fmt.Println("rate为", rate, "的队列已经释放队列锁，队列长度为", Queues[rate].Len())

		// go serveRequest(u)

		// 没有下面这段抢占的东西，都能正常执行，但是有的话会导致运行过任务的队列，再一次AddReq的时候阻塞在54行了，所以肯定是因为下面哪里没有正确释放
		select {
		case <-u.Timer.C:
			go serveRequest(u)
		default:
		PreepmtOver: // break到这里的时候下面的for循环就不会执行了
			for {
				if rate == 1 {
					break
				}
				// hasPreempted := false
				for lowerRateIndex := 19; lowerRateIndex > rateIndex; lowerRateIndex-- {
					lowerRate := Joblen[lowerRateIndex]
					// 尝试非阻塞地获取锁，并检查是否有任务可以偷取
					if QueueMutex[lowerRate].TryLock() {
						for Queues[lowerRate].Len() > 0 {
							lowerE := Queues[lowerRate].Front()
							lowerU := lowerE.Value.(SchedulingUnit)
							Queues[lowerRate].Remove(lowerE)

							// 将该任务抢占的任务的rate加入请求头
							lowerU.Req.Header.Set("X-Last-Rate", strconv.Itoa(rate))

							go serveRequest(lowerU)
							// hasPreempted = true

							// 检查当前任务的计时器是否到期，防止饥饿
							if <-u.Timer.C; true {
								QueueMutex[lowerRate].Unlock()
								break PreepmtOver
							}
						}
						QueueMutex[lowerRate].Unlock()
						break
					} else {
						// 检查当前任务的计时器是否到期，防止饥饿
						if <-u.Timer.C; true {
							break PreepmtOver
						}
					}
				}

				// 先暂定一点：如果扫了所有队列都没有任务可以抢占（没取得锁不算），就直接执行队头任务，不再尝试
				// if !hasPreempted {
				// 	break
				// }
			}

			// 该抢占的都抢占完了，执行队头任务
			go serveRequest(u)
		}
	}
}

func serveRequest(u SchedulingUnit) {
	fmt.Println("Serve一个请求")
	u.Handler.ServeHTTP(u.Writer, u.Req)
	close(u.Done)
}
