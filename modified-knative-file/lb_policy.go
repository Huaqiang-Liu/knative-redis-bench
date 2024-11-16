/*
Copyright 2020 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// This file contains the load load balancing policies for Activator load balancing.

package net

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	"knative.dev/serving/pkg/activator/handler"
	"knative.dev/serving/pkg/shared"
)

// lbPolicy is a functor that selects a target pod from the list, or (noop, nil) if
// no such target can be currently acquired.
// Policies will presume that `targets` list is appropriately guarded by the caller,
// that is while podTrackers themselves can change during this call, the list
// and pointers therein are immutable.
type lbPolicy func(ctx context.Context, targets []*podTracker) (func(), *podTracker)

// randomLBPolicy is a load balancer policy that picks a random target.
// This approximates the LB policy done by K8s Service (IPTables based).
//
// nolint // This is currently unused but kept here for posterity.
func randomLBPolicy(_ context.Context, targets []*podTracker) (func(), *podTracker) {
	return noop, targets[rand.Intn(len(targets))]
}

// randomChoice2Policy implements the Power of 2 choices LB algorithm
func randomChoice2Policy(_ context.Context, targets []*podTracker) (func(), *podTracker) {
	// Avoid random if possible.
	l := len(targets)
	// One tracker = no choice.
	if l == 1 {
		pick := targets[0]
		pick.increaseWeight()
		return pick.decreaseWeight, pick
	}
	r1, r2 := 0, 1
	// Two trackers - we know both contestants,
	// otherwise pick 2 random unequal integers.
	if l > 2 {
		r1, r2 = rand.Intn(l), rand.Intn(l-1) //nolint:gosec // We don't need cryptographic randomness here.
		// shift second half of second rand.Intn down so we're picking
		// from range of numbers other than r1.
		// i.e. rand.Intn(l-1) range is now from range [0,r1),[r1+1,l).
		if r2 >= r1 {
			r2++
		}
	}

	pick, alt := targets[r1], targets[r2]
	// Possible race here, but this policy is for CC=0,
	// so fine.
	if pick.getWeight() > alt.getWeight() {
		pick = alt
	} else if pick.getWeight() == alt.getWeight() {
		//nolint:gosec // We don't need cryptographic randomness here.
		if rand.Int63()%2 == 0 {
			pick = alt
		}
	}
	pick.increaseWeight()
	return pick.decreaseWeight, pick
}

// firstAvailableLBPolicy is a load balancer policy, that picks the first target
// that has capacity to serve the request right now.
func firstAvailableLBPolicy(ctx context.Context, targets []*podTracker) (func(), *podTracker) {
	for _, t := range targets {
		if cb, ok := t.Reserve(ctx); ok {
			return cb, t
		}
	}
	return noop, nil
}

// newRoundRobinPolicy如果发不出去就不发了，但是我想搞早期绑定，所以发不出去我嗯要发出去
func pureRoundRobinPolicy() lbPolicy {
	var (
		mu  sync.Mutex
		idx int
	)
	return func(ctx context.Context, targets []*podTracker) (func(), *podTracker) {
		mu.Lock()
		defer mu.Unlock()
		// The number of trackers might have shrunk, so reset to 0.
		l := len(targets)
		if idx >= l {
			idx = 0
		}
		// 直接发target[idx]，然后idx+1，不检查是否空闲
		p := idx
		idx = (idx + 1) % l
		return noop, targets[p]
	}
}

// 延迟绑定，每次都要检查是否空闲
func newRoundRobinPolicy() lbPolicy {
	var (
		mu  sync.Mutex
		idx int
	)
	return func(ctx context.Context, targets []*podTracker) (func(), *podTracker) {
		mu.Lock()
		defer mu.Unlock()
		// The number of trackers might have shrunk, so reset to 0.
		l := len(targets)
		if idx >= l {
			idx = 0
		}

		for {
			for i := 0; i < l; i++ {
				p := (idx + i) % l
				if !shared.CheckPodBusy(strings.Split(targets[p].dest, ":")[0]) {
					idx = (p + 1) % l
					return noop, targets[p]
				}
			}
		}

	}
}

// 不开计时器，直接随机从targets中选两个，将当前负载轻的作为target
func simpleRandomChoice2Policy() lbPolicy {
	var (
		mu sync.Mutex
	)
	return func(ctx context.Context, targets []*podTracker) (func(), *podTracker) {
		mu.Lock()
		defer mu.Unlock()
		l := len(targets)
		if l == 1 {
			pick := targets[0]
			return noop, pick
		}
		r1, r2 := rand.Intn(l), rand.Intn(l-1)
		if r2 >= r1 {
			r2++
		}
		pick1, pick2 := targets[r1], targets[r2]

		pick1ip, pick2ip := strings.Split(pick1.dest, ":")[0], strings.Split(pick2.dest, ":")[0]
		if pick1ip == shared.ChoosePodByRate(pick1ip, pick2ip) {
			return noop, pick1
		} else {
			return noop, pick2
		}
	}
}

// 支持简单抢占的power of two
func unfixedWaitRandomChoice2Policy() lbPolicy {
	var (
		mu sync.Mutex
	)
	return func(ctx context.Context, targets []*podTracker) (func(), *podTracker) {
		mu.Lock()
		defer mu.Unlock()
		l := len(targets)
		if l == 1 {
			// 只有一个pod，直接选择它，不管它是否空闲
			pick := targets[0]
			return noop, pick
		}
		// 随机选择两个pod
		r1, r2 := rand.Intn(l), rand.Intn(l-1)
		if r2 >= r1 {
			r2++
		}
		pick1, pick2 := targets[r1], targets[r2]
		pick1ip, pick2ip := strings.Split(pick1.dest, ":")[0], strings.Split(pick2.dest, ":")[0]

		fmt.Println("现在有两个pod可以选择，分别是：", pick1.dest, pick2.dest)

		startTime := time.Now()
		timer := time.NewTimer(time.Duration(shared.MaxWaitingTime) * time.Millisecond)
		defer timer.Stop()

		thisrate := handler.GetRate(ctx)

		for {
			select {
			case <-timer.C:
				// 超过最大等待时间，直接返回负载较轻的pod
				if pick1.dest == shared.ChoosePodByRate(pick1.dest, pick2.dest) {
					return noop, pick1
				} else {
					return noop, pick2
				}
			default:
				// 检查ActivatorQueue队头是否有元素，以及适不适合抢占——如果队头任务的rate比当前任务的rate大就能抢占
				if u, ok := shared.PeekQueueHead(); ok {
					rateStr := u.Req.Header.Get("X-Rate")
					rate, _ := strconv.Atoi(rateStr)
					elapsed := time.Since(startTime)
					remainingTime := time.Duration(shared.MaxWaitingTime)*time.Millisecond - elapsed
					if thisrate > rate {
						timer.Stop()

						// fmt.Println("现在有一个可以抢占的任务在队头，队列的长度为：", shared.ActivatorQueue.Len())
						// 从队列中取出被抢占的任务
						shared.QueueMutex.Lock()
						e := shared.ActivatorQueue.Front()
						shared.ActivatorQueue.Remove(e)
						shared.QueueMutex.Unlock()

						u := e.Value.(shared.SchedulingUnit)
						u.Req.Header.Set("X-LbPolicy", "simpleRandomChoice2Policy")

						// 从被抢占任务的请求上下文中获取 schedulingDone 通道
						schedulingDone, ok := u.Req.Context().Value(shared.SchedulingDoneKey).(chan struct{})
						if !ok {
							fmt.Println("错误：未找到当前抢占任务的schedulingDone通道")
						}

						// 异步执行抢占任务的ServeHTTP方法
						go func(u shared.SchedulingUnit) {
							u.Handler.ServeHTTP(u.Writer, u.Req)
							fmt.Println("_______抢占的任务执行完毕_______")
							close(u.Done)
						}(u)

						// 等待被抢占任务完成调度
						select {
						case <-schedulingDone:
							fmt.Println("抢占的任务已经完成调度")
						case <-time.After(5 * time.Second):
							fmt.Println("抢占的任务调度超时")
							close(u.Done)
						}

						// 重置计时器，继续当前任务的调度
						timer.Reset(remainingTime)
					}
				}

				// 没有可以抢占的任务，看看是否有空闲pod
				idle := shared.ChooseIdlePod(pick1ip, pick2ip)
				if pick1ip == idle {
					return noop, pick1
				} else if pick2ip == idle {
					return noop, pick2
				}
			}
		}
	}
}
