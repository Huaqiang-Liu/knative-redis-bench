FROM python:3.8-slim

RUN pip install redis
RUN pip install flask

# 复制应用程序代码
COPY benchmark.py /app/benchmark.py

# 设置工作目录
WORKDIR /app

EXPOSE 8080

ENTRYPOINT ["python", "benchmark.py"]
# CMD ["python", "benchmark.py", "&&", "sleep", "infinity"]

