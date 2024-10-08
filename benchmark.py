import os
import redis
import random
import time
from flask import Flask, request

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
    
    request_route_timestamp = request.headers.get('X-Request-Timestamp')
    if not request_route_timestamp:
        return "No timestamp found in request headers."
    request_route_timestamp = int(request_route_timestamp)
    
    rate = random.choices(job_len, job_weight)[0]
    start_time = time.time() * 1000
    unit_task(redis_host, redis_port, rate)
    end_time = time.time() * 1000
    
    # 返回“任务大小 开始时间戳 结束时间戳 持续时间 JCT”
    ret = f'{rate} {start_time} {end_time} {end_time - start_time} {end_time - request_route_timestamp}\n'
    
    # 将ret发送给activator，在终端里向activator发包的方式：curl -X POST http://172.18.0.10:30001/ -v
    # activator_url = 'http://172.18.0.3:30001/'
    # try:
    #     response = requests.post(activator_url, data=ret)
    #     response.raise_for_status()
    # except requests.exceptions.RequestException as e:
    #     print(f'Error sending data to activator: {e}')
    return ret

if __name__ == "__main__":
    # 监听所有接口（0.0.0.0），端口8080
    app.run(host='0.0.0.0', port=8080)
    