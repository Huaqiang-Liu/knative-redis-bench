from locust import HttpUser, task
import random
import math

class PoissonLoadTest(HttpUser):
    # 设置默认头信息
    def on_start(self):
        # self.client.headers.update({"Host": "redis-benchmark.default.example.com"})
        self.client.headers.update({"Host": "alu-bench.default.example.com"})

    @task
    def my_task(self):
        response = self.client.get("/")
        # print(response.text)
        with open("./tmp.txt", "a") as f:
            f.write(response.text)        
    
    def wait_time(self):
        lambda_param = float(50)/float(128)  # 每秒请求数的期望值，128pod就是128*它=50
        return -math.log(1.0 - random.random()) / lambda_param
