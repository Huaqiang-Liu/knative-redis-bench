. ./enable-node-port.sh
wrk -t16 -c16 -d600s -s ./print_response1.lua -H "Host: redis-benchmark.default.example.com" http://$GATEWAY_URL -v > ./logs/Oct16/等待时间与请求频次/lambda1/500.txt
wrk -t16 -c16 -d600s -s ./print_response10.lua -H "Host: redis-benchmark.default.example.com" http://$GATEWAY_URL -v > ./logs/Oct16/等待时间与请求频次/lambda10/500.txt
wrk -t16 -c16 -d600s -s ./print_response50.lua -H "Host: redis-benchmark.default.example.com" http://$GATEWAY_URL -v > ./logs/Oct16/等待时间与请求频次/lambda50/500.txt
wrk -t16 -c16 -d600s -s ./print_response100.lua -H "Host: redis-benchmark.default.example.com" http://$GATEWAY_URL -v > ./logs/Oct16/等待时间与请求频次/lambda100/500.txt
wrk -t16 -c16 -d600s -s ./print_response200.lua -H "Host: redis-benchmark.default.example.com" http://$GATEWAY_URL -v > ./logs/Oct16/等待时间与请求频次/lambda200/500.txt
wrk -t16 -c16 -d600s -s ./print_response500.lua -H "Host: redis-benchmark.default.example.com" http://$GATEWAY_URL -v > ./logs/Oct16/等待时间与请求频次/lambda500/500.txt