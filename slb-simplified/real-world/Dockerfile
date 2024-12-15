FROM python:3.8-slim

RUN pip install requests
RUN pip install flask
RUN pip install numpy
RUN pip install pyyaml

# 复制应用程序代码
COPY alu.py /app/alu.py

# 设置工作目录
WORKDIR /app

EXPOSE 8080

ENTRYPOINT ["python", "alu.py"]
