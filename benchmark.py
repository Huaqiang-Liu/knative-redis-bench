import os
import socket
import redis
import random
import time
from flask import Flask, request
import requests

app = Flask(__name__)

def unit_task(redis_host, redis_port, rate = 1):
    redis_password = 'pmT0bVLwFr' # os.getenv('REDIS_PASSWORD', None)
    r = redis.StrictRedis(host=redis_host, port=redis_port, password=redis_password, decode_responses=True)

    for i in range(rate):
        r.set(f'key_{i}', f'value_{i}')

@app.route('/', methods=['GET'])
def gen_task():
    redis_host = os.getenv('REDIS_HOST', 'localhost')
    redis_port = int(os.getenv('REDIS_PORT', 6379))
    
    route_time_str = request.headers.get('X-Request-Timestamp')
    arrive_time_str = request.headers.get('X-Arrive-Timestamp')
    rate_str = request.headers.get('X-Rate')
    last_rate_str = request.headers.get('X-Last-Rate')
    if not route_time_str or not arrive_time_str or not rate_str:
        return "lack headers"
    
    rate = int(rate_str)
    if last_rate_str == "":
        last_rate_str = "0"
    
    route_time = int(route_time_str)
    arrive_time = int(arrive_time_str)
    
    start_time = time.time() * 1000
    unit_task(redis_host, redis_port, rate)
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
    