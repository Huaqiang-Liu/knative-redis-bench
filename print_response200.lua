-- 使用 os.execute 来实现延迟
local lambda = 200.0  -- 期望值（每秒请求数）和方差

-- 任务到达时间是泊松分布，故任务间隔时间服从指数分布
local function poisson_delay()
   return -math.log(1.0 - math.random()) / lambda
end

-- 请求生成函数，通过规定下一个请求的延迟时间来控制所有请求的分布
request = function()
   local req = wrk.format(nil, "/")
   local delay = poisson_delay()
   -- 使用 os.execute 来延迟时间（单位为秒）
   os.execute("sleep " .. tonumber(delay))
   return req
end

-- 请求完成后的回调函数
done = function(summary, latency, requests)
   print("Requests: ", requests, " completed.")
end

-- 响应处理函数
response = function(status, headers, body)
   io.write(body)
end
