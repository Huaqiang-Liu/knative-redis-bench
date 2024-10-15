import os
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
        value = r.get(f'key_{i}')
        # print(f'key_{i}: {value}')

@app.route('/', methods=['GET'])
def gen_task():
    redis_host = os.getenv('REDIS_HOST', 'localhost')
    redis_port = int(os.getenv('REDIS_PORT', 6379))

    job_len = [8000, 4000, 2000, 1000, 700, 500, 350, 250, 200, 150, 100, 50, 40, 30, 25, 15, 5, 3, 2, 1]
    job_weight = [0.05] * 20
    
    # 若使用scalable.yaml部署任务，则有环境变量TASK_SIZE
    # env:
    #   - name: TASK_SIZE
    #     value: "7000"
    # 根据上面yaml中的配置提取任务长度
    # job_len = [int(os.getenv('TASK_SIZE', 1))]
    # job_weight = [1]
    
    route_time_str = request.headers.get('X-Request-Timestamp')
    arrive_time_str = request.headers.get('X-Arrive-Timestamp')
    if not route_time_str:
        return "No timestamp found in request headers."
    if not arrive_time_str:
        return "No arrive timestamp found in request headers."
    route_time = int(route_time_str)
    arrive_time = int(arrive_time_str)
    
    rate = random.choices(job_len, job_weight)[0]
    start_time = time.time() * 1000
    unit_task(redis_host, redis_port, rate)
    end_time = time.time() * 1000
    
    jct = end_time - route_time # 任务发往pod到执行结束
    responsetime = start_time - arrive_time # 响应时间：任务到达activator到开始执行
    latency = end_time - arrive_time # 总延迟：任务到达activator到执行结束
    # pod_name = os.getenv('HOSTNAME')
    
    # 返回“任务大小 responsetime JCT latency(任务到达activator到执行结束)”
    ret = f'{rate} {responsetime} {jct} {latency}\n'
    
    # 将ret发送给activator，在终端里向activator发包的方式：curl -X POST http://172.18.0.10:30001/ -v
    node_of_activator = os.getenv('NODE_OF_ACTIVATOR')
    activator_url = f'http://172.18.0.{node_of_activator}:30001/'
    try:
        headers = {'X-Arrive-Timestamp': arrive_time_str}
        response = requests.post(activator_url, data=ret, headers=headers)
        response.raise_for_status()
    except requests.exceptions.RequestException as e:
        print(f'Error sending data to activator: {e}')
    return ret

if __name__ == "__main__":
    # 监听所有接口（0.0.0.0），端口8080
    app.run(host='0.0.0.0', port=8080)
    