package algorithms

// ============================================================================
// LUA SCRIPTS FOR ATOMIC RATE LIMITING
// ============================================================================
//
// These Lua scripts run atomically in Redis, eliminating race conditions
// that occur with separate Get/Set operations in high-concurrency scenarios.
//
// All scripts follow Redis Lua conventions:
// - KEYS[] array contains key names
// - ARGV[] array contains arguments
// - Return values are converted to Go types by redis client

// TokenBucketScript performs atomic token bucket rate limiting
//
// Algorithm:
// 1. Get current bucket state (or initialize with full tokens)
// 2. Calculate tokens to add based on elapsed time
// 3. Refill tokens (capped at capacity)
// 4. Check if enough tokens available
// 5. If allowed: consume tokens, update state
// 6. Return: {allowed, remaining, tokens_before, tokens_after}
//
// KEYS[1] = rate limit key
// ARGV[1] = limit (capacity)
// ARGV[2] = window duration in seconds
// ARGV[3] = tokens to consume (n)
// ARGV[4] = current timestamp in nanoseconds
// ARGV[5] = window expiration in seconds (for key TTL)
//
// Returns: {allowed (1/0), remaining, tokens_before, tokens_after, reset_timestamp}
const TokenBucketScript = `
-- Parse arguments
local key = KEYS[1]
local capacity = tonumber(ARGV[1])
local window_sec = tonumber(ARGV[2])
local tokens_requested = tonumber(ARGV[3])
local now_nano = tonumber(ARGV[4])
local ttl_sec = tonumber(ARGV[5])

-- Validate timestamp to prevent overflow attacks
-- Max int64 is 9223372036854775807 (~9.2e18)
-- Current Unix nano (~1.7e18) is safe, but validate anyway
local MAX_TIMESTAMP = 9000000000000000000 -- 9e18, well below int64 max
local MIN_TIMESTAMP = 0

if not now_nano or now_nano < MIN_TIMESTAMP or now_nano > MAX_TIMESTAMP then
    -- Invalid timestamp - return error (deny request)
    return {0, 0, 0, 0, now_nano or 0}
end

-- Calculate refill rate (tokens per nanosecond)
-- Using nanoseconds for precision
local refill_rate_nano = capacity / (window_sec * 1000000000)

-- Get current state from Redis
local state = redis.call('GET', key)
local tokens, last_refill_nano, total_requests, denied_requests

if state then
    -- Deserialize state: "tokens,last_refill_nano,total_requests,denied_requests"
    local parts = {}
    for part in string.gmatch(state, '([^,]+)') do
        table.insert(parts, part)
    end
    tokens = tonumber(parts[1])
    last_refill_nano = tonumber(parts[2])
    total_requests = tonumber(parts[3]) or 0
    denied_requests = tonumber(parts[4]) or 0
else
    -- Initialize with full bucket
    tokens = capacity
    last_refill_nano = now_nano
    total_requests = 0
    denied_requests = 0
end

local tokens_before = tokens

-- Calculate elapsed time and refill tokens
-- Protect against overflow in subtraction
if last_refill_nano > MAX_TIMESTAMP then
    -- Corrupted state - reset
    last_refill_nano = now_nano
end

local elapsed_nano = now_nano - last_refill_nano
if elapsed_nano > 0 then
    -- Cap elapsed time to prevent overflow in multiplication
    local MAX_ELAPSED = 86400000000000 -- 24 hours in nanoseconds
    if elapsed_nano > MAX_ELAPSED then
        elapsed_nano = MAX_ELAPSED
    end

    local tokens_to_add = refill_rate_nano * elapsed_nano
    tokens = math.min(tokens + tokens_to_add, capacity)
    last_refill_nano = now_nano
elseif elapsed_nano < 0 then
    -- Clock went backwards (NTP adjustment) - reset to now but don't refill
    last_refill_nano = now_nano
    elapsed_nano = 0
end

-- Check if request is allowed
local allowed = (tokens >= tokens_requested)
local remaining = 0
local reset_timestamp = now_nano

if allowed then
    -- Consume tokens
    tokens = tokens - tokens_requested
    remaining = math.floor(tokens)
    total_requests = total_requests + tokens_requested

    -- Calculate reset time (when bucket will be full)
    local tokens_needed = capacity - tokens
    if tokens_needed > 0 then
        local nanos_to_full = tokens_needed / refill_rate_nano
        -- Protect against overflow in addition
        if nanos_to_full > (MAX_TIMESTAMP - now_nano) then
            reset_timestamp = MAX_TIMESTAMP
        else
            reset_timestamp = now_nano + nanos_to_full
        end
    else
        reset_timestamp = now_nano
    end
else
    -- Request denied
    remaining = 0
    denied_requests = denied_requests + tokens_requested

    -- Calculate retry after time
    local tokens_needed = tokens_requested - tokens
    local nanos_to_available = tokens_needed / refill_rate_nano
    -- Protect against overflow in addition
    if nanos_to_available > (MAX_TIMESTAMP - now_nano) then
        reset_timestamp = MAX_TIMESTAMP
    else
        reset_timestamp = now_nano + nanos_to_available
    end
end

-- Serialize and save state
local new_state = string.format("%.10f,%d,%d,%d", tokens, last_refill_nano, total_requests, denied_requests)
redis.call('SETEX', key, ttl_sec, new_state)

-- Return: {allowed, remaining, tokens_before, tokens_after, reset_timestamp}
local allowed_int = allowed and 1 or 0
return {allowed_int, remaining, tokens_before, tokens, reset_timestamp}
`

