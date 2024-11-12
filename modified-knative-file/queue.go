package shared

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
)

type SchedulingUnit struct {
	Handler http.Handler
	Writer  http.ResponseWriter
	Req     *http.Request
	Done    chan struct{} // 用于通知请求执行完成的通道，执行完了HandlerFunc才能关闭，否则会丢失上下文
}

// 用于存储请求的队列。channel本身就是线程安全的，不需要额外配置sync.Cond。10000是队列最大长度而非字节数
var ActivatorQueue = make(chan SchedulingUnit, 10000)

const TmpTaskTime = 50 // 暂定的任务执行时间，应该换为队头任务实际的执行时间（只不过现在rate和具体时间还没对上）

func (u *SchedulingUnit) GetRate() int {
	ratestr := u.Req.Header.Get("X-Rate")
	rate, _ := strconv.Atoi(ratestr)
	return rate
}

func AddReq(h http.Handler, w http.ResponseWriter, r *http.Request, done chan struct{}) {
	// 创建用于通知调度完成的通道

	u := SchedulingUnit{Handler: h, Writer: w, Req: r, Done: done}
	ActivatorQueue <- u // 将请求加入队列
}

// 定义一个公共的上下文键类型，不能直接用string（静态检查为了防止混淆不让用内置基本类型）
type ContextKey string

const SchedulingDoneKey ContextKey = "schedulingDone"

// 管理调度策略的主函数，在main.go中作为goroutine运行
func ManageQueue() {
	for {
		fmt.Println("现在队列中有", len(ActivatorQueue), "个任务")
		u := <-ActivatorQueue

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
		<-schedulingDone
		fmt.Println("_______当前任务调度完毕，准备处理下一个请求_______")
	}
}
