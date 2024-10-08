# 实验环境
- Ubuntu 22.04 server，kernel 5.15.0-113-generic
- docker 24.0.7
- go1.22.5 linux/amd64
- kind v0.23.0
- kubernetes：Client Version: v1.30.3，Kustomize Version: v5.0.4-0.20230601165947-6ce0bf390ce3，Server Version: v1.30.0
- knative serving：v1.15.2，包括kn CLI v1.14.0，istio v1.15.1
- 镜像中的python 3.8slim
- 8worker1controller本地k8s集群，每个worker node上安装有redis-server，每个node的虚拟cpu数和虚拟内存容量都跟主机一致

# 实验描述
- 部署的knative服务：python程序（benchmark.py，用run.sh运行），连接节点上的redis-server来执行负载，有一个函数作为基本任务单元，每个实例随机一种任务大小，总体上符合[这篇论文](https://www.microsoft.com/en-us/research/uploads/prod/2021/09/sosp21-final604.pdf)中给出的分布，具体地：任务长度为1, 3, 25, 250, 7500个单位的概率是0.05, 0.03, 0.17, 0.33, 0.37。单跑7500单位的任务需时275s，任务时长超过300s会导致activator超时错误
- 执行的命令：`wrk -t16 -c16 -d1800s -s ./print_response.lua -H "Host: redis-benchmark.default.example.com" http://$GATEWAY_URL -v`，压测过程中原本能跑完的也引发了activator timeout，最终一个7500单位的任务也没跑出来
- benchmark的输出：每行4个数：任务大小（单位数），任务开始时间戳，任务结束时间戳，任务持续时间。最后是执行wrk的时间戳（单位都为毫秒）
- 若不启用autoscaler，即总在单个节点上跑，总执行成功任务数量70/125，包括：
	- 55次“activator request timeout”
	- 4次1单位长度的任务
	- 5次3单位长度的任务
	- 25次25单位长度的任务
	- 36次250单位长度的任务
- 若每个pod的并发期望为1，则共有23个pod实例，分布在各个worker上。总执行成功任务数量100/175，包括：
	- 75次“activator request timeout”
	- 15次1单位长度的任务
	- 5次3单位长度的任务
	- 34次25单位长度的任务
	- 46次250单位长度的任务

# 改进（使用默认的早期绑定策略）
这一部分使用的是knative默认的早期绑定，但不同于knative默认的那种“早期绑定”，为了获得任务被调度到pod上的时间戳，我让activator始终处于流量路径中负责将任务轮流路由给pod，而非交给k8s系统来轮转调度。需要说明的统计数据如下：
- JCT的记录方式：serving/pkg/handler/handler.go的proxyRequest函数中，给发出去的http请求添加一条请求头。实验证明这个发自activator的请求确实会发到相应的pod上，并由服务程序实例接收。这样就可以让实例程序读取到包含发送请求时间戳（也即调度发生的时间戳），并计算得到JCT。
- 响应时间=任务开始执行的时间戳-请求发出的时间戳。

负载的具体设置上也有改进：在分布基本不变的基础上，将时间缩一下，同时细化粒度，单位任务大小减少为原来的1/10，免得时间最长的任务跑不起来，另外减少压测时间到600秒，运行过程中pod的最大数量和autoscaler配置不变
![distribution](./assets/distribution.png)

现在的分布为：
`[8000, 4000, 2000, 1000, 700, 500, 350, 250, 200, 150, 100, 50, 40, 30, 25, 15, 5, 3, 2, 1]`，每种概率5%

[原始数据](logs/Oct8/earlybounding/log23：00.txt)，[统计数据](logs/Oct8/earlybounding/result23：00.txt)。

# 实现延迟绑定
实现延迟绑定的方式：在redis-service.yaml中规定containerConcurrency为1（或者其它非零数），然后修改serving/pkg/net/throttler.go的newRevisionThrottler函数，指定负载均衡算法始终为firstAvailableLBPolicy，这样activator就始终负责选择空闲出来的pod，并将请求路由给相应的pod，而无需k8s系统自身的调度。

- 并发度为1，即批处理的情况：[原始数据](logs/Oct8/latebounding/log22：00.txt)，[统计数据](logs/Oct8/latebounding/result22：00.txt)。