// SlidingWindowScript performs atomic sliding window rate limiting
//
// Algorithm:
// 1. Get current request timestamps
// 2. Remove expired timestamps (outside window)
// 3. Check if adding N requests would exceed limit
// 4. If allowed: append N timestamps
// 5. Return: {allowed, remaining, current_count}
//
// KEYS[1] = rate limit key
// ARGV[1] = limit
// ARGV[2] = window duration in nanoseconds
// ARGV[3] = requests to add (n)
// ARGV[4] = current timestamp in nanoseconds
// ARGV[5] = window expiration in seconds (for key TTL)
// ARGV[6] = max trackable requests (safety limit)
//
// Returns: {allowed (1/0), remaining, current_count, reset_timestamp}
const SlidingWindowScript = `
-- Parse arguments
local key = KEYS[1]
local limit = tonumber(ARGV[1])
local window_nano = tonumber(ARGV[2])
local requests_to_add = tonumber(ARGV[3])
local now_nano = tonumber(ARGV[4])
local ttl_sec = tonumber(ARGV[5])
local max_requests = tonumber(ARGV[6])

-- Validate timestamp to prevent overflow attacks
-- Max int64 is 9223372036854775807 (~9.2e18)
local MAX_TIMESTAMP = 9000000000000000000 -- 9e18, well below int64 max
local MIN_TIMESTAMP = 0

if not now_nano or now_nano < MIN_TIMESTAMP or now_nano > MAX_TIMESTAMP then
    -- Invalid timestamp - return error (deny request)
    return {0, 0, 0, now_nano or 0}
end

-- Validate window duration to prevent underflow
local MAX_WINDOW = 86400000000000 -- 24 hours in nanoseconds
if not window_nano or window_nano < 0 or window_nano > MAX_WINDOW then
    -- Invalid window - return error
    return {0, 0, 0, now_nano}
end

-- Calculate window start time (protect against underflow)
local window_start = now_nano - window_nano
if window_start < 0 then
    window_start = 0
end

-- Get current state from Redis
local state = redis.call('GET', key)
local timestamps = {}
local total_allowed = 0
local total_denied = 0

if state then
    -- Deserialize state: "timestamp1,timestamp2,...;total_allowed,total_denied"
    local main_parts = {}
    for part in string.gmatch(state, '([^;]+)') do
        table.insert(main_parts, part)
    end

    -- Parse timestamps
    if main_parts[1] and main_parts[1] ~= "" then
        for ts in string.gmatch(main_parts[1], '([^,]+)') do
            table.insert(timestamps, tonumber(ts))
        end
    end

    -- Parse counters
    if main_parts[2] then
        local counter_parts = {}
        for part in string.gmatch(main_parts[2], '([^,]+)') do
            table.insert(counter_parts, part)
        end
        total_allowed = tonumber(counter_parts[1]) or 0
        total_denied = tonumber(counter_parts[2]) or 0
    end
end

-- Remove expired timestamps (cleanup)
-- Also remove "future" timestamps from clock skew and invalid timestamps
local valid_timestamps = {}
for _, ts in ipairs(timestamps) do
    -- Validate timestamp is reasonable
    if ts and ts >= MIN_TIMESTAMP and ts <= MAX_TIMESTAMP then
        -- Keep timestamp if it's within window AND not from future
        -- Future timestamps occur when clock jumps backward
        if ts >= window_start and ts <= now_nano then
            table.insert(valid_timestamps, ts)
        end
    end
    -- Silently drop expired, future, or invalid timestamps
end
timestamps = valid_timestamps

-- Check current usage
local current_count = #timestamps

-- Safety check: prevent unbounded memory growth
if current_count >= max_requests then
    -- Deny the request to prevent memory exhaustion
    total_denied = total_denied + requests_to_add

    -- Save state and return denied
    local new_state = table.concat(timestamps, ',') .. ';' .. total_allowed .. ',' .. total_denied
    redis.call('SETEX', key, ttl_sec, new_state)

    -- Calculate reset time (oldest timestamp + window) with overflow protection
    local reset_timestamp = now_nano + window_nano
    if #timestamps > 0 then
        local oldest_ts = timestamps[1]
        -- Protect against overflow in addition
        if window_nano > (MAX_TIMESTAMP - oldest_ts) then
            reset_timestamp = MAX_TIMESTAMP
        else
            reset_timestamp = oldest_ts + window_nano
        end
    end
    return {0, 0, current_count, reset_timestamp}
end

-- Check if request is allowed
local allowed = (current_count + requests_to_add <= limit)
local remaining = 0
local reset_timestamp = now_nano + window_nano

if allowed then
    -- Add new timestamps
    for i = 1, requests_to_add do
        table.insert(timestamps, now_nano)
    end

    remaining = limit - #timestamps
    total_allowed = total_allowed + requests_to_add

    -- Calculate reset time (oldest timestamp + window) with overflow protection
    if #timestamps > 0 then
        local oldest_ts = timestamps[1]
        -- Protect against overflow in addition
        if window_nano > (MAX_TIMESTAMP - oldest_ts) then
            reset_timestamp = MAX_TIMESTAMP
        else
            reset_timestamp = oldest_ts + window_nano
        end
    end
else
    -- Request denied
    remaining = 0
    total_denied = total_denied + requests_to_add

    -- Calculate reset time (when oldest request will expire) with overflow protection
    if #timestamps > 0 then
        local oldest_ts = timestamps[1]
        -- Protect against overflow in addition
        if window_nano > (MAX_TIMESTAMP - oldest_ts) then
            reset_timestamp = MAX_TIMESTAMP
        else
            reset_timestamp = oldest_ts + window_nano
        end
    end
end

-- Serialize and save state
local timestamp_str = table.concat(timestamps, ',')
local new_state = timestamp_str .. ';' .. total_allowed .. ',' .. total_denied
redis.call('SETEX', key, ttl_sec, new_state)

-- Return: {allowed, remaining, current_count, reset_timestamp}
local allowed_int = allowed and 1 or 0
local new_count = #timestamps
return {allowed_int, remaining, new_count, reset_timestamp}
`
