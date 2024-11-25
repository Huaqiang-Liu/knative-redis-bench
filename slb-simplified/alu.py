# Copyright (c) 2020 Institution of Parallel and Distributed System, Shanghai Jiao Tong University
# ServerlessBench is licensed under the Mulan PSL v1.
# You can use this software according to the terms and conditions of the Mulan PSL v1.
# You may obtain a copy of Mulan PSL v1 at:
#     http://license.coscl.org.cn/MulanPSL
# THIS SOFTWARE IS PROVIDED ON AN "AS IS" BASIS, WITHOUT WARRANTIES OF ANY KIND, EITHER EXPRESS OR
# IMPLIED, INCLUDING BUT NOT LIMITED TO NON-INFRINGEMENT, MERCHANTABILITY OR FIT FOR A PARTICULAR
# PURPOSE.
# See the Mulan PSL v1 for more details.

import socket
import time
import os
import requests
import random
from flask import Flask, request

app = Flask(__name__)

# 原本是用多线程分摊执行次数times，但是考虑到每个pod只分配单核，这里不妨去掉多线程的写法
def alu(times):
    a = random.randint(10, 100)
    b = random.randint(10, 100)
    temp = 0
    for i in range(times * 25000):
        if i % 4 == 0:
            temp = a + b
        elif i % 4 == 1:
            temp = a - b
        elif i % 4 == 2:
            temp = a * b
        else:
            temp = a / b
    print(temp)

@app.route('/', methods=['GET'])
def handle_request():
    route_time_str = request.headers.get('X-Request-Timestamp')
    arrive_time_str = request.headers.get('X-Arrive-Timestamp')
    rate_str = request.headers.get('X-Rate')
    last_rate_str = request.headers.get('X-Last-Rate')
    if not route_time_str or not arrive_time_str or not rate_str:
        return "lack headers"
    
    rate = int(rate_str)
    if last_rate_str == "":
        last_rate_str = "0"
    
    route_time = float(route_time_str)
    arrive_time = float(arrive_time_str)
    
    start_time = time.time() * 1000
    alu(rate)
    end_time = time.time() * 1000
    
    jct = end_time - route_time # 任务发往pod到执行结束
    responsetime = start_time - arrive_time # 响应时间：任务到达activator到开始执行
    latency = end_time - arrive_time # 总延迟：任务到达activator到执行结束
    
    # 返回“任务大小 responsetime JCT latency(任务到达activator到执行结束) 上一次的任务大小（0表示没有或不符合要求）”
    ret = f'{rate} {responsetime} {jct} {latency} {last_rate_str}\n'
    
    # 将ret发送给activator，在终端里向activator发包的方式：curl -X POST http://172.18.0.10:30001/ -v
    node_of_activator = os.getenv('NODE_OF_ACTIVATOR')
    activator_url = f'http://172.18.0.{node_of_activator}:30001/store'
    try:
        headers = {'X-PodIP': socket.gethostbyname(socket.gethostname())}
        response = requests.post(activator_url, data=ret, headers=headers)
        response.raise_for_status()
    except requests.exceptions.RequestException as e:
        return f"{ret} 无法将任务信息发给activator {activator_url}：{e}"
    return ret

if __name__ == "__main__":
    # 监听所有接口（0.0.0.0），端口8080
    app.run(host='0.0.0.0', port=8080)


