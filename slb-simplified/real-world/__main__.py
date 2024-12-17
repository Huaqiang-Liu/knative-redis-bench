# Copyright (c) 2020 Institution of Parallel and Distributed System, Shanghai Jiao Tong University
# ServerlessBench is licensed under the Mulan PSL v1.
# You can use this software according to the terms and conditions of the Mulan PSL v1.
# You may obtain a copy of Mulan PSL v1 at:
#     http://license.coscl.org.cn/MulanPSL
# THIS SOFTWARE IS PROVIDED ON AN "AS IS" BASIS, WITHOUT WARRANTIES OF ANY KIND, EITHER EXPRESS OR
# IMPLIED, INCLUDING BUT NOT LIMITED TO NON-INFRINGEMENT, MERCHANTABILITY OR FIT FOR A PARTICULAR
# PURPOSE.
# See the Mulan PSL v1 for more details.

import time
import json
from subprocess import call
import os, stat
import utils

import socket
import requests
import random
from flask import Flask, request

app = Flask(__name__)

global memCDFFilename, execCDFFilename
memCDFFilename  = "CDFs/memCDF.csv"
execCDFFilename = "CDFs/execTimeCDF.csv"

def mallocRandMem():
    bias = 30
    randMem = utils.getRandValueRefByCDF(memCDFFilename) - bias
    os.chmod("./function",stat.S_IRWXU)
    call(["./function","%s" %randMem])
    return randMem

def execRandTime(mmExecTime, randExecTime):
    exactAluTime = randExecTime - mmExecTime # 单位都是毫秒。这么做是因为内存操作和计算加起来才是csv文件中的所谓执行时间
    if exactAluTime > 0:
        utils.alu(exactAluTime)
    return randExecTime

@app.route('/', methods=['GET'])
def main():
    route_time_str = request.headers.get('X-Request-Timestamp')
    arrive_time_str = request.headers.get('X-Arrive-Timestamp')
    seq_start_time_str = request.headers.get('X-Seq-Start-Time')
    rate_str = request.headers.get('X-Rate') # 因为activator有时候需要知道理论的执行时间（这里还是称作rate，尽管就是所谓randExecTime）
    last_rate_str = request.headers.get('X-Last-Rate')
    if last_rate_str == "":
        last_rate_str = "0"
    if not route_time_str or not arrive_time_str or not seq_start_time_str or not rate_str or not last_rate_str:
        return "lack headers"
    
    route_time = float(route_time_str)
    arrive_time = float(arrive_time_str)
    seq_start_time = float(seq_start_time_str)
    randExecTime = int(rate_str)

    start_time = time.time() * 1000

    mmStartTime = utils.getTime()
    memSize = mallocRandMem()
    mmEndTime = utils.getTime()
    
    mmExecTime = mmEndTime - mmStartTime
    execRandTime(mmExecTime, randExecTime)
    
    end_time = time.time() * 1000
    response_time = start_time - arrive_time
    latency = end_time - arrive_time

    # sequence开始的时间为0则表示不是最后一个action，所以只需!=0的时候算出latency即可
    seq_lat = 0
    if seq_start_time != 0:
        seq_end_time = time.time() * 1000
        seq_lat = seq_end_time - seq_start_time
    ret = f'{seq_lat} {response_time} {memSize} {latency} {last_rate_str}\n'
    
    node_of_activator = os.getenv('NODE_OF_ACTIVATOR')
    activator_url = f'http://172.18.0.{node_of_activator}:30001/store'
    try:
        headers = {'X-PodIP': socket.gethostbyname(socket.gethostname())}
        response = requests.post(activator_url, data=ret, headers=headers)
        response.raise_for_status()
    except requests.exceptions.RequestException as e:
        return f"{ret} 无法将任务信息发给activator {activator_url}：{e}"
    return ret


if __name__ == '__main__':
    # 监听所有接口（0.0.0.0），端口8080
    app.run(host='0.0.0.0', port=8080)
