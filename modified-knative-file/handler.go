/*
Copyright 2018 The Knative Authors

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

package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptrace"
	"net/http/httputil"
	"strconv"
	"strings"
	"time"

	"go.opencensus.io/plugin/ochttp"
	"go.opencensus.io/trace"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/types"

	netheader "knative.dev/networking/pkg/http/header"
	netproxy "knative.dev/networking/pkg/http/proxy"
	"knative.dev/pkg/logging/logkey"
	pkghandler "knative.dev/pkg/network/handlers"
	tracingconfig "knative.dev/pkg/tracing/config"
	"knative.dev/pkg/tracing/propagation/tracecontextb3"
	"knative.dev/serving/pkg/activator"
	activatorconfig "knative.dev/serving/pkg/activator/config"
	apiconfig "knative.dev/serving/pkg/apis/config"
	pkghttp "knative.dev/serving/pkg/http"
	"knative.dev/serving/pkg/networking"
	"knative.dev/serving/pkg/queue"
	"knative.dev/serving/pkg/reconciler/serverlessservice/resources/names"

	"knative.dev/serving/pkg/shared"
)

// Throttler is the interface that Handler calls to Try to proxy the user request.
type Throttler interface {
	Try(ctx context.Context, revID types.NamespacedName, fn func(string) error) error
}

// activationHandler will wait for an active endpoint for a revision
// to be available before proxying the request
type activationHandler struct {
	transport        http.RoundTripper
	tracingTransport http.RoundTripper
	usePassthroughLb bool
	throttler        Throttler
	bufferPool       httputil.BufferPool
	logger           *zap.SugaredLogger
	tls              bool
}

// New constructs a new http.Handler that deals with revision activation.
func New(_ context.Context, t Throttler, transport http.RoundTripper, usePassthroughLb bool, logger *zap.SugaredLogger, tlsEnabled bool) http.Handler {
	return &activationHandler{
		transport: transport,
		tracingTransport: &ochttp.Transport{
			Base:        transport,
			Propagation: tracecontextb3.TraceContextB3Egress,
		},
		usePassthroughLb: usePassthroughLb,
		throttler:        t,
		bufferPool:       netproxy.NewBufferPool(),
		logger:           logger,
		tls:              tlsEnabled,
	}
}

// 定义用于在 context 中存储和检索 lbPolicy 的键
type lbPolicyKey struct{}

// 将lbPolicy存储到context中
func WithLBPolicy(ctx context.Context, lbPolicy string) context.Context {
	return context.WithValue(ctx, lbPolicyKey{}, lbPolicy)
}

// 检索context中存储的lbPolicy
func GetLbPolicy(ctx context.Context) string {
	lbPolicy, _ := ctx.Value(lbPolicyKey{}).(string)
	return lbPolicy
}

func (a *activationHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	config := activatorconfig.FromContext(r.Context())
	tracingEnabled := config.Tracing.Backend != tracingconfig.None

	tryContext, trySpan := r.Context(), (*trace.Span)(nil)
	if tracingEnabled {
		tryContext, trySpan = trace.StartSpan(r.Context(), "throttler_try")
	}

	revID := RevIDFrom(r.Context())

	// 从请求头中提取lbPolicy，并存储到context中，默认为unfixedWaitRandomChoice2Policy
	lbPolicy := r.Header.Get("X-LbPolicy")
	ctx_with_lbpolicy := WithLBPolicy(tryContext, lbPolicy)

	// 取单位为毫秒的时间戳，作为请求到达activator的时间
	arrive_timestamp := strconv.FormatInt(time.Now().UnixNano()/int64(time.Millisecond), 10)
	if err := a.throttler.Try(ctx_with_lbpolicy, revID, func(dest string) error {
		trySpan.End()

		proxyCtx, proxySpan := r.Context(), (*trace.Span)(nil)
		if tracingEnabled {
			proxyCtx, proxySpan = trace.StartSpan(r.Context(), "activator_proxy")
		}

		rate := r.Header.Get("X-Rate")
		last_arrive_timestamp := shared.GetlastArriveTime()
		last_rate := shared.GetLastRate()
		if last_arrive_timestamp != "" && last_rate != "" { // 检查两次到达时间的差是否<=最大等待时间，以及这次的rate是否小于上次的rate，如果二者有一不满足，last_rate设为空
			last_arrive_time, _ := strconv.ParseInt(last_arrive_timestamp, 10, 64)
			arrive_time, _ := strconv.ParseInt(arrive_timestamp, 10, 64)
			rate_int, _ := strconv.Atoi(rate)
			last_rate_int, _ := strconv.Atoi(last_rate)
			if arrive_time-last_arrive_time > int64(shared.MaxWaitingTime) || rate_int >= last_rate_int {
				last_rate = ""
			}
			fmt.Println(arrive_time-last_arrive_time, rate, "啊啊啊啊啊啊啊啊啊啊啊")
		}
		shared.SetlastArriveTime(arrive_timestamp)
		shared.SetLastRate(rate)

		a.proxyRequest(revID, w, r.WithContext(proxyCtx), dest, tracingEnabled, a.usePassthroughLb,
			arrive_timestamp, // rate,
			last_rate)
		proxySpan.End()

		return nil
	}); err != nil {
		// Set error on our capacity waiting span and end it.
		trySpan.Annotate([]trace.Attribute{trace.StringAttribute("activator.throttler.error", err.Error())}, "ThrottlerTry")
		trySpan.End()

		a.logger.Errorw("Throttler try error", zap.String(logkey.Key, revID.String()), zap.Error(err))

		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, queue.ErrRequestQueueFull) {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}
}

// 执行完了负载均衡算法才会回调这个函数，将请求发给pod
func (a *activationHandler) proxyRequest(revID types.NamespacedName, w http.ResponseWriter,
	r *http.Request, target string, tracingEnabled bool, usePassthroughLb bool,
	arrive_timestamp string, // rate string,
	last_rate string) {
	netheader.RewriteHostIn(r)
	r.Header.Set(netheader.ProxyKey, activator.Name)

	// 添加时间戳到请求头，精确到毫秒
	timestamp := strconv.FormatInt(time.Now().UnixNano()/int64(time.Millisecond), 10)
	r.Header.Set("X-Request-Timestamp", timestamp)
	r.Header.Set("X-Arrive-Timestamp", arrive_timestamp)
	r.Header.Set("X-Last-Rate", last_rate)

	// 调度成功，将目标pod的ip和当前任务的rate加入到requestStatic中
	rate, _ := strconv.Atoi(r.Header.Get("X-Rate"))
	targetip := strings.Split(target, ":")[0]
	shared.AddReqToRS(targetip, rate)
	fmt.Println("请求调度成功", targetip, rate)
	shared.PrintRequestStatic()

	// Set up the reverse proxy.
	hostOverride := pkghttp.NoHostOverride
	if usePassthroughLb {
		hostOverride = names.PrivateService(revID.Name) + "." + revID.Namespace
	}

	var proxy *httputil.ReverseProxy
	if a.tls {
		proxy = pkghttp.NewHeaderPruningReverseProxy(useSecurePort(target), hostOverride, activator.RevisionHeaders, true /* uss HTTPS */)
	} else {
		proxy = pkghttp.NewHeaderPruningReverseProxy(target, hostOverride, activator.RevisionHeaders, false /* use HTTPS */)
	}

	proxy.BufferPool = a.bufferPool
	proxy.Transport = a.transport
	if tracingEnabled {
		proxy.Transport = a.tracingTransport
	}
	proxy.FlushInterval = netproxy.FlushInterval
	proxy.ErrorHandler = func(w http.ResponseWriter, req *http.Request, err error) {
		pkghandler.Error(a.logger.With(zap.String(logkey.Key, revID.String())))(w, req, err)
	}

	// 从请求的上下文中获取schedulingDone通道
	schedulingDone, ok := r.Context().Value(shared.SchedulingDoneKey).(chan struct{})

	// 设置 httptrace.ClientTrace，用于在请求发送后关闭 schedulingDone 通道
	trace := &httptrace.ClientTrace{
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			if ok {
				fmt.Println("_______请求已发往目标pod，准备关闭schedulingDone通道_______")
				close(schedulingDone)
			} else {
				fmt.Println("_______请求已发往目标pod，但schedulingDone通道不存在_______")
			}
		},
	}

	// 将 ClientTrace 添加到请求的上下文中
	r = r.WithContext(httptrace.WithClientTrace(r.Context(), trace))

	// 自定义ReverseProxy并设置Director函数
	proxy = &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			// 将原始请求的上下文复制到新的请求中
			req = req.WithContext(r.Context())
			// 设置目标 URL 和 Host
			req.URL.Scheme = "http"
			req.URL.Host = target
			req.Host = target
			// 复制原始请求的 Header
			req.Header = r.Header.Clone()
		},
		Transport:     proxy.Transport,
		BufferPool:    proxy.BufferPool,
		FlushInterval: proxy.FlushInterval,
		ErrorHandler:  proxy.ErrorHandler,
	}

	// 将请求发往目标pod
	proxy.ServeHTTP(w, r)
}

