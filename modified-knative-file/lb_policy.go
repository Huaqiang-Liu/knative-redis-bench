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
	"math/rand"
	"strings"
	"sync"

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

// 早期绑定的power of 2
func simpleRandomChoice2Policy() lbPolicy { // 直接用它//////////
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
			// if pick1ip == shared.ChoosePodByNumOfJobs(pick1ip, pick2ip) {
			return noop, pick1
		} else {
			return noop, pick2
		}
	}
}

// 延迟绑定的power of 2：随机选两个，看谁先空闲就发过去
func lateRandomChoice2Policy() lbPolicy {
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
		for {
			cho := shared.ChooseIdlePod(pick1ip, pick2ip)
			if cho == pick1ip {
				return noop, pick1
			} else if cho == pick2ip {
				return noop, pick2
			}
		}
	}
}
