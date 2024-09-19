import os
import redis
import random
import time
from flask import Flask, request

app = Flask(__name__)


# 一倍的负载对应1ms的任务，10ms等等以此类推
def task(redis_host, redis_port, rate = 1):
    redis_password = 'pmT0bVLwFr' # os.getenv('REDIS_PASSWORD', None)
    r = redis.StrictRedis(host=redis_host, port=redis_port, password=redis_password, decode_responses=True)

    for i in range(rate * 10):
        r.set(f'key_{i}', f'value_{i}')
        value = r.get(f'key_{i}')
        # print(f'key_{i}: {value}')

@app.route('/', methods=['GET'])
def gen_task():
    redis_host = os.getenv('REDIS_HOST', 'localhost')
    redis_port = int(os.getenv('REDIS_PORT', 6379))

    job_len = [1, 3, 25, 250, 7500]
    job_weight = [0.05, 0.03, 0.17, 0.33, 0.37]
    
    # 若使用scalable.yaml部署任务，则有环境变量TASK_SIZE
    # env:
    #   - name: TASK_SIZE
    #     value: "7000"
    # 根据上面yaml中的配置提取任务长度
    # job_len = [int(os.getenv('TASK_SIZE', 1))]
    # job_weight = [1]
    
    rate = random.choices(job_len, job_weight)[0]
    start_time = time.time() * 1000
    task(redis_host, redis_port, rate)
    end_time = time.time() * 1000
    
    # 返回“任务大小 开始时间戳 结束时间戳 持续时间”
    ret = f'{rate} {start_time} {end_time} {end_time - start_time}\n'
    return ret

if __name__ == "__main__":
    # 监听所有接口（0.0.0.0），端口8080
    app.run(host='0.0.0.0', port=8080)
    