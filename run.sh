. ./enable-node-port.sh
start_time=$(date +%s%3N) # wrk开始运行的时间戳，精确到毫秒
wrk -t16 -c16 -d1800s -s ./print_response.lua -H "Host: redis-benchmark.default.example.com" http://$GATEWAY_URL -v
echo "start time: $start_time"