// useSecurePort replaces the default port with HTTPS port (8112).
// TODO: endpointsToDests() should support HTTPS instead of this overwrite but it needs metadata request to be encrypted.
// This code should be removed when https://github.com/knative/serving/issues/12821 was solved.
func useSecurePort(target string) string {
	target = strings.Split(target, ":")[0]
	return target + ":" + strconv.Itoa(networking.BackendHTTPSPort)
}

func WrapActivatorHandlerWithFullDuplex(h http.Handler, logger *zap.SugaredLogger) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		revEnableHTTP1FullDuplex := strings.EqualFold(RevAnnotation(r.Context(), apiconfig.AllowHTTPFullDuplexFeatureKey), "Enabled")
		if revEnableHTTP1FullDuplex {
			rc := http.NewResponseController(w)
			if err := rc.EnableFullDuplex(); err != nil {
				logger.Errorw("Unable to enable full duplex", zap.Error(err))
			}
		}
		// 产生rate并记录进r的请求头
		rate := shared.GenRate()
		r.Header.Set("X-Rate", rate)
		// 设置“X-LbPolicy”为“unfixedWaitRandomChoice2Policy”，表示当前是正经从队头取出的元素，而不是抢占后不等待的任务（决定使用算法的不同）
		r.Header.Set("X-LbPolicy", "unfixedWaitRandomChoice2Policy")
		// 创建一个用于同步的通道
		done := make(chan struct{})
		// 将请求加入队列，传递同步通道
		shared.AddReq(h, w, r, done)
		// 等待请求处理完成
		select {
		case <-done:
			fmt.Println("###rate为", rate, "的任务已经执行完成并返回到http.HandlerFunc")
		case <-time.After(60 * time.Second):
			fmt.Println("###rate为", rate, "的任务整体超时，终止当前handler并清空ActivatorQueue")
			shared.ClearActivatorQueue()
			close(done)
		}
	})
}
