package shared

import (
	"net/http"
	"strconv"
)

type SchedulingUnit struct {
	Handler http.Handler
	Writer  http.ResponseWriter
	Req     *http.Request
	Done    chan struct{}
}

var ActivatorQueue = make(chan SchedulingUnit, 10000) // 用于存储请求的队列。channel本身就是线程安全的，不需要额外配置sync.Cond

const TmpTaskTime = 50 // 暂定的任务执行时间，应该换为队头任务实际的执行时间（只不过现在rate和具体时间还没对上）

func (u *SchedulingUnit) GetRate() int {
	ratestr := u.Req.Header.Get("X-Rate")
	rate, _ := strconv.Atoi(ratestr)
	return rate
}

func AddReq(h http.Handler, w http.ResponseWriter, r *http.Request, done chan struct{}) {
	u := SchedulingUnit{Handler: h, Writer: w, Req: r, Done: done}
	ActivatorQueue <- u // 将请求加入队列
}

// 管理调度策略的主函数，在main.go中作为goroutine运行
func ManageQueue() {
	for {
		u := <-ActivatorQueue
		u.Handler.ServeHTTP(u.Writer, u.Req)
		close(u.Done) // ServeHTTP不执行完就不会取下一个请求，所以可以在算法中取下一个来抢占
	}
}
