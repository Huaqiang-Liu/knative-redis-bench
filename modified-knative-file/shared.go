// 将决定算法的全局数据结构及其暴露接口放在这个包下，由main.go添加或删除数据项，lb_policy.go中的函数读取
// 该数据结构以实现新的负载均衡算法

package shared

import (
	"sync"
)

type RequestStatic struct {
	sync.RWMutex
	Data map[string]string
}

var requestStatic = &RequestStatic{
	Data: make(map[string]string),
}

func GetRequestStatic() *RequestStatic {
	requestStatic.RLock()
	defer requestStatic.RUnlock()
	return requestStatic
}

func SetRequestStatic(key string, value string) {
	requestStatic.Lock()
	defer requestStatic.Unlock()
	requestStatic.Data[key] = value
}
