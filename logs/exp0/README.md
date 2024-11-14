早期绑定&延迟绑定性能对比
用基于轮询的算法，早期绑定就是简单的轮流发；延迟绑定就是不限制调度花费时间（等待时间），直到当前轮询到的pod空闲

**问题：如果和实验二一样用基于power of 2的算法，延迟绑定可能会消耗大量时间在调度上，因为队头任务调度完成了才会取下一个任务，而用这个算法就会不断检查选出的两个pod是否空闲了（而不是再随机选两个pod）。所以最坏结果就是任务执行多长时间就要调度多长时间——任何情况下这都是无法接受的。加入最大等待时间能解决这一点（所以后面的算法都是基于power of two，不过这相比原本纯并发的写法，带来了不必要的延迟**

测试用例：ServerlessBench的Testcase1-Resource-efficiency，CPU密集型的ALU程序

测试压力与任务分布：128连接，测试配置locust.py中的lambda，即每连接每秒任务数的期望为4

统计指标：已知请求到达activator的时间戳t1，选择目标pod完成准备发出的时间戳t2，任务开始执行的时间戳t3，任务执行完成的时间戳t4。对每个rate，统计平均响应时间（t3-t1）、平均和P99latency（t4-t1）、平均和P99slowdown（latency/JCT（t4-t2））

统计结果：每个单元格中左为早期绑定，右为延迟绑定。横为指标，纵为rate，时间单位均为毫秒

|      | avg res | avg lat | p99 lat | avg slo | p99 slo |
| :--: | :-----: | ------- | ------- | ------- | ------- |
|  1   |         |         |         |         |         |
|  2   |         |         |         |         |         |
|  3   |         |         |         |         |         |
|  5   |         |         |         |         |         |
|  15  |         |         |         |         |         |
|  25  |         |         |         |         |         |
|  30  |         |         |         |         |         |
|  40  |         |         |         |         |         |
|  50  |         |         |         |         |         |
| 100  |         |         |         |         |         |
| 150  |         |         |         |         |         |
| 200  |         |         |         |         |         |
| 250  |         |         |         |         |         |
| 350  |         |         |         |         |         |
| 500  |         |         |         |         |         |
| 700  |         |         |         |         |         |
| 1000 |         |         |         |         |         |
| 2000 |         |         |         |         |         |
| 4000 |         |         |         |         |         |
| 8000 |         |         |         |         |         |


问题：早期绑定有极巨量的下列4个错误，可能是因为queue-proxy自身的策略，长任务执行的极少，几乎全超时了
- upstream connect error or disconnect/reset before headers. reset reason: connection termination
- upstream connect error or disconnect/reset before headers. reset reason: remote connection failure, transport failure reason: delayed connect error: 111
- lack headers
- timed out dialing 127.0.0.1:8080 after 多少多少秒

