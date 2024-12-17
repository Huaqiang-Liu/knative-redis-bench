FROM python:3.8-slim

RUN pip install requests
RUN pip install flask
RUN pip install numpy
RUN pip install pyyaml

# 复制应用程序代码
COPY __main__.py /app/__main__.py
COPY function /app/function
COPY utils.py /app/utils.py
COPY CDFs/execTimeCDF.csv /app/CDFs/execTimeCDF.csv
COPY CDFs/memCDF.csv /app/CDFs/memCDF.csv

# 设置工作目录
WORKDIR /app

EXPOSE 8080

ENTRYPOINT ["python", "__main__.py"